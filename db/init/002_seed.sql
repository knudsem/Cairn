-- Données de démonstration (voir docs/PLAN.md, section 3.4).
-- Exécuté une seule fois par l'image PostgreSQL, après 001_schema.sql.
--
-- Les ordres d'insertion sont volontairement mélangés : les produits ne
-- suivent pas l'ordre des emplacements, et les dates de création ne suivent
-- pas l'ordre des identifiants. Ainsi, les vérifications de tri (par
-- emplacement, par date) échouent si le tri est oublié dans le code.

-- Stock actuel, après les prélèvements déjà faits sur CMD-0005 et CMD-0006.
-- Les identifiants suivent l'ordre d'insertion : SAC-30 a l'id 1, CAR-40 l'id 2.
INSERT INTO products (sku, name, location, stock_qty) VALUES
    ('SAC-30', 'Sac kraft 30 L',                 'B-11-1', 50),
    ('CAR-40', 'Carton 40x30x30',                'A-01-2', 12),
    ('DEV-48', 'Dévidoir de ruban adhésif',      'C-10-1',  2),  -- CMD-0004 en demande 5
    ('ETQ-15', 'Étiquettes 100x150, rouleau',    'B-03-1',  6),
    ('BUL-50', 'Papier bulle 1 x 50 m',          'C-02-1',  5),  -- CMD-0002 et CMD-0003 en demandent 4 chacune
    ('ADH-48', 'Ruban adhésif 48 mm',            'A-05-3', 30),
    ('CAL-80', 'Papier de calage 80 g, rouleau', 'C-06-2',  9),
    ('CAR-60', 'Carton 60x40x40',                'A-02-1',  8),
    ('GAN-09', 'Gants de manutention taille 9',  'C-04-3', 24),
    ('FIL-50', 'Film étirable 50 cm',            'B-07-2', 10);

-- CMD-0001 a l'id 1. Les deux plus anciennes (CMD-0006 et CMD-0005) sont
-- déjà terminée ou commencée : la liste « la plus ancienne en premier »
-- commence donc par elles.
INSERT INTO orders (reference, customer, status, created_at) VALUES
    ('CMD-0001', 'Boulangerie Martin',  'a_preparer', now() - interval '2 hours'),
    ('CMD-0002', 'Pharmacie du Centre', 'a_preparer', now() - interval '1 hour 45 minutes'),
    ('CMD-0003', 'Fromagerie Lefèvre',  'a_preparer', now() - interval '1 hour 30 minutes'),
    ('CMD-0004', 'Épicerie Moreau',     'a_preparer', now() - interval '1 hour 15 minutes'),
    ('CMD-0005', 'Librairie Dumas',     'en_cours',   now() - interval '2 hours 30 minutes'),
    ('CMD-0006', 'Café des Halles',     'prete',      now() - interval '3 hours');

-- Chaque ligne retrouve sa commande et son produit par leur référence et
-- leur SKU, jamais par un identifiant écrit en dur. Une faute de frappe donne
-- NULL, refusé par NOT NULL : l'initialisation s'arrête au lieu de créer une
-- donnée fausse.
INSERT INTO order_lines (order_id, product_id, quantity, picked_at, picked_by) VALUES
    -- CMD-0001 : parcours complet jusqu'à « prête ». Lignes insérées hors de
    -- l'ordre des emplacements ; CAR-40 est la ligne 3 (exemple de PLAN.md 4.2).
    ((SELECT id FROM orders WHERE reference = 'CMD-0001'), (SELECT id FROM products WHERE sku = 'SAC-30'), 20, NULL, NULL),
    ((SELECT id FROM orders WHERE reference = 'CMD-0001'), (SELECT id FROM products WHERE sku = 'ETQ-15'),  1, NULL, NULL),
    ((SELECT id FROM orders WHERE reference = 'CMD-0001'), (SELECT id FROM products WHERE sku = 'CAR-40'),  4, NULL, NULL),
    ((SELECT id FROM orders WHERE reference = 'CMD-0001'), (SELECT id FROM products WHERE sku = 'ADH-48'),  6, NULL, NULL),

    -- CMD-0002 et CMD-0003 : 4 BUL-50 chacune pour un stock de 5. Une seule
    -- des deux peut être servie : démonstration de concurrence.
    ((SELECT id FROM orders WHERE reference = 'CMD-0002'), (SELECT id FROM products WHERE sku = 'BUL-50'),  4, NULL, NULL),
    ((SELECT id FROM orders WHERE reference = 'CMD-0003'), (SELECT id FROM products WHERE sku = 'BUL-50'),  4, NULL, NULL),

    -- CMD-0004 : CAR-60 est disponible, DEV-48 ne l'est pas (5 demandés,
    -- 2 en stock) : refus reproductible.
    ((SELECT id FROM orders WHERE reference = 'CMD-0004'), (SELECT id FROM products WHERE sku = 'CAR-60'),  2, NULL, NULL),
    ((SELECT id FROM orders WHERE reference = 'CMD-0004'), (SELECT id FROM products WHERE sku = 'DEV-48'),  5, NULL, NULL),

    -- CMD-0005 : 1 ligne sur 3 déjà préparée.
    ((SELECT id FROM orders WHERE reference = 'CMD-0005'), (SELECT id FROM products WHERE sku = 'GAN-09'), 12, now() - interval '40 minutes', 'Karim'),
    ((SELECT id FROM orders WHERE reference = 'CMD-0005'), (SELECT id FROM products WHERE sku = 'FIL-50'),  3, NULL, NULL),
    ((SELECT id FROM orders WHERE reference = 'CMD-0005'), (SELECT id FROM products WHERE sku = 'CAL-80'),  2, NULL, NULL),

    -- CMD-0006 : toutes les lignes préparées.
    ((SELECT id FROM orders WHERE reference = 'CMD-0006'), (SELECT id FROM products WHERE sku = 'CAR-40'), 10, now() - interval '35 minutes', 'Léa'),
    ((SELECT id FROM orders WHERE reference = 'CMD-0006'), (SELECT id FROM products WHERE sku = 'ADH-48'), 12, now() - interval '30 minutes', 'Léa');
