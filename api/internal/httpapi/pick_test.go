package httpapi_test

// Tests de POST /api/orders/{id}/lines/{lineId}/pick (docs/PLAN.md,
// étape 7) : réponse en cas de succès, traduction des erreurs métier, et
// validation de chaque entrée de la requête.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/knudsem/cairn/api/internal/httpapi"
	"github.com/knudsem/cairn/api/internal/store"
	"github.com/knudsem/cairn/api/internal/testdb"
)

// Messages de refus attendus, repris de la route.
const (
	msgNotJSON   = "Le corps de la requête doit être du JSON, avec l'en-tête Content-Type: application/json."
	msgEmpty     = `Le corps de la requête est vide. Format attendu : {"picker": "Nom du préparateur"}.`
	msgBadBody   = `Le corps de la requête doit être un seul objet JSON de la forme {"picker": "Nom du préparateur"}, sans autre champ.`
	msgTooLarge  = "Le corps de la requête dépasse 1024 octets."
	msgRequired  = "Le nom du préparateur est obligatoire."
	msgTooLong   = "Le nom du préparateur dépasse 50 caractères."
	msgForbidden = "Le nom du préparateur contient un caractère non autorisé : caractère de contrôle, espace spécial ou caractère invisible."
	msgNotUTF8   = "Le nom du préparateur contient des octets invalides : le corps doit être encodé en UTF-8."
	msgNoLetter  = "Le nom du préparateur doit contenir au moins une lettre ou un chiffre."
	msgReread    = "La validation est enregistrée, mais la commande n'a pas pu être relue. Rechargez la commande."
)

// Une validation réussie renvoie 200 et la commande mise à jour, octet pour
// octet comme GET /api/orders/{id} juste après. Le nom est enregistré sans
// ses espaces de début et de fin.
func TestPickLineReturnsUpdatedOrder(t *testing.T) {
	setup(t)

	rec := pick(t, 101, 201, `{"picker": "  Karim  "}`)
	var order orderJSON
	decodeJSON(t, rec, &order)
	if after := get(t, "/api/orders/101"); after.Body.String() != rec.Body.String() {
		t.Errorf("réponse du POST différente de GET /api/orders/101 :\nPOST %s\nGET  %s", rec.Body, after.Body)
	}
	if order.ID != 101 || order.Status != "en_cours" {
		t.Errorf("commande %d au statut %q, attendu 101 en_cours", order.ID, order.Status)
	}
	if by, stock := lineOf(t, order, 201); by != "Karim" || stock != 6 {
		t.Errorf("ligne 201 : préparée par %q, stock %d ; attendu \"Karim\" et 6 (10 - 4)", by, stock)
	}
	if by, stock := lineOf(t, order, 202); by != "" || stock != 2 {
		t.Errorf("ligne 202 : préparée par %q, stock %d ; attendu non préparée et 2", by, stock)
	}

	// La dernière ligne d'une commande la fait passer à « prete ».
	decodeJSON(t, pick(t, 102, 203, `{"picker": "Léa"}`), &order)
	if order.ID != 102 || order.Status != "prete" {
		t.Errorf("commande %d au statut %q, attendu 102 prete", order.ID, order.Status)
	}
}

// L'en-tête Content-Type est accepté avec un paramètre, comme celui
// qu'envoie l'interface Blazor (application/json; charset=utf-8), et quelle
// que soit la casse.
func TestPickLineAcceptsJSONContentTypes(t *testing.T) {
	for _, c := range []struct{ name, contentType string }{
		{"avec charset", "application/json; charset=utf-8"},
		{"en majuscules", "Application/JSON"},
	} {
		t.Run(c.name, func(t *testing.T) {
			setup(t)
			rec := request(t, handler, http.MethodPost, "/api/orders/101/lines/201/pick", c.contentType, `{"picker": "Karim"}`)
			var order orderJSON
			decodeJSON(t, rec, &order)
		})
	}
}

// Chaque erreur métier donne son code HTTP et son code d'erreur (tableau 4.3
// du plan), et ne modifie rien.
func TestPickLineBusinessErrors(t *testing.T) {
	setup(t)
	cases := []struct {
		name            string
		orderID, lineID int64
		status          int
		code, message   string
	}{
		{"commande inexistante", 999, 201, http.StatusNotFound, "commande_introuvable", "La commande 999 n'existe pas."},
		{"ligne inexistante", 101, 999, http.StatusNotFound, "ligne_introuvable", "La ligne 999 n'existe pas dans la commande 101."},
		{"ligne d'une autre commande", 101, 203, http.StatusNotFound, "ligne_introuvable", "La ligne 203 n'existe pas dans la commande 101."},
		{"stock insuffisant", 101, 202, http.StatusConflict, "stock_insuffisant", "Stock insuffisant : 2 disponibles, 5 demandés."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := pick(t, c.orderID, c.lineID, `{"picker": "Karim"}`)
			if got := checkError(t, rec, c.status, c.code); got != c.message {
				t.Errorf("message %q, attendu %q", got, c.message)
			}
		})
	}
	checkNothingPicked(t)
}

// Une seconde validation de la même ligne donne 409 deja_preparee : le stock
// n'a baissé qu'une fois, et le premier préparateur reste enregistré.
func TestPickLineTwiceGivesConflict(t *testing.T) {
	setup(t)
	if rec := pick(t, 101, 201, `{"picker": "Karim"}`); rec.Code != http.StatusOK {
		t.Fatalf("première validation : code %d, attendu 200 (corps : %s)", rec.Code, rec.Body)
	}

	rec := pick(t, 101, 201, `{"picker": "Léa"}`)
	if got := checkError(t, rec, http.StatusConflict, "deja_preparee"); got != "Cette ligne a déjà été préparée." {
		t.Errorf("message %q", got)
	}
	var order orderJSON
	decodeJSON(t, get(t, "/api/orders/101"), &order)
	if by, stock := lineOf(t, order, 201); by != "Karim" || stock != 6 {
		t.Errorf("ligne 201 : préparée par %q, stock %d ; attendu \"Karim\" et 6", by, stock)
	}
}

// Les identifiants du chemin doivent être des entiers positifs, écrits avec
// des chiffres seulement et sans zéro en tête. Le message nomme l'identifiant
// fautif : commande ou ligne.
func TestPickLineInvalidIDs(t *testing.T) {
	setup(t)
	for _, c := range []struct{ name, what, target string }{
		{"commande non numérique", "commande", "/api/orders/abc/lines/201/pick"},
		{"commande zéro", "commande", "/api/orders/0/lines/201/pick"},
		{"commande avec un zéro en tête", "commande", "/api/orders/0101/lines/201/pick"},
		{"commande négative", "commande", "/api/orders/-101/lines/201/pick"},
		{"commande avec un signe +", "commande", "/api/orders/+101/lines/201/pick"},
		{"commande décimale", "commande", "/api/orders/101.0/lines/201/pick"},
		{"commande en notation scientifique", "commande", "/api/orders/1e2/lines/201/pick"},
		{"commande précédée d'un espace", "commande", "/api/orders/%20101/lines/201/pick"},
		{"commande trop grande pour int64", "commande", "/api/orders/9223372036854775808/lines/201/pick"},
		{"ligne non numérique", "ligne", "/api/orders/101/lines/abc/pick"},
		{"ligne zéro", "ligne", "/api/orders/101/lines/0/pick"},
		{"ligne avec un zéro en tête", "ligne", "/api/orders/101/lines/0201/pick"},
		{"ligne négative", "ligne", "/api/orders/101/lines/-201/pick"},
		{"ligne avec un signe +", "ligne", "/api/orders/101/lines/+201/pick"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := request(t, handler, http.MethodPost, c.target, "application/json", `{"picker": "Karim"}`)
			message := checkError(t, rec, http.StatusBadRequest, "requete_invalide")
			if want := "Identifiant de " + c.what + " invalide"; !strings.HasPrefix(message, want) {
				t.Errorf("message %q, attendu un message qui commence par %q", message, want)
			}
		})
	}
	checkNothingPicked(t)
}

// Le plus grand identifiant possible (celui d'int64) est accepté : il
// n'existe pas, d'où un 404, alors que le suivant donne 400.
func TestPickLineLargestIDs(t *testing.T) {
	setup(t)
	rec := request(t, handler, http.MethodPost, "/api/orders/9223372036854775807/lines/201/pick", "application/json", `{"picker": "Karim"}`)
	checkError(t, rec, http.StatusNotFound, "commande_introuvable")
	rec = request(t, handler, http.MethodPost, "/api/orders/101/lines/9223372036854775807/pick", "application/json", `{"picker": "Karim"}`)
	checkError(t, rec, http.StatusNotFound, "ligne_introuvable")
	checkNothingPicked(t)
}

// Chaque corps mal formé donne 400 requete_invalide, avec un message qui
// dit quoi corriger.
func TestPickLineInvalidBody(t *testing.T) {
	setup(t)
	const appJSON = "application/json"
	cases := []struct {
		name, contentType, body, message string
	}{
		{"sans Content-Type", "", `{"picker": "Karim"}`, msgNotJSON},
		{"Content-Type texte", "text/plain", `{"picker": "Karim"}`, msgNotJSON},
		{"Content-Type de formulaire", "application/x-www-form-urlencoded", `picker=Karim`, msgNotJSON},
		{"Content-Type mal formé", "application/json; charset", `{"picker": "Karim"}`, msgNotJSON},
		{"corps vide", appJSON, ``, msgEmpty},
		{"corps fait d'espaces", appJSON, " \n\t ", msgEmpty},
		{"JSON tronqué", appJSON, `{"picker": "Karim"`, msgBadBody},
		{"JSON invalide", appJSON, `{picker: Karim}`, msgBadBody},
		{"tableau", appJSON, `["Karim"]`, msgBadBody},
		{"chaîne seule", appJSON, `"Karim"`, msgBadBody},
		{"nombre seul", appJSON, `42`, msgBadBody},
		{"picker nombre", appJSON, `{"picker": 42}`, msgBadBody},
		{"picker objet", appJSON, `{"picker": {"nom": "Karim"}}`, msgBadBody},
		{"champ inconnu", appJSON, `{"picker": "Karim", "admin": true}`, msgBadBody},
		{"deux objets", appJSON, `{"picker": "Karim"} {"picker": "Léa"}`, msgBadBody},
		{"objet vide en trop", appJSON, `{"picker": "Karim"} {}`, msgBadBody},
		{"objet inachevé en trop", appJSON, `{"picker": "Karim"} {`, msgBadBody},
		{"texte après l'objet", appJSON, `{"picker": "Karim"} x`, msgBadBody},
		{"null", appJSON, `null`, msgRequired},
		{"objet vide", appJSON, `{}`, msgRequired},
		{"picker null", appJSON, `{"picker": null}`, msgRequired},
		{"corps énorme", appJSON, `{"picker": "` + strings.Repeat("a", 1<<20) + `"}`, msgTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := request(t, handler, http.MethodPost, "/api/orders/101/lines/201/pick", c.contentType, c.body)
			if got := checkError(t, rec, http.StatusBadRequest, "requete_invalide"); got != c.message {
				t.Errorf("message %q, attendu %q", got, c.message)
			}
		})
	}
	checkNothingPicked(t)
}

// Le corps est limité à 1 024 octets, à l'octet près. Le test utilise un JSON
// valide complété par des espaces : sans la limite, il serait accepté.
func TestPickLineBodySizeLimit(t *testing.T) {
	setup(t)
	valid := `{"picker": "Karim"}`
	atLimit := valid + strings.Repeat(" ", 1024-len(valid))

	rec := pick(t, 101, 201, atLimit+" ")
	if got := checkError(t, rec, http.StatusBadRequest, "requete_invalide"); got != msgTooLarge {
		t.Errorf("1 025 octets : message %q, attendu %q", got, msgTooLarge)
	}
	checkNothingPicked(t)

	if rec := pick(t, 101, 201, atLimit); rec.Code != http.StatusOK {
		t.Errorf("1 024 octets : code %d, attendu 200 (corps : %s)", rec.Code, rec.Body)
	}
}

// Noms acceptés, et nom enregistré : espaces de début et de fin retirés,
// accents (même écrits en deux caractères), apostrophe typographique,
// échappements JSON, émoji, un seul caractère, et 50 caractères, même quand
// ils occupent 100 octets.
func TestPickLineAcceptedPickerNames(t *testing.T) {
	fifty := strings.Repeat("a", 50)
	for _, c := range []struct{ name, picker, stored string }{
		{"espaces autour", `"  Karim  "`, "Karim"},
		{"tabulation et retour à la ligne autour", `"\tLéa\n"`, "Léa"},
		{"espaces spéciaux autour", `"` + jsonEscape(0xa0) + `Léa` + jsonEscape(0x2003) + `"`, "Léa"},
		{"accents, trait d'union et apostrophe", `"Hélène-Marie O’Connor"`, "Hélène-Marie O’Connor"},
		{"échappement JSON", `"L` + jsonEscape(0xe9) + `a"`, "Léa"},
		{"accent écrit en deux caractères", `"Le` + jsonEscape(0x301) + `a"`, "Le" + string(rune(0x301)) + "a"},
		{"émoji", `"Zoë 🙂"`, "Zoë 🙂"},
		{"un seul caractère", `"Q"`, "Q"},
		{"chiffres seulement", `"42"`, "42"},
		{"50 caractères", `"` + fifty + `"`, fifty},
		{"50 caractères entourés d'espaces", `"  ` + fifty + `  "`, fifty},
		{"50 caractères accentués", `"` + strings.Repeat("é", 50) + `"`, strings.Repeat("é", 50)},
	} {
		t.Run(c.name, func(t *testing.T) {
			setup(t)
			var order orderJSON
			decodeJSON(t, pick(t, 101, 201, `{"picker": `+c.picker+`}`), &order)
			if by, _ := lineOf(t, order, 201); by != c.stored {
				t.Errorf("préparateur enregistré %q, attendu %q", by, c.stored)
			}
		})
	}
}

// Noms refusés : vide une fois les espaces retirés, plus de 50 caractères,
// caractère de contrôle (NUL compris), espace spécial ou caractère invisible,
// au début, au milieu ou à la fin, nom sans lettre ni chiffre, et octets qui
// ne sont pas de l'UTF-8. Les caractères invisibles sont écrits par leur
// numéro avec jsonEscape : tels quels, ils ne se verraient pas dans le source.
func TestPickLineRejectedPickerNames(t *testing.T) {
	setup(t)
	for _, c := range []struct{ name, picker, message string }{
		{"vide", `""`, msgRequired},
		{"espaces seulement", `"   "`, msgRequired},
		{"espaces spéciaux seulement", `"` + jsonEscape(0xa0) + jsonEscape(0x2003) + `"`, msgRequired},
		{"51 caractères", `"` + strings.Repeat("a", 51) + `"`, msgTooLong},
		{"51 caractères accentués", `"` + strings.Repeat("é", 51) + `"`, msgTooLong},
		{"caractère NUL au milieu", `"Ka` + jsonEscape(0) + `rim"`, msgForbidden},
		{"caractère NUL à la fin", `"Karim` + jsonEscape(0) + `"`, msgForbidden},
		{"tabulation au milieu", `"Ka` + jsonEscape('\t') + `rim"`, msgForbidden},
		{"retour à la ligne au milieu", `"Ka` + jsonEscape('\n') + `rim"`, msgForbidden},
		{"caractère DEL", `"Ka` + jsonEscape(0x7f) + `rim"`, msgForbidden},
		{"caractère de contrôle C1", `"Ka` + jsonEscape(0x85) + `rim"`, msgForbidden},
		{"espace insécable au milieu", `"Ka` + jsonEscape(0xa0) + `rim"`, msgForbidden},
		{"espace de largeur nulle au début", `"` + jsonEscape(0x200b) + `Karim"`, msgForbidden},
		{"inversion du sens d'écriture", `"Ka` + jsonEscape(0x202e) + `rim"`, msgForbidden},
		{"remplissage coréen seul", `"` + jsonEscape(0x3164) + `"`, msgForbidden},
		{"remplissage coréen au milieu", `"Ka` + jsonEscape(0x3164) + `rim"`, msgForbidden},
		{"motif braille vide", `"` + jsonEscape(0x2800) + `"`, msgNoLetter},
		{"accent seul", `"` + jsonEscape(0x301) + `"`, msgNoLetter},
		{"ponctuation seulement", `"..."`, msgNoLetter},
		{"émoji seul", `"🙂"`, msgNoLetter},
		{"octet qui n'est pas de l'UTF-8", "\"Ka\xffrim\"", msgNotUTF8},
		{"moitié de caractère UTF-16 isolée", `"Ka` + jsonEscape(0xd800) + `rim"`, msgNotUTF8},
	} {
		t.Run(c.name, func(t *testing.T) {
			rec := pick(t, 101, 201, `{"picker": `+c.picker+`}`)
			if got := checkError(t, rec, http.StatusBadRequest, "requete_invalide"); got != c.message {
				t.Errorf("message %q, attendu %q", got, c.message)
			}
		})
	}
	checkNothingPicked(t)
}

// Une panne de la base donne 500 erreur_interne avec un message générique :
// le détail technique reste dans les journaux du serveur, avec la route.
func TestPickLineDatabaseErrorHidesDetails(t *testing.T) {
	logs := captureLogs(t)
	rec := request(t, brokenHandler(t), http.MethodPost, "/api/orders/101/lines/201/pick",
		"application/json", `{"picker": "Karim"}`)
	if got := checkError(t, rec, http.StatusInternalServerError, "erreur_interne"); got != internalMessage {
		t.Errorf("message %q, attendu %q", got, internalMessage)
	}
	if !strings.Contains(logs.String(), "POST /api/orders/101/lines/201/pick : ") {
		t.Errorf("erreur absente des journaux : %q", logs.String())
	}
}

// Si la relecture de la commande échoue alors que la validation est déjà
// enregistrée, la réponse est 500 avec un message qui le dit, et la
// validation reste enregistrée : un nouvel essai répondrait deja_preparee.
// Le traceur fait échouer le BEGIN de la transaction en lecture seule de
// GetOrder (pgx l'écrit « begin isolation level repeatable read read
// only »), qui vient après le COMMIT de PickLine.
func TestPickLineRereadFailsAfterCommit(t *testing.T) {
	setup(t)
	logs := captureLogs(t)
	interrupted := httpapi.NewHandler(store.New(testdb.InterruptingPool(t, pool, "read only")))

	rec := request(t, interrupted, http.MethodPost, "/api/orders/101/lines/201/pick", "application/json", `{"picker": "Karim"}`)
	if got := checkError(t, rec, http.StatusInternalServerError, "erreur_interne"); got != msgReread {
		t.Errorf("message %q, attendu %q", got, msgReread)
	}
	if !strings.Contains(logs.String(), "validation enregistrée, mais relecture impossible") {
		t.Errorf("journal sans la mention de la validation enregistrée : %q", logs.String())
	}

	var order orderJSON
	decodeJSON(t, get(t, "/api/orders/101"), &order)
	if by, stock := lineOf(t, order, 201); by != "Karim" || stock != 6 {
		t.Errorf("ligne 201 : préparée par %q, stock %d ; attendu \"Karim\" et 6 : la validation doit être enregistrée", by, stock)
	}
}
