package store_test

// Infrastructure commune aux tests du paquet store : connexion à la base de
// test, remise à zéro, création de données et outils de concurrence.
//
// Les tests tournent sur le service db-test de compose.yaml :
//
//	docker compose up -d --wait db-test
//	go test -count=1 ./...        (dans le dossier api)
//
// -count=1 désactive le cache des résultats de go test, inadapté à des tests
// qui dépendent d'une base de données.
//
// Les tests de ce paquet s'exécutent l'un après l'autre : aucun n'appelle
// t.Parallel. En revanche, go test ./... teste plusieurs paquets en même
// temps. Si un autre paquet utilisait un jour la base de test, il faudrait
// ajouter l'option -p 1 (un paquet à la fois), sinon leurs remises à zéro se
// mélangeraient.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/knudsem/cairn/api/internal/store"
)

// Base de test par défaut : le service db-test. La variable d'environnement
// TEST_DATABASE_URL permet d'en désigner une autre.
const defaultTestDatabaseURL = "postgres://cairn:cairn@127.0.0.1:5433/cairn_test?sslmode=disable"

// pool et st sont partagés par tous les tests du paquet. Chaque test vide la
// base avant de créer ses propres données.
var (
	pool *pgxpool.Pool
	st   *store.Store
)

// TestMain s'exécute avant tous les tests du paquet : il ouvre la connexion,
// lance les tests (m.Run), puis la ferme. C'est l'équivalent d'un
// setUpModule en Python, ou d'un beforeAll en JavaScript.
func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = defaultTestDatabaseURL
	}
	var err error
	pool, err = connectTestDB(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tests impossibles : %v\n", err)
		fmt.Fprintln(os.Stderr, "La base de test se lance avec : docker compose up -d --wait db-test")
		os.Exit(1)
	}
	defer closePool(pool)
	st = store.New(pool)
	m.Run()
}

// closePool ferme le pool p, sauf si une connexion ne lui a pas été rendue :
// Close attend le retour de chaque connexion, et bloquerait jusqu'au délai de
// go test. C'est ce qui arrive quand une transaction reste ouverte, ce que
// resetDB signale déjà ; la fin du programme fermera alors la connexion.
func closePool(p *pgxpool.Pool) {
	if p.Stat().AcquiredConns() == 0 {
		p.Close()
	}
}

// connectTestDB ouvre le pool et vérifie qu'il s'agit bien d'une base de
// test : les tests vident les tables, ils ne doivent jamais toucher aux
// données de démonstration (docs/PLAN.md, section 1).
func connectTestDB(url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	// Assez de connexions pour les validations simultanées des tests.
	cfg.MaxConns = 16

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var name string
	if err := p.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		p.Close()
		return nil, err
	}
	if !strings.HasSuffix(name, "_test") {
		p.Close()
		return nil, fmt.Errorf("refus de lancer les tests sur la base %q : son nom ne se termine pas par _test, et les tests vident les tables", name)
	}
	return p, nil
}

// resetDB vide les trois tables et fait repartir les identifiants de plages
// distinctes : produits 1, 2... ; commandes 101, 102... ; lignes 201, 202...
// Si tous repartaient de 1, un code qui confond l'identifiant d'une commande
// avec celui d'une ligne pourrait passer les tests par coïncidence.
//
// À la fin du test, elle vérifie aussi qu'aucune transaction n'est restée
// ouverte, ce qui trahirait un Commit ou un Rollback oublié.
func resetDB(t *testing.T) {
	t.Helper()
	// Une transaction restée ouverte garde ses verrous, et TRUNCATE
	// l'attendrait jusqu'au délai global de go test (10 minutes). Avec ce
	// délai de 5 secondes, le test échoue vite et avec un message clair.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for _, sql := range []string{
		`TRUNCATE order_lines, orders, products RESTART IDENTITY`,
		`ALTER TABLE orders ALTER COLUMN id RESTART WITH 101`,
		`ALTER TABLE order_lines ALTER COLUMN id RESTART WITH 201`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("remise à zéro impossible (%s), une transaction est peut-être restée ouverte : %v", sql, err)
		}
	}

	// t.Cleanup enregistre une fonction exécutée à la fin du test, comme un
	// tearDown en Python. Le contexte du test est alors déjà annulé, d'où
	// context.Background().
	t.Cleanup(func() {
		var open int
		err := pool.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND state LIKE 'idle in transaction%'`).Scan(&open)
		if err != nil {
			t.Errorf("recherche des transactions restées ouvertes : %v", err)
		} else if open > 0 {
			t.Errorf("%d transaction(s) restée(s) ouverte(s) à la fin du test : Commit ou Rollback oublié ?", open)
		}
	})
}

// insertProduct crée un produit et renvoie son identifiant.
func insertProduct(t *testing.T, sku, location string, stock int) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(t.Context(), `
		INSERT INTO products (sku, name, location, stock_qty)
		VALUES ($1, $2, $3, $4) RETURNING id`, sku, "Produit "+sku, location, stock).Scan(&id)
	if err != nil {
		t.Fatalf("création du produit %s : %v", sku, err)
	}
	return id
}

// insertOrder crée une commande « à préparer » et renvoie son identifiant.
func insertOrder(t *testing.T, reference string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(t.Context(), `
		INSERT INTO orders (reference, customer) VALUES ($1, 'Client de test')
		RETURNING id`, reference).Scan(&id)
	if err != nil {
		t.Fatalf("création de la commande %s : %v", reference, err)
	}
	return id
}

// insertLine crée une ligne non préparée et renvoie son identifiant.
func insertLine(t *testing.T, orderID, productID int64, quantity int) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(t.Context(), `
		INSERT INTO order_lines (order_id, product_id, quantity) VALUES ($1, $2, $3)
		RETURNING id`, orderID, productID, quantity).Scan(&id)
	if err != nil {
		t.Fatalf("création d'une ligne de la commande %d : %v", orderID, err)
	}
	return id
}

// stockOf renvoie le stock d'un produit.
func stockOf(t *testing.T, productID int64) int {
	t.Helper()
	var stock int
	if err := pool.QueryRow(t.Context(), `SELECT stock_qty FROM products WHERE id = $1`, productID).Scan(&stock); err != nil {
		t.Fatalf("lecture du stock du produit %d : %v", productID, err)
	}
	return stock
}

// statusOf renvoie le statut d'une commande.
func statusOf(t *testing.T, orderID int64) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		t.Fatalf("lecture du statut de la commande %d : %v", orderID, err)
	}
	return status
}

// pickState renvoie l'état d'une ligne : préparée ou non, et par qui (chaîne
// vide si elle ne l'est pas).
func pickState(t *testing.T, lineID int64) (picked bool, pickedBy string) {
	t.Helper()
	var at *time.Time
	var by *string
	if err := pool.QueryRow(t.Context(), `SELECT picked_at, picked_by FROM order_lines WHERE id = $1`, lineID).Scan(&at, &by); err != nil {
		t.Fatalf("lecture de la ligne %d : %v", lineID, err)
	}
	if by != nil {
		pickedBy = *by
	}
	return at != nil, pickedBy
}

// pickedLinesOf compte les lignes préparées d'un produit.
func pickedLinesOf(t *testing.T, productID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*) FROM order_lines WHERE product_id = $1 AND picked_at IS NOT NULL`,
		productID).Scan(&n); err != nil {
		t.Fatalf("comptage des lignes préparées du produit %d : %v", productID, err)
	}
	return n
}

// runTogether lance chaque fonction dans sa propre goroutine, les libère
// toutes au même instant, puis attend qu'elles aient fini et renvoie leurs
// résultats dans l'ordre. La barrière de départ est un canal : tant qu'il est
// ouvert, <-start bloque ; close(start) débloque toutes les goroutines à la
// fois. Les goroutines ne doivent pas appeler t.Fatal : elles renvoient une
// erreur, que le test examine ensuite.
func runTogether(fns ...func() error) []error {
	errs := make([]error, len(fns))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, fn := range fns {
		wg.Go(func() {
			<-start
			errs[i] = fn()
		})
	}
	close(start)
	wg.Wait()
	return errs
}
