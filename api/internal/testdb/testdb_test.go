package testdb

// Test du garde-fou d'Open. Ce fichier est dans le paquet testdb lui-même
// (et non testdb_test) pour appeler open, qui n'est pas exportée.

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

// Le garde-fou refuse une base dont le nom ne se termine pas par _test, avant
// toute modification et avant la prise du verrou entre paquets. La base
// « postgres » existe sur tout serveur PostgreSQL. Le second cas met _test
// dans l'adresse, mais pas dans le nom de la base : le garde-fou lit le nom
// sur le serveur (current_database), pas dans l'adresse.
func TestOpenRefusesNonTestDatabase(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = defaultURL
	}
	address, err := url.Parse(base)
	if err != nil {
		t.Fatalf("adresse de la base de test : %v", err)
	}
	address.Path = "/postgres"

	for _, c := range []struct{ name, appName string }{
		{"base postgres", ""},
		{"nom d'application en _test", "faux_test"},
	} {
		t.Run(c.name, func(t *testing.T) {
			target := *address
			if c.appName != "" {
				query := target.Query()
				query.Set("application_name", c.appName)
				target.RawQuery = query.Encode()
			}
			db, err := open(target.String())
			if err == nil {
				db.Close()
				t.Fatal("base postgres acceptée, attendu un refus")
			}
			if !strings.Contains(err.Error(), `refus de lancer les tests sur la base "postgres"`) {
				t.Errorf("erreur %q, attendu le refus du garde-fou", err)
			}
		})
	}
}
