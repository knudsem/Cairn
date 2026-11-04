package httpapi_test

// Tests de GET et POST /api/pickers (docs/PLAN.md, étape 7 bis) : réponses,
// conflits, contrôle du corps et du nom, panne de base. Les règles du nom
// sont celles de l'étape 7 (body.go) : pick_test.go les teste toutes, ce
// fichier vérifie chacune une fois pour cette route.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Messages propres à la création d'un profil.
const (
	msgPickerEmpty   = `Le corps de la requête est vide. Format attendu : {"name": "Nom du profil"}.`
	msgPickerBadBody = `Le corps de la requête doit être un seul objet JSON de la forme {"name": "Nom du profil"}, sans autre champ.`
)

// pickerJSON est un profil tel que l'API l'envoie.
type pickerJSON struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// createPicker envoie POST /api/pickers avec body comme corps JSON.
func createPicker(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, handler, http.MethodPost, "/api/pickers", "application/json", body)
}

// pickerCount renvoie le nombre de profils en base.
func pickerCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM pickers`).Scan(&n); err != nil {
		t.Fatalf("comptage des profils : %v", err)
	}
	return n
}

// GET /api/pickers : [] sans profil, puis les profils triés par nom, quel que
// soit l'ordre de création.
func TestListPickersRoute(t *testing.T) {
	setup(t)
	rec := get(t, "/api/pickers")
	var pickers []pickerJSON
	decodeJSON(t, rec, &pickers)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("sans profil : corps %s, attendu []", rec.Body)
	}

	for _, name := range []string{"Virginie", "Mathias", "Romain"} {
		if rec := createPicker(t, `{"name": "`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("création de %s : code %d (corps : %s)", name, rec.Code, rec.Body)
		}
	}
	decodeJSON(t, get(t, "/api/pickers"), &pickers)
	want := []pickerJSON{{ID: 302, Name: "Mathias"}, {ID: 303, Name: "Romain"}, {ID: 301, Name: "Virginie"}}
	if len(pickers) != len(want) {
		t.Fatalf("%d profils, attendu %d", len(pickers), len(want))
	}
	for i, w := range want {
		if pickers[i].ID != w.ID || pickers[i].Name != w.Name {
			t.Errorf("position %d : %d %q, attendu %d %q", i+1, pickers[i].ID, pickers[i].Name, w.ID, w.Name)
		}
	}
}

// POST /api/pickers : 201 et le profil créé, avec exactement ses trois
// champs ; le nom est enregistré sans ses espaces de début et de fin.
func TestCreatePickerRoute(t *testing.T) {
	setup(t)
	rec := createPicker(t, `{"name": "  Karim  "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP %d, attendu 201 (corps : %s)", rec.Code, rec.Body)
	}
	if got := contentType(rec); got != jsonContentType {
		t.Errorf("Content-Type %q, attendu %q", got, jsonContentType)
	}
	var fields map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil || len(fields) != 3 {
		t.Errorf("corps %s, attendu un objet à trois champs (id, name, createdAt)", rec.Body)
	}
	var p pickerJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("corps illisible : %v", err)
	}
	if p.ID != 301 || p.Name != "Karim" || !strings.HasSuffix(p.CreatedAt, "Z") {
		t.Errorf("profil %+v, attendu 301, « Karim » et une date en UTC (suffixe Z)", p)
	}
	if got := pickerCount(t); got != 1 {
		t.Errorf("%d profils en base, attendu 1", got)
	}
}

// Les deux conflits : 409 profil_existant, avec le nom tel qu'il est
// enregistré, et 409 trop_de_profils à partir de 20 profils.
func TestCreatePickerConflicts(t *testing.T) {
	setup(t)
	if rec := createPicker(t, `{"name": "Mathias"}`); rec.Code != http.StatusCreated {
		t.Fatalf("création de Mathias : code %d (corps : %s)", rec.Code, rec.Body)
	}
	if got := checkError(t, createPicker(t, `{"name": "mathias"}`), http.StatusConflict, "profil_existant"); got != "Le profil « Mathias » existe déjà." {
		t.Errorf("message %q", got)
	}

	// 19 profils de plus, en SQL : la table est pleine.
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO pickers (name)
		SELECT 'Profil ' || lpad(g::text, 2, '0') FROM generate_series(1, 19) AS g`); err != nil {
		t.Fatalf("remplissage : %v", err)
	}
	if got := checkError(t, createPicker(t, `{"name": "Nouveau"}`), http.StatusConflict, "trop_de_profils"); got != "20 profils au maximum : impossible d'en créer un autre." {
		t.Errorf("message %q", got)
	}
	if got := pickerCount(t); got != 20 {
		t.Errorf("%d profils, attendu 20", got)
	}
}

// Chaque règle du corps et du nom donne 400 requete_invalide, avec un
// message qui parle du profil, et ne crée rien.
func TestCreatePickerInvalidRequests(t *testing.T) {
	setup(t)
	const appJSON = "application/json"
	for _, c := range []struct{ name, contentType, body, message string }{
		{"sans Content-Type", "", `{"name": "Karim"}`, msgNotJSON},
		{"corps vide", appJSON, ``, msgPickerEmpty},
		{"JSON invalide", appJSON, `{name: Karim}`, msgPickerBadBody},
		{"champ inconnu", appJSON, `{"name": "Karim", "admin": true}`, msgPickerBadBody},
		{"champ de la validation au lieu de name", appJSON, `{"picker": "Karim"}`, msgPickerBadBody},
		{"deux objets", appJSON, `{"name": "Karim"} {"name": "Léa"}`, msgPickerBadBody},
		{"corps énorme", appJSON, `{"name": "` + strings.Repeat("a", 2000) + `"}`, msgTooLarge},
		{"nom vide", appJSON, `{"name": "   "}`, "Le nom du profil est obligatoire."},
		{"51 caractères", appJSON, `{"name": "` + strings.Repeat("a", 51) + `"}`, "Le nom du profil dépasse 50 caractères."},
		{"caractère NUL", appJSON, `{"name": "Ka` + jsonEscape(0) + `rim"}`, "Le nom du profil contient un caractère non autorisé : caractère de contrôle, espace spécial ou caractère invisible."},
		{"sans lettre ni chiffre", appJSON, `{"name": "..."}`, "Le nom du profil doit contenir au moins une lettre ou un chiffre."},
		{"octet qui n'est pas de l'UTF-8", appJSON, "{\"name\": \"Ka\xffrim\"}", "Le nom du profil contient des octets invalides : le corps doit être encodé en UTF-8."},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := request(t, handler, http.MethodPost, "/api/pickers", c.contentType, c.body)
			if got := checkError(t, rec, http.StatusBadRequest, "requete_invalide"); got != c.message {
				t.Errorf("message %q, attendu %q", got, c.message)
			}
		})
	}
	if got := pickerCount(t); got != 0 {
		t.Errorf("%d profils créés malgré les refus, attendu 0", got)
	}
}

// Une panne de la base donne 500 erreur_interne avec le message générique,
// sur les deux routes ; l'erreur est journalisée avec la route.
func TestPickersDatabaseError(t *testing.T) {
	logs := captureLogs(t)
	broken := brokenHandler(t)
	for _, c := range []struct{ method, body string }{
		{http.MethodGet, ""},
		{http.MethodPost, `{"name": "Karim"}`},
	} {
		t.Run(c.method, func(t *testing.T) {
			contentType := ""
			if c.body != "" {
				contentType = "application/json"
			}
			rec := request(t, broken, c.method, "/api/pickers", contentType, c.body)
			if got := checkError(t, rec, http.StatusInternalServerError, "erreur_interne"); got != internalMessage {
				t.Errorf("message %q, attendu %q", got, internalMessage)
			}
			if !strings.Contains(logs.String(), c.method+" /api/pickers : ") {
				t.Errorf("erreur absente des journaux : %q", logs.String())
			}
		})
	}
}

// Une autre méthode que GET ou POST reçoit 405 du routeur.
func TestPickersOtherMethods(t *testing.T) {
	rec := request(t, handler, http.MethodDelete, "/api/pickers", "", "")
	if allow := rec.Result().Header.Get("Allow"); rec.Code != http.StatusMethodNotAllowed || !strings.Contains(allow, "GET") || !strings.Contains(allow, "POST") {
		t.Errorf("DELETE /api/pickers : code %d, Allow %q ; attendu 405, GET et POST", rec.Code, allow)
	}
}
