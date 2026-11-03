package store_test

// Tests des lectures de l'étape 4 : ordre de la liste et avancement, filtre,
// tri des lignes et des produits par emplacement, listes vides, dates en UTC,
// et statuts acceptés par le filtre (ValidStatus).

import (
	"errors"
	"testing"
	"time"

	"github.com/knudsem/cairn/api/internal/store"
)

// insertOrderAt crée une commande dont la date de création est décalée dans
// le passé (par exemple "3 hours"), et renvoie son identifiant.
func insertOrderAt(t *testing.T, reference, ago string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(t.Context(), `
		INSERT INTO orders (reference, customer, created_at)
		VALUES ($1, 'Client de test', now() - $2::interval) RETURNING id`, reference, ago).Scan(&id)
	if err != nil {
		t.Fatalf("création de la commande %s : %v", reference, err)
	}
	return id
}

func TestListOrdersOldestFirstWithProgress(t *testing.T) {
	resetDB(t)
	ctx := t.Context()

	// Base vide : une liste vide, et non nil, pour obtenir [] en JSON.
	orders, err := st.ListOrders(ctx, "")
	if err != nil {
		t.Fatalf("ListOrders sur une base vide : %v", err)
	}
	if orders == nil || len(orders) != 0 {
		t.Fatalf("base vide : liste = %#v, attendu une liste vide non nil", orders)
	}

	// Les commandes sont créées de la plus récente à la plus ancienne :
	// l'ordre attendu est l'inverse exact de l'ordre de création, donc des
	// identifiants. Sans ORDER BY, PostgreSQL rend les lignes dans un ordre
	// qu'il ne garantit pas, souvent celui des identifiants : un tri oublié se
	// voit. La commande sans ligne, la plus ancienne, doit apparaître avec 0/0.
	product := insertProduct(t, "P-1", "A-01-1", 100)
	recent := insertOrderAt(t, "CMD-RECENTE", "1 hour")
	middle := insertOrderAt(t, "CMD-MILIEU", "2 hours")
	oldest := insertOrderAt(t, "CMD-ANCIENNE", "3 hours")
	insertOrderAt(t, "CMD-SANS-LIGNE", "4 hours")
	oldestFirst := insertLine(t, oldest, product, 1)
	insertLine(t, oldest, product, 1)
	insertLine(t, middle, product, 1)
	recentOnly := insertLine(t, recent, product, 1)
	for _, p := range []struct{ order, line int64 }{{oldest, oldestFirst}, {recent, recentOnly}} {
		if err := st.PickLine(ctx, p.order, p.line, "Alice"); err != nil {
			t.Fatalf("préparation des données : %v", err)
		}
	}

	orders, err = st.ListOrders(ctx, "")
	if err != nil {
		t.Fatalf("ListOrders : %v", err)
	}
	want := []struct {
		reference, status string
		picked, lines     int
	}{
		{"CMD-SANS-LIGNE", store.StatusToPick, 0, 0},
		{"CMD-ANCIENNE", store.StatusPicking, 1, 2},
		{"CMD-MILIEU", store.StatusToPick, 0, 1},
		{"CMD-RECENTE", store.StatusReady, 1, 1},
	}
	if len(orders) != len(want) {
		t.Fatalf("%d commandes, attendu %d", len(orders), len(want))
	}
	for i, w := range want {
		o := orders[i]
		if o.Reference != w.reference || o.Status != w.status || o.PickedCount != w.picked || o.LineCount != w.lines {
			t.Errorf("position %d : %s %s %d/%d, attendu %s %s %d/%d", i+1,
				o.Reference, o.Status, o.PickedCount, o.LineCount, w.reference, w.status, w.picked, w.lines)
		}
		if o.CreatedAt.Location() != time.UTC {
			t.Errorf("%s : date dans le fuseau %v, attendu UTC", o.Reference, o.CreatedAt.Location())
		}
	}

	ready, err := st.ListOrders(ctx, store.StatusReady)
	if err != nil {
		t.Fatalf("ListOrders(prete) : %v", err)
	}
	if len(ready) != 1 || ready[0].Reference != "CMD-RECENTE" {
		t.Errorf("filtre prete : %v, attendu seulement CMD-RECENTE", ready)
	}
}

func TestGetOrderLinesSortedByLocation(t *testing.T) {
	resetDB(t)
	ctx := t.Context()
	// Produits et lignes créés dans un ordre différent de celui des emplacements.
	pc := insertProduct(t, "P-C", "C-02-1", 10)
	pa := insertProduct(t, "P-A", "A-10-1", 10)
	pb := insertProduct(t, "P-B", "B-01-3", 10)
	order := insertOrder(t, "CMD-1")
	insertLine(t, order, pc, 1)
	insertLine(t, order, pa, 2)
	lineB := insertLine(t, order, pb, 3)
	if err := st.PickLine(ctx, order, lineB, "Alice"); err != nil {
		t.Fatalf("préparation des données : %v", err)
	}

	o, err := st.GetOrder(ctx, order)
	if err != nil {
		t.Fatalf("GetOrder : %v", err)
	}
	if o.Reference != "CMD-1" || o.Status != store.StatusPicking {
		t.Errorf("commande %s %s, attendu CMD-1 %s", o.Reference, o.Status, store.StatusPicking)
	}
	if o.CreatedAt.Location() != time.UTC {
		t.Errorf("date de création dans le fuseau %v, attendu UTC", o.CreatedAt.Location())
	}
	if len(o.Lines) != 3 {
		t.Fatalf("%d lignes, attendu 3", len(o.Lines))
	}
	for i, location := range []string{"A-10-1", "B-01-3", "C-02-1"} {
		if o.Lines[i].Location != location {
			t.Errorf("ligne %d : emplacement %s, attendu %s (tri par emplacement)", i+1, o.Lines[i].Location, location)
		}
	}
	picked := o.Lines[1] // B-01-3
	if picked.ID != lineB || picked.ProductID != pb || picked.SKU != "P-B" || picked.ProductName != "Produit P-B" ||
		picked.PickedBy == nil || *picked.PickedBy != "Alice" || picked.PickedAt == nil {
		t.Errorf("ligne préparée mal lue : %+v", picked)
	}
	if picked.PickedAt != nil && picked.PickedAt.Location() != time.UTC {
		t.Errorf("date de préparation dans le fuseau %v, attendu UTC", picked.PickedAt.Location())
	}
	if picked.Quantity != 3 || picked.StockQty != 7 {
		t.Errorf("quantité %d et stock %d, attendu 3 et 7 (10 - 3)", picked.Quantity, picked.StockQty)
	}
	if o.Lines[0].PickedAt != nil || o.Lines[0].PickedBy != nil {
		t.Errorf("ligne non préparée mal lue : %+v", o.Lines[0])
	}

	if _, err := st.GetOrder(ctx, 999); !errors.Is(err, store.ErrOrderNotFound) {
		t.Errorf("commande 999 : erreur = %v, attendu ErrOrderNotFound", err)
	}

	empty := insertOrder(t, "CMD-SANS-LIGNE")
	o, err = st.GetOrder(ctx, empty)
	if err != nil {
		t.Fatalf("GetOrder d'une commande sans ligne : %v", err)
	}
	if o.Lines == nil || len(o.Lines) != 0 {
		t.Errorf("commande sans ligne : lignes = %#v, attendu une liste vide non nil", o.Lines)
	}
}

func TestListProductsSortedByLocation(t *testing.T) {
	resetDB(t)
	// Base vide : une liste vide, et non nil, pour obtenir [] en JSON.
	products, err := st.ListProducts(t.Context())
	if err != nil {
		t.Fatalf("ListProducts sur une base vide : %v", err)
	}
	if products == nil || len(products) != 0 {
		t.Fatalf("base vide : liste = %#v, attendu une liste vide non nil", products)
	}

	insertProduct(t, "P-C", "C-02-1", 5)
	insertProduct(t, "P-A", "A-10-1", 7)
	insertProduct(t, "P-B", "B-01-3", 9)

	products, err = st.ListProducts(t.Context())
	if err != nil {
		t.Fatalf("ListProducts : %v", err)
	}
	if len(products) != 3 {
		t.Fatalf("%d produits, attendu 3", len(products))
	}
	for i, w := range []struct {
		sku   string
		stock int
	}{{"P-A", 7}, {"P-B", 9}, {"P-C", 5}} {
		if products[i].SKU != w.sku || products[i].StockQty != w.stock {
			t.Errorf("position %d : %s (stock %d), attendu %s (stock %d)", i+1,
				products[i].SKU, products[i].StockQty, w.sku, w.stock)
		}
	}
}

// ValidStatus accepte exactement les trois statuts de la base : c'est elle qui
// fait répondre 400 à GET /api/orders?status=inconnu. Les valeurs sont écrites
// en clair, et non avec les constantes : ce sont celles du contrat de l'API.
func TestValidStatus(t *testing.T) {
	for _, s := range []string{"a_preparer", "en_cours", "prete"} {
		if !store.ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = false, attendu true", s)
		}
	}
	for _, s := range []string{"", "inconnu", "PRETE", "prête"} {
		if store.ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = true, attendu false", s)
		}
	}
}
