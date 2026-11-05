package store

import (
	"context"
	"fmt"

	"github.com/knudsem/cairn/api/internal/demo"
)

// ResetDemo remet les données de démonstration à leur état de départ
// (docs/PLAN.md, étape 15), dans une seule transaction : les quatre tables
// sont vidées, compteurs d'identifiants compris, puis les données de
// demo.Seed (le fichier 002_seed.sql) sont rechargées. Le schéma ne change
// pas : aucun DROP TABLE. En cas d'erreur, Rollback annule tout, et rien
// n'est changé. Deux remises à zéro de suite donnent le même résultat : la
// recommencer après un échec est sans risque.
func (s *Store) ResetDemo(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("début de la remise à zéro : %w", err)
	}
	// Si la fonction se termine avant le Commit, Rollback annule tout.
	defer tx.Rollback(ctx)

	// orders d'abord, comme dans testdb.Reset et la remise à zéro de la
	// section 3.4 : TRUNCATE verrouille les tables dans l'ordre de la liste,
	// et PickLine verrouille la commande avant ses lignes. Le même ordre
	// évite qu'une remise à zéro et une validation s'interbloquent.
	if _, err := tx.Exec(ctx, `TRUNCATE orders, order_lines, products, pickers RESTART IDENTITY`); err != nil {
		return fmt.Errorf("vidage des tables : %w", err)
	}
	// Sans argument, pgx envoie le texte tel quel (protocole simple) :
	// toutes les instructions du fichier passent en un seul appel, dans la
	// transaction.
	if _, err := tx.Exec(ctx, demo.Seed); err != nil {
		return fmt.Errorf("chargement des données de démonstration : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("enregistrement de la remise à zéro : %w", err)
	}
	return nil
}
