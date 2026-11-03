package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// PickLine valide la ligne lineID de la commande orderID au nom de picker :
// la ligne passe à « préparée », le stock du produit baisse de la quantité de
// la ligne, et le statut de la commande est recalculé.
//
// Tout se fait dans une seule transaction (docs/PLAN.md, section 3.3) : soit
// tout est enregistré, soit rien. Erreurs métier possibles :
//   - ErrOrderNotFound : la commande n'existe pas ;
//   - ErrLineNotFound : la ligne n'existe pas, ou appartient à une autre commande ;
//   - ErrAlreadyPicked : la ligne est déjà préparée ;
//   - *InsufficientStockError, reconnue par errors.Is(err, ErrInsufficientStock).
func (s *Store) PickLine(ctx context.Context, orderID, lineID int64, picker string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("début de la validation de la ligne %d : %w", lineID, err)
	}
	// Si la fonction se termine avant le Commit (erreur, stock insuffisant),
	// Rollback annule tout ce que la transaction a modifié. Après un Commit
	// réussi, il n'a aucun effet.
	defer tx.Rollback(ctx)

	// 1. Verrouiller la commande et vérifier qu'elle existe. Toute autre
	// validation sur la même commande attend ici que celle-ci se termine : ses
	// instructions verront donc tout ce que celle-ci aura enregistré, et le
	// recalcul du statut (étape 4) restera juste. Le statut lu ne sert pas :
	// la requête pose le verrou et prouve que la commande existe.
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("verrou de la commande %d : %w", orderID, err)
	}

	// 2. Marquer la ligne, seulement si elle appartient à la commande et n'est
	// pas déjà préparée. RETURNING renvoie le produit et la quantité de la
	// ligne modifiée, sans requête supplémentaire.
	var productID int64
	var quantity int
	err = tx.QueryRow(ctx, `
		UPDATE order_lines SET picked_at = now(), picked_by = $3
		WHERE id = $2 AND order_id = $1 AND picked_at IS NULL
		RETURNING product_id, quantity`, orderID, lineID, picker).Scan(&productID, &quantity)
	if errors.Is(err, pgx.ErrNoRows) {
		// Aucune ligne modifiée : soit la ligne n'existe pas dans cette
		// commande, soit elle est déjà préparée. Une lecture tranche. Elle est
		// fiable : le verrou de l'étape 1 empêche toute autre validation de
		// cette commande pendant ce temps.
		var exists bool
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM order_lines WHERE id = $2 AND order_id = $1)`,
			orderID, lineID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("lecture de la ligne %d : %w", lineID, err)
		}
		if !exists {
			return ErrLineNotFound
		}
		return ErrAlreadyPicked
	}
	if err != nil {
		return fmt.Errorf("marquage de la ligne %d : %w", lineID, err)
	}

	// 3. Baisser le stock, seulement s'il suffit : la vérification et la baisse
	// tiennent en une seule instruction. Si le stock visible au début de
	// l'instruction suffit mais qu'une autre transaction est en train de
	// modifier le même produit, celle-ci attend la fin de l'autre, puis
	// réévalue la condition sur le stock à jour (docs/PLAN.md, section 3.3).
	// Si le stock visible ne suffit pas, le refus est immédiat.
	tag, err := tx.Exec(ctx, `
		UPDATE products SET stock_qty = stock_qty - $2
		WHERE id = $1 AND stock_qty >= $2`, productID, quantity)
	if err != nil {
		return fmt.Errorf("baisse du stock du produit %d : %w", productID, err)
	}
	if tag.RowsAffected() == 0 {
		// Stock insuffisant. Le retour de la fonction déclenche le Rollback,
		// qui annule aussi le marquage de l'étape 2. Le stock disponible est
		// relu pour le message (« 3 disponibles, 4 demandés ») : c'est une
		// nouvelle instruction, qui voit une image plus récente de la base.
		var available int
		err = tx.QueryRow(ctx, `SELECT stock_qty FROM products WHERE id = $1`, productID).Scan(&available)
		if err != nil {
			return fmt.Errorf("lecture du stock du produit %d : %w", productID, err)
		}
		return &InsufficientStockError{Available: available, Requested: quantity}
	}

	// 4. Recalculer le statut de la commande : « prete » si plus aucune ligne
	// n'attend, « en_cours » sinon.
	_, err = tx.Exec(ctx, `
		UPDATE orders SET status = CASE
		    WHEN EXISTS (SELECT 1 FROM order_lines
		                 WHERE order_id = $1 AND picked_at IS NULL)
		    THEN 'en_cours' ELSE 'prete' END
		WHERE id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("statut de la commande %d : %w", orderID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("enregistrement de la validation de la ligne %d : %w", lineID, err)
	}
	return nil
}
