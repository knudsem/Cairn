package httpapi

import (
	"net/http"
	"os"
)

// DemoResetEnabled dit si la remise à zéro de la démo est permise (étape
// 15) : la variable d'environnement DEMO_RESET_ENABLED vaut exactement
// true. Elle est activée dans compose.yaml et render.yaml ; absente
// ailleurs, par prudence : une vraie installation ne doit pas pouvoir
// effacer ses données d'un seul appel.
func DemoResetEnabled() bool {
	return os.Getenv("DEMO_RESET_ENABLED") == "true"
}

// resetDemo traite POST /api/demo/reset : les commandes, le stock et les
// profils reviennent à leur état de départ (store.ResetDemo), avec le code
// 204 No Content et sans corps. Le corps de la requête éventuel est ignoré.
// La route n'est déclarée que si DemoResetEnabled (routes.go).
func (h *handlers) resetDemo(w http.ResponseWriter, r *http.Request) {
	if err := h.store.ResetDemo(r.Context()); err != nil {
		writeInternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
