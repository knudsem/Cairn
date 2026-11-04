package httpapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/knudsem/cairn/api/internal/store"
)

// listPickers traite GET /api/pickers : les profils de préparateurs, triés
// par nom.
func (h *handlers) listPickers(w http.ResponseWriter, r *http.Request) {
	pickers, err := h.store.ListPickers(r.Context())
	if err != nil {
		writeInternalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pickers)
}

// pickerRequest est le corps attendu par la création d'un profil :
// {"name": "Nom"}.
type pickerRequest struct {
	Name string `json:"name"`
}

// createPicker traite POST /api/pickers : le profil est créé et renvoyé,
// avec le code 201 Created. Le nom suit les mêmes règles que celui du
// préparateur à l'étape 7 (body.go).
func (h *handlers) createPicker(w http.ResponseWriter, r *http.Request) {
	var body pickerRequest
	if !readJSON(w, r, &body, `{"name": "Nom du profil"}`) {
		return
	}
	name, ok := readName(w, body.Name, "du profil")
	if !ok {
		return
	}

	picker, err := h.store.CreatePicker(r.Context(), name)
	var exists *store.PickerExistsError
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, picker)
	case errors.As(err, &exists):
		// Le message donne le nom tel qu'il est enregistré : « Mathias »
		// pour une demande « mathias ».
		writeError(w, http.StatusConflict, codePickerExists,
			fmt.Sprintf("Le profil « %s » existe déjà.", exists.Name))
	case errors.Is(err, store.ErrTooManyPickers):
		writeError(w, http.StatusConflict, codeTooManyPickers,
			fmt.Sprintf("%d profils au maximum : impossible d'en créer un autre.", store.MaxPickers))
	default:
		writeInternalError(w, r, err)
	}
}
