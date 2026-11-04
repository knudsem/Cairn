package store_test

// Infrastructure commune aux tests du paquet store : connexion à la base de
// test, remise à zéro, création de données et outils de concurrence. La
// connexion et la remise à zéro viennent du paquet testdb, partagé avec les
// tests de la couche HTTP.
//
// Les tests tournent sur le service db-test de compose.yaml :
//
//	docker compose up -d --wait db-test
//	go test -count=1 ./...        (dans le dossier api)
//
// -count=1 désactive le cache des résultats de go test, inadapté à des tests
// qui dépendent d'une base de données. Les tests de ce paquet s'exécutent
// l'un après l'autre (aucun n'appelle t.Parallel), et testdb.Open fait
// attendre les autres paquets qui utilisent la base de test.

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/knudsem/cairn/api/internal/store"
	"github.com/knudsem/cairn/api/internal/testdb"
)

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
	db, err := testdb.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tests impossibles : %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	pool = db.Pool
	st = store.New(pool)
	m.Run()
}

// resetDB vide la base avant un test (voir testdb.Reset).
func resetDB(t *testing.T) {
	t.Helper()
	testdb.Reset(t, pool)
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
