package httpapi_test

// Tests des routes de lecture (étape 4) et des réponses du routeur : codes
// HTTP, format JSON, et erreurs.

import (
	"net/http"
	"strings"
	"testing"
)

// GET /health : 200 {"status":"ok"} quand la base répond, 503
// base_indisponible sinon, avec le détail dans les journaux.
func TestHealth(t *testing.T) {
	rec := get(t, "/health")
	var body map[string]string
	decodeJSON(t, rec, &body)
	if len(body) != 1 || body["status"] != "ok" {
		t.Errorf(`corps %s, attendu {"status":"ok"}`, rec.Body)
	}

	logs := captureLogs(t)
	rec = request(t, brokenHandler(t), http.MethodGet, "/health", "", "")
	checkError(t, rec, http.StatusServiceUnavailable, "base_indisponible")
	if !strings.Contains(logs.String(), "GET /health : base injoignable : ") {
		t.Errorf("erreur absente des journaux : %q", logs.String())
	}
}

// GET /api/orders : la liste avec son avancement, le filtre par statut, et
// les paramètres refusés.
func TestListOrdersRoute(t *testing.T) {
	setup(t)
	var orders []struct {
		Reference   string `json:"reference"`
		Status      string `json:"status"`
		LineCount   int    `json:"lineCount"`
		PickedCount int    `json:"pickedCount"`
	}
	decodeJSON(t, get(t, "/api/orders"), &orders)
	if len(orders) != 2 || orders[0].Reference != "CMD-A" || orders[0].LineCount != 2 || orders[0].PickedCount != 0 ||
		orders[1].Reference != "CMD-B" || orders[1].LineCount != 1 {
		t.Errorf("liste %+v, attendu CMD-A (0/2) puis CMD-B (0/1)", orders)
	}

	// Un filtre sans résultat donne une liste vide [], jamais null.
	rec := get(t, "/api/orders?status=prete")
	decodeJSON(t, rec, &orders)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("filtre prete : corps %s, attendu []", rec.Body)
	}

	for _, query := range []string{"?status=inconnu", "?status=a_preparer&status=prete", "?status=%zz"} {
		t.Run(query, func(t *testing.T) {
			checkError(t, get(t, "/api/orders"+query), http.StatusBadRequest, "requete_invalide")
		})
	}
}

// GET /api/orders/{id} : la commande et ses lignes triées par emplacement,
// 404 pour une commande inexistante, 400 pour un identifiant invalide.
func TestGetOrderRoute(t *testing.T) {
	setup(t)
	var order orderJSON
	decodeJSON(t, get(t, "/api/orders/101"), &order)
	if order.ID != 101 || order.Reference != "CMD-A" || order.Status != "a_preparer" || len(order.Lines) != 2 ||
		order.Lines[0].Location != "A-01-1" || order.Lines[1].Location != "B-02-1" {
		t.Errorf("commande %+v, attendu CMD-A avec ses lignes en A-01-1 puis B-02-1", order)
	}

	if got := checkError(t, get(t, "/api/orders/999"), http.StatusNotFound, "commande_introuvable"); got != "La commande 999 n'existe pas." {
		t.Errorf("message %q", got)
	}
	for _, id := range []string{"abc", "0", "0101", "-101", "+101"} {
		t.Run(id, func(t *testing.T) {
			checkError(t, get(t, "/api/orders/"+id), http.StatusBadRequest, "requete_invalide")
		})
	}
}

// GET /api/products : les produits et leur stock, triés par emplacement.
func TestListProductsRoute(t *testing.T) {
	setup(t)
	var products []struct {
		SKU      string `json:"sku"`
		StockQty int    `json:"stockQty"`
	}
	decodeJSON(t, get(t, "/api/products"), &products)
	if len(products) != 2 || products[0].SKU != "P-1" || products[0].StockQty != 10 ||
		products[1].SKU != "P-2" || products[1].StockQty != 2 {
		t.Errorf("produits %+v, attendu P-1 (10) puis P-2 (2)", products)
	}
}

// Une panne de la base donne aussi 500 erreur_interne sur les lectures, avec
// le message générique.
func TestReadRoutesDatabaseError(t *testing.T) {
	captureLogs(t)
	broken := brokenHandler(t)
	for _, target := range []string{"/api/orders", "/api/orders/101", "/api/products"} {
		t.Run(target, func(t *testing.T) {
			rec := request(t, broken, http.MethodGet, target, "", "")
			if got := checkError(t, rec, http.StatusInternalServerError, "erreur_interne"); got != internalMessage {
				t.Errorf("message %q, attendu %q", got, internalMessage)
			}
		})
	}
}

// Le routeur répond lui-même, en texte brut, à une route inconnue (404) et
// à une méthode non prévue (405) : ces réponses sont hors du format JSON
// unique (docs/PLAN.md, section 4.4).
func TestRouterErrors(t *testing.T) {
	rec := get(t, "/api/inconnu")
	if rec.Code != http.StatusNotFound || !strings.HasPrefix(contentType(rec), "text/plain") {
		t.Errorf("route inconnue : code %d, type %q ; attendu 404 en texte brut", rec.Code, contentType(rec))
	}

	rec = get(t, "/api/orders/101/lines/201/pick")
	if allow := rec.Result().Header.Get("Allow"); rec.Code != http.StatusMethodNotAllowed || allow != "POST" {
		t.Errorf("GET sur la validation : code %d, Allow %q ; attendu 405 et POST", rec.Code, allow)
	}
	rec = request(t, handler, http.MethodPost, "/api/orders", "application/json", `{}`)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/orders : code %d, attendu 405", rec.Code)
	}
}
