package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ListPickers renvoie les profils de préparateurs, triés par nom.
func (s *Store) ListPickers(ctx context.Context) ([]Picker, error) {
	// La collation ICU fr-x-icu donne l'ordre alphabétique français
	// (« Émile » entre « Bob » et « Ève », « Jean-Yves » avant « Jeanne »),
	// quelle que soit la configuration du serveur : la collation C mettrait
	// les majuscules d'abord et les lettres accentuées à la fin. fr-x-icu est
	// déterministe et l'index unique interdit deux noms identiques : deux
	// profils ne sont jamais à égalité, l'ordre est donc toujours le même.
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, created_at
		FROM pickers
		ORDER BY name COLLATE "fr-x-icu"`)
	if err != nil {
		return nil, fmt.Errorf("liste des profils : %w", err)
	}
	defer rows.Close()

	// Liste initialisée vide, et non nil : elle s'encode [] en JSON, pas null.
	pickers := []Picker{}
	for rows.Next() {
		var p Picker
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("lecture d'un profil : %w", err)
		}
		p.CreatedAt = p.CreatedAt.UTC()
		pickers = append(pickers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("liste des profils : %w", err)
	}
	return pickers, nil
}

// CreatePicker crée le profil name, déjà contrôlé par la couche HTTP, et le
// renvoie. Erreurs métier possibles :
//   - *PickerExistsError, reconnue par errors.Is(err, ErrPickerExists) : un
//     profil porte déjà ce nom, sans tenir compte des majuscules ;
//   - ErrTooManyPickers : MaxPickers profils existent déjà.
func (s *Store) CreatePicker(ctx context.Context, name string) (Picker, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Picker{}, fmt.Errorf("début de la création du profil : %w", err)
	}
	// Si la fonction se termine avant le Commit, Rollback annule tout.
	defer tx.Rollback(ctx)

	// 1. Verrouiller la table. Le mode SHARE ROW EXCLUSIVE laisse passer les
	// lectures, mais une seconde création attend ici la fin de celle-ci :
	// la recherche du nom et le comptage qui suivent restent justes jusqu'au
	// Commit. Sans ce verrou, deux créations simultanées à 19 profils
	// compteraient chacune 19 profils, et en feraient 21.
	if _, err := tx.Exec(ctx, `LOCK TABLE pickers IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return Picker{}, fmt.Errorf("verrou des profils : %w", err)
	}

	// 2. Un nom déjà pris, sans tenir compte des majuscules, l'emporte sur la
	// limite : le préparateur n'a qu'à choisir ce profil. La recherche
	// compare la même expression que l'index unique de la table : elle
	// trouve exactement les noms que l'index refuserait.
	var existing string
	err = tx.QueryRow(ctx, `
		SELECT name FROM pickers
		WHERE lower(normalize(name, NFC)) = lower(normalize($1, NFC))`, name).Scan(&existing)
	if err == nil {
		return Picker{}, &PickerExistsError{Name: existing}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Picker{}, fmt.Errorf("recherche du profil : %w", err)
	}

	// 3. La limite du nombre de profils.
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pickers`).Scan(&count); err != nil {
		return Picker{}, fmt.Errorf("comptage des profils : %w", err)
	}
	if count >= MaxPickers {
		return Picker{}, ErrTooManyPickers
	}

	// 4. L'insertion. RETURNING renvoie l'identifiant et la date de création,
	// sans requête supplémentaire.
	var p Picker
	err = tx.QueryRow(ctx, `
		INSERT INTO pickers (name) VALUES ($1)
		RETURNING id, name, created_at`, name).Scan(&p.ID, &p.Name, &p.CreatedAt)
	if err != nil {
		return Picker{}, fmt.Errorf("création du profil : %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Picker{}, fmt.Errorf("enregistrement du profil : %w", err)
	}
	p.CreatedAt = p.CreatedAt.UTC()
	return p, nil
}
