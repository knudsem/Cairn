package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Les dates lues sont ramenées en UTC : pgx les renvoie dans le fuseau du
// processus (UTC dans le conteneur, heure de Paris avec go run sous Windows).
// L'API répond ainsi partout de la même façon, avec le suffixe Z.

// ListOrders renvoie les commandes, de la plus ancienne à la plus récente.
// Si status n'est pas vide, seules les commandes de ce statut sont renvoyées.
func (s *Store) ListOrders(ctx context.Context, status string) ([]OrderSummary, error) {
	// Le filtre est facultatif : une chaîne vide laisse passer toutes les
	// commandes. count(l.picked_at) ne compte que les valeurs non NULL, donc
	// les lignes préparées. o.id départage deux commandes créées au même instant.
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.reference, o.customer, o.status, o.created_at,
		       count(l.id), count(l.picked_at)
		FROM orders o
		LEFT JOIN order_lines l ON l.order_id = o.id
		WHERE $1 = '' OR o.status = $1
		GROUP BY o.id
		ORDER BY o.created_at, o.id`, status)
	if err != nil {
		return nil, fmt.Errorf("liste des commandes : %w", err)
	}
	defer rows.Close()

	// Liste initialisée vide, et non nil : elle s'encode [] en JSON, pas null.
	orders := []OrderSummary{}
	for rows.Next() {
		var o OrderSummary
		if err := rows.Scan(&o.ID, &o.Reference, &o.Customer, &o.Status, &o.CreatedAt,
			&o.LineCount, &o.PickedCount); err != nil {
			return nil, fmt.Errorf("lecture d'une commande : %w", err)
		}
		o.CreatedAt = o.CreatedAt.UTC()
		orders = append(orders, o)
	}
	// rows.Err signale une erreur survenue pendant le parcours, par exemple une
	// connexion coupée : sans ce test, la liste serait incomplète sans le dire.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("liste des commandes : %w", err)
	}
	return orders, nil
}

// GetOrder renvoie la commande id avec ses lignes, triées par emplacement,
// c'est-à-dire dans l'ordre du chemin de prélèvement. Elle renvoie
// ErrOrderNotFound si la commande n'existe pas.
func (s *Store) GetOrder(ctx context.Context, id int64) (Order, error) {
	// Deux requêtes, la commande puis ses lignes, dans une transaction en
	// lecture seule de niveau REPEATABLE READ : elles lisent la même image de
	// la base. Sinon, une validation enregistrée entre les deux pourrait donner
	// un statut qui ne correspond pas aux lignes (voir docs/PLAN.md, 3.3).
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Order{}, fmt.Errorf("lecture de la commande %d : %w", id, err)
	}
	// Une transaction en lecture seule n'a rien à enregistrer : Rollback la
	// termine, quelle que soit la sortie de la fonction.
	defer tx.Rollback(ctx)

	var o Order
	err = tx.QueryRow(ctx, `
		SELECT id, reference, customer, status, created_at
		FROM orders
		WHERE id = $1`, id).Scan(&o.ID, &o.Reference, &o.Customer, &o.Status, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lecture de la commande %d : %w", id, err)
	}
	o.CreatedAt = o.CreatedAt.UTC()

	rows, err := tx.Query(ctx, `
		SELECT l.id, l.product_id, p.sku, p.name, p.location, l.quantity,
		       p.stock_qty, l.picked_at, l.picked_by
		FROM order_lines l
		JOIN products p ON p.id = l.product_id
		WHERE l.order_id = $1
		ORDER BY p.location, l.id`, id)
	if err != nil {
		return Order{}, fmt.Errorf("lignes de la commande %d : %w", id, err)
	}
	defer rows.Close()

	o.Lines = []Line{}
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.ID, &l.ProductID, &l.SKU, &l.ProductName, &l.Location,
			&l.Quantity, &l.StockQty, &l.PickedAt, &l.PickedBy); err != nil {
			return Order{}, fmt.Errorf("lecture d'une ligne de la commande %d : %w", id, err)
		}
		if l.PickedAt != nil {
			t := l.PickedAt.UTC()
			l.PickedAt = &t
		}
		o.Lines = append(o.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return Order{}, fmt.Errorf("lignes de la commande %d : %w", id, err)
	}
	return o, nil
}

// ListProducts renvoie les produits et leur stock, dans l'ordre des
// emplacements.
func (s *Store) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, sku, name, location, stock_qty
		FROM products
		ORDER BY location, id`)
	if err != nil {
		return nil, fmt.Errorf("liste des produits : %w", err)
	}
	defer rows.Close()

	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Location, &p.StockQty); err != nil {
			return nil, fmt.Errorf("lecture d'un produit : %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("liste des produits : %w", err)
	}
	return products, nil
}
