package httpapi

import (
	"net/http"

	"github.com/knudsem/cairn/api/internal/store"
)

// NewHandler renvoie le routeur de l'API : la table de toutes ses routes
// (docs/PLAN.md, section 4.1). Le routeur répond lui-même 404 à une route
// inconnue, et 405 à une méthode non prévue.
func NewHandler(s *store.Store) http.Handler {
	h := &handlers{store: s}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /api/orders", h.listOrders)
	mux.HandleFunc("GET /api/orders/{id}", h.getOrder)
	mux.HandleFunc("POST /api/orders/{id}/lines/{lineId}/pick", h.pickLine)
	mux.HandleFunc("GET /api/products", h.listProducts)
	mux.HandleFunc("GET /api/pickers", h.listPickers)
	mux.HandleFunc("POST /api/pickers", h.createPicker)
	mux.HandleFunc("DELETE /api/pickers/{id}", h.deletePicker)
	// Remise à zéro de la démo (étape 15) : la route n'existe que si
	// DEMO_RESET_ENABLED vaut true. Sinon, le routeur répond 404, comme pour
	// toute route inconnue.
	if DemoResetEnabled() {
		mux.HandleFunc("POST /api/demo/reset", h.resetDemo)
	}
	return mux
}
