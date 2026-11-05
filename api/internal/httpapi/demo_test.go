package httpapi_test

// Tests de POST /api/demo/reset (docs/PLAN.md, étape 15) : 204 et retour à
// l'état de départ quand DEMO_RESET_ENABLED vaut true, 404 sinon, et panne
// de base.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/knudsem/cairn/api/internal/httpapi"
	"github.com/knudsem/cairn/api/internal/store"
)

// postReset envoie POST /api/demo/reset à h, sans corps.
func postReset(t *testing.T, h http.Handler) (int, string) {
	t.Helper()
	rec := request(t, h, http.MethodPost, "/api/demo/reset", "", "")
	return rec.Code, rec.Body.String()
}

// Avec DEMO_RESET_ENABLED=true : 204 sans corps, puis les données de
// démonstration, quel que soit l'état d'avant (ici le jeu de données des
// tests, plus un profil Test). Deux appels de suite : même résultat.
func TestDemoResetRoute(t *testing.T) {
	setup(t)
	if rec := postPicker(t, `{"name": "Test"}`); rec.Code != http.StatusCreated {
		t.Fatalf("création de Test : code %d", rec.Code)
	}
	t.Setenv("DEMO_RESET_ENABLED", "true")
	h := httpapi.NewHandler(store.New(pool))

	for round := 1; round <= 2; round++ {
		if code, body := postReset(t, h); code != http.StatusNoContent || body != "" {
			t.Fatalf("remise à zéro %d : code %d, corps %q ; attendu 204 sans corps", round, code, body)
		}
		var pickers []pickerJSON
		decodeJSON(t, get(t, "/api/pickers"), &pickers)
		names := make([]string, len(pickers))
		for i, p := range pickers {
			names[i] = p.Name
		}
		if strings.Join(names, ",") != "Claire,Romain,Virginie" {
			t.Errorf("remise à zéro %d : profils %v, attendu Claire, Romain, Virginie", round, names)
		}
		var orders []struct {
			Reference string `json:"reference"`
		}
		decodeJSON(t, get(t, "/api/orders"), &orders)
		if len(orders) != 6 || orders[0].Reference != "CMD-0006" {
			t.Errorf("remise à zéro %d : %d commandes (la première : %+v), attendu 6, CMD-0006 en premier", round, len(orders), orders)
		}
	}
}

// Sans DEMO_RESET_ENABLED=true, la route n'existe pas : le routeur répond
// 404, et rien ne change.
func TestDemoResetDisabled(t *testing.T) {
	setup(t)
	for _, value := range []string{"", "false", "TRUE", "1", "yes"} {
		t.Run("DEMO_RESET_ENABLED="+value, func(t *testing.T) {
			t.Setenv("DEMO_RESET_ENABLED", value)
			if code, _ := postReset(t, httpapi.NewHandler(store.New(pool))); code != http.StatusNotFound {
				t.Errorf("code %d, attendu 404", code)
			}
			if n := pickerCount(t); n != 0 {
				t.Errorf("%d profils après l'appel refusé, attendu 0 (jeu de données des tests)", n)
			}
			checkNothingPicked(t)
		})
	}
}

// Une panne de la base donne 500 erreur_interne avec le message générique,
// et l'erreur est journalisée avec la route.
func TestDemoResetDatabaseError(t *testing.T) {
	logs := captureLogs(t)
	t.Setenv("DEMO_RESET_ENABLED", "true")
	rec := request(t, brokenHandler(t), http.MethodPost, "/api/demo/reset", "", "")
	if got := checkError(t, rec, http.StatusInternalServerError, "erreur_interne"); got != internalMessage {
		t.Errorf("message %q, attendu %q", got, internalMessage)
	}
	if !strings.Contains(logs.String(), "POST /api/demo/reset : ") {
		t.Errorf("erreur absente des journaux : %q", logs.String())
	}
}
