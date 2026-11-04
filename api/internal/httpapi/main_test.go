package httpapi_test

// Infrastructure des tests de la couche HTTP : base de test, jeu de données,
// envoi des requêtes et vérification des réponses.
//
// Les requêtes passent par net/http/httptest : NewRequest fabrique une
// requête, NewRecorder enregistre la réponse, sans réseau ni port ouvert.
// Le routeur, les gestionnaires et la base sont les vrais : ces tests
// vérifient la traduction entre HTTP et le store, de bout en bout.
//
// Lancement, comme les tests du store : go test -count=1 ./... (dans le
// dossier api), avec la base de test lancée (docker compose up -d --wait
// db-test).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/knudsem/cairn/api/internal/httpapi"
	"github.com/knudsem/cairn/api/internal/store"
	"github.com/knudsem/cairn/api/internal/testdb"
)

// Type et message attendus dans les réponses de l'API.
const (
	jsonContentType = "application/json; charset=utf-8"
	internalMessage = "Erreur interne du serveur. Réessayez dans un instant."
)

var (
	pool    *pgxpool.Pool
	handler http.Handler // le routeur de l'API, branché sur la base de test
)

// TestMain ouvre la base de test, lance les tests, puis la ferme. testdb.Open
// fait attendre ce paquet si les tests du store tournent déjà.
func TestMain(m *testing.M) {
	db, err := testdb.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tests impossibles : %v\n", err)
		os.Exit(1)
	}
	defer db.Close()
	pool = db.Pool
	handler = httpapi.NewHandler(store.New(pool))
	m.Run()
}

// fixture est le jeu de données des tests. Les identifiants sont connus
// d'avance grâce aux plages de testdb.Reset : produits 1, 2 ; commandes 101,
// 102 ; lignes 201, 202, 203.
const fixture = `
	INSERT INTO products (sku, name, location, stock_qty) VALUES
	    ('P-1', 'Produit 1', 'A-01-1', 10),  -- produit 1
	    ('P-2', 'Produit 2', 'B-02-1', 2);   -- produit 2
	INSERT INTO orders (reference, customer, created_at) VALUES
	    ('CMD-A', 'Client A', now() - interval '2 hours'),  -- commande 101
	    ('CMD-B', 'Client B', now() - interval '1 hour');   -- commande 102
	INSERT INTO order_lines (order_id, product_id, quantity) VALUES
	    (101, 1, 4),  -- ligne 201 : 4 demandés, 10 en stock
	    (101, 2, 5),  -- ligne 202 : 5 demandés, 2 en stock
	    (102, 1, 1);  -- ligne 203 : seule ligne de la commande 102`

// setup remet la base à zéro et charge le jeu de données.
func setup(t *testing.T) {
	t.Helper()
	testdb.Reset(t, pool)
	if _, err := pool.Exec(t.Context(), fixture); err != nil {
		t.Fatalf("chargement du jeu de données : %v", err)
	}
}

// request envoie une requête à h et renvoie la réponse enregistrée. Sans
// contentType, la requête n'a pas d'en-tête Content-Type.
func request(t *testing.T, h http.Handler, method, target, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// get envoie GET target à l'API.
func get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, handler, http.MethodGet, target, "", "")
}

// pick envoie la validation de la ligne lineID de la commande orderID, avec
// body comme corps JSON.
func pick(t *testing.T, orderID, lineID int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	target := fmt.Sprintf("/api/orders/%d/lines/%d/pick", orderID, lineID)
	return request(t, handler, http.MethodPost, target, "application/json", body)
}

// jsonEscape renvoie l'échappement JSON du caractère numéro n : une barre
// oblique inverse, la lettre u, puis le numéro en quatre chiffres
// hexadécimaux. jsonEscape(0xe9) donne ainsi l'échappement de « é ». Les
// tests l'utilisent pour les caractères invisibles, qui ne se voient pas
// écrits tels quels dans le source. \x5c est le code de la barre oblique
// inverse.
func jsonEscape(n rune) string {
	return fmt.Sprintf("\x5cu%04x", n)
}

// contentType renvoie l'en-tête Content-Type que le client reçoit vraiment.
// rec.Header() est la table que le gestionnaire peut encore modifier : un
// en-tête fixé après WriteHeader y apparaît, mais n'est jamais envoyé.
// rec.Result().Header montre ce qui est parti au moment de WriteHeader.
func contentType(rec *httptest.ResponseRecorder) string {
	return rec.Result().Header.Get("Content-Type")
}

// checkError vérifie une réponse d'erreur : le code HTTP, le type JSON, et le
// format unique {"error": code, "message": "..."}, sans autre champ
// (docs/PLAN.md, section 4.2). Elle renvoie le message, que chaque test
// compare au texte attendu.
func checkError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) string {
	t.Helper()
	// Le corps finit par un saut de ligne (json.Encoder) : retiré pour que les
	// messages d'échec restent sur une ligne.
	text := strings.TrimSpace(rec.Body.String())
	if rec.Code != status {
		t.Errorf("code HTTP %d, attendu %d (corps : %s)", rec.Code, status, text)
	}
	if got := contentType(rec); got != jsonContentType {
		t.Errorf("Content-Type %q, attendu %q", got, jsonContentType)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("corps de l'erreur illisible : %v (corps : %s)", err, text)
	}
	message, _ := body["message"].(string)
	if len(body) != 2 || body["error"] != code || message == "" {
		t.Errorf(`corps %s, attendu {"error": %q, "message": "..."} sans autre champ`, text, code)
	}
	return message
}

// decodeJSON vérifie une réponse réussie (200, JSON) et décode son corps
// dans v.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("code HTTP %d, attendu 200 (corps : %s)", rec.Code, rec.Body)
	}
	if got := contentType(rec); got != jsonContentType {
		t.Errorf("Content-Type %q, attendu %q", got, jsonContentType)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("corps illisible : %v (corps : %s)", err, rec.Body)
	}
}

// orderJSON est une commande telle que l'API l'envoie, réduite aux champs
// que vérifient les tests. Les noms des champs JSON sont écrits ici en
// toutes lettres : ils font partie du contrat de l'API.
type orderJSON struct {
	ID        int64  `json:"id"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Lines     []struct {
		ID       int64   `json:"id"`
		SKU      string  `json:"sku"`
		Location string  `json:"location"`
		StockQty int     `json:"stockQty"`
		PickedAt *string `json:"pickedAt"`
		PickedBy *string `json:"pickedBy"`
	} `json:"lines"`
}

// lineOf renvoie l'état de la ligne lineID dans o : préparateur (chaîne vide
// si la ligne n'est pas préparée) et stock du produit.
func lineOf(t *testing.T, o orderJSON, lineID int64) (pickedBy string, stockQty int) {
	t.Helper()
	for _, l := range o.Lines {
		if l.ID != lineID {
			continue
		}
		if (l.PickedAt == nil) != (l.PickedBy == nil) {
			t.Errorf("ligne %d : pickedAt et pickedBy incohérents", lineID)
		}
		if l.PickedBy != nil {
			pickedBy = *l.PickedBy
		}
		return pickedBy, l.StockQty
	}
	t.Fatalf("ligne %d absente de la commande %d", lineID, o.ID)
	return "", 0
}

// checkNothingPicked vérifie qu'aucune ligne n'est préparée et que les stocks
// sont ceux du jeu de données : les requêtes refusées n'ont rien modifié.
func checkNothingPicked(t *testing.T) {
	t.Helper()
	var picked, stock1, stock2 int
	err := pool.QueryRow(t.Context(), `
		SELECT (SELECT count(*) FROM order_lines WHERE picked_at IS NOT NULL),
		       (SELECT stock_qty FROM products WHERE id = 1),
		       (SELECT stock_qty FROM products WHERE id = 2)`).Scan(&picked, &stock1, &stock2)
	if err != nil {
		t.Fatalf("lecture de l'état de la base : %v", err)
	}
	if picked != 0 || stock1 != 10 || stock2 != 2 {
		t.Errorf("%d ligne(s) préparée(s), stocks %d et %d : attendu 0 ligne, stocks 10 et 2", picked, stock1, stock2)
	}
}

// brokenHandler renvoie le routeur de l'API branché sur un pool fermé :
// chaque accès à la base échoue, comme si elle était tombée.
func brokenHandler(t *testing.T) http.Handler {
	t.Helper()
	p, err := pgxpool.NewWithConfig(t.Context(), pool.Config())
	if err != nil {
		t.Fatalf("pool de test : %v", err)
	}
	p.Close()
	return httpapi.NewHandler(store.New(p))
}

// captureLogs redirige les journaux du serveur (paquet log) vers un tampon
// jusqu'à la fin du test, et renvoie ce tampon.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &logs
}
