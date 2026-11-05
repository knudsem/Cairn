// Package demo contient les données de démonstration de Cairn (docs/PLAN.md,
// section 3.4 et étape 15) : le fichier 002_seed.sql, leur seule source.
package demo

// L'import « _ » (blank import) charge le paquet embed sans en utiliser de
// nom : il suffit pour que la directive go:embed ci-dessous fonctionne.
import _ "embed"

// Seed est le contenu de 002_seed.sql, copié dans l'exécutable à la
// compilation : l'API n'a besoin d'aucun fichier à côté d'elle, en local
// comme dans l'image Render. go:embed ne lit que les fichiers du dossier du
// paquet ou de ses sous-dossiers : le fichier est donc ici, et non dans
// db/init. compose.yaml et db/reset.sql le lisent à cet endroit.
//
//go:embed 002_seed.sql
var Seed string
