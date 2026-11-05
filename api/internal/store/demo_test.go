package store_test

// Tests de la remise à zéro de la démo (docs/PLAN.md, étape 15) : retour à
// l'état de départ après des changements, deux remises à zéro de suite, et
// interruption sans effet.

import (
	"testing"

	"github.com/knudsem/cairn/api/internal/store"
	"github.com/knudsem/cairn/api/internal/testdb"
)

// demoState décrit tout le contenu des quatre tables, sans les dates : elles
// sont recalculées à chaque chargement (now() - interval). L'ordre des
// commandes par date de création y figure quand même.
func demoState(t *testing.T) string {
	t.Helper()
	var state string
	err := pool.QueryRow(t.Context(), `
		SELECT concat_ws(' | ',
			(SELECT string_agg(id || ':' || name, ',' ORDER BY id) FROM pickers),
			(SELECT string_agg(id || ':' || sku || ':' || location || ':' || stock_qty, ',' ORDER BY id) FROM products),
			(SELECT string_agg(id || ':' || reference || ':' || customer || ':' || status, ',' ORDER BY id) FROM orders),
			(SELECT string_agg(reference, ',' ORDER BY created_at) FROM orders),
			(SELECT string_agg(id || ':' || order_id || ':' || product_id || ':' || quantity || ':' ||
				coalesce(picked_by, '-') || ':' || (picked_at IS NOT NULL)::text, ',' ORDER BY id) FROM order_lines))`).Scan(&state)
	if err != nil {
		t.Fatalf("lecture de l'état de la base : %v", err)
	}
	return state
}

// countOf renvoie le résultat d'une requête de comptage.
func countOf(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%s : %v", sql, err)
	}
	return n
}

// La remise à zéro rétablit l'état de départ après des changements : une
// ligne validée (et son stock consommé), un profil créé, un profil
// supprimé. Une seconde remise à zéro donne exactement le même résultat.
func TestResetDemoRestoresStartingState(t *testing.T) {
	resetDB(t)
	if err := st.ResetDemo(t.Context()); err != nil {
		t.Fatalf("première remise à zéro : %v", err)
	}
	start := demoState(t)

	// L'état de départ est bien celui des données de démonstration.
	pickers, err := st.ListPickers(t.Context())
	if err != nil {
		t.Fatalf("ListPickers : %v", err)
	}
	if len(pickers) != 3 || pickers[0].Name != "Claire" || pickers[1].Name != "Romain" || pickers[2].Name != "Virginie" {
		t.Fatalf("profils de départ %+v, attendu Claire, Romain, Virginie", pickers)
	}
	if n := countOf(t, `SELECT count(*) FROM orders`); n != 6 {
		t.Errorf("%d commandes, attendu 6", n)
	}
	if n := countOf(t, `SELECT count(*) FROM order_lines WHERE picked_at IS NOT NULL`); n != 3 {
		t.Errorf("%d lignes préparées, attendu 3", n)
	}
	if n := countOf(t, `SELECT stock_qty FROM products WHERE sku = 'BUL-50'`); n != 5 {
		t.Errorf("stock BUL-50 = %d, attendu 5", n)
	}

	// Des changements : une ligne de CMD-0001 validée, du stock consommé,
	// un profil créé, un profil supprimé.
	var orderID, lineID, productID int64
	err = pool.QueryRow(t.Context(), `
		SELECT o.id, l.id, l.product_id FROM order_lines l JOIN orders o ON o.id = l.order_id
		WHERE o.reference = 'CMD-0001' ORDER BY l.id LIMIT 1`).Scan(&orderID, &lineID, &productID)
	if err != nil {
		t.Fatalf("ligne de CMD-0001 : %v", err)
	}
	stockBefore := stockOf(t, productID)
	if err := st.PickLine(t.Context(), orderID, lineID, "Claire"); err != nil {
		t.Fatalf("PickLine : %v", err)
	}
	if stockOf(t, productID) >= stockBefore {
		t.Fatalf("stock %d après la validation, attendu moins de %d", stockOf(t, productID), stockBefore)
	}
	if _, err := st.CreatePicker(t.Context(), "Test"); err != nil {
		t.Fatalf("création de Test : %v", err)
	}
	romain := pickers[1]
	if err := st.DeletePicker(t.Context(), romain.ID); err != nil {
		t.Fatalf("suppression de Romain : %v", err)
	}
	if demoState(t) == start {
		t.Fatal("les changements n'ont rien modifié : le test ne prouverait rien")
	}

	for round := 1; round <= 2; round++ {
		if err := st.ResetDemo(t.Context()); err != nil {
			t.Fatalf("remise à zéro %d : %v", round, err)
		}
		if got := demoState(t); got != start {
			t.Errorf("remise à zéro %d : état\n%s\nattendu l'état de départ\n%s", round, got, start)
		}
	}
}

// Interrompue pendant le chargement des données, la remise à zéro n'a rien
// changé : le vidage des tables, déjà fait dans la transaction, est annulé.
func TestResetDemoInterruptedChangesNothing(t *testing.T) {
	resetDB(t)
	if err := st.ResetDemo(t.Context()); err != nil {
		t.Fatalf("remise à zéro : %v", err)
	}
	if _, err := st.CreatePicker(t.Context(), "Test"); err != nil {
		t.Fatalf("création de Test : %v", err)
	}
	before := demoState(t)

	interrupted := store.New(testdb.InterruptingPool(t, pool, "INSERT INTO pickers"))
	if err := interrupted.ResetDemo(t.Context()); err == nil {
		t.Fatal("remise à zéro interrompue : pas d'erreur, attendu une erreur")
	}
	if got := demoState(t); got != before {
		t.Errorf("après l'interruption, état\n%s\nattendu l'état d'avant (profil Test compris)\n%s", got, before)
	}
}
