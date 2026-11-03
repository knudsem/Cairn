// Package store regroupe l'accès à la base PostgreSQL de Cairn : les types
// métier, leurs lectures (queries.go) et les erreurs métier. Il parle SQL et
// ignore tout de HTTP.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Statuts possibles d'une commande, garantis en base par la contrainte CHECK
// de la colonne orders.status (db/init/001_schema.sql).
const (
	StatusToPick  = "a_preparer"
	StatusPicking = "en_cours"
	StatusReady   = "prete"
)

// ValidStatus indique si status est l'un des statuts d'une commande.
func ValidStatus(status string) bool {
	switch status {
	case StatusToPick, StatusPicking, StatusReady:
		return true
	}
	return false
}

// Erreurs métier : déclarées une seule fois, et reconnues avec errors.Is.
// La couche HTTP les traduit en codes de réponse (docs/PLAN.md, section 4.3).
var (
	ErrOrderNotFound     = errors.New("commande introuvable")
	ErrLineNotFound      = errors.New("ligne introuvable")
	ErrAlreadyPicked     = errors.New("ligne déjà préparée")
	ErrInsufficientStock = errors.New("stock insuffisant")
)

// InsufficientStockError précise ErrInsufficientStock avec les quantités en
// jeu. errors.Is(err, ErrInsufficientStock) la reconnaît grâce à sa méthode
// Is, et errors.As permet d'en lire les champs.
//
// Available est relu juste après le refus, par une nouvelle instruction : le
// stock a pu baisser entre-temps. Il reste inférieur à Requested tant que rien
// n'augmente le stock, ce qui est le cas : aucune opération de l'application
// ne le fait remonter.
type InsufficientStockError struct {
	Available int // stock relu juste après le refus
	Requested int // quantité demandée par la ligne
}

// Quantities décrit les quantités en jeu, par exemple « 3 disponibles,
// 4 demandés » : c'est la fin du message montré au préparateur.
func (e *InsufficientStockError) Quantities() string {
	return countOf(e.Available, "disponible") + ", " + countOf(e.Requested, "demandé")
}

// Error renvoie le texte de l'erreur, pour les journaux.
func (e *InsufficientStockError) Error() string {
	return "stock insuffisant : " + e.Quantities()
}

// Is fait reconnaître cette erreur par errors.Is(err, ErrInsufficientStock).
func (e *InsufficientStockError) Is(target error) bool {
	return target == ErrInsufficientStock
}

// countOf écrit un nombre suivi de son mot, au pluriel à partir de 2, comme
// le veut le français : « 0 disponible », « 1 disponible », « 3 disponibles ».
func countOf(n int, word string) string {
	if n >= 2 {
		word += "s"
	}
	return fmt.Sprintf("%d %s", n, word)
}

// Store donne accès à la base par un pool de connexions.
type Store struct {
	pool *pgxpool.Pool
}

// New crée un Store qui passe par pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ping vérifie que la base répond.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// OrderSummary est une commande telle qu'elle apparaît dans la liste : sans
// ses lignes, mais avec leur nombre et le nombre de lignes déjà préparées,
// pour afficher l'avancement (« 2/5 »).
type OrderSummary struct {
	ID          int64     `json:"id"`
	Reference   string    `json:"reference"`
	Customer    string    `json:"customer"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	LineCount   int       `json:"lineCount"`
	PickedCount int       `json:"pickedCount"`
}

// Order est le détail d'une commande, avec ses lignes.
type Order struct {
	ID        int64     `json:"id"`
	Reference string    `json:"reference"`
	Customer  string    `json:"customer"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	Lines     []Line    `json:"lines"`
}

// Line est une ligne de commande, avec ce qu'il faut savoir du produit pour
// la préparer. PickedAt et PickedBy sont des pointeurs : nil (null en JSON)
// tant que la ligne n'est pas préparée, comme NULL en base.
type Line struct {
	ID          int64      `json:"id"`
	ProductID   int64      `json:"productId"`
	SKU         string     `json:"sku"`
	ProductName string     `json:"productName"`
	Location    string     `json:"location"`
	Quantity    int        `json:"quantity"`
	StockQty    int        `json:"stockQty"`
	PickedAt    *time.Time `json:"pickedAt"`
	PickedBy    *string    `json:"pickedBy"`
}

// Product est un produit de l'entrepôt, avec son emplacement et son stock.
type Product struct {
	ID       int64  `json:"id"`
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Location string `json:"location"`
	StockQty int    `json:"stockQty"`
}
