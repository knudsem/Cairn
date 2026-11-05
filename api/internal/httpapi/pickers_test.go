package httpapi_test

// Tests de GET et POST /api/pickers (docs/PLAN.md, étape 7 bis) : réponses,
// conflits, contrôle du corps et du nom, panne de base. Puis DELETE
// /api/pickers/{id} (étape 13) : 204, 404, 400, historique intact. Les règles du nom
// sont celles de l'étape 7 (body.go) : pick_test.go les teste toutes, ce
// fichier vérifie chacune une fois pour cette route, ainsi que la limite de
// 50 caractères, que la base vérifie aussi pour les profils.

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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

// postPicker envoie POST /api/pickers avec body comme corps JSON.
func postPicker(t *testing.T, body string) *httptest.ResponseRecorder {
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

// checkPickerFields vérifie qu'un profil en JSON a exactement les champs id,
// name et createdAt, à la majuscule près. Le décodage dans pickerJSON ne
// suffit pas : encoding/json accepte aussi une clé « Name » ou « NAME » pour
// le champ name.
func checkPickerFields(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("profil illisible : %v (%s)", err, raw)
	}
	keys := slices.Sorted(maps.Keys(fields))
	if want := []string{"createdAt", "id", "name"}; !slices.Equal(keys, want) {
		t.Errorf("champs %v, attendu %v", keys, want)
	}
}

// GET /api/pickers : [] sans profil, puis les profils triés par nom, quel que
// soit l'ordre de création, chacun avec exactement ses trois champs.
func TestListPickersRoute(t *testing.T) {
	setup(t)
	rec := get(t, "/api/pickers")
	var pickers []pickerJSON
	decodeJSON(t, rec, &pickers)
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("sans profil : corps %s, attendu []", strings.TrimSpace(rec.Body.String()))
	}

	for _, name := range []string{"Virginie", "Claire", "Romain"} {
		if rec := postPicker(t, `{"name": "`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("création de %s : code %d (corps : %s)", name, rec.Code, rec.Body)
		}
	}
	rec = get(t, "/api/pickers")
	decodeJSON(t, rec, &pickers)
	want := []pickerJSON{{ID: 302, Name: "Claire"}, {ID: 303, Name: "Romain"}, {ID: 301, Name: "Virginie"}}
	if len(pickers) != len(want) {
		t.Fatalf("%d profils, attendu %d", len(pickers), len(want))
	}
	for i, w := range want {
		if pickers[i].ID != w.ID || pickers[i].Name != w.Name {
			t.Errorf("position %d : %d %q, attendu %d %q", i+1, pickers[i].ID, pickers[i].Name, w.ID, w.Name)
		}
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil || len(raw) == 0 {
		t.Fatalf("liste illisible : %v", err)
	}
	checkPickerFields(t, raw[0])
}

// POST /api/pickers : 201 et le profil créé, avec exactement ses trois
// champs ; le nom est enregistré sans ses espaces de début et de fin.
func TestCreatePickerRoute(t *testing.T) {
	setup(t)
	rec := postPicker(t, `{"name": "  Karim  "}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code HTTP %d, attendu 201 (corps : %s)", rec.Code, rec.Body)
	}
	if got := contentType(rec); got != jsonContentType {
		t.Errorf("Content-Type %q, attendu %q", got, jsonContentType)
	}
	checkPickerFields(t, rec.Body.Bytes())
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

// Noms acceptés à la limite et nettoyés : 50 « é » (100 octets : l'API et la
// base comptent des caractères), entourés d'espaces ou non, et des espaces
// doublées au milieu, réduites à une seule.
func TestCreatePickerAcceptedNames(t *testing.T) {
	fifty := strings.Repeat("é", 50)
	for _, c := range []struct{ label, name, stored string }{
		{"50 caractères accentués", fifty, fifty},
		{"50 caractères accentués entourés d'espaces", "  " + fifty + "  ", fifty},
		{"espaces doublées au milieu", "Jean   Pierre", "Jean Pierre"},
	} {
		t.Run(c.label, func(t *testing.T) {
			setup(t)
			rec := postPicker(t, `{"name": "`+c.name+`"}`)
			if rec.Code != http.StatusCreated {
				t.Fatalf("code HTTP %d, attendu 201 (corps : %s)", rec.Code, rec.Body)
			}
			var p pickerJSON
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Name != c.stored {
				t.Errorf("nom enregistré %q (erreur %v), attendu %q", p.Name, err, c.stored)
			}
		})
	}
}

// Les deux conflits : 409 profil_existant, avec le nom tel qu'il est
// enregistré, et 409 trop_de_profils à partir de 20 profils.
func TestCreatePickerConflicts(t *testing.T) {
	setup(t)
	for _, name := range []string{"Claire", "Jean Pierre"} {
		if rec := postPicker(t, `{"name": "`+name+`"}`); rec.Code != http.StatusCreated {
			t.Fatalf("création de %s : code %d (corps : %s)", name, rec.Code, rec.Body)
		}
	}
	for _, c := range []struct{ name, message string }{
		{"claire", "Le profil « Claire » existe déjà."},
		{"Jean  Pierre", "Le profil « Jean Pierre » existe déjà."},
	} {
		if got := checkError(t, postPicker(t, `{"name": "`+c.name+`"}`), http.StatusConflict, "profil_existant"); got != c.message {
			t.Errorf("%q : message %q, attendu %q", c.name, got, c.message)
		}
	}

	// 18 profils de plus, en SQL : la table est pleine.
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO pickers (name)
		SELECT 'Profil ' || lpad(g::text, 2, '0') FROM generate_series(1, 18) AS g`); err != nil {
		t.Fatalf("remplissage : %v", err)
	}
	const full = "20 profils au maximum : impossible d'en créer un autre."
	if got := checkError(t, postPicker(t, `{"name": "Nouveau"}`), http.StatusConflict, "trop_de_profils"); got != full {
		t.Errorf("message %q, attendu %q", got, full)
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
// sur les trois routes ; l'erreur est journalisée avec la route.
func TestPickersDatabaseError(t *testing.T) {
	logs := captureLogs(t)
	broken := brokenHandler(t)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/pickers", ""},
		{http.MethodPost, "/api/pickers", `{"name": "Karim"}`},
		{http.MethodDelete, "/api/pickers/1", ""},
	} {
		t.Run(c.method, func(t *testing.T) {
			contentType := ""
			if c.body != "" {
				contentType = "application/json"
			}
			rec := request(t, broken, c.method, c.path, contentType, c.body)
			if got := checkError(t, rec, http.StatusInternalServerError, "erreur_interne"); got != internalMessage {
				t.Errorf("message %q, attendu %q", got, internalMessage)
			}
			if !strings.Contains(logs.String(), c.method+" "+c.path+" : ") {
				t.Errorf("erreur absente des journaux : %q", logs.String())
			}
		})
	}
}

// Une autre méthode que GET ou POST sur la liste, ou que DELETE sur un
// profil, reçoit 405 du routeur : la liste entière ne se supprime pas.
func TestPickersOtherMethods(t *testing.T) {
	rec := request(t, handler, http.MethodDelete, "/api/pickers", "", "")
	if allow := rec.Result().Header.Get("Allow"); rec.Code != http.StatusMethodNotAllowed || !strings.Contains(allow, "GET") || !strings.Contains(allow, "POST") {
		t.Errorf("DELETE /api/pickers : code %d, Allow %q ; attendu 405, GET et POST", rec.Code, allow)
	}
	rec = request(t, handler, http.MethodPut, "/api/pickers/1", "application/json", `{"name": "Test"}`)
	if allow := rec.Result().Header.Get("Allow"); rec.Code != http.StatusMethodNotAllowed || allow != "DELETE" {
		t.Errorf("PUT /api/pickers/1 : code %d, Allow %q ; attendu 405 et DELETE", rec.Code, allow)
	}
}

// deletePicker envoie DELETE /api/pickers/{id}, avec id écrit tel quel dans
// le chemin.
func deletePicker(t *testing.T, id string) *httptest.ResponseRecorder {
	t.Helper()
	return request(t, handler, http.MethodDelete, "/api/pickers/"+id, "", "")
}

// createTestPicker crée le profil Test et renvoie son identifiant, écrit
// pour le chemin.
func createTestPicker(t *testing.T) string {
	t.Helper()
	rec := postPicker(t, `{"name": "Test"}`)
	var p pickerJSON
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &p) != nil {
		t.Fatalf("création de Test : code %d (corps : %s)", rec.Code, rec.Body)
	}
	return strconv.FormatInt(p.ID, 10)
}

// Étape 13 : DELETE /api/pickers/{id} répond 204, sans corps, et le profil
// disparaît de la liste. Une seconde suppression, ou un identifiant
// inconnu, répond 404 profil_introuvable au format d'erreur unique.
func TestDeletePickerRoute(t *testing.T) {
	setup(t)
	id := createTestPicker(t)

	rec := deletePicker(t, id)
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Errorf("suppression : code %d, corps %q ; attendu 204 sans corps", rec.Code, rec.Body)
	}
	rec = get(t, "/api/pickers")
	var pickers []pickerJSON
	decodeJSON(t, rec, &pickers)
	if len(pickers) != 0 {
		t.Errorf("profils après la suppression : %s, attendu []", strings.TrimSpace(rec.Body.String()))
	}

	for _, unknown := range []string{id, "999"} {
		want := "Le profil " + unknown + " n'existe pas."
		if got := checkError(t, deletePicker(t, unknown), http.StatusNotFound, "profil_introuvable"); got != want {
			t.Errorf("profil %s : message %q, attendu %q", unknown, got, want)
		}
	}
}

// Un identifiant invalide donne 400 requete_invalide, avec le message de
// parseID, et rien n'est supprimé.
func TestDeletePickerInvalidIDs(t *testing.T) {
	setup(t)
	createTestPicker(t)
	for _, id := range []string{"abc", "0", "-1", "+1", "01", "1.5", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			want := `Identifiant de profil invalide : "` + id + `". Un nombre entier positif, sans zéro en tête, est attendu.`
			if got := checkError(t, deletePicker(t, id), http.StatusBadRequest, "requete_invalide"); got != want {
				t.Errorf("message %q, attendu %q", got, want)
			}
		})
	}
	if got := pickerCount(t); got != 1 {
		t.Errorf("%d profil(s) après les refus, attendu 1", got)
	}
}

// L'historique reste juste, de bout en bout : une ligne validée au nom de
// Test affiche toujours Test dans GET /api/orders/{id} après la suppression
// du profil, et le même nom peut ensuite être recréé.
func TestDeletePickerKeepsHistoryAndFreesName(t *testing.T) {
	setup(t)
	id := createTestPicker(t)
	if rec := pick(t, 101, 201, `{"picker": "Test"}`); rec.Code != http.StatusOK {
		t.Fatalf("validation au nom de Test : code %d (corps : %s)", rec.Code, rec.Body)
	}
	if rec := deletePicker(t, id); rec.Code != http.StatusNoContent {
		t.Fatalf("suppression de Test : code %d (corps : %s)", rec.Code, rec.Body)
	}

	var order orderJSON
	decodeJSON(t, get(t, "/api/orders/101"), &order)
	if pickedBy, stock := lineOf(t, order, 201); pickedBy != "Test" || stock != 6 {
		t.Errorf("ligne 201 après la suppression : préparée par %q, stock %d ; attendu Test et 6", pickedBy, stock)
	}

	if again := createTestPicker(t); again == id {
		t.Errorf("Test recréé avec l'identifiant %s de l'ancien profil, attendu un nouvel identifiant", again)
	}
}
