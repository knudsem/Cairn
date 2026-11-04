// Package testdb donne aux tests l'accès à la base de test, le service
// db-test de compose.yaml : connexion avec garde-fou, passage d'un paquet de
// tests à la fois, remise à zéro des tables, et panne simulée au milieu
// d'une requête.
//
// Seuls des fichiers _test.go l'importent : il n'entre jamais dans
// l'exécutable de l'API.
package testdb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultURL désigne le service db-test. La variable d'environnement
// TEST_DATABASE_URL permet d'en viser une autre.
const defaultURL = "postgres://cairn:cairn@127.0.0.1:5433/cairn_test?sslmode=disable"

// lockKey identifie le verrou consultatif (advisory lock) qui fait passer
// les paquets de tests l'un après l'autre. Sa valeur n'a pas de sens
// particulier : elle doit seulement être la même pour tous les paquets.
const lockKey = 20261105

// DB est une connexion à la base de test, réservée à un paquet de tests.
type DB struct {
	Pool *pgxpool.Pool
	lock *pgx.Conn // session qui détient le verrou entre paquets
}

// Open ouvre la base de test désignée par TEST_DATABASE_URL, ou à défaut le
// service db-test. À appeler une seule fois par programme de test, dans
// TestMain : un second appel attendrait le verrou que le premier détient.
//
// Open vérifie d'abord que le nom de la base se termine par _test : les tests
// vident les tables, ils ne doivent jamais toucher aux données de
// démonstration (docs/PLAN.md, section 1).
//
// Elle prend ensuite le verrou entre paquets. go test ./... lance les
// paquets de tests en parallèle, chacun dans son propre programme : sans ce
// verrou, les remises à zéro de l'un effaceraient les données de l'autre en
// plein test. Un paquet attend donc ici que le précédent ait fini.
func Open() (*DB, error) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = defaultURL
	}
	return open(url)
}

// open fait le travail d'Open pour l'adresse url.
func open(url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("adresse de la base de test : %w", err)
	}
	// 16 connexions, au moins autant que les validations simultanées des
	// tests de concurrence du store (10) : avec moins, elles attendraient
	// leur tour dans le pool, ne se chevaucheraient plus, et ces tests ne
	// prouveraient plus rien.
	cfg.MaxConns = 16
	// Nom affiché dans la vue pg_stat_activity de PostgreSQL : on y voit
	// quelles connexions viennent des tests, celle du verrou comprise.
	cfg.ConnConfig.RuntimeParams["application_name"] = "cairn-tests"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, connectError(err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		pool.Close()
		return nil, connectError(err)
	}
	if !strings.HasSuffix(name, "_test") {
		pool.Close()
		return nil, fmt.Errorf("refus de lancer les tests sur la base %q : son nom ne se termine pas par _test, et les tests vident les tables", name)
	}

	// Le verrou est pris sur une connexion à part, hors du pool : il dure
	// autant que cette session, et PostgreSQL le libère dès qu'elle se
	// ferme, même si le programme de test s'arrête brutalement.
	lock, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		pool.Close()
		return nil, connectError(err)
	}
	if err := takeLock(lock); err != nil {
		lock.Close(context.Background())
		pool.Close()
		return nil, fmt.Errorf("verrou entre paquets de tests : %w", err)
	}
	return &DB{Pool: pool, lock: lock}, nil
}

// connectError complète une erreur de connexion par la commande qui lance la
// base de test.
func connectError(err error) error {
	return fmt.Errorf("connexion à la base de test : %w\nLa base de test se lance avec : docker compose up -d --wait db-test", err)
}

// takeLock prend le verrou entre paquets. Si un autre paquet le détient, elle
// le signale avant d'attendre : l'attente a lieu dans TestMain, avant
// m.Run, et l'option -timeout de go test ne la couvre pas. Elle n'a pas de
// délai : le paquet précédent peut tourner une minute.
func takeLock(conn *pgx.Conn) error {
	ctx := context.Background()
	var taken bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return nil
	}
	fmt.Fprintln(os.Stderr, "testdb : en attente de la base de test, utilisée par un autre paquet de tests")
	_, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey)
	return err
}

// Close libère le verrou entre paquets, puis ferme le pool avec ClosePool.
func (db *DB) Close() {
	db.lock.Close(context.Background())
	ClosePool(db.Pool)
}

// ClosePool ferme le pool p, sauf si une connexion ne lui a pas été rendue :
// Close attend le retour de chaque connexion, et bloquerait jusqu'au délai de
// go test. C'est ce qui arrive quand une transaction reste ouverte, ce que
// Reset signale déjà ; la fin du programme fermera alors la connexion.
func ClosePool(p *pgxpool.Pool) {
	if p.Stat().AcquiredConns() == 0 {
		p.Close()
	}
}

// Reset vide les trois tables et fait repartir les identifiants de plages
// distinctes : produits 1, 2... ; commandes 101, 102... ; lignes 201, 202...
// Si tous repartaient de 1, un code qui confond l'identifiant d'une commande
// avec celui d'une ligne pourrait passer les tests par coïncidence.
//
// À la fin du test, elle vérifie aussi qu'aucune transaction n'est restée
// ouverte, ce qui trahirait un Commit ou un Rollback oublié.
func Reset(t *testing.T, pool *pgxpool.Pool) {
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

// InterruptingPool renvoie un pool configuré comme pool, dont les connexions
// font échouer la requête SQL qui contient sql : elle reçoit un contexte
// déjà annulé, et n'est pas envoyée, comme si l'appelant abandonnait à cet
// instant précis. Le pool est fermé à la fin du test.
func InterruptingPool(t *testing.T, pool *pgxpool.Pool, sql string) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config() // une copie : le pool d'origine ne change pas
	cfg.ConnConfig.Tracer = interruptQuery{sql: sql}
	p, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatalf("pool avec traceur : %v", err)
	}
	t.Cleanup(func() { ClosePool(p) })
	return p
}

// interruptQuery est un traceur pgx : pgx appelle sa méthode TraceQueryStart
// juste avant chaque requête SQL, et TraceQueryEnd juste après, un peu comme
// un middleware. Il n'a pas besoin de déclarer qu'il implémente l'interface
// pgx.QueryTracer : avoir ces deux méthodes suffit.
type interruptQuery struct{ sql string }

// TraceQueryStart renvoie le contexte que pgx utilisera pour la requête : un
// contexte déjà annulé pour celle qui contient q.sql.
func (q interruptQuery) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, q.sql) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		return canceled
	}
	return ctx
}

// TraceQueryEnd n'a rien à faire, mais l'interface l'exige.
func (interruptQuery) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
