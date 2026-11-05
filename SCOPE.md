# Cairn : scope du prototype

## 1. Objectif

Cairn est un prototype de suivi des préparations de commandes dans un entrepôt.

Il sert à démontrer la prise en main de trois technologies nouvelles pour moi : Go, .NET (C#) et Blazor, avec PostgreSQL. Le but n'est pas de construire un produit complet, mais une fonctionnalité complète et propre, de l'interface jusqu'à l'enregistrement en base de données, que je peux expliquer ligne par ligne.

Principe directeur : **petit, fini, testé et bien expliqué**, plutôt que gros et inachevé.

## 2. Utilisateur et besoin

**Utilisateur :** un préparateur de commandes dans un entrepôt, sur tablette, souvent debout, avec des gants, pressé, et pas forcément à l'aise avec l'informatique.

**Besoin :** savoir quelles commandes préparer, où trouver chaque produit, et indiquer ce qui est préparé, sans erreur de stock.

**Parcours principal :**
1. Le préparateur choisit son profil dans la liste des préparateurs, ou crée le sien.
2. Il voit la liste des commandes à préparer.
3. Il ouvre une commande et voit ses lignes : produit, quantité, emplacement.
4. Il valide chaque ligne une fois le produit prélevé. Le stock baisse automatiquement, et la ligne garde le nom du préparateur.
5. Si le stock est insuffisant, la validation est refusée avec un message clair.
6. Quand toutes les lignes sont validées, la commande passe automatiquement au statut « prête ».

## 3. Périmètre

### Obligatoire (le prototype doit être complet sur ces points)
- Liste des commandes avec leur statut : `a_preparer`, `en_cours`, `prete`.
- Détail d'une commande avec ses lignes (produit, quantité, emplacement, état préparé ou non).
- Validation d'une ligne : la ligne passe à préparée, le stock du produit baisse, et le statut de la commande se met à jour.
- Règle absolue : le stock ne devient jamais négatif, même si deux préparateurs valident en même temps.
- Une ligne déjà préparée ne peut pas être validée une seconde fois.
- Profils de préparateurs : liste des profils, création d'un profil (nom unique sans tenir compte des majuscules, 20 profils au maximum, même en cas de créations simultanées), choix d'un profil pour la session.
- Données de démonstration chargées au démarrage, avec trois profils (« Claire », « Virginie » et « Romain »), et remise à zéro qui les rétablit exactement.
- Tests automatisés sur les règles métier.
- Lancement complet en une commande avec Docker Compose.

### Bonus (seulement si tout l'obligatoire est terminé et testé)
- Déploiement en ligne gratuit, avec un lien public en HTTPS, pour tester sans rien installer (Render et Neon, voir docs/PLAN.md, étape 11). Il passe avant les autres bonus, mais après les tests, le README et la préparation de la présentation.
- Historique des mouvements de stock (qui, quoi, quand).
- Alertes de stock bas.
- Annulation de la validation d'une ligne, avec remise en stock.
- Suppression d'un profil depuis « Qui prépare ? », avec confirmation ; les lignes déjà préparées gardent son nom (docs/PLAN.md, étape 13).
- Bouton « Réinitialiser la démo » sur « Qui prépare ? », avec confirmation : les testeurs remettent les commandes, le stock et les profils à leur état de départ sans commande (POST /api/demo/reset, actif seulement si DEMO_RESET_ENABLED vaut true ; docs/PLAN.md, étape 15).

### Hors périmètre
- Authentification : les profils de préparateurs n'ont ni mot de passe ni droits. N'importe qui peut choisir un profil ou en créer un. C'est un choix assumé pour ce prototype, à revoir avant tout usage réel.
- Modification d'un profil (son nom).
- Création ou modification de commandes depuis l'interface.

## 4. Stack technique

| Couche | Choix | Remarque |
|---|---|---|
| API | Go, librairie standard `net/http` | Routage avec méthodes et paramètres de chemin (disponible depuis Go 1.22) |
| Accès base | `pgx` | Driver PostgreSQL de référence en Go |
| Base de données | PostgreSQL | Contraintes en base en plus des règles dans le code |
| Interface | Blazor (.NET 10), Blazor Web App | Appelle l'API Go en HTTP |
| Tests | `go test` | Tests des règles métier, y compris la concurrence |
| Lancement | Docker Compose | PostgreSQL, API et interface |

Pas de framework lourd côté Go : l'objectif est de comprendre les bases du langage. Une petite librairie est acceptable si elle est justifiée dans le README.

## 5. Modèle de données (proposition, à valider pendant la conception)

- **products** : `id`, `sku`, `name`, `location` (ex. `A-03-2` pour allée, rack, niveau), `stock_qty` avec la contrainte `CHECK (stock_qty >= 0)`.
- **orders** : `id`, `reference`, `customer`, `status`, `created_at`.
- **order_lines** : `id`, `order_id`, `product_id`, `quantity` (strictement positive), `picked_at` (vide si non préparée), `picked_by` (nom du profil, recopié au moment de la validation).
- **pickers** : `id`, `name` (unique sans tenir compte des majuscules, 50 caractères au maximum), `created_at`.
- **stock_movements** (bonus) : `id`, `product_id`, `delta`, `reason`, `order_line_id`, `created_by`, `created_at`.

### Garantir que le stock ne devient jamais négatif
La validation d'une ligne se fait dans **une seule transaction** :
1. Marquer la ligne comme préparée, seulement si elle ne l'est pas déjà.
2. Décrémenter le stock avec une mise à jour conditionnelle, du type `UPDATE products SET stock_qty = stock_qty - $1 WHERE id = $2 AND stock_qty >= $1`. Si aucune ligne n'est modifiée, le stock est insuffisant et on annule la transaction.
3. Recalculer le statut de la commande.

La contrainte `CHECK` en base sert de filet de sécurité supplémentaire. Le choix final (mise à jour conditionnelle ou verrou `SELECT ... FOR UPDATE`) doit être expliqué dans le README.

## 6. API (proposition)

| Méthode | Route | Rôle |
|---|---|---|
| `GET` | `/health` | Vérifier que l'API et la base répondent |
| `GET` | `/api/orders?status=a_preparer` | Lister les commandes, avec filtre optionnel |
| `GET` | `/api/orders/{id}` | Détail d'une commande et de ses lignes |
| `POST` | `/api/orders/{id}/lines/{lineId}/pick` | Valider une ligne (corps : `{ "picker": "Nom" }`) |
| `GET` | `/api/products` | Lister les produits et leur stock |
| `GET` | `/api/pickers` | Lister les profils de préparateurs, triés par nom |
| `POST` | `/api/pickers` | Créer un profil (corps : `{ "name": "Nom" }`) |
| `DELETE` | `/api/pickers/{id}` | Supprimer un profil (bonus) ; les lignes déjà préparées gardent son nom |
| `POST` | `/api/demo/reset` | Remettre la démo à son état de départ (bonus ; seulement si `DEMO_RESET_ENABLED` vaut `true`, sinon 404) |

### Codes de réponse
- `200` : succès ; `201` : profil créé ; `204` : profil supprimé, ou démo remise à zéro.
- `400` : requête invalide.
- `404` : commande, ligne ou profil introuvable.
- `409` : conflit (stock insuffisant, ligne déjà préparée, profil existant ou trop de profils), avec un message lisible.

Les erreurs sont renvoyées dans un format JSON unique, par exemple `{ "error": "stock_insuffisant", "message": "..." }`.

## 7. Interface Blazor

Écrans :
1. **Qui prépare ?** : les profils en gros boutons, et un bouton « + Ajouter un profil » qui ouvre un champ « Nom du profil » avec les boutons « Créer » et « Annuler ». Le profil choisi est gardé pour la session ; la barre du haut affiche son nom et un bouton « Changer ». En bonus, un bouton « Gérer les profils » ouvre un mode gestion, où toucher un profil ouvre la confirmation de sa suppression dans la page. En bas à droite, un pied de page discret « Réinitialiser la démo », avec confirmation dans le même coin (bonus).
2. **Commandes** : liste des commandes, statut visible par une couleur et un libellé, la plus ancienne en premier.
3. **Préparation d'une commande** : une carte par ligne, qui se lit comme une consigne : « Allez à » et l'emplacement bien visible, « Prenez » et la quantité en très grand, le produit, la référence à vérifier sur l'étiquette, et un gros bouton « C'est pris ». Aucun mot technique à l'écran (ni « ligne », ni « emplacement », ni « réf. »).

Règles d'ergonomie :
- Sur écran tactile (tablette, téléphone), boutons d'au moins 64 px de haut, utilisables avec des gants ; à la souris (ordinateur), boutons de 40 à 48 px, avec la densité d'une application de bureau.
- Texte lisible à un mètre, contraste élevé.
- Retour immédiat après chaque action : confirmation, ou message d'erreur clair en cas de stock insuffisant.
- Pas de jargon technique dans les messages affichés.

## 8. Tests

Minimum attendu :
- Valider une ligne baisse le stock de la bonne quantité.
- Valider une ligne avec un stock insuffisant est refusé, et le stock ne change pas.
- Valider deux fois la même ligne est refusé.
- Deux validations simultanées sur un stock qui ne suffit que pour une seule : une réussit, l'autre est refusée, et le stock final est correct.
- La commande passe à `prete` quand toutes ses lignes sont préparées.
- Deux profils ne peuvent pas porter le même nom, sans tenir compte des majuscules, et la limite de 20 profils tient même en cas de créations simultanées.

Les tests qui touchent la base tournent sur une vraie base PostgreSQL de test, lancée avec Docker.

## 9. Livrables

1. **Le code**, sur GitHub, avec un historique de commits lisible.
2. **`README.md`** :
   - Présentation du projet et capture d'écran.
   - Lancement en une commande.
   - Architecture et choix techniques argumentés.
   - Fonctionnement de la garantie de stock.
   - Comment les tests sont faits et comment les lancer.
   - Comparaison argumentée : cette API en Go face à la même API en FastAPI (avantages et limites de chacun).
   - Ce que je ferais ensuite avec plus de temps.
   - Comment j'ai utilisé l'IA, et ce que j'ai vérifié moi-même.
3. **`NOTES.md`** : mon journal d'apprentissage. Pour Go, C#/.NET et Blazor :
   - ce que j'ai compris de son fonctionnement ;
   - ses principaux usages et avantages ;
   - les différences avec ce que je connais (Python, Django, JavaScript, React) ;
   - les difficultés rencontrées ;
   - son intérêt dans un environnement logistique.

   Avec les ressources consultées.
4. **Une courte vidéo de démo**, en secours si la démo en direct ne fonctionne pas.

## 10. Planning (2 jours, rendu jeudi soir)

**Mercredi**
1. Matin : apprentissage de Go, puis des bases de C# et Blazor, avec `NOTES.md` rempli au fur et à mesure.
2. Après-midi : schéma de la base, API Go, règles métier et tests.
3. Soir : Docker Compose et données de démonstration.

**Jeudi**
1. Matin : interface Blazor connectée à l'API.
2. Début d'après-midi : vérifications de bout en bout, README, vidéo de démo.
3. Fin d'après-midi : préparation de la présentation.
4. Soir : répétition et envoi.

En cas de retard, on retire des bonus, jamais les tests ni le README.

## 11. Définition de « terminé »

- `docker compose up` lance tout sans étape manuelle.
- Le parcours principal fonctionne dans le navigateur, du choix du profil jusqu'à la commande « prête ».
- Tous les tests passent.
- Le README permet à quelqu'un qui ne me connaît pas de lancer et de comprendre le projet.
- Je peux expliquer chaque fichier et chaque choix technique.
