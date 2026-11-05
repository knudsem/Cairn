-- Schéma de la base Cairn (voir docs/PLAN.md, section 3).
-- Exécuté une seule fois par l'image PostgreSQL, quand le volume est vide.

-- Produits stockés dans l'entrepôt.
CREATE TABLE products (
    id         bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sku        text    NOT NULL UNIQUE,
    name       text    NOT NULL,
    location   text    NOT NULL,              -- ex. 'A-03-2' : allée, rack, niveau
    -- Règle absolue : le stock ne devient jamais négatif. PostgreSQL refuse
    -- toute écriture qui enfreindrait cette contrainte, qu'elle vienne de
    -- l'API ou d'une requête manuelle.
    stock_qty  integer NOT NULL CHECK (stock_qty >= 0)
);

-- Commandes à préparer.
CREATE TABLE orders (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reference   text        NOT NULL UNIQUE,
    customer    text        NOT NULL,
    status      text        NOT NULL DEFAULT 'a_preparer'
                CHECK (status IN ('a_preparer', 'en_cours', 'prete')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Lignes d'une commande : un produit et la quantité à prélever.
CREATE TABLE order_lines (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    bigint      NOT NULL REFERENCES orders (id),
    product_id  bigint      NOT NULL REFERENCES products (id),
    quantity    integer     NOT NULL CHECK (quantity > 0),
    picked_at   timestamptz,                  -- NULL = pas encore préparée
    picked_by   text,
    -- Une ligne préparée a une date et un préparateur ; une ligne non
    -- préparée n'a ni l'un ni l'autre.
    CHECK ((picked_at IS NULL) = (picked_by IS NULL))
);

-- PostgreSQL n'indexe pas automatiquement les clés étrangères. Cet index
-- accélère la lecture des lignes d'une commande.
CREATE INDEX order_lines_order_id_idx ON order_lines (order_id);

-- Profils de préparateurs (docs/PLAN.md, étape 7 bis) : un nom choisi dans
-- l'interface, sans mot de passe. Le nom d'une ligne préparée
-- (order_lines.picked_by) est une copie, sans lien vers cette table :
-- l'historique ne dépend pas du profil.
CREATE TABLE pickers (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- L'API contrôle le nom (docs/PLAN.md, étape 7) ; la base refuse quand
    -- même un nom vide ou trop long, comme filet de sécurité.
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Unicité sans tenir compte des majuscules : « Claire » et « CLAIRE » ne
-- peuvent pas coexister. normalize(name, NFC) ramène d'abord les deux façons
-- d'écrire une lettre accentuée (« é » en un caractère, ou « e » suivi de
-- l'accent) à la même forme : sinon, deux « Émile » identiques à l'écran
-- pourraient coexister.
CREATE UNIQUE INDEX pickers_name_key ON pickers (lower(normalize(name, NFC)));
