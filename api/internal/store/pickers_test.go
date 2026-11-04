package store_test

// Tests des profils de préparateurs (docs/PLAN.md, étape 7 bis) : tri par
// nom, création, unicité sans tenir compte des majuscules, limite de 20
// profils, et créations simultanées.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/knudsem/cairn/api/internal/store"
)

// pickerLimit est la limite du scope (SCOPE.md) : écrite en clair, pour qu'un
// changement de store.MaxPickers fasse échouer les tests.
const pickerLimit = 20

// fillPickers crée n profils d'un coup, en SQL : « Profil 01 », « Profil
// 02 »...
func fillPickers(t *testing.T, n int) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `
		INSERT INTO pickers (name)
		SELECT 'Profil ' || lpad(g::text, 2, '0') FROM generate_series(1, $1) AS g`, n)
	if err != nil {
		t.Fatalf("création de %d profils : %v", n, err)
	}
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

// Base vide : une liste vide, et non nil, pour obtenir [] en JSON. Ensuite,
// les profils sont triés dans l'ordre alphabétique français : une majuscule
// ou un accent ne déplace pas un nom (« Émile » entre « Bob » et « Ève »).
//
// « Jean-Yves » et « Jeanne » sont là pour la collation : celle de la base de
// test (en_US.utf8) ignore d'abord le trait d'union et range « Jeanne »
// avant « Jean-Yves », alors que fr-x-icu range « Jean-Yves » d'abord. Sans
// eux, retirer COLLATE "fr-x-icu" de la requête passerait inaperçu ici, alors
// que Neon trie par défaut en C.UTF-8 (Bob avant alice).
func TestListPickersSortedByName(t *testing.T) {
	resetDB(t)
	pickers, err := st.ListPickers(t.Context())
	if err != nil {
		t.Fatalf("ListPickers sur une base vide : %v", err)
	}
	if pickers == nil || len(pickers) != 0 {
		t.Fatalf("base vide : liste = %#v, attendu une liste vide non nil", pickers)
	}

	for _, name := range []string{"zoé", "Jeanne", "Émile", "alice", "Jean-Yves", "Bob", "Ève"} {
		if _, err := st.CreatePicker(t.Context(), name); err != nil {
			t.Fatalf("création de %q : %v", name, err)
		}
	}
	pickers, err = st.ListPickers(t.Context())
	if err != nil {
		t.Fatalf("ListPickers : %v", err)
	}
	want := []string{"alice", "Bob", "Émile", "Ève", "Jean-Yves", "Jeanne", "zoé"}
	if len(pickers) != len(want) {
		t.Fatalf("%d profils, attendu %d", len(pickers), len(want))
	}
	for i, name := range want {
		if pickers[i].Name != name {
			t.Errorf("position %d : %q, attendu %q", i+1, pickers[i].Name, name)
		}
		if pickers[i].CreatedAt.Location() != time.UTC {
			t.Errorf("%q : date dans le fuseau %v, attendu UTC", pickers[i].Name, pickers[i].CreatedAt.Location())
		}
	}
}

// La création renvoie le profil enregistré, que ListPickers relit à
// l'identique : identifiant, nom, date de création en UTC.
func TestCreatePicker(t *testing.T) {
	resetDB(t)
	p, err := st.CreatePicker(t.Context(), "Mathias")
	if err != nil {
		t.Fatalf("CreatePicker : %v", err)
	}
	if p.ID != 301 || p.Name != "Mathias" || p.CreatedAt.IsZero() || p.CreatedAt.Location() != time.UTC {
		t.Errorf("profil créé %+v, attendu l'identifiant 301, le nom Mathias et une date en UTC", p)
	}

	pickers, err := st.ListPickers(t.Context())
	if err != nil {
		t.Fatalf("ListPickers : %v", err)
	}
	if len(pickers) != 1 || pickers[0].ID != p.ID || pickers[0].Name != p.Name || !pickers[0].CreatedAt.Equal(p.CreatedAt) {
		t.Errorf("profils relus %+v, attendu seulement %+v", pickers, p)
	}
}

// emileNFD est « Émile » écrit avec un « E » suivi de l'accent aigu
// combinant (U+0301) : deux caractères là où « É » n'en fait qu'un. À l'écran,
// les deux écritures sont identiques. Le caractère est construit par son
// numéro, pour être visible dans le source.
var emileNFD = "E" + string(rune(0x301)) + "mile"

// Un nom déjà pris, sans tenir compte des majuscules (lettres accentuées
// comprises) ni de la façon d'écrire un accent, est refusé avec le nom tel
// qu'il est enregistré, et rien n'est créé. Un nom qui ne diffère que par un
// accent est un autre nom ; « _ » et « % » sont des caractères ordinaires.
func TestCreatePickerRefusesExistingName(t *testing.T) {
	resetDB(t)
	for _, name := range []string{"Mathias", "Émile", "Karim"} {
		if _, err := st.CreatePicker(t.Context(), name); err != nil {
			t.Fatalf("création de %q : %v", name, err)
		}
	}

	for _, c := range []struct{ label, name, existing string }{
		{"identique", "Mathias", "Mathias"},
		{"minuscules", "mathias", "Mathias"},
		{"majuscules", "MATHIAS", "Mathias"},
		{"majuscule accentuée", "ÉMILE", "Émile"},
		{"minuscule accentuée", "émile", "Émile"},
		{"accent écrit en deux caractères", emileNFD, "Émile"},
	} {
		t.Run(c.label, func(t *testing.T) {
			_, err := st.CreatePicker(t.Context(), c.name)
			var exists *store.PickerExistsError
			if !errors.Is(err, store.ErrPickerExists) || !errors.As(err, &exists) || exists.Name != c.existing {
				t.Errorf("erreur = %v, attendu un profil existant nommé %q", err, c.existing)
			}
			// Elle n'est pas confondue avec l'autre erreur des profils.
			if errors.Is(err, store.ErrTooManyPickers) {
				t.Errorf("errors.Is(profil existant, ErrTooManyPickers) = true, attendu false")
			}
		})
	}
	if got := pickerCount(t); got != 3 {
		t.Errorf("%d profils, attendu 3 : un refus ne crée rien", got)
	}

	// Noms différents, donc acceptés. « K_rim » et « Ka% » échoueraient avec
	// une recherche par LIKE ou ILIKE, où _ et % sont des jokers.
	for _, name := range []string{"Emile", "K_rim", "Ka%"} {
		if _, err := st.CreatePicker(t.Context(), name); err != nil {
			t.Errorf("« %s » à côté d'« Émile » et de « Karim » : erreur %v, attendu une création", name, err)
		}
	}
}

// Les filets de sécurité de la table, essayés en SQL direct, sans passer par
// CreatePicker : la base refuse elle-même un doublon (même en majuscules ou
// avec un accent écrit en deux caractères), un nom vide et un nom de plus de
// 50 caractères, mais accepte 50 « é » (100 octets : la limite compte des
// caractères). C'est le pendant du refus d'un stock négatif (étape 1).
func TestPickersTableSafetyNets(t *testing.T) {
	resetDB(t)
	insert := func(name string) error {
		_, err := pool.Exec(t.Context(), `INSERT INTO pickers (name) VALUES ($1)`, name)
		return err
	}
	if err := insert("Émile"); err != nil {
		t.Fatalf("premier profil : %v", err)
	}
	for _, c := range []struct{ label, name, code string }{
		{"doublon en majuscules", "ÉMILE", "23505"},
		{"doublon avec l'accent en deux caractères", emileNFD, "23505"},
		{"nom vide", "", "23514"},
		{"51 caractères", strings.Repeat("é", 51), "23514"},
	} {
		t.Run(c.label, func(t *testing.T) {
			var pgErr *pgconn.PgError
			if err := insert(c.name); !errors.As(err, &pgErr) || pgErr.Code != c.code {
				t.Errorf("erreur %v, attendu le code PostgreSQL %s", err, c.code)
			}
		})
	}
	if err := insert(strings.Repeat("é", 50)); err != nil {
		t.Errorf("50 « é » : erreur %v, attendu une insertion", err)
	}
}

// La limite : le 20e profil est créé, le 21e est refusé. Un nom déjà pris
// l'emporte sur la limite : le préparateur n'a qu'à choisir ce profil.
func TestCreatePickerLimit(t *testing.T) {
	resetDB(t)
	fillPickers(t, pickerLimit-1)

	if _, err := st.CreatePicker(t.Context(), "Vingtième"); err != nil {
		t.Fatalf("20e profil : erreur %v, attendu une création", err)
	}
	if _, err := st.CreatePicker(t.Context(), "Vingt et unième"); !errors.Is(err, store.ErrTooManyPickers) {
		t.Errorf("21e profil : erreur %v, attendu ErrTooManyPickers", err)
	}
	if _, err := st.CreatePicker(t.Context(), "profil 01"); !errors.Is(err, store.ErrPickerExists) {
		t.Errorf("nom déjà pris, table pleine : erreur %v, attendu ErrPickerExists", err)
	}
	if got := pickerCount(t); got != pickerLimit {
		t.Errorf("%d profils, attendu %d", got, pickerLimit)
	}
}

// Avec 18 profils, 5 créations simultanées donnent exactement 2 succès et
// 3 refus : la limite tient même en cas de requêtes simultanées, grâce au
// verrou de la table. Le scénario est répété pour multiplier les
// entrelacements, comme le cas 4 de PickLine.
func TestCreatePickerConcurrentCreationsRespectLimit(t *testing.T) {
	const rounds = 10
	for round := 1; round <= rounds; round++ {
		resetDB(t)
		fillPickers(t, pickerLimit-2)
		ctx := t.Context()
		creations := make([]func() error, 5)
		for i := range creations {
			name := fmt.Sprintf("Nouveau %d", i+1)
			creations[i] = func() error {
				_, err := st.CreatePicker(ctx, name)
				return err
			}
		}

		errs := runTogether(creations...)

		successes, refusals := 0, 0
		for i, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, store.ErrTooManyPickers):
				refusals++
			default:
				t.Errorf("tour %d, création %d : erreur inattendue : %v", round, i+1, err)
			}
		}
		if successes != 2 || refusals != 3 {
			t.Fatalf("tour %d : %d succès et %d refus, attendu 2 et 3", round, successes, refusals)
		}
		if got := pickerCount(t); got != pickerLimit {
			t.Fatalf("tour %d : %d profils, attendu %d", round, got, pickerLimit)
		}
	}
}

// Le même nom, créé 5 fois en même temps dans des casses différentes, donne
// un succès et 4 refus « profil existant », et non une erreur brute de
// l'index unique : le verrou fait attendre les créations suivantes, qui
// trouvent alors le profil créé.
func TestCreatePickerConcurrentSameName(t *testing.T) {
	const rounds = 10
	for round := 1; round <= rounds; round++ {
		resetDB(t)
		ctx := t.Context()
		var creations []func() error
		for _, name := range []string{"Karim", "karim", "KARIM", "KaRiM", "kArIm"} {
			creations = append(creations, func() error {
				_, err := st.CreatePicker(ctx, name)
				return err
			})
		}

		errs := runTogether(creations...)

		successes, refusals := 0, 0
		for i, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, store.ErrPickerExists):
				refusals++
			default:
				t.Errorf("tour %d, création %d : erreur inattendue : %v", round, i+1, err)
			}
		}
		if successes != 1 || refusals != 4 {
			t.Fatalf("tour %d : %d succès et %d refus, attendu 1 et 4", round, successes, refusals)
		}
		if got := pickerCount(t); got != 1 {
			t.Fatalf("tour %d : %d profils, attendu 1", round, got)
		}
	}
}
