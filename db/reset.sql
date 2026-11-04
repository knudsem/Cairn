-- Remise à zéro d'une base hors Docker, par exemple la base Neon de la
-- version en ligne (docs/PLAN.md, étape 11). Supprime les quatre tables, puis
-- rejoue le schéma et les données de démonstration, avec des dates de
-- création recalculées et les trois profils d'origine.
--
-- À lancer en une seule transaction (psql -1) : en cas d'erreur, rien n'est
-- changé. Depuis C:\dev\cairn, sans installer psql :
--   docker run --rm -v "${PWD}/db:/db:ro" postgres:18 psql $env:DEMO_DATABASE_URL -v ON_ERROR_STOP=1 -1 -f /db/reset.sql
--
-- orders vient en premier, comme dans le TRUNCATE de la section 3.4 :
-- DROP verrouille les tables dans l'ordre de sa liste, et une validation
-- verrouille la commande avant ses lignes.
DROP TABLE IF EXISTS orders, order_lines, products, pickers;

-- \ir lit un fichier à partir du dossier de ce script.
\ir init/001_schema.sql
\ir init/002_seed.sql
