package store_test

// Tests de la règle métier PickLine (docs/PLAN.md, étape 6). Les numéros des
// commentaires renvoient aux cas de la liste du plan.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/knudsem/cairn/api/internal/store"
	"github.com/knudsem/cairn/api/internal/testdb"
)

// Cas 1 : la validation baisse le stock de la quantité exacte et enregistre
// le préparateur.
func TestPickLineDecreasesStockAndRecordsPicker(t *testing.T) {
	resetDB(t)
	product := insertProduct(t, "P-1", "A-01-1", 10)
	order := insertOrder(t, "CMD-1")
	line := insertLine(t, order, product, 4)

	if err := st.PickLine(t.Context(), order, line, "Alice"); err != nil {
		t.Fatalf("PickLine : %v", err)
	}

	if got := stockOf(t, product); got != 6 {
		t.Errorf("stock = %d, attendu 6 (10 - 4)", got)
	}
	picked, by := pickState(t, line)
	if !picked || by != "Alice" {
		t.Errorf("ligne préparée = %v par %q, attendu true par \"Alice\"", picked, by)
	}
}

// Cas 2 : avec un stock insuffisant, la validation est refusée, et ni le
// stock ni la ligne ne changent. L'erreur porte les quantités du message.
func TestPickLineInsufficientStockChangesNothing(t *testing.T) {
	resetDB(t)
	product := insertProduct(t, "P-1", "A-01-1", 2)
	order := insertOrder(t, "CMD-1")
	line := insertLine(t, order, product, 5)

	err := st.PickLine(t.Context(), order, line, "Alice")

	if !errors.Is(err, store.ErrInsufficientStock) {
		t.Fatalf("erreur = %v, attendu ErrInsufficientStock", err)
	}
	var ise *store.InsufficientStockError
	if !errors.As(err, &ise) {
		t.Fatalf("erreur = %v, attendu une *InsufficientStockError", err)
	}
	if ise.Available != 2 || ise.Requested != 5 {
		t.Errorf("quantités = %d disponibles, %d demandés, attendu 2 et 5", ise.Available, ise.Requested)
	}
	if got := stockOf(t, product); got != 2 {
		t.Errorf("stock = %d, attendu 2 (inchangé)", got)
	}
	if picked, _ := pickState(t, line); picked {
		t.Error("la ligne est marquée préparée malgré le refus : le Rollback n'a pas annulé l'étape 2")
	}
	if got := statusOf(t, order); got != store.StatusToPick {
		t.Errorf("statut = %q, attendu %q (inchangé)", got, store.StatusToPick)
	}
}

// Cas 3 : une seconde validation de la même ligne est refusée, et le stock
// n'a baissé qu'une fois.
func TestPickLineTwiceIsRefused(t *testing.T) {
	resetDB(t)
	product := insertProduct(t, "P-1", "A-01-1", 10)
	order := insertOrder(t, "CMD-1")
	line := insertLine(t, order, product, 4)

	if err := st.PickLine(t.Context(), order, line, "Alice"); err != nil {
		t.Fatalf("première validation : %v", err)
	}
	err := st.PickLine(t.Context(), order, line, "Bob")

	if !errors.Is(err, store.ErrAlreadyPicked) {
		t.Errorf("seconde validation : erreur = %v, attendu ErrAlreadyPicked", err)
	}
	if got := stockOf(t, product); got != 6 {
		t.Errorf("stock = %d, attendu 6 : il ne doit baisser qu'une fois", got)
	}
	if _, by := pickState(t, line); by != "Alice" {
		t.Errorf("préparateur = %q, attendu \"Alice\" (la seconde validation ne change rien)", by)
	}
}

// Cas 4 : 10 validations simultanées sur un produit dont le stock n'en couvre
// que 3 donnent exactement 3 succès, 7 refus et un stock final de 0.
// Le scénario est répété pour multiplier les entrelacements possibles.
func TestPickLineConcurrentPicksNeverOversell(t *testing.T) {
	const rounds = 10
	for round := 1; round <= rounds; round++ {
		resetDB(t)
		ctx := t.Context()
		product := insertProduct(t, "RARE", "A-01-1", 3)
		picks := make([]func() error, 10)
		for i := range picks {
			order := insertOrder(t, fmt.Sprintf("CMD-%02d", i+1))
			line := insertLine(t, order, product, 1)
			picks[i] = func() error {
				return st.PickLine(ctx, order, line, fmt.Sprintf("Préparateur %d", i+1))
			}
		}

		errs := runTogether(picks...)

		successes, refusals := 0, 0
		for i, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, store.ErrInsufficientStock):
				refusals++
			default:
				t.Errorf("tour %d, validation %d : erreur inattendue : %v", round, i+1, err)
			}
		}
		if successes != 3 || refusals != 7 {
			t.Fatalf("tour %d : %d succès et %d refus, attendu 3 et 7", round, successes, refusals)
		}
		if got := stockOf(t, product); got != 0 {
			t.Fatalf("tour %d : stock final = %d, attendu 0", round, got)
		}
		if got := pickedLinesOf(t, product); got != 3 {
			t.Fatalf("tour %d : %d lignes préparées, attendu 3", round, got)
		}
	}
}

// Cas 5 : la première ligne validée fait passer la commande à en_cours, la
// dernière à prete.
func TestPickLineUpdatesOrderStatus(t *testing.T) {
	resetDB(t)
	product := insertProduct(t, "P-1", "A-01-1", 10)
	order := insertOrder(t, "CMD-1")
	first := insertLine(t, order, product, 1)
	last := insertLine(t, order, product, 1)

	if err := st.PickLine(t.Context(), order, first, "Alice"); err != nil {
		t.Fatalf("première ligne : %v", err)
	}
	if got := statusOf(t, order); got != store.StatusPicking {
		t.Errorf("après la première ligne : statut = %q, attendu %q", got, store.StatusPicking)
	}

	if err := st.PickLine(t.Context(), order, last, "Bob"); err != nil {
		t.Fatalf("dernière ligne : %v", err)
	}
	if got := statusOf(t, order); got != store.StatusReady {
		t.Errorf("après la dernière ligne : statut = %q, attendu %q", got, store.StatusReady)
	}
	// Chaque ligne garde le nom de son propre préparateur.
	if _, by := pickState(t, first); by != "Alice" {
		t.Errorf("première ligne : préparateur = %q, attendu \"Alice\"", by)
	}
	if _, by := pickState(t, last); by != "Bob" {
		t.Errorf("dernière ligne : préparateur = %q, attendu \"Bob\"", by)
	}
}

// Cas 6 : les deux dernières lignes d'une commande validées en parallèle
// donnent toujours prete. Elles portent sur deux produits différents : avec
// un seul produit, le verrou sur la ligne de products ferait passer les deux
// validations l'une après l'autre, et ce test réussirait même sans le verrou
// sur la commande (docs/PLAN.md, étape 6).
func TestPickLineLastLinesConcurrentlyGiveReady(t *testing.T) {
	const rounds = 50
	for round := 1; round <= rounds; round++ {
		resetDB(t)
		ctx := t.Context()
		productA := insertProduct(t, "P-A", "A-01-1", 10)
		productB := insertProduct(t, "P-B", "B-01-1", 10)
		order := insertOrder(t, "CMD-1")
		lineA := insertLine(t, order, productA, 1)
		lineB := insertLine(t, order, productB, 1)

		errs := runTogether(
			func() error { return st.PickLine(ctx, order, lineA, "Alice") },
			func() error { return st.PickLine(ctx, order, lineB, "Bob") },
		)

		for i, err := range errs {
			if err != nil {
				t.Fatalf("tour %d, validation %d : %v", round, i+1, err)
			}
		}
		if got := statusOf(t, order); got != store.StatusReady {
			t.Fatalf("tour %d : statut = %q alors que les deux lignes sont préparées, attendu %q",
				round, got, store.StatusReady)
		}
	}
}

// Cas 7 : la même ligne validée deux fois en parallèle donne un succès et un
// refus, et le stock ne baisse qu'une fois.
func TestPickLineSameLineConcurrentlyOnlyOnce(t *testing.T) {
	const rounds = 20
	for round := 1; round <= rounds; round++ {
		resetDB(t)
		ctx := t.Context()
		product := insertProduct(t, "P-1", "A-01-1", 10)
		order := insertOrder(t, "CMD-1")
		line := insertLine(t, order, product, 4)

		errs := runTogether(
			func() error { return st.PickLine(ctx, order, line, "Alice") },
			func() error { return st.PickLine(ctx, order, line, "Bob") },
		)

		successes, refusals := 0, 0
		for i, err := range errs {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, store.ErrAlreadyPicked):
				refusals++
			default:
				t.Errorf("tour %d, validation %d : erreur inattendue : %v", round, i+1, err)
			}
		}
		if successes != 1 || refusals != 1 {
			t.Fatalf("tour %d : %d succès et %d refus, attendu 1 et 1", round, successes, refusals)
		}
		if got := stockOf(t, product); got != 6 {
			t.Fatalf("tour %d : stock = %d, attendu 6 : il ne doit baisser qu'une fois", round, got)
		}
	}
}

// Commande ou ligne introuvable, ligne d'une autre commande : chaque cas
// renvoie la bonne erreur et ne modifie rien. C'est un test « en table » :
// une liste de cas, parcourue par une seule boucle.
func TestPickLineUnknownOrderOrLine(t *testing.T) {
	resetDB(t)
	product := insertProduct(t, "P-1", "A-01-1", 10)
	order := insertOrder(t, "CMD-1")
	line := insertLine(t, order, product, 1)
	otherOrder := insertOrder(t, "CMD-2")
	otherLine := insertLine(t, otherOrder, product, 1)

	cases := []struct {
		name    string
		orderID int64
		lineID  int64
		want    error
	}{
		{"commande inexistante", 999, line, store.ErrOrderNotFound},
		{"ligne inexistante", order, 999, store.ErrLineNotFound},
		{"ligne d'une autre commande", order, otherLine, store.ErrLineNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := st.PickLine(t.Context(), c.orderID, c.lineID, "Alice")
			if !errors.Is(err, c.want) {
				t.Errorf("erreur = %v, attendu %v", err, c.want)
			}
		})
	}

	// Aucun de ces appels n'a rien modifié.
	if got := stockOf(t, product); got != 10 {
		t.Errorf("stock = %d, attendu 10 (inchangé)", got)
	}
	for _, id := range []int64{line, otherLine} {
		if picked, _ := pickState(t, id); picked {
			t.Errorf("la ligne %d est marquée préparée", id)
		}
	}
	if got := statusOf(t, otherOrder); got != store.StatusToPick {
		t.Errorf("statut de l'autre commande = %q, attendu %q (inchangé)", got, store.StatusToPick)
	}
}

// Une validation interrompue après la baisse du stock ne laisse aucune trace :
// le Rollback annule aussi la baisse du stock et le marquage de la ligne. Le
// cas 2 vérifie un refus pendant la baisse du stock ; ici, l'échec survient
// après, à l'étape 4 (recalcul du statut). En pratique : une requête
// abandonnée en cours de route, par exemple après un délai dépassé.
func TestPickLineInterruptedAfterStockUpdateChangesNothing(t *testing.T) {
	resetDB(t)
	product := insertProduct(t, "P-1", "A-01-1", 10)
	order := insertOrder(t, "CMD-1")
	line := insertLine(t, order, product, 4)

	// Un second pool, dont un traceur pgx fait échouer la requête de l'étape
	// 4, celle qui contient « UPDATE orders » (voir testdb.InterruptingPool).
	traced := testdb.InterruptingPool(t, pool, "UPDATE orders")

	err := store.New(traced).PickLine(t.Context(), order, line, "Alice")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erreur = %v, attendu une interruption (context.Canceled) à l'étape 4", err)
	}
	if got := stockOf(t, product); got != 10 {
		t.Errorf("stock = %d, attendu 10 : la baisse doit être annulée avec le reste", got)
	}
	if picked, _ := pickState(t, line); picked {
		t.Error("la ligne est marquée préparée malgré l'interruption")
	}
	if got := statusOf(t, order); got != store.StatusToPick {
		t.Errorf("statut = %q, attendu %q (inchangé)", got, store.StatusToPick)
	}
}

// Le message du stock insuffisant accorde le pluriel comme en français, et
// l'erreur reste reconnaissable une fois emballée avec %w.
func TestInsufficientStockErrorText(t *testing.T) {
	cases := []struct {
		available, requested int
		want                 string
	}{
		{0, 1, "stock insuffisant : 0 disponible, 1 demandé"},
		{1, 4, "stock insuffisant : 1 disponible, 4 demandés"},
		{3, 4, "stock insuffisant : 3 disponibles, 4 demandés"},
		{2, 5, "stock insuffisant : 2 disponibles, 5 demandés"}, // CMD-0004 de la démonstration
	}
	for _, c := range cases {
		err := &store.InsufficientStockError{Available: c.available, Requested: c.requested}
		if got := err.Error(); got != c.want {
			t.Errorf("Error() = %q, attendu %q", got, c.want)
		}
	}

	wrapped := fmt.Errorf("validation : %w", &store.InsufficientStockError{Available: 1, Requested: 2})
	var ise *store.InsufficientStockError
	if !errors.Is(wrapped, store.ErrInsufficientStock) || !errors.As(wrapped, &ise) {
		t.Errorf("erreur emballée non reconnue par errors.Is ou errors.As : %v", wrapped)
	}
	// Elle n'est pas confondue avec les autres erreurs métier : l'étape 7
	// choisira le code HTTP avec errors.Is.
	for _, other := range []error{store.ErrOrderNotFound, store.ErrLineNotFound, store.ErrAlreadyPicked} {
		if errors.Is(wrapped, other) {
			t.Errorf("errors.Is(erreur de stock, %v) = true, attendu false", other)
		}
	}
}
