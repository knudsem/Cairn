package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/knudsem/cairn/api/internal/store"
)

// healthTimeout limite l'attente de la base dans GET /health.
const healthTimeout = 2 * time.Second

// handlers regroupe les fonctions qui traitent les requêtes : des méthodes
// qui partagent l'accès au store.
type handlers struct {
	store *store.Store
}

// health traite GET /health : 200 {"status":"ok"} si la base répond, 503 sinon.
func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
	defer cancel()

	if err := h.store.Ping(ctx); err != nil {
		log.Printf("GET /health : base injoignable : %v", err)
		writeError(w, http.StatusServiceUnavailable, codeDatabaseDown, "La base de données ne répond pas.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// listOrders traite GET /api/orders, avec le filtre facultatif ?status=.
func (h *handlers) listOrders(w http.ResponseWriter, r *http.Request) {
	// url.ParseQuery signale une chaîne mal formée (%zz, point-virgule), que
	// r.URL.Query() ignorerait en silence : le filtre disparaîtrait alors sans
	// erreur. Un statut répété (?status=a&status=b) est refusé aussi.
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["status"]) > 1 {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, "Paramètres de recherche mal formés.")
		return
	}
	status := query.Get("status")
	if status != "" && !store.ValidStatus(status) {
		writeError(w, http.StatusBadRequest, codeInvalidRequest,
			fmt.Sprintf("Statut inconnu : %q. Valeurs possibles : a_preparer, en_cours, prete.", status))
		return
	}

	orders, err := h.store.ListOrders(r.Context(), status)
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// getOrder traite GET /api/orders/{id} : la commande et ses lignes.
func (h *handlers) getOrder(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r.PathValue("id"), "commande")
	if !ok {
		return
	}

	order, err := h.store.GetOrder(r.Context(), id)
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, codeOrderNotFound, fmt.Sprintf("La commande %d n'existe pas.", id))
		return
	}
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// listProducts traite GET /api/products : les produits et leur stock.
func (h *handlers) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := h.store.ListProducts(r.Context())
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

// parseID lit un identifiant pris dans le chemin : un entier strictement
// positif. S'il est invalide, parseID répond 400 lui-même et renvoie false.
// what nomme l'objet dans le message (« commande », « ligne »).
func parseID(w http.ResponseWriter, raw, what string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, codeInvalidRequest,
			fmt.Sprintf("Identifiant de %s invalide : %q. Un nombre entier positif est attendu.", what, raw))
		return 0, false
	}
	return id, true
}
