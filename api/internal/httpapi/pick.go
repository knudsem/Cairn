package httpapi

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/knudsem/cairn/api/internal/store"
)

// pickRequest est le corps attendu par la validation : {"picker": "Nom"}.
type pickRequest struct {
	Picker string `json:"picker"`
}

// pickLine traite POST /api/orders/{id}/lines/{lineId}/pick : la ligne
// lineId de la commande id est validée au nom du préparateur du corps. La
// réponse est la commande mise à jour, au même format que GET
// /api/orders/{id}.
func (h *handlers) pickLine(w http.ResponseWriter, r *http.Request) {
	orderID, ok := parseID(w, r.PathValue("id"), "commande")
	if !ok {
		return
	}
	lineID, ok := parseID(w, r.PathValue("lineId"), "ligne")
	if !ok {
		return
	}
	// Contrôle du corps et du nom : body.go, commun avec la création d'un
	// profil.
	var body pickRequest
	if !readJSON(w, r, &body, `{"picker": "Nom du préparateur"}`) {
		return
	}
	picker, ok := readName(w, body.Picker, "du préparateur")
	if !ok {
		return
	}

	if err := h.store.PickLine(r.Context(), orderID, lineID, picker); err != nil {
		writePickError(w, r, err, orderID, lineID)
		return
	}

	// La validation est enregistrée : la commande est relue pour la réponse.
	// L'interface voit ainsi tout de suite le nouveau stock et le passage
	// éventuel à « prete », sans second appel (docs/PLAN.md, section 4.4).
	order, err := h.store.GetOrder(r.Context(), orderID)
	if err != nil {
		// Toujours une erreur 500, mais le message dit la vérité au client :
		// la validation a réussi. Un nouvel essai répondrait deja_preparee.
		log.Printf("%s %s : validation enregistrée, mais relecture impossible : %v", r.Method, r.URL.Path, err)
		writeError(w, http.StatusInternalServerError, codeInternal,
			"La validation est enregistrée, mais la commande n'a pas pu être relue. Rechargez la commande.")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// writePickError traduit une erreur de PickLine en réponse HTTP, d'après le
// tableau de docs/PLAN.md, section 4.3. errors.Is reconnaît une erreur
// métier même emballée avec %w ; errors.As récupère en plus l'erreur
// détaillée du stock insuffisant, avec ses quantités.
func writePickError(w http.ResponseWriter, r *http.Request, err error, orderID, lineID int64) {
	var stockErr *store.InsufficientStockError
	switch {
	case errors.Is(err, store.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, codeOrderNotFound,
			fmt.Sprintf("La commande %d n'existe pas.", orderID))
	case errors.Is(err, store.ErrLineNotFound):
		writeError(w, http.StatusNotFound, codeLineNotFound,
			fmt.Sprintf("La ligne %d n'existe pas dans la commande %d.", lineID, orderID))
	case errors.Is(err, store.ErrAlreadyPicked):
		writeError(w, http.StatusConflict, codeAlreadyPicked, "Cette ligne a déjà été préparée.")
	case errors.As(err, &stockErr):
		writeError(w, http.StatusConflict, codeInsufficientStock,
			"Stock insuffisant : "+stockErr.Quantities()+".")
	default:
		writeInternalError(w, r, err)
	}
}
