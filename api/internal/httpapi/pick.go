package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/knudsem/cairn/api/internal/store"
)

// maxPickBodyBytes limite la taille du corps de la validation. {"picker":
// "..."} avec 50 caractères tient largement dans 1 024 octets, même si chaque
// caractère est écrit en échappement JSON : 6 octets par caractère, 12 pour
// un émoji, soit 614 octets pour 50 émojis.
const maxPickBodyBytes = 1024

// maxPickerLength est la longueur maximale du nom du préparateur, en
// caractères et non en octets : « é » compte pour un caractère.
const maxPickerLength = 50

// Messages des corps refusés, partagés par plusieurs cas.
const (
	msgNotJSON = "Le corps de la requête doit être du JSON, avec l'en-tête Content-Type: application/json."
	msgBadBody = `Le corps de la requête doit être un seul objet JSON de la forme {"picker": "Nom du préparateur"}, sans autre champ.`
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
	picker, ok := readPicker(w, r)
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

// readPicker lit le corps de la requête et renvoie le nom du préparateur,
// débarrassé de ses espaces de début et de fin. Si le corps est refusé,
// readPicker répond 400 lui-même et renvoie false, comme parseID.
func readPicker(w http.ResponseWriter, r *http.Request) (string, bool) {
	// mime.ParseMediaType accepte un paramètre comme « ; charset=utf-8 », et
	// met le type en minuscules.
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, msgNotJSON)
		return "", false
	}

	// MaxBytesReader coupe la lecture au-delà de maxPickBodyBytes : un corps
	// énorme ne peut pas occuper la mémoire du serveur. DisallowUnknownFields
	// refuse un champ autre que picker, au lieu de l'ignorer en silence.
	// Limites connues d'encoding/json (docs/PLAN.md, étape 7) : la
	// comparaison des noms de champs ignore la casse (« PICKER » est
	// accepté), et une clé en double garde sa dernière valeur. Sans
	// conséquence avec un seul champ.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPickBodyBytes))
	dec.DisallowUnknownFields()
	var body pickRequest
	if err := decodeOne(dec, &body); err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				`Le corps de la requête est vide. Format attendu : {"picker": "Nom du préparateur"}.`)
		case errors.As(err, &tooLarge):
			writeError(w, http.StatusBadRequest, codeInvalidRequest,
				fmt.Sprintf("Le corps de la requête dépasse %d octets.", tooLarge.Limit))
		default:
			writeError(w, http.StatusBadRequest, codeInvalidRequest, msgBadBody)
		}
		return "", false
	}

	picker := strings.TrimSpace(body.Picker)
	if message := checkPicker(picker); message != "" {
		writeError(w, http.StatusBadRequest, codeInvalidRequest, message)
		return "", false
	}
	return picker, true
}

// decodeOne lit exactement une valeur JSON dans v : après elle, le corps ne
// doit contenir que des espaces. Sans ce contrôle, json.Decoder s'arrêterait
// à la fin du premier objet et ignorerait la suite (« {...} {...} »). Un
// corps vide donne io.EOF.
func decodeOne(dec *json.Decoder, v any) error {
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Une seconde lecture doit trouver la fin du corps (io.EOF). Elle peut
	// aussi buter sur la limite de taille, ou trouver une autre valeur.
	err := dec.Decode(&struct{}{})
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("plusieurs valeurs JSON dans le corps")
}

// checkPicker vérifie le nom du préparateur, déjà débarrassé de ses espaces
// de début et de fin. Elle renvoie le message de refus, ou une chaîne vide si
// le nom est accepté.
func checkPicker(name string) string {
	if name == "" {
		return "Le nom du préparateur est obligatoire."
	}
	if utf8.RuneCountInString(name) > maxPickerLength {
		return fmt.Sprintf("Le nom du préparateur dépasse %d caractères.", maxPickerLength)
	}
	hasLetterOrDigit := false
	for _, c := range name {
		switch {
		case c == utf8.RuneError:
			// U+FFFD : le décodeur JSON le met à la place des octets qui ne
			// sont pas de l'UTF-8 valide.
			return "Le nom du préparateur contient des octets invalides : le corps doit être encodé en UTF-8."
		case !unicode.IsPrint(c) || unicode.Is(unicode.Other_Default_Ignorable_Code_Point, c):
			// unicode.IsPrint accepte les lettres, les marques (accents), les
			// chiffres, la ponctuation, les symboles et l'espace ordinaire.
			// Il refuse les caractères de contrôle (NUL compris, que
			// PostgreSQL refuserait avec une erreur 500), les autres espaces
			// et les caractères de format, invisibles. Quelques lettres qui
			// s'affichent vides, comme le remplissage coréen U+3164, passent
			// IsPrint : Other_Default_Ignorable_Code_Point les refuse.
			return "Le nom du préparateur contient un caractère non autorisé : caractère de contrôle, espace spécial ou caractère invisible."
		case unicode.IsLetter(c) || unicode.IsNumber(c):
			hasLetterOrDigit = true
		}
	}
	// Un nom fait seulement de ponctuation, de symboles ou d'accents isolés
	// s'afficherait vide ou presque : « préparée par ... ».
	if !hasLetterOrDigit {
		return "Le nom du préparateur doit contenir au moins une lettre ou un chiffre."
	}
	return ""
}
