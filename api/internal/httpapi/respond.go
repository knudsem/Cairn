// Package httpapi traduit les requêtes HTTP en appels au store, et ses
// résultats en réponses JSON. Il parle HTTP et ignore tout de SQL.
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

// Codes d'erreur de l'API (docs/PLAN.md, section 4.3) : des valeurs stables,
// que le programme client peut tester, à côté d'un message lisible.
const (
	codeInvalidRequest    = "requete_invalide"
	codeOrderNotFound     = "commande_introuvable"
	codeLineNotFound      = "ligne_introuvable"
	codeAlreadyPicked     = "deja_preparee"
	codeInsufficientStock = "stock_insuffisant"
	codePickerExists      = "profil_existant"
	codeTooManyPickers    = "trop_de_profils"
	codeInternal          = "erreur_interne"
	codeDatabaseDown      = "base_indisponible"
)

// errorResponse est le format unique des erreurs renvoyées par les handlers
// (docs/PLAN.md, section 4.2). Le routeur, lui, répond en texte brut pour une
// route inconnue (404) ou une méthode non prévue (405) : le client ne lit le
// JSON d'une erreur que si la réponse est de type application/json.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeJSON envoie value encodée en JSON, avec le code HTTP status. Les
// en-têtes doivent être fixés avant WriteHeader : ensuite, ils sont figés et
// toute modification est ignorée.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("écriture de la réponse JSON : %v", err)
	}
}

// writeError envoie une erreur au format unique.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}

// writeInternalError journalise err et renvoie une erreur 500 au message
// générique : le détail technique reste dans les journaux du serveur.
func writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("%s %s : %v", r.Method, r.URL.Path, err)
	writeError(w, http.StatusInternalServerError, codeInternal,
		"Erreur interne du serveur. Réessayez dans un instant.")
}
