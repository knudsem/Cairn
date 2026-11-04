# Cairn : plan de conception et de réalisation

Ce document décrit l'architecture retenue, le modèle de données, l'API et le découpage du travail. Le périmètre fonctionnel est défini dans [SCOPE.md](../SCOPE.md). Le journal d'apprentissage est tenu dans [NOTES.md](../NOTES.md).

Sommaire :

1. Décisions structurantes
2. Architecture
3. Base de données
4. API
5. Plan de réalisation
6. Planning
7. Environnement de développement

Versions de référence : Go 1.27, .NET 10 (LTS), PostgreSQL 18. Version en ligne : Render (offre gratuite) et Neon (PostgreSQL 18), voir l'étape 11.

---

## 1. Décisions structurantes

| Décision | Raison |
|---|---|
| Valider un squelette de bout en bout dès le premier jour | Le principal risque n'est pas une technologie prise isolément, mais leur assemblage : réseau Docker, ports, mode de rendu Blazor. Une page Blazor qui affiche l'état de l'API, lancée par Docker Compose, valide toute la chaîne avant d'écrire les fonctionnalités. |
| Une base PostgreSQL de test séparée et jetable | Les tests vident les tables avant chaque cas. Ils ne doivent jamais toucher aux données de démonstration. |
| Verrouiller la commande au début de la transaction de validation | Sans ce verrou, deux validations simultanées des dernières lignes d'une commande peuvent la laisser au statut `en_cours` alors que tout est préparé. Mesure de l'étape 5, avec un départ simultané et deux lignes sur deux produits différents : 200 commandes sur 200 restées `en_cours` sans le verrou, 0 sur 100 avec. Explication en section 3.3. |
| Des profils de préparateurs, sans authentification | Le préparateur choisit son nom dans une liste de profils, ou crée le sien ; aucun mot de passe, choix assumé du scope pour un prototype. Le nom du profil est recopié dans la ligne validée (`picked_by`) : l'historique ne dépend pas du profil. Étape 7 bis. |

---

## 2. Architecture

### 2.1 Vue d'ensemble

```
Tablette (navigateur)
    ↓  connexion permanente (SignalR, WebSocket)
web : Blazor .NET 10, mode Interactive Server     http://localhost:8081
    ↓  HTTP + JSON, réseau interne Docker : http://api:8080
api : Go, net/http + pgx                          http://localhost:8080
    ↓  protocole PostgreSQL
db  : PostgreSQL 18                               127.0.0.1:5432
db-test : PostgreSQL 18 jetable, tests seulement  127.0.0.1:5433
```

Tous les ports (base, API, interface) ne sont publiés que sur `127.0.0.1` : rien n'est joignable depuis le réseau, ce qui compte avec un mot de passe de prototype et une API sans authentification. Depuis Windows (API lancée avec `go run`, tests), on vise donc toujours `127.0.0.1`, jamais `localhost` : Windows essaie d'abord l'adresse IPv6 `::1`, sur laquelle Docker n'écoute pas, et chaque connexion attend environ 2 secondes avant de passer à `127.0.0.1`. Un navigateur accepte `localhost` : il bascule vers IPv4 en quelques centaines de millisecondes. Dans le réseau Docker, chaque service joint les autres par leur nom : `db:5432`, `api:8080`. En local, la démonstration se fait donc sur le poste, avec le mode tablette des outils de développement (étape 10) ; le lien public vient du déploiement (étape 11).

### 2.2 Mode de rendu Blazor

| Mode | Où s'exécute le C# | Conséquence |
|---|---|---|
| Static SSR | serveur, page HTML classique | pas d'interaction sans formulaire |
| **Interactive Server** (retenu) | serveur ; le navigateur transmet les clics par une connexion permanente | un seul projet ; l'API Go est appelée depuis le serveur |
| Interactive WebAssembly | navigateur | deux projets ; l'API doit gérer le CORS ; la tablette télécharge le runtime .NET |

Avec Interactive Server, le navigateur ne communique jamais directement avec l'API Go : pas de CORS à configurer, une seule base de code C#. La contrepartie est la connexion permanente : en cas de coupure Wi-Fi, Blazor affiche une fenêtre de reconnexion. Le modèle de projet .NET 10 génère ce composant (`ReconnectModal`).

Le pré-rendu est désactivé (`new InteractiveServerRenderMode(prerender: false)`). Activé, il rend chaque page deux fois (une version HTML statique, puis la version interactive), ce qui appelle l'API deux fois. Pour une application interne sur tablette, il n'apporte presque rien.

### 2.3 Arborescence

```
Cairn/
  README.md
  NOTES.md                     journal d'apprentissage
  SCOPE.md                     périmètre fonctionnel
  compose.yaml                 services db, api, web (+ db-test)
  render.yaml                  déploiement sur Render (étape 11)
  .gitattributes               fins de ligne LF
  .editorconfig                réglages communs aux éditeurs
  .gitignore
  .claude/settings.json
  docs/
    PLAN.md                    ce document
    logos SVG, capture d'écran du README
  db/
    init/
      001_schema.sql           tables et contraintes
      002_seed.sql             données de démonstration
    reset.sql                  remise à zéro d'une base hors Docker (Neon, étape 11)
  api/                         module Go
    go.mod, go.sum
    main.go                    démarrage : configuration, connexion, serveur HTTP
    Dockerfile
    internal/
      store/
        store.go               types (Order, Line, Product, Picker) et erreurs métier
        queries.go             lectures : liste, détail, produits
        pick.go                règle métier : PickLine, une transaction
        pickers.go             profils de préparateurs : liste et création (étape 7 bis)
        pickers_test.go        tests des profils, concurrence comprise
        db_test.go             tests : connexion à la base de test, outils communs
        pick_test.go           tests de PickLine, concurrence comprise
        queries_test.go        tests des lectures
      httpapi/
        routes.go              table des routes
        handlers.go            traduction HTTP ↔ store
        body.go                lecture des corps JSON et contrôle des noms, communs aux routes (étape 7 bis)
        pick.go                validation d'une ligne : traduction des erreurs
        pickers.go             routes des profils (étape 7 bis)
        respond.go             réponses JSON et format d'erreur unique
        main_test.go           tests HTTP (httptest) : outils communs
        pick_test.go           tests de la route de validation
        pickers_test.go        tests des routes des profils
        routes_test.go         tests des lectures et du routeur
      testdb/
        testdb.go              base de test des deux paquets : garde-fou, verrou, remise à zéro
  web/                         projet Blazor
    Cairn.Web.csproj
    Program.cs                 injection de dépendances, HttpClient
    Dockerfile
    appsettings.json, appsettings.Development.json
    Api/
      CairnApiClient.cs        seul endroit qui connaît les URL de l'API
      Models.cs                types échangés avec l'API : records du JSON, état ApiHealth
    State/
      PickerState.cs           profil du préparateur choisi pour la session
    Components/
      App.razor, Routes.razor, _Imports.razor   (générés ; Routes.razor renvoie les pages sans profil)
      RedirectToPickers.razor  renvoi vers « Qui prépare ? » (étape 7 bis)
      Layout/
        MainLayout.razor       barre du haut : nom du profil et bouton « Changer »
      Pages/
        Pickers.razor          écran 1 : « Qui prépare ? » (étape 7 bis)
        Orders.razor           écran 2 : liste des commandes
        OrderDetail.razor      écran 3 : préparation d'une commande
      StatusBadge.razor        pastille de statut : couleur et libellé
    wwwroot/
      app.css, logo
```

### 2.4 Justification des choix

| Choix | Justification |
|---|---|
| Un dossier par conteneur (`db`, `api`, `web`) | La structure du dépôt reflète l'architecture. |
| Scripts `db/init/` exécutés par l'image officielle PostgreSQL | Mécanisme documenté de l'image, sans code à écrire. Limite : les scripts ne s'exécutent que sur un volume vide ; `docker compose down -v` repart de zéro. Il n'y a pas d'outil de migration, ce qui est assumé pour un prototype ; un projet durable utiliserait goose, golang-migrate ou l'outil déjà en place. Les deux scripts restent du SQL standard, sans commande propre à psql ni à Docker : `db/reset.sql` les rejoue sur la base en ligne (étape 11). |
| `main.go` à la racine du module Go et paquets dans `internal/` | Disposition officielle « commande avec paquets annexes » ([go.dev/doc/modules/layout](https://go.dev/doc/modules/layout)). Le compilateur interdit d'importer `internal/` depuis l'extérieur du module. |
| Deux paquets : `store` et `httpapi` | `store` parle SQL et ignore HTTP ; `httpapi` parle HTTP et ignore SQL. Les erreurs métier (`ErrInsufficientStock`, etc.) font le lien. Le nom `http` est évité car il entrerait en conflit avec `net/http`. |
| Pas de couche « service » ni « repository » | La règle métier est une transaction SQL. Une couche qui ne ferait que déléguer ajouterait des fichiers sans rien protéger. Elle sera extraite si la logique grandit. |
| SQL écrit à la main avec pgx, sans ORM | La garantie de stock se lit directement dans le SQL. pgx est le driver PostgreSQL de référence en Go. |
| `CairnApiClient` | Les pages Blazor n'émettent jamais de requête HTTP directement : tous les appels passent par cette classe. |
| `PickerState` en service « scoped » | En Interactive Server, une instance par onglet connecté. Il garde le profil choisi (identifiant et nom). Le choix est perdu au rechargement de la page, ce qui est acceptable sans authentification : l'écran « Qui prépare ? » le redemande. |
| Modèle Blazor `--empty`, CSS sans librairie | Pas de Bootstrap ni de librairie de composants : une feuille de style courte, aux couleurs du logo (`#1B2A41`, `#F4EFE6`, `#D9A066`). |
| Render (offre gratuite) et Neon pour la version en ligne | Gratuits, sans carte bancaire ; HTTPS automatique ; Render accepte les images Docker et les WebSockets de Blazor Server ; la base Neon n'expire pas, contrairement à la base gratuite de Render. Limites et parades à l'étape 11. |

---

## 3. Base de données

### 3.1 Schéma

```sql
CREATE TABLE products (
    id         bigint  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sku        text    NOT NULL UNIQUE,
    name       text    NOT NULL,
    location   text    NOT NULL,              -- ex. 'A-03-2' : allée, rack, niveau
    stock_qty  integer NOT NULL CHECK (stock_qty >= 0)
);

CREATE TABLE orders (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reference   text        NOT NULL UNIQUE,
    customer    text        NOT NULL,
    status      text        NOT NULL DEFAULT 'a_preparer'
                CHECK (status IN ('a_preparer', 'en_cours', 'prete')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE order_lines (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    bigint      NOT NULL REFERENCES orders (id),
    product_id  bigint      NOT NULL REFERENCES products (id),
    quantity    integer     NOT NULL CHECK (quantity > 0),
    picked_at   timestamptz,                  -- NULL = pas encore préparée
    picked_by   text,
    CHECK ((picked_at IS NULL) = (picked_by IS NULL))
);

CREATE INDEX order_lines_order_id_idx ON order_lines (order_id);

-- Étape 7 bis : profils de préparateurs.
CREATE TABLE pickers (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Unicité sans tenir compte des majuscules : « Mathias » et « MATHIAS »
-- ne peuvent pas coexister. normalize(name, NFC) ramène d'abord les deux
-- façons d'écrire une lettre accentuée à la même forme.
CREATE UNIQUE INDEX pickers_name_key ON pickers (lower(normalize(name, NFC)));
```

### 3.2 Choix de modélisation

| Choix | Explication |
|---|---|
| `GENERATED ALWAYS AS IDENTITY` | Forme standard et moderne de l'identifiant auto-incrémenté. |
| `text` plutôt que `varchar(n)` | Aucune différence de performance en PostgreSQL. |
| `status` en `text` avec `CHECK` plutôt qu'un type `ENUM` | Plus simple à faire évoluer : ajouter un statut revient à modifier la contrainte. |
| `CHECK (quantity > 0)` | Une quantité négative ferait remonter le stock à la validation. |
| `picked_at` et contrainte de cohérence | L'état « préparée » se déduit de `picked_at`, sans booléen redondant. La contrainte interdit une date sans préparateur, et inversement. |
| Index sur `order_lines(order_id)` | PostgreSQL n'indexe pas automatiquement les clés étrangères. |
| `timestamptz` | Date avec fuseau horaire, recommandée par PostgreSQL. |
| Index unique sur `lower(normalize(pickers.name, NFC))` | L'unicité sans tenir compte des majuscules est garantie par la base, même pour deux créations simultanées. `normalize(..., NFC)` (fonction `IMMUTABLE` de PostgreSQL, utilisable dans un index) ramène « é » écrit en un caractère et « e » suivi de l'accent combinant à la même forme : sinon, deux « Émile » identiques à l'écran pourraient coexister. Avec la configuration de l'image Docker (`en_US.utf8`), `lower()` traite aussi les lettres accentuées : `lower('ÉMILE')` donne `émile` (vérifié). À revérifier sur Neon (étape 11). |
| `picked_by` reste un texte, sans clé étrangère vers `pickers` | Le nom du profil est recopié au moment de la validation : l'historique d'une ligne ne dépend pas du profil, même s'il est un jour renommé ou supprimé. La validation (étape 7) n'exige pas un profil existant : sans authentification, cette vérification ne protégerait rien. |
| `CHECK` sur la longueur du nom | Filet de sécurité, comme pour le stock : l'API valide le nom (étape 7), la base refuse quand même un nom vide ou trop long. |

### 3.3 Garantie : le stock ne devient jamais négatif

Trois protections se complètent.

**1. La contrainte `CHECK (stock_qty >= 0)`.** Filet de sécurité absolu : ni un bug applicatif ni une requête manuelle ne peuvent l'enfreindre. Sans autre protection, la seconde de deux validations concurrentes échoue avec l'erreur PostgreSQL `23514` (violation de contrainte) et le stock reste correct. Cette erreur brute ne permet toutefois pas de renvoyer un message clair, d'où la protection 2.

**2. La mise à jour conditionnelle.** La vérification et la baisse du stock tiennent en une seule instruction.

**3. Le verrou sur la commande, posé en premier.** Il rend fiable le recalcul du statut.

Transaction de validation d'une ligne :

```sql
BEGIN;

-- 1. Verrouiller la commande et vérifier qu'elle existe
SELECT status FROM orders WHERE id = $1 FOR UPDATE;
--    aucune ligne → 404 commande_introuvable

-- 2. Marquer la ligne, seulement si elle n'est pas déjà préparée
UPDATE order_lines SET picked_at = now(), picked_by = $3
WHERE id = $2 AND order_id = $1 AND picked_at IS NULL
RETURNING product_id, quantity;
--    aucune ligne → une lecture distingue « ligne inexistante » (404)
--    de « ligne déjà préparée » (409 deja_preparee)

-- 3. Baisser le stock, seulement s'il suffit
UPDATE products SET stock_qty = stock_qty - $2
WHERE id = $1 AND stock_qty >= $2;
--    0 ligne modifiée → ROLLBACK, 409 stock_insuffisant
--    (le ROLLBACK annule aussi l'étape 2)

-- 4. Recalculer le statut de la commande
UPDATE orders SET status = CASE
    WHEN EXISTS (SELECT 1 FROM order_lines
                 WHERE order_id = $1 AND picked_at IS NULL)
    THEN 'en_cours' ELSE 'prete' END
WHERE id = $1;

COMMIT;
```

**Pourquoi l'étape 3 résiste à la concurrence.** Une transaction qui modifie une ligne de table la verrouille jusqu'à son `COMMIT`. Une seconde transaction qui veut modifier la même ligne attend ; à sa reprise, PostgreSQL relit la version à jour de la ligne et réévalue la clause `WHERE`. C'est le comportement documenté du niveau d'isolation par défaut, `READ COMMITTED`. Exemple avec un stock de 5 et deux validations de 4 :

| Temps | Validation A | Validation B |
|---|---|---|
| t1 | `UPDATE ... WHERE stock_qty >= 4` : 5 ≥ 4, stock = 1, ligne verrouillée | |
| t2 | | même `UPDATE` : attend le verrou |
| t3 | `COMMIT` | |
| t4 | | réévaluation sur la version à jour : 1 ≥ 4 est faux, 0 ligne modifiée, refus |

Vérifié sur 50 exécutions : toujours un succès, un refus, et un stock final de 1.

**Pourquoi le verrou sur la commande est nécessaire.** Chaque instruction SQL lit une image de la base prise à son démarrage. Le verrou fait attendre une instruction, mais l'image lue par sa sous-requête est déjà fixée. Sans l'étape 1 :

| Temps | A valide la ligne 1 | B valide la ligne 2 |
|---|---|---|
| t1 | recalcul du statut : la ligne 2 n'est pas préparée, statut `en_cours` | |
| t2 | | recalcul du statut : image prise avant le `COMMIT` de A ; attente du verrou sur la commande |
| t3 | `COMMIT` | |
| t4 | | recalcul avec l'ancienne image : la ligne 1 semble non préparée, statut `en_cours` |
| Résultat | toutes les lignes sont préparées, la commande reste `en_cours` | |

Avec l'étape 1, B attend avant de commencer : toutes ses instructions voient le travail de A, et la commande passe à `prete`.

**Absence d'interblocage.** Toutes les transactions acquièrent leurs verrous dans le même ordre (commande, ligne, produit). Deux transactions ne peuvent donc pas s'attendre mutuellement en boucle.

**Alternatives écartées :**

| Alternative | Raison de l'écart |
|---|---|
| Verrou applicatif en Go (`sync.Mutex`) | Ne protège qu'une instance de l'API, pas la base. |
| `SELECT ... FOR UPDATE` sur le produit puis contrôle en Go | Fonctionne, mais demande deux requêtes au lieu d'une. |
| Niveau d'isolation `SERIALIZABLE` | Fonctionne, mais impose de rejouer les transactions refusées (erreur `40001`). |
| Statut calculé à la volée plutôt que stocké | Supprime le problème, mais une commande réelle a des statuts qui ne se déduisent pas des lignes (expédiée, annulée). |

### 3.4 Données de démonstration

| Commande | Statut | Rôle |
|---|---|---|
| CMD-0001 | à préparer | parcours nominal, 3 ou 4 lignes, stocks suffisants |
| CMD-0002 et CMD-0003 | à préparer | chacune demande 4 unités d'un produit en stock 5 : démonstration de concurrence |
| CMD-0004 | à préparer | une ligne au stock insuffisant : refus reproductible |
| CMD-0005 | en cours | 1 ligne sur 3 déjà préparée, par Romain |
| CMD-0006 | prête | toutes les lignes préparées, par Virginie |

Une dizaine de produits, avec des emplacements numérotés sur deux chiffres (`A-03-2`). Le tri alphabétique des emplacements suit alors l'ordre physique de l'allée : les lignes d'une commande sont présentées dans l'ordre du chemin de prélèvement.

Trois profils de préparateurs (étape 7 bis), insérés dans cet ordre : « Mathias », « Virginie », « Romain ». L'ordre d'insertion diffère de l'ordre alphabétique (Mathias, Romain, Virginie) : un tri oublié se verrait.

**Remise à zéro de la démonstration** (locale), en une seule transaction, qui remet exactement ces données, les trois profils compris :

```powershell
docker compose exec -T db psql -U cairn -d cairn -v ON_ERROR_STOP=1 -1 -c "TRUNCATE orders, order_lines, products, pickers RESTART IDENTITY" -f /docker-entrypoint-initdb.d/002_seed.sql
```

`orders` vient en premier : `TRUNCATE` verrouille les tables dans l'ordre de sa liste, et `PickLine` verrouille la commande avant ses lignes. Dans l'autre ordre, une remise à zéro pendant une validation pourrait s'interbloquer avec elle (relevé par la relecture de l'étape 7 bis).

### 3.5 Évolutions optionnelles

Chaque évolution ajoute un script `003_...sql`, justifié dans le README. Comme les scripts d'initialisation ne s'exécutent que sur un volume vide, leur application demande `docker compose down -v`. Si le script modifie le schéma, il doit aussi être monté dans le service `db-test` de `compose.yaml`, qui ne reçoit que les scripts de schéma.

```sql
CREATE TABLE stock_movements (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    product_id     bigint      NOT NULL REFERENCES products (id),
    delta          integer     NOT NULL CHECK (delta <> 0),
    reason         text        NOT NULL CHECK (reason IN ('preparation', 'annulation')),
    order_line_id  bigint      REFERENCES order_lines (id),
    created_by     text        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
```

L'historique s'alimente par un `INSERT` dans la transaction de validation. L'alerte de stock bas demande une colonne `low_stock_threshold` sur `products`.

---

## 4. API

### 4.1 Routes

| Méthode | Route | Succès | Erreurs |
|---|---|---|---|
| `GET` | `/health` | 200 `{"status":"ok"}` | 503 si la base ne répond pas |
| `GET` | `/api/orders?status=a_preparer` | 200 : liste, la plus ancienne en premier ; filtre optionnel | 400 si statut inconnu |
| `GET` | `/api/orders/{id}` | 200 : commande et lignes triées par emplacement | 400 si l'identifiant n'est pas un nombre, 404 |
| `POST` | `/api/orders/{id}/lines/{lineId}/pick` | 200 : commande mise à jour | 400, 404, 409 |
| `GET` | `/api/products` | 200 : produits et stock | |
| `GET` | `/api/pickers` | 200 : profils triés par nom | |
| `POST` | `/api/pickers` | 201 : profil créé | 400, 409 |

### 4.2 Format des réponses

```json
// GET /api/orders/1
{
  "id": 1, "reference": "CMD-0001", "customer": "Boulangerie Martin",
  "status": "en_cours", "createdAt": "2026-11-04T07:30:00Z",
  "lines": [
    { "id": 3, "productId": 2, "sku": "CAR-40", "productName": "Carton 40x30x30",
      "location": "A-01-2", "quantity": 4, "stockQty": 12,
      "pickedAt": null, "pickedBy": null }
  ]
}

// POST /api/orders/1/lines/3/pick, corps : { "picker": "Mathias" }
// Réponse : la commande mise à jour, au même format.

// GET /api/pickers : les profils, triés par nom
[
  { "id": 1, "name": "Mathias", "createdAt": "2026-11-04T07:00:00Z" },
  { "id": 3, "name": "Romain", "createdAt": "2026-11-04T07:00:00Z" },
  { "id": 2, "name": "Virginie", "createdAt": "2026-11-04T07:00:00Z" }
]

// POST /api/pickers, corps : { "name": "Test" }
// Réponse 201 : le profil créé, au même format qu'un élément de la liste.

// Toute erreur :
{ "error": "stock_insuffisant", "message": "Stock insuffisant : 3 disponibles, 4 demandés." }
```

La liste (`GET /api/orders`) inclut `lineCount` et `pickedCount` pour afficher l'avancement (« 2/5 »). Les dates sont en UTC, au format RFC 3339 (`2026-11-04T07:30:00Z`), quel que soit le fuseau du serveur.

### 4.3 Codes d'erreur

| Code HTTP | `error` | Cas |
|---|---|---|
| 400 | `requete_invalide` | identifiant non numérique, corps refusé (type, taille, JSON invalide, champ inconnu), nom de préparateur ou de profil invalide, statut de filtre inconnu |
| 404 | `commande_introuvable` | commande inexistante |
| 404 | `ligne_introuvable` | ligne inexistante, ou n'appartenant pas à la commande |
| 409 | `deja_preparee` | ligne déjà validée |
| 409 | `stock_insuffisant` | stock inférieur à la quantité demandée |
| 409 | `profil_existant` | un profil porte déjà ce nom, sans tenir compte des majuscules ; le message donne le nom existant |
| 409 | `trop_de_profils` | 20 profils existent déjà |
| 500 | `erreur_interne` | message générique ; le détail reste dans les journaux du serveur |
| 503 | `base_indisponible` | `GET /health` : la base de données ne répond pas |

### 4.4 Points d'attention

- **Le POST renvoie la commande à jour** : l'interface remplace son état et voit immédiatement le passage à `prete`, sans second appel.
- **Noms JSON en camelCase** (`stockQty`) : les méthodes JSON de `HttpClient` les associent automatiquement aux propriétés C# `StockQty`.
- **Listes vides** : en Go, une slice non initialisée est encodée `null` et non `[]`. Toutes les listes renvoyées sont initialisées (`[]Line{}`), sinon le client C# reçoit une liste `null`.
- **Méthode HTTP incorrecte** : le routeur de `net/http` répond 405 automatiquement.
- **Erreurs du routeur en texte brut** : une route inconnue (404) ou une méthode non prévue (405) reçoit une réponse `text/plain`, hors du format JSON unique. Le client ne lit donc le JSON d'une erreur que si la réponse est de type `application/json` ; sinon il affiche un message générique. La même règle couvre la page d'attente de Render (étape 11).
- **Paramètres validés** : une chaîne de requête mal formée (`?status=%zz`) ou un paramètre répété donne 400, au lieu d'être ignoré.
- **Chemins non canoniques** : un chemin qui contient `//` ou `..` reçoit du routeur une redirection 307 vers le chemin nettoyé, sans corps JSON. La règle des erreurs du routeur s'applique aussi : JSON lu seulement si la réponse est de type `application/json`.
- **201 sans en-tête `Location`** : la création d'un profil renvoie 201 et le profil créé, sans l'en-tête `Location` qui donne d'habitude l'adresse de la nouvelle ressource. Il n'existe pas d'adresse pour un profil seul (pas de route `GET /api/pickers/{id}`), et le corps décrit déjà le profil.

---

## 5. Plan de réalisation

Chaque étape se termine par :

1. la vérification décrite ;
2. un commit ;
3. la mise à jour de [NOTES.md](../NOTES.md) : les cinq rubriques des technologies concernées, les ressources consultées et les expérimentations.

Les commandes sont données pour PowerShell. La plupart sont identiques sous bash ; les différences sont listées en section 7.1.

### Étape 0 : environnement et initialisation (1 h 30)

- **Installation :** Git, Go 1.27, SDK .NET 10, Docker Desktop.
- **Fichiers créés :** `.gitattributes`, `.editorconfig`, `.gitignore`, `NOTES.md`, `docs/PLAN.md`.
- **Vérification :**
  - le dépôt est hors d'un dossier synchronisé par OneDrive (section 7.2) ;
  - `go version`, `dotnet --version` et `docker compose version` répondent ;
  - `docker run --rm hello-world` affiche « Hello from Docker! ».
- **Apprentissage :** Tour of Go, parties *Basics*, *Methods and interfaces*, *Concurrency* jusqu'à `sync.Mutex`.
- **Notions Go :**
  - `:=` ;
  - valeurs de retour multiples ;
  - erreurs traitées comme des valeurs (`if err != nil`) ;
  - majuscule pour exporter un nom ;
  - structs et méthodes ;
  - interfaces implicites ;
  - `defer`.
- **Documentation :** [installation de Go](https://go.dev/doc/install), [Tour of Go](https://go.dev/tour/), [Effective Go](https://go.dev/doc/effective_go), [.NET 10](https://dotnet.microsoft.com/download/dotnet/10.0), [Tour of C#](https://learn.microsoft.com/dotnet/csharp/tour-of-csharp/), [Docker Desktop pour Windows](https://docs.docker.com/desktop/setup/install/windows-install/)

### Étape 1 : base de données dans Docker (1 h)

- **Fichiers :** `compose.yaml` (service `db`), `db/init/001_schema.sql`, `db/init/002_seed.sql`.
- **Points de vigilance :**
  - avec PostgreSQL 18, le volume se monte sur `/var/lib/postgresql` ; l'ancien chemin `/var/lib/postgresql/data` empêche le conteneur de démarrer ;
  - les scripts d'initialisation ne s'exécutent que sur un volume vide ;
  - le healthcheck utilise `pg_isready -h 127.0.0.1 -U cairn` : pendant l'initialisation, le serveur temporaire n'écoute que sur le socket Unix, donc la base n'est déclarée prête qu'une fois les scripts exécutés ; `-U cairn` évite l'erreur `role "root" does not exist` dans les journaux.
- **Vérification :**
  - `docker compose up -d --wait db` ;
  - `docker compose exec db psql -U cairn -d cairn`, puis `UPDATE products SET stock_qty = -1 WHERE id = 1;` doit renvoyer `violates check constraint "products_stock_qty_check"`.
- **Documentation :** [image postgres](https://hub.docker.com/_/postgres), [ordre de démarrage Compose](https://docs.docker.com/compose/how-tos/startup-order/), [contraintes](https://www.postgresql.org/docs/18/ddl-constraints.html), [colonnes identity](https://www.postgresql.org/docs/18/ddl-identity-columns.html)

### Étape 2 : API Go minimale, `/health` (1 h)

- **Commandes :** `go mod init github.com/knudsem/cairn/api`, `go get github.com/jackc/pgx/v5`.
- **Contenu de `main.go` :**
  - lecture de `DATABASE_URL`, avec la base locale de développement par défaut (`postgres://cairn:cairn@127.0.0.1:5432/cairn?sslmode=disable`) ;
  - lecture de `API_ADDR`, `127.0.0.1:8080` par défaut : en local, l'API n'est pas exposée au réseau et le pare-feu de Windows ne se déclenche pas à chaque `go run` ; dans le conteneur, `API_ADDR` vaut `:8080` ;
  - pool de connexions ;
  - route `GET /health`.
- **Notions Go :**
  - modules, `go.mod` et `go.sum` ;
  - handlers `func(w http.ResponseWriter, r *http.Request)` ;
  - `context.Context` ;
  - pool de connexions.
- **Vérification :**
  - `go run .` dans `api/`, puis http://localhost:8080/health affiche `{"status":"ok"}` ;
  - après `docker compose stop db`, la même URL renvoie 503 ;
  - `docker compose start db` pour relancer la base.
- **Documentation :** [premier programme](https://go.dev/doc/tutorial/getting-started), [routage depuis Go 1.22](https://go.dev/blog/routing-enhancements), [net/http](https://pkg.go.dev/net/http), [context](https://pkg.go.dev/context), [pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool)

### Étape 3 : squelette de bout en bout (2 h), point de contrôle

- **Création du projet :** `dotnet new blazor -o web -n Cairn.Web --interactivity Server --all-interactive --empty --no-https`.
- **Contenu :**
  - page d'accueil qui appelle `/health` et affiche « API connectée » ;
  - Dockerfile de l'API en deux étapes : `golang:1.27` pour compiler, `alpine` pour exécuter ;
  - Dockerfile du web : `sdk:10.0` pour compiler, `aspnet:10.0` pour exécuter (écoute sur le port 8080) ;
  - `compose.yaml` avec les trois services, et `ApiBaseUrl=http://api:8080` pour le web.
  - pré-rendu désactivé (section 2.2).
- **Points de vigilance :**
  - avec .NET 10, le script `blazor.web.js` vient d'un paquet que le SDK n'ajoute que s'il voit des fichiers `.razor` au moment de la restauration. Le Dockerfile restaurant les paquets avec le seul fichier projet, `Cairn.Web.csproj` déclare `<RequiresAspNetWebAssets>true</RequiresAspNetWebAssets>`, et le Dockerfile vérifie la présence du script : sans lui, les pages resteraient blanches ;
  - sans pré-rendu, seul `App.razor` existe au premier chargement : le bandeau d'erreur de Blazor y est placé, et non dans `MainLayout` ;
  - un bouton désactivé ne bloque pas un second appui déjà parti du navigateur : le gestionnaire d'événement ignore un appui quand une opération est en cours.
- **Notions C# :**
  - `Program.cs` ;
  - injection de dépendances (`AddHttpClient<CairnApiClient>`) ;
  - configuration (`appsettings.json` et variables d'environnement) ;
  - `async`/`await` et `Task` ;
  - `enum` ;
  - types nullables.
- **Notions Blazor :**
  - fichiers `.razor` ;
  - `@page`, `@inject`, `@code` ;
  - `OnInitializedAsync` ;
  - modes de rendu.
- **Vérification :**
  - `docker compose up --build`, puis http://localhost:8081 affiche le logo et « API connectée » ;
  - après `docker compose stop api` puis rechargement, la page affiche « API injoignable » au lieu d'une page d'erreur.
- **Documentation :** [structure d'un projet Blazor](https://learn.microsoft.com/aspnet/core/blazor/project-structure), [modes de rendu](https://learn.microsoft.com/aspnet/core/blazor/components/render-modes), [pré-rendu](https://learn.microsoft.com/aspnet/core/blazor/components/prerender), [appeler une API](https://learn.microsoft.com/aspnet/core/blazor/call-web-api), [IHttpClientFactory](https://learn.microsoft.com/aspnet/core/fundamentals/http-requests), [configuration](https://learn.microsoft.com/aspnet/core/fundamentals/configuration/), [Go dans Docker](https://docs.docker.com/guides/golang/), [.NET dans Docker](https://learn.microsoft.com/dotnet/core/docker/build-container), [builds multi-étapes](https://docs.docker.com/build/building/multi-stage/)

### Étape 4 : lectures, liste et détail (1 h 30)

- **Contenu :**
  - dans `store` : `ListOrders`, `GetOrder`, `ListProducts` ;
  - dans `httpapi` : les handlers, `writeJSON` et `writeError` ;
  - reprise de deux corrections issues de la relecture de l'étape 3 : `go 1.27.0` dans `go.mod` (l'image `golang:1.27` refuse de compiler un module qui exige une version plus récente que la sienne) et arrêt propre de l'API sur Ctrl+C ou `docker compose stop` (requêtes en cours terminées, code de sortie 0).
- **Point de vigilance :** une fois déployée, l'API sera joignable publiquement (étape 11) : chaque paramètre est validé, et aucune route d'administration n'est ajoutée.
- **Notions Go :**
  - tags JSON ;
  - `*time.Time` et `*string` pour les valeurs `NULL` ;
  - `rows.Scan`, `rows.Close`, `rows.Err` ;
  - `errors.Is(err, pgx.ErrNoRows)` ;
  - slices vides et `null` ;
  - en-têtes HTTP fixés avant `WriteHeader`.
- **Vérification :**
  - http://localhost:8080/api/orders renvoie les commandes, de la plus ancienne à la plus récente ;
  - `?status=prete` ne renvoie que CMD-0006 ;
  - `?status=inconnu` renvoie 400 ;
  - `/api/orders/1` renvoie les lignes triées par emplacement ;
  - `/api/orders/999` renvoie 404 ;
  - `/api/orders/abc` renvoie 400 ;
  - `?status=%zz` et `?status=prete&status=inconnu` renvoient 400.
- **Documentation :** [encoding/json](https://pkg.go.dev/encoding/json), [pgx](https://pkg.go.dev/github.com/jackc/pgx/v5), [débuter avec pgx](https://github.com/jackc/pgx/wiki/Getting-started-with-pgx), [erreurs en Go](https://go.dev/blog/go1.13-errors)

### Étape 5 : règle métier `PickLine` (1 h 30)

- **Contenu :**
  - la transaction de la section 3.3 ;
  - en cas de stock insuffisant, lecture du stock disponible dans la même transaction, pour le message de la section 4.2 (« 3 disponibles, 4 demandés ») : une erreur de type `InsufficientStockError{Available, Requested}`, reconnue par `errors.Is(err, ErrInsufficientStock)` grâce à sa méthode `Is`, et dont le handler lit les quantités avec `errors.As`.
- **Notions Go :**
  - `pool.Begin` ;
  - `defer tx.Rollback(ctx)`, sans effet après un `Commit` ;
  - `RETURNING` avec `QueryRow` ;
  - `RowsAffected()` ;
  - erreurs déclarées une fois (`var ErrInsufficientStock = errors.New(...)`) ;
  - type d'erreur avec des champs, `errors.As` ;
  - emballage d'erreur avec `%w`.
- **Vérification :** par les tests de l'étape 6.
- **Documentation :** [transactions en Go](https://go.dev/doc/database/execute-transactions), [isolation PostgreSQL](https://www.postgresql.org/docs/18/transaction-iso.html) (§13.2.1), [verrous](https://www.postgresql.org/docs/18/explicit-locking.html), [clause FOR UPDATE](https://www.postgresql.org/docs/18/sql-select.html)

### Étape 6 : tests Go (2 h)

**Base de test :** service `db-test` dans `compose.yaml`, avec `profiles: ["test"]` et ses données en mémoire (`tmpfs`). Lancement : `docker compose up -d --wait db-test`. Elle ne reçoit que le schéma (`001_schema.sql`), et s'appelle `cairn_test`. Chaque test vide les tables, fait repartir les identifiants de plages distinctes (produits 1, commandes 101, lignes 201 : un code qui confond deux identifiants ne passe pas par coïncidence), puis insère ses propres données. À la fin de chaque test, aucune transaction ne doit rester ouverte (Commit ou Rollback oublié). Garde-fou : les tests refusent de tourner sur une base dont le nom ne se termine pas par `_test`, ce qui protège les données de démonstration. La variable `TEST_DATABASE_URL` permet de viser une autre base de test.

**Cas testés :**

1. La validation baisse le stock de la quantité exacte et enregistre le préparateur.
2. Avec un stock insuffisant, la validation est refusée, et ni le stock ni la ligne ne changent.
3. Une seconde validation de la même ligne est refusée, et le stock n'a baissé qu'une fois.
4. 10 validations simultanées sur un produit dont le stock n'en couvre que 3 donnent exactement 3 succès, 7 refus, et un stock final exact.
5. La première ligne validée fait passer la commande à `en_cours`, la dernière à `prete`.
6. Les deux dernières lignes d'une commande validées en parallèle, répété 50 fois, donnent toujours `prete`. Les deux lignes portent sur deux produits différents : avec un seul produit, le verrou sur la ligne de `products` (étape 3 de la transaction) ferait passer les deux validations l'une après l'autre, et le test réussirait même sans le verrou sur la commande.
7. La même ligne validée deux fois en parallèle donne un succès et un refus.

S'y ajoutent :

- commande et ligne introuvables, ligne d'une autre commande (test en table) ;
- validation interrompue juste après la baisse du stock : rien ne doit être enregistré (un traceur pgx fait échouer l'étape 4 de la transaction) ;
- texte et reconnaissance de `InsufficientStockError`, et `ValidStatus` ;
- les lectures de l'étape 4 : ordre de la liste et avancement, commande sans ligne, filtre, tri par emplacement, listes vides, dates en UTC.

**Contrôle de l'efficacité des tests :**

- retirer temporairement `AND stock_qty >= $2` doit faire échouer le test 4 (erreur `23514`) ;
- retirer le `FOR UPDATE` doit faire échouer le test 6 ;
- retirer l'`ORDER BY` de `ListOrders` doit faire échouer le test de la liste : ses commandes sont créées dans l'ordre inverse de l'ordre attendu ;
- résultat à consigner dans NOTES.md, section Expérimentations.

Résultat obtenu : sans la condition, le test 4 échoue sur la contrainte `products_stock_qty_check` (erreur 23514), qui joue son rôle de filet de sécurité ; sans le `FOR UPDATE`, le test 6 échoue (« statut = en_cours alors que les deux lignes sont préparées »), au premier tour ou seulement au deuxième. Au premier tour, la première utilisation des connexions décale souvent les deux transactions, qui ne se chevauchent plus : d'où les 50 répétitions. Sans l'`ORDER BY`, le test de la liste échoue à chaque lancement.

Une relecture par mutations a introduit 66 erreurs plausibles, une à une, dans une copie du code. 16 passaient les tests, qui ont été renforcés en conséquence. Il n'en reste que 3, sans conséquence ou écartées : `FOR NO KEY UPDATE` au lieu de `FOR UPDATE` (verrou équivalent ici), le départage par `o.id` de deux commandes créées au même instant (l'ordre des ex aequo n'est pas vérifiable de façon fiable), et le niveau d'isolation de `GetOrder` (REPEATABLE READ), dont le test demanderait d'intercaler une validation entre ses deux lectures.

**Notions Go :**

- `t.Fatal`, `t.Errorf`, `t.Helper` ;
- goroutines ;
- `sync.WaitGroup` et `wg.Go` (depuis Go 1.25) ;
- barrière de départ : un channel fermé pour libérer toutes les goroutines en même temps ;
- `t.Cleanup` ;
- interfaces implicites : un type qui a les deux méthodes de `pgx.QueryTracer` en est un, sans le déclarer.

**Commande :** `go test -count=1 ./...`. L'option `-count=1` désactive le cache des résultats, inadapté à des tests qui dépendent d'une base. Par défaut, `go test ./...` teste plusieurs paquets en même temps : depuis l'étape 7, un verrou consultatif PostgreSQL, pris par `testdb.Open`, fait passer l'un après l'autre les paquets qui utilisent la base de test.

**Arrêt complet, base de test comprise :** `docker compose --profile test down`. Un simple `docker compose down` laisse `db-test` tourner.

**Documentation :** [écrire un test](https://go.dev/doc/tutorial/add-a-test), [testing](https://pkg.go.dev/testing), [tests en table](https://go.dev/wiki/TableDrivenTests), [WaitGroup](https://pkg.go.dev/sync#WaitGroup), [race detector](https://go.dev/doc/articles/race_detector), [profils Compose](https://docs.docker.com/compose/how-tos/profiles/), [PgError](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn#PgError), [QueryTracer](https://pkg.go.dev/github.com/jackc/pgx/v5#QueryTracer), [ordre sans ORDER BY](https://www.postgresql.org/docs/18/queries-order.html), [pg_stat_activity](https://www.postgresql.org/docs/18/monitoring-stats.html#MONITORING-PG-STAT-ACTIVITY-VIEW)

### Étape 7 : endpoint de validation (45 min)

- **Contenu :** l'API sera publique une fois déployée : chaque entrée est validée.
  - identifiants du chemin : entiers positifs écrits avec des chiffres seulement et sans zéro en tête (`+101` et `0101` refusés : une ressource n'a qu'une adresse), lus par `strconv.ParseUint`, pour toutes les routes ;
  - corps : en-tête `Content-Type: application/json` exigé (un paramètre comme `; charset=utf-8` est accepté), 1 024 octets au maximum (`http.MaxBytesReader`), un seul objet `{"picker": "..."}` sans autre champ (`DisallowUnknownFields`, et rien après l'objet) ;
  - nom du préparateur : espaces de début et de fin retirés, obligatoire, 50 caractères au maximum (des caractères, pas des octets), au moins une lettre ou un chiffre, et seulement des caractères imprimables : `unicode.IsPrint` refuse les caractères de contrôle (NUL compris), les espaces spéciaux et les caractères de format invisibles, et `unicode.Other_Default_Ignorable_Code_Point` les quelques lettres qui s'affichent vides (U+3164). PostgreSQL refuse le caractère NUL : sans ce contrôle, il donnerait une erreur 500 au lieu d'un 400. L'UTF-8 invalide, que le décodeur JSON remplace par U+FFFD, a son propre message ;
  - traduction des erreurs métier en codes HTTP par un `switch` sur `errors.Is` et `errors.As` (section 4.3), message du stock insuffisant construit par `InsufficientStockError.Quantities` ;
  - en cas de succès, la commande relue par `GetOrder`, au même format que `GET /api/orders/{id}`. Si cette relecture échoue après une validation enregistrée, la réponse est 500 `erreur_interne`, avec un message qui le dit : « La validation est enregistrée, mais la commande n'a pas pu être relue. Rechargez la commande. » ;
  - serveur : `ReadTimeout` (10 s) pour les clients qui envoient leur requête trop lentement, et `IdleTimeout` (2 minutes) pour les connexions inactives : plus long que le délai du client HTTP de .NET (1 minute), pour que ce soit toujours le client qui ferme une connexion inutilisée ;
  - limites connues d'`encoding/json` : la comparaison des noms de champs ignore la casse (`"PICKER"` est accepté), et une clé en double garde sa dernière valeur. Sans conséquence avec un seul champ ; `encoding/json/v2`, plus strict, reste expérimental dans Go 1.27.
- **Tests** (`net/http/httptest`, sur la base de test) : succès et format de la réponse, en-têtes `Content-Type` acceptés, chaque erreur métier, identifiants invalides et identifiant maximal, chaque corps refusé, limite de taille à l'octet près, noms acceptés et refusés (caractère interdit au début, au milieu ou à la fin), panne de base (500 sans détail technique, 503 pour `/health`), relecture impossible après une validation enregistrée, routes de lecture, réponses du routeur (404 et 405 en texte brut). Les en-têtes sont lus avec `rec.Result().Header`, c'est-à-dire ce que le client reçoit vraiment. Le paquet `internal/testdb` rassemble la connexion, le garde-fou (testé lui aussi), la remise à zéro et le traceur qui simule une panne, partagés avec les tests du store.
- **Contrôle de l'efficacité des tests :** chaque mutation de la couche HTTP doit faire échouer un test, et pas seulement la compilation ; sans le verrou entre paquets, `go test ./...` doit échouer. Résultat : 58 mutations, toutes détectées.
- **Notions Go :** `json.Decoder`, `DisallowUnknownFields`, `http.MaxBytesReader`, `mime.ParseMediaType`, `strconv.ParseUint`, `unicode.IsPrint`, `net/http/httptest`.
- **Vérification :** requêtes HTTP avec `curl.exe`. Le corps est écrit dans un fichier temporaire par `[IO.File]::WriteAllText`, qui n'ajoute pas de BOM. Un tube PowerShell (`'...' | curl.exe`), `Set-Content -Encoding utf8` et `Out-File` en ajoutent un, que l'API refuse, comme le permet la norme JSON (RFC 8259, section 8.1). Et dans Windows PowerShell 5.1, les guillemets d'un JSON passé directement à `curl.exe -d` disparaissent.
  - une première validation renvoie 200 et la commande mise à jour ;
  - la même validation renvoie ensuite 409 `deja_preparee` ;
  - une ligne de CMD-0004 renvoie 409 `stock_insuffisant` ;
  - un nom de préparateur contenant le caractère NUL (U+0000) renvoie 400 `requete_invalide` ;
  - remise à zéro de la base de démonstration, en une seule transaction : la commande de la section 3.4.

### Étape 7 bis : profils de préparateurs (3 h 30)

Ajoutée le 4 novembre. Le préparateur choisit son profil dans une liste, ou crée le sien, sans mot de passe : c'est un choix assumé du scope (section 1). L'étape a deux parties, chacune avec sa vérification et son commit.

#### 7 bis.1 : base de données, API et tests (2 h)

- **Base :**
  - table `pickers` dans `db/init/001_schema.sql` (section 3.1) ; trois profils de départ dans `002_seed.sql` ; les lignes déjà préparées passent à « Romain » (CMD-0005) et à « Virginie » (CMD-0006) ;
  - la table entre dans `001_schema.sql`, et non dans un script `003` (section 3.5), car le projet n'est pas encore livré. Une seule fois, la base locale est donc recréée (`docker compose down -v`, puis `docker compose up -d --build --wait`), ainsi que la base de test (`docker compose up -d --force-recreate --wait db-test`) ;
  - remise à zéro de la démonstration : `pickers` s'ajoute au `TRUNCATE`, et `orders` passe en tête (section 3.4). Côté tests, `testdb.Reset` fait de même, et les identifiants des profils repartent de 301.
- **Store (`store/pickers.go`) :**
  - `ListPickers` : `ORDER BY name COLLATE "fr-x-icu"`. La collation ICU donne l'ordre alphabétique français, sans tenir compte des majuscules, quelle que soit la configuration du serveur. Vérifié sur l'image Docker : « alice, Bob, Émile, Ève, zoé, Zoé », alors que la collation `C` donnerait « Bob, Zoé, alice, zoé, Ève, Émile ». La collation par défaut de l'image (`en_US.utf8`) donne le même ordre sur ces noms ; elle diffère sur les traits d'union et les espaces (« Jeanne » avant « Jean-Yves ») : le test du tri contient ces deux noms, pour que le retrait du `COLLATE` se voie aussi sur la base de test. À revérifier sur Neon (étape 11), qui trie par défaut en `C.UTF-8` ;
  - `CreatePicker` : une transaction qui verrouille d'abord la table (`LOCK TABLE pickers IN SHARE ROW EXCLUSIVE MODE` : les lectures continuent, deux créations passent l'une après l'autre), cherche un nom identique avec la même expression que l'index unique (`ErrPickerExists`, avec le nom existant), compte les profils (`ErrTooManyPickers` à partir de 20), puis insère. Un nom existant l'emporte sur la limite. Sans le verrou, deux créations simultanées à 19 profils pourraient en faire 21 ; l'index unique reste le filet de sécurité des doublons ;
  - dates en UTC, comme à l'étape 4.
- **API (`httpapi/pickers.go`) :**
  - `GET /api/pickers` : 200 et la liste, `[]` si elle est vide ;
  - `POST /api/pickers`, corps `{"name": "..."}` : mêmes règles que le nom du préparateur à l'étape 7 (Content-Type, taille, champ inconnu, espaces retirés, obligatoire, 50 caractères, caractères autorisés, au moins une lettre ou un chiffre). Le code de validation de l'étape 7 devient commun aux deux routes, dans `httpapi/body.go` ; sa réorganisation est couverte par les tests de l'étape 7, dont les messages attendus n'ont pas changé. Règle ajoutée pour les deux routes : les espaces ordinaires répétées à l'intérieur d'un nom sont réduites à une seule, car une page web les affiche comme une seule (« Jean  Pierre » et « Jean Pierre » seraient deux profils identiques à l'écran). Réponse 201 et le profil créé, sans en-tête `Location` (section 4.4) ; 409 `profil_existant` ou `trop_de_profils` (section 4.3).
- **Tests, sur la base de test :**
  - store : tri par nom (majuscules, accents, « Jean-Yves » et « Jeanne »), liste vide `[]`, création, doublon sans tenir compte des majuscules ni de l'écriture de l'accent (« mathias » face à « Mathias », « ÉMILE » et « Émile » écrit avec l'accent combinant face à « Émile »), noms proches acceptés (« Emile », « T_st », « Te% »), limite (le 20e profil passe, le 21e est refusé) ;
  - filets de sécurité de la base, par des `INSERT` directs : un doublon (majuscules ou accent combinant) est refusé par l'index unique, un nom vide ou de 51 caractères par le `CHECK`, et 50 « é » passent ;
  - concurrence, sur 10 tours : avec 18 profils, 5 créations simultanées donnent exactement 2 succès et 3 refus ; le même nom créé 5 fois en même temps, dans des casses différentes, donne 1 succès et 4 `profil_existant` ;
  - HTTP : 200, 201 (le profil a exactement les champs `id`, `name` et `createdAt`), noms acceptés à la limite (50 « é », espaces doublées réduites), 400 (chaque règle du nom, comme à l'étape 7), 409 `profil_existant` et `trop_de_profils`, 500 sans détail technique, 405 pour une autre méthode, format JSON des erreurs ;
  - `testdb` : un `lock_timeout` de 5 s sur les connexions des tests, pour qu'un verrou jamais relâché fasse échouer un test au lieu de le bloquer ;
  - contrôle par mutations, comme aux étapes 6 et 7 : 43 mutations, toutes détectées. 38 touchent le code (dont le retrait du `LOCK TABLE`, du `COLLATE` ou de `normalize`, une recherche par `ILIKE`, un nom de champ JSON en majuscule, l'oubli du `Rollback`) et 5 le schéma, chargé dans une base jetable (index sans `normalize`, sensible à la casse ou retiré, `CHECK` en octets ou retiré).
- **Notions Go et SQL :** `LOCK TABLE` et ses modes de verrou, index sur une expression, normalisation Unicode (NFC), collation ICU, réponse 201 Created.
- **Vérification (`curl.exe`) :** `GET /api/pickers` donne Mathias, Romain, Virginie ; la création de « Test » renvoie 201 ; « mathias » renvoie 409 `profil_existant` ; « Jean   Pierre » est enregistré « Jean Pierre » ; après la remise à zéro, les trois profils d'origine reviennent.

#### 7 bis.2 : écran « Qui prépare ? » (1 h 30)

- **Contenu :**
  - écran « Qui prépare ? » (`/pickers`) : un panneau centré, blanc sur le fond crème (36 rem de large à la souris, 40 rem au toucher). Les profils, dans l'ordre de l'API, en cartes sur une grille : une pastille ronde à l'initiale (bleu nuit et crème), le nom en dessous ; survol à la souris (bordure et pastille ocre), focus clavier visible. La dernière carte, en pointillés avec un « + », est « Ajouter un profil » : elle remplace la grille par le formulaire « Nouveau profil », dans le même panneau, avec le champ « Nom du profil » et les boutons « Créer » et « Annuler ». « Annuler » revient à la liste, relue pour montrer aussi les profils créés entre-temps ;
  - états de la liste : chargement (emplacements des boutons), encadré « Impossible de charger les profils » avec le message et « Réessayer », liste vide ;
  - « Créer » est désactivé pendant l'appel, pour ne pas envoyer deux fois la même création. En cas de succès, le nouveau profil est choisi d'office, avec le nom renvoyé par l'API (nettoyé de ses espaces en trop) et non le texte saisi ; sinon, un message sous le champ, choisi d'après le code d'erreur : « Ce profil existe déjà : choisissez-le dans la liste. », « 20 profils au maximum : choisissez un profil existant. », ou le message de l'API pour un nom refusé ;
  - `PickerState` (service « scoped ») garde le profil choisi pour la session, c'est-à-dire pour le circuit Blazor : après un rechargement de la page, le préparateur choisit de nouveau son profil. Une page ouverte sans profil choisi renvoie vers « Qui prépare ? », puis vers la page demandée. Le renvoi se fait dans `Routes.razor` : sans profil, toute page absente d'une courte liste (« Qui prépare ? », page introuvable, page d'erreur) est remplacée par le renvoi, sans s'afficher ni appeler l'API. L'adresse de retour n'accepte qu'un chemin relatif du site (liste blanche de caractères), pour éviter une redirection ouverte ;
  - barre du haut (`MainLayout`) : le nom du profil et un bouton « Changer », qui oublie le profil et ramène à « Qui prépare ? », puis à la page en cours une fois un profil choisi. Sur téléphone, l'icône seule remplace alors le logo complet, pour laisser la place au nom ;
  - premier écran qui lit l'API, il pose les bases du client que reprennent les étapes 8 et 9 : chemins relatifs dans `CairnApiClient` ; chaque appel renvoie un `ApiResult<T>`, la valeur ou la raison de l'échec (API injoignable ou délai dépassé, refus de l'API avec son code et son message, réponse inattendue), sans exception qui couperait le circuit Blazor ; JSON d'une erreur lu seulement si la réponse est de type `application/json` ; JSON lu strictement (un champ absent ou `null` à tort donne une réponse inattendue) ; état de chargement, message et bouton « Réessayer » si l'API ne répond pas.
- **Notions C# et Blazor :** service « scoped », `NavigationManager`, `[SupplyParameterFromQuery]`, `@bind`, `@ref` et `FocusAsync`, rendu conditionnel, `record`, expression `switch` sur des propriétés, lecture du code HTTP.
- **Vérification :** http://localhost:8081 renvoie vers « Qui prépare ? », avec Mathias, Romain et Virginie ; après la création de « Test », l'accueil s'ouvre (la liste des commandes à partir de l'étape 8) et la barre du haut affiche « Test » et « Changer » ; « mathias » affiche « Ce profil existe déjà » ; « Changer » ramène à « Qui prépare ? » ; API arrêtée, l'encadré d'erreur et « Réessayer » ; après la remise à zéro et un rechargement, les trois profils d'origine. Vérifié aussi dans Edge avec Playwright, en 1440 x 900, 820 x 1180 et 390 x 844.
- **Documentation :** [injection de dépendances dans Blazor](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/dependency-injection), [navigation](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/navigation), [liaison de données](https://learn.microsoft.com/aspnet/core/blazor/components/data-binding), [LOCK](https://www.postgresql.org/docs/18/sql-lock.html), [index sur une expression](https://www.postgresql.org/docs/18/indexes-expressional.html), [collations](https://www.postgresql.org/docs/18/collation.html)

### Étape 8 : écran liste des commandes (1 h 15)

- **Contenu :**
  - une seule page, `Pages/Orders.razor`, pour `/` et `/orders/{id}`, comme dans les maquettes : sur un écran large (1024 px et plus), la liste à gauche et la commande ouverte à droite ; sur un écran plus étroit, un seul des deux volets, avec « ← Commandes » pour revenir. Blazor garde le composant d'une adresse à l'autre : la liste reste chargée et suit l'état de la commande ouverte (étape 9) ;
  - en tête de liste, « Commandes » et « Actualiser », puis deux onglets, « À préparer » (à préparer et en cours) et « Prêtes », avec leur nombre une fois la liste chargée ;
  - une carte par commande : référence, statut (`StatusBadge.razor` : couleur et libellé, jamais la couleur seule), client, avancement (« 1 / 3 lignes préparées »), heure de création, barre d'avancement ; la commande ouverte est marquée ;
  - états : chargement (cartes vides), encadré « Impossible de charger les commandes » avec « Réessayer », onglet vide ;
  - barre du haut avec le profil choisi et le bouton « Changer » (étape 7 bis) ; sans profil choisi, l'écran renvoie vers « Qui prépare ? » ;
  - priorité d'affichage : ordinateur et tablette d'abord ; mobile utilisable, finitions après le déploiement (section 6, priorités) ;
  - bases du client posées à l'étape 7 bis, reprises ici : chemins relatifs sans barre oblique initiale (`api/orders`), pour fonctionner avec n'importe quelle adresse de base (Compose ou Render) ; une API lente est un cas normal (réveil sur Render, étape 11) : état de chargement visible, puis message clair et bouton « Réessayer » si elle ne répond pas ; JSON d'une erreur lu seulement si la réponse est de type `application/json`, et `JsonException` traduite en état « réponse inattendue » plutôt qu'en exception qui couperait le circuit Blazor (section 4.4) ;
  - dans `Models.cs`, les dates en `DateTimeOffset` (jamais `DateTime`), converties pour l'affichage vers le fuseau `Europe/Paris` : le conteneur web tourne en UTC ; les statuts restent des chaînes (`a_preparer`), qu'un convertisseur d'énumération par défaut ne saurait pas lire.
- **Notions C# et Blazor :**
  - `@foreach` ;
  - composants et `[Parameter]` ;
  - CSS isolé (`.razor.css`) ;
  - durée de vie « scoped » ;
  - `record` (données JSON des commandes) ;
  - expressions `switch`.
- **Vérification :** http://localhost:8081, une fois le profil choisi, affiche les commandes de la plus ancienne à la plus récente, avec trois couleurs de statut ; un appui sur une carte ouvre `/orders/{id}`.
- **Documentation :** [composants](https://learn.microsoft.com/aspnet/core/blazor/components/), [routage](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/routing), [injection de dépendances dans Blazor](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/dependency-injection), [CSS isolé](https://learn.microsoft.com/aspnet/core/blazor/components/css-isolation), [records](https://learn.microsoft.com/dotnet/csharp/fundamentals/types/records), [expression switch](https://learn.microsoft.com/dotnet/csharp/language-reference/operators/switch-expression)

### Étape 9 : écran de préparation (2 h)

- **Contenu :**
  - `OrderDetail.razor`, affiché par `Orders.razor` : à droite de la liste sur un écran large, à sa place sur un écran étroit (avec « ← Commandes ») ;
  - en-tête : référence, statut, client, « 1 / 3 lignes préparées », barre d'avancement ;
  - la carte « Prochaine ligne » (la première à préparer, dans l'ordre des emplacements), encadrée d'ocre : emplacement et quantité en très grand (`--font-size-display`), nom du produit, SKU et stock, bouton « Préparé » sur toute la largeur ; puis les autres lignes à préparer, plus compactes ; puis « Déjà préparées (n) », avec « ✓ Préparé par Romain à 09:20 » (heure de Paris) ;
  - bouton « Préparé » à la hauteur des contrôles (`--control-height` : 64 px au toucher, pour un usage avec des gants, 44 px à la souris) ; la validation envoie le nom du profil choisi (`PickerState`, étape 7 bis) ;
  - après une validation : bandeau « ✓ Ligne préparée : B-07-2 », qui reçoit le focus ; la carte de la liste suit (statut, avancement) sans recharger la liste ;
  - commande prête : bandeau « ✓ Commande prête », boutons « Commande suivante » (la plus ancienne commande qui n'est pas prête) et « Retour aux commandes » ;
  - bouton désactivé pendant l'appel (« Validation... »), et garde côté serveur : un appui pendant un appel, ou dans les 700 ms qui suivent une validation ou l'ouverture d'une commande, est ignoré. Avec des gants, le second appui d'un double appui tomberait sinon sur le bouton de la ligne suivante, qui vient de prendre la place du premier ;
  - après toute réponse autre que 200 à la validation (409, 500, délai dépassé, coupure), rechargement de la commande, qui décide du message : ligne préparée par ce profil (la validation a été enregistrée malgré l'erreur) ; « Cette ligne a déjà été préparée par Mathias. » ; sinon, l'erreur sous la ligne, avec « Réessayer » à la place de « Préparé » : « Stock insuffisant : 2 disponibles, 5 demandés. La ligne n'est pas validée et le stock n'a pas changé. », ou « La validation n'a peut-être pas été enregistrée » si la commande n'a pas pu être relue ;
  - une réponse arrivée après l'ouverture d'une autre commande est ignorée ;
  - nom du préparateur dans un élément `<bdi>`, pour qu'un nom écrit de droite à gauche ne dérange pas la phrase ; un nom long passe à la ligne.
- **Notions C# et Blazor :**
  - paramètres de route (`{Id:long}`) ;
  - `@onclick` asynchrone ;
  - rendu conditionnel ;
  - lecture du code HTTP d'une réponse ;
  - `HttpRequestException` ;
  - `EventCallback` : un composant enfant prévient son parent ;
  - `with` : copie d'un record avec quelques champs changés.
- **Vérification :**
  - sur http://localhost:8081/orders/1, chaque validation change l'état de la carte, puis le bandeau « Commande prête » apparaît ;
  - sur CMD-0004, le message de stock insuffisant s'affiche ;
  - deux fenêtres ouvertes sur CMD-0002, avec deux profils : la seconde validation est refusée, avec « Cette ligne a déjà été préparée par Mathias. ». La preuve formelle de concurrence reste le test 4 ;
  - vérifié aussi avec Playwright, aux trois formats : double appui ignoré, API suspendue pendant une validation puis « Réessayer », commande inexistante.
- **Documentation :** [événements](https://learn.microsoft.com/aspnet/core/blazor/components/event-handling), [cycle de vie](https://learn.microsoft.com/aspnet/core/blazor/components/lifecycle), [programmation asynchrone](https://learn.microsoft.com/dotnet/csharp/asynchronous-programming/), [types nullables](https://learn.microsoft.com/dotnet/csharp/nullable-references)

### Étape 10 : vérifications de bout en bout (1 h)

- Dans un clone neuf du dépôt, `docker compose up --build` doit suffire.
- Parcours complet : choix ou création d'un profil, « Changer », commande jusqu'à « prête » ; puis remise à zéro de la démonstration (section 3.4) et retour des trois profils d'origine.
- Affichage tablette dans les outils de développement du navigateur.
- API coupée puis navigateur hors ligne : la fenêtre de reconnexion apparaît.
- **Résultats (4 novembre, 18 h 30, refaits à 18 h 50 après la relecture) :** clone neuf, avec tous les changements non commités appliqués (étape 11 comprise) : `docker compose up --build` suffit, profils d'origine présents. Parcours complet réussi dans Edge (création de « Test », « Changer », CMD-0001 jusqu'à « prête » par Romain). Conteneur web arrêté : la fenêtre de reconnexion apparaît ; relancé : la page se recharge seule sur « Qui prépare ? », puis revient à la commande. L'émulation « hors ligne » de Playwright ne coupe pas un WebSocket déjà ouvert : la coupure se teste donc en arrêtant le conteneur. Corrections : pendant les nouveaux essais, la fenêtre affichait deux messages à la fois (composant du modèle .NET) ; « Nouvel essai dans 1 secondes » devient « dans 1 s. ». Le script vérifie qu'un seul message est visible. Toutes les vérifications des étapes 7 bis.2, 8 et 9 et les tests Go repassent.
- **Documentation :** [SignalR et Blazor](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/signalr)

### Relecture finale du projet complet (2 h 45)

Ajoutée le 4 novembre, entre l'étape 10 et le déploiement (section 6, point d'étape de 16 h 30) :

1. relecture complète par Claude : plusieurs relecteurs, chacun sur un angle (règles métier et concurrence, API, interface, tests, Docker et documentation), puis contre-vérification de chaque constat ;
2. relecture par un autre modèle, qui rend un rapport sans modifier le code ;
3. corrections ensemble, par ordre de gravité, chacune vérifiée par les tests.

### Étape 11 : déploiement en ligne (1 h)

**Objectif :** un lien public en HTTPS, que la dirigeante et le développeur ouvrent dans leur navigateur sans rien installer. **Priorité :** inférieure aux tests, au README et à la préparation de la présentation ; si l'étape déborde, elle passe en soirée, après la répétition.

**Cible, vérifiée le 3 novembre 2026 sur les documentations officielles :**

- **Render, offre gratuite**, pour `api` et `web` : deux services web Docker construits depuis le dépôt GitHub, région Frankfurt, 0,1 CPU et 512 Mo de mémoire chacun. Render accepte les WebSockets, sans durée maximale : Blazor Server fonctionne, sans affinité de session puisqu'il n'y a qu'une instance. HTTPS automatique (Render termine le TLS et transmet du HTTP au conteneur).
- **Neon, offre gratuite**, pour la base : PostgreSQL 18 (même version majeure qu'en local), région AWS Francfort, sans carte bancaire et sans expiration.
- **Base gratuite de Render écartée :** elle expire 30 jours après sa création (puis 14 jours de grâce, puis suppression), sans sauvegarde, une seule par compte.

**Contraintes de l'offre gratuite qui orientent les choix :**

- un service s'endort après 15 minutes sans trafic entrant (requêtes HTTP ou messages WebSocket) et met environ une minute à se réveiller ; le navigateur voit pendant ce temps une page d'attente de Render. Ce que reçoit un client qui n'est pas un navigateur (l'interface qui appelle l'API) n'est pas documenté : probablement un 503, à mesurer ;
- 750 heures gratuites par mois pour tout le compte, partagées entre les deux services ; épuisées, elles suspendent tout jusqu'au mois suivant. Deux services éveillés en permanence les épuisent vers le 16e jour ;
- un service gratuit ne reçoit pas de trafic du réseau privé : l'interface appelle l'API par son URL publique HTTPS, et non par `http://api:8080` comme dans Compose. L'API est donc joignable publiquement : n'importe qui peut valider des lignes ou créer des profils, jusqu'à 20 (étape 7 bis). C'est la conséquence assumée de l'absence d'authentification ; la remise à zéro ramène les données et les trois profils d'origine ;
- pas de commande « pre-deploy », de shell ni de tâche ponctuelle : le schéma et les données s'appliquent depuis le poste, avec psql ;
- Neon endort la base après 5 minutes sans requête (réveil en quelques centaines de millisecondes, connexions coupées puis recréées par pgxpool) et accorde 100 heures de calcul (CU-heures) par mois, soit environ 400 heures avec la plus petite taille ;
- sans `plan: free` explicite, Render crée un service payant ; sans `region: frankfurt`, il choisit l'Oregon, et la région ne change plus ensuite.

**Fichiers :**

- `render.yaml` (Blueprint), à la racine : deux services `type: web`, `runtime: docker`, `plan: free`, `region: frankfurt`, avec `dockerfilePath` et `dockerContext` (`./api` et `./web`), sans `rootDir` (la documentation se contredit sur son effet), et un `buildFilter` par dossier pour ne reconstruire que le service modifié. Pas de `healthCheckPath` : la sonde TCP par défaut suffit, alors qu'un contrôle sur `/health` interrogerait Neon toutes les quelques secondes. Variables :
  - `cairn-knudsem-api` : `PORT=8080` (Render attend 10000 par défaut) ; `DATABASE_URL` avec `sync: false`.
  - `cairn-knudsem-web` : `PORT=8080` ; `ApiBaseUrl`, l'URL publique de l'API, avec `sync: false` : saisie à la création du Blueprint (`https://cairn-knudsem-api.onrender.com/`), puis corrigée dans le tableau de bord si Render a donné une autre adresse au service. `fromService` ne fournit que l'adresse privée, inutilisable par un service gratuit ; `RENDER_EXTERNAL_URL` n'existe qu'à l'exécution, dans le service lui-même. `ApiTimeoutSeconds=90`.
- `db/reset.sql` : supprime les quatre tables (profils compris), `orders` en tête pour la même raison que le `TRUNCATE` de la section 3.4, puis rejoue `init/001_schema.sql` et `init/002_seed.sql` (commande `\ir` de psql), le tout en une seule transaction (`psql -1`). Il sert à initialiser la base Neon et à remettre les données de démonstration à zéro, avec des dates de création recalculées et les trois profils d'origine.
- `web/Program.cs` : délai d'appel à l'API configurable (`ApiTimeoutSeconds`, 5 s par défaut, 90 s sur Render, plus que le réveil de l'API) ; appel à `/health` au démarrage de l'interface, pour réveiller l'API en même temps qu'elle. Dans `Models.cs`, un 503 sans le JSON de l'API (page d'attente de l'hébergeur) donne « Le serveur démarre. Réessayez dans une minute. ».

**Noms des services :** `cairn-knudsem-api` et `cairn-knudsem-web`. Le nom donne l'adresse (`https://<nom>.onrender.com`), et `cairn-api` est déjà pris par le service d'un tiers (relevé par la relecture le 4 novembre) : avec cette adresse, l'interface aurait envoyé ses requêtes, noms des préparateurs compris, à un inconnu. Avant de créer le Blueprint, `curl.exe -sI https://cairn-knudsem-api.onrender.com/` doit répondre `x-render-routing: no-server` (nom libre).

**Heures gratuites :** un onglet resté ouvert garde l'interface éveillée (Blazor envoie un message toutes les 15 secondes, et Render compte les messages WebSocket comme du trafic). Mot aux testeurs : fermer l'onglet après usage ; surveiller le compteur d'heures dans le tableau de bord.

**Secrets :** l'URL de connexion Neon contient le mot de passe. Elle n'est jamais écrite dans le dépôt : avec `sync: false`, le tableau de bord de Render demande sa valeur à la création du Blueprint, et le script de remise à zéro la lit dans une variable d'environnement de la session PowerShell. On prend la chaîne directe de Neon (bouton « Connection pooling » désactivé : pgxpool regroupe déjà les connexions), avec `sslmode=require&channel_binding=require`. Neon refuse les connexions sans TLS. pgx doit être en version 5.12.0 au moins (c'est le cas) : il gère `channel_binding` depuis la 5.9, et la 5.12.0 corrige un contournement de `channel_binding=require`.

**Remise à zéro des données de démonstration**, sans installer psql (celui de l'image `postgres:18` suffit) :

```powershell
$env:DEMO_DATABASE_URL = "<chaîne directe Neon>"
docker run --rm -v "${PWD}/db:/db:ro" postgres:18 psql $env:DEMO_DATABASE_URL -v ON_ERROR_STOP=1 -1 -f /db/reset.sql
```

Pas de route d'administration dans l'API pour cela : elle serait publique.

**Limiter l'effet de la mise en veille :**

- le délai de 90 s et le réveil de l'API au démarrage de l'interface évitent « API injoignable » pendant un réveil, et ramènent le premier chargement à une minute environ au lieu de deux ;
- pendant la période d'évaluation seulement : un ping gratuit planifié avec cron-job.org sur `https://<web>/` et `https://<api>/health`, toutes les 10 minutes, en semaine de 8 h à 20 h (Europe/Paris). Cela fait environ 2 × 12 h × 22 jours = 528 heures, sous les 750 heures du mois, et Neon reste loin de ses 100 CU-heures. Render ne publie pas clairement sa position sur ces pings : on les limite à la période d'évaluation et on les arrête ensuite ;
- un mot aux testeurs avec le lien : « le premier chargement peut prendre une minute ; une page d'attente s'affiche, c'est normal ».

**Vérification sur le lien public :**

- depuis un téléphone en 4G, donc hors du réseau local : `https://<web>.onrender.com` affiche le cadenas HTTPS, le logo et la liste des commandes ;
- parcours complet : choix d'un profil, CMD-0001 jusqu'à « prête », refus de stock insuffisant sur CMD-0004 ;
- profils : vérifiés sur Neon, dont la configuration peut différer de l'image Docker (collation par défaut `C.UTF-8`) : la collation `fr-x-icu` existe (sinon `GET /api/pickers` renverrait 500), le tri place « Émile » avant « Ève » et « Jean-Yves » avant « Jeanne », `lower()` traite une lettre accentuée et l'index avec `normalize` refuse un doublon. Si le temps le permet, les tests Go sont lancés sur une base `cairn_test` créée sur Neon ;
- après plus de 15 minutes sans visite : page d'attente de Render, puis l'application, sans « API injoignable » ; durée du réveil notée ;
- remise à zéro avec `db/reset.sql`, puis rechargement : données de démonstration d'origine ;
- journaux des deux services sans erreur dans le tableau de bord Render ; aucun secret dans le dépôt (`git grep -E "@ep-[a-z0-9]"` ne trouve rien (le nom d'hôte d'une chaîne Neon commence par `ep-`)).

**Documentation :** [offre gratuite de Render](https://render.com/docs/free), [spécification Blueprint](https://render.com/docs/blueprint-spec), [Docker sur Render](https://render.com/docs/docker), [WebSockets sur Render](https://render.com/docs/websocket), [port des services web](https://render.com/docs/web-services), [health checks](https://render.com/docs/health-checks), [offres Neon](https://neon.com/docs/introduction/plans), [cycle de vie d'une compute Neon](https://neon.com/docs/introduction/compute-lifecycle), [choisir sa connexion Neon](https://neon.com/docs/connect/choose-connection), [connexion sécurisée à Neon](https://neon.com/docs/connect/connect-securely), [changelog de pgx](https://github.com/jackc/pgx/blob/master/CHANGELOG.md), [FAQ de cron-job.org](https://cron-job.org/en/faq/)

### Étape 12 : README, capture d'écran, vidéo de démonstration (1 h 30)

Contenu défini dans SCOPE.md, section 9, avec le lien public de l'étape 11 s'il existe.

### Étape 13 : évolutions optionnelles

Seulement s'il reste du temps. Dans l'ordre :

1. historique des mouvements de stock, avec un test ;
2. alerte de stock bas ;
3. annulation d'une validation, écartée par défaut car c'est la plus risquée : elle ferait remonter le stock, ce qu'aucune opération ne fait aujourd'hui. Il faudrait alors traiter le cas où le stock relu après un refus suffit (`InsufficientStockError`), avec un test.

Pour aller plus loin : [parcours Blazor de Microsoft Learn](https://learn.microsoft.com/training/paths/build-web-apps-with-blazor/), [persistance de l'état pré-rendu avec `[PersistentState]`](https://learn.microsoft.com/aspnet/core/blazor/state-management/prerendered-state-persistence).

---

## 6. Planning

L'étape 0 peut être réalisée la veille, ce qui libère 1 h 30 le jour 1.

### Jour 1 : mercredi

| Horaire | Contenu |
|---|---|
| 9h00 à 10h30 | Étape 0, si elle n'a pas été faite la veille |
| 10h30 à 11h30 | Étape 1 : base de données |
| 11h30 à 12h30 | Étape 2 : API minimale |
| 12h30 à 13h30 | Pause |
| 13h30 à 15h30 | Étape 3 : squelette de bout en bout. **Point de contrôle à 15h30** |
| 15h30 à 17h00 | Étape 4 : lectures |
| 17h00 à 18h30 | Étape 5 : règle métier |
| 18h30 à 19h30 | Pause |
| 19h30 à 21h30 | Étape 6 : tests |
| 21h30 à 22h15 | Étape 7 : endpoint de validation (peut glisser au jour 2 à 8h00) |

**Objectif de fin de journée :** API complète et tests au vert.

### Jour 2 : jeudi

| Horaire | Contenu |
|---|---|
| 8h30 à 10h00 | Étape 8 : écran liste |
| 10h00 à 12h00 | Étape 9 : écran de préparation |
| 12h00 à 13h00 | Pause |
| 13h00 à 14h00 | Étape 10 : vérifications de bout en bout |
| 14h00 à 15h00 | Étape 11 : déploiement en ligne (reporté en soirée s'il déborde) |
| 15h00 à 16h30 | Étape 12 : README, capture, vidéo |
| 16h30 à 18h30 | **Préparation de la présentation (2 h)** |
| 18h30 à 19h00 | **Répétition (30 min)** |
| Soirée | Relecture finale, déploiement s'il a été reporté, livraison |

Le déploiement prend la place du créneau tampon : les évolutions optionnelles (étape 13) ne se font qu'avec de l'avance.

La présentation comporte quatre parties :

1. les apprentissages sur Go, C# / .NET et Blazor ;
2. la démonstration de Cairn : architecture, choix techniques, tests ;
3. la méthode de reprise d'une application existante ;
4. la veille technologique.

NOTES.md en fournit la matière.

### Point d'étape du mercredi 4 novembre, 13 h 30

- **Fait :** étapes 0 à 7, terminées mercredi à 13 h 10 au lieu de 22 h 15. L'avance correspond au reste de la journée de mercredi : environ 7 h 45 de créneaux de travail.
- **Constat :** avec les relectures indépendantes et les contrôles par mutations, une étape prend deux à quatre fois son estimation (étape 7 : 45 min prévues, environ 3 h 20 réelles). C'est le prix de la qualité, mais il faut le compter.
- **Ajout :** étape 7 bis, profils de préparateurs : 3 h 30 estimées, environ 5 h réalistes. Une partie de l'étape 8 (les bases du client) passe dans l'étape 7 bis, et l'étape 8 passe de 1 h 30 à 1 h 15.

| Horaire | Contenu |
|---|---|
| Mercredi, 13 h 45 à 17 h | Étape 7 bis.1 : profils, base de données, API et tests |
| Mercredi, 17 h à 18 h | Pause |
| Mercredi, 18 h à 20 h 30 | Étape 7 bis.2 : écran « Qui prépare ? » |
| Mercredi, soirée | Étape 8 si les wireframes sont prêts |
| Jeudi | Planning du jour 2 inchangé |

- **Effet sur l'avance :** il en reste environ 2 h, mercredi soir. Si les étapes 8 et 9 dépassent jeudi leurs créneaux autant que les précédentes, le déploiement (étape 11) est le premier poste menacé, comme le prévoient les priorités ci-dessous ; il peut passer en soirée.
- **Pour tenir :** relectures complètes (plusieurs relecteurs, mutations) pour le code Go, où se trouvent les règles et les tests ; une seule relecture indépendante pour les écrans, qui n'ont pas de tests automatiques.

### Point d'étape du mercredi 4 novembre, 16 h 30

- **Fait :** étape 7 bis.1 terminée à 16 h 15, relecture complète comprise, au lieu de 17 h.
- **Relectures, à partir de l'étape 7 bis.2 :** une seule relecture indépendante par étape (7 bis.2, 8, 9, 10 et 11), sans contre-vérification multiple ni mutations. Une étape qui modifie du code touchant au stock, à la validation d'une ligne ou à la concurrence garde la relecture complète (plusieurs relecteurs et contre-vérification ; les mutations ne valent que pour du code couvert par des tests). L'étape 9 est dans ce cas : elle envoie la validation d'une ligne et doit résister au double appui.
- **Ajout : relecture finale du projet complet**, après l'étape 10 et avant le déploiement :
  1. relecture complète par Claude : plusieurs relecteurs, contre-vérification (1 h 15) ;
  2. relecture par un autre modèle, qui rend un rapport sans modifier le code (45 min). Aucune des deux ne modifie le code : elles sont lancées en même temps, et le README (étape 12) avance pendant qu'elles tournent ;
  3. corrections ensemble, par ordre de gravité (1 h 30).

  Durée réaliste : 2 h 45, ou 3 h 30 si les deux relectures se suivent.

| Horaire | Contenu |
|---|---|
| Mercredi, 16 h 45 à 19 h 30 | Étape 7 bis.2 : écran « Qui prépare ? » |
| Mercredi, 19 h 30 à 20 h 30 | Pause |
| Mercredi, 20 h 30 à 22 h 30 | Étape 8 : liste des commandes |
| Jeudi, 8 h 30 à 12 h | Étape 9 : écran de préparation, avec relecture complète |
| Jeudi, 12 h à 12 h 45 | Pause |
| Jeudi, 12 h 45 à 13 h 45 | Étape 10 : vérifications de bout en bout |
| Jeudi, 13 h 45 à 15 h | **Relecture finale**, première partie : les deux relectures, en même temps ; rédaction du README pendant ce temps |
| Jeudi, 15 h à 16 h 30 | **Relecture finale**, seconde partie : corrections par ordre de gravité |
| Jeudi, 16 h 30 à 18 h 30 | **Préparation de la présentation (2 h)** |
| Jeudi, 18 h 30 à 19 h | **Répétition (30 min)** |
| Jeudi, soirée | Capture et vidéo (étape 12), déploiement (étape 11), livraison |

- **Effet :** l'avance de mercredi absorbe la relecture finale, mais le déploiement passe en soirée, après la répétition, comme le prévoyaient déjà les priorités. Si la relecture finale déborde, les corrections s'arrêtent à 16 h 30 : les constats graves sont traités avant, les autres après le déploiement. La préparation de la présentation ne recule pas.

### Point d'étape du mercredi 4 novembre, 18 h

- **Fait :** étape 7 bis terminée (7 bis.2 avec sa refonte du design : deux densités, souris ou toucher).
- **Décision :** les étapes 8, 9, 10 et 11 s'enchaînent ce soir, avec une relecture légère par étape, étape 9 comprise, pour une mise en ligne ce soir. La relecture finale du projet complet a lieu jeudi, sur la version en ligne ; ses corrections sont ensuite redéployées. Le reste du planning de jeudi (README, présentation, répétition) ne change pas.

### Priorités en cas de retard

- **Supprimés en premier :** les évolutions optionnelles, puis les finitions visuelles, puis le déploiement en ligne.
- **Relecture finale :** les relectures elles-mêmes ne sont pas réduites ; les corrections se font par ordre de gravité, et les constats mineurs qui n'ont pas pu être traités avant 16 h 30 passent après le déploiement.
- **Si l'étape 7 bis déborde nettement de son estimation réaliste (5 h) :** on fait le point avant de continuer. La liste des profils et le choix du profil restent, car les étapes 8 et 9 en dépendent ; le formulaire « + Ajouter un profil » passe après le déploiement. La route de création reste dans l'API, testée.
- **Tailles, pour tous les écrans (décidé le 4 novembre au soir) :** deux densités selon le type d'écran, et non selon sa largeur, avec les requêtes média `(pointer: coarse)` et `(pointer: fine)`. Au toucher (tablette, téléphone) : contrôles de 64 px au moins, texte de 20 px. À la souris (ordinateur) : contrôles de 44 px (40 à 48), texte de 16 px, titres proportionnés, en-tête de 60 px. Ces tailles sont des variables CSS de `app.css` (`--control-height`, `--font-size-base`, `--header-height`...), utilisées par tous les écrans, sans valeur en dur. La largeur ne règle que la mise en page. Contenu simple dans un panneau centré plutôt que collé à gauche ; cartes avec survol et focus clavier visible (contour bleu nuit).
- **Écrans (étapes 7 bis.2, 8 et 9) :** d'abord l'ordinateur et la tablette, qui servent à la démonstration. La version mobile doit rester utilisable : sur un téléphone (390 px de large), pas de défilement horizontal, tout le texte lisible et chaque bouton accessible. Ses finitions passent après le déploiement.
- **Jamais réduits :** les tests, le README, le créneau de préparation de la présentation et la répétition.

---

## 7. Environnement de développement

### 7.1 Windows et PowerShell

Les commandes `go`, `dotnet`, `docker` et `git` s'écrivent de la même façon dans PowerShell et dans bash. Les différences utiles au projet :

| Besoin | PowerShell | bash |
|---|---|---|
| Définir une variable pour la session en cours | `$env:DATABASE_URL = "postgres://..."` | `export DATABASE_URL="postgres://..."` |
| Appeler curl | `curl.exe` (dans Windows PowerShell 5.1, `curl` est un alias d'`Invoke-WebRequest`) | `curl` |
| Enchaîner deux commandes | une commande par ligne (`&&` n'existe pas dans Windows PowerShell 5.1) | `&&` |

`go test -race` nécessite un compilateur C (cgo), généralement absent sous Windows. L'image `golang:1.27` en contient un : avec `db-test` lancée, depuis le dossier du dépôt :

```powershell
docker run --rm --network cairn_default -v "${PWD}\api:/src:ro" -w /src -e TEST_DATABASE_URL=postgres://cairn:cairn@db-test:5432/cairn_test?sslmode=disable golang:1.27 go test -race -count=1 ./...
```

### 7.2 Emplacement du dépôt : hors OneDrive

Le dépôt doit se trouver hors de tout dossier synchronisé par OneDrive, par exemple `C:\dev\Cairn`. Sous Windows, les dossiers Documents et Bureau sont souvent redirigés vers OneDrive.

Un dossier synchronisé pose trois problèmes :

- OneDrive synchronise en continu `.git`, `bin` et `obj`, ce qui verrouille des fichiers pendant les compilations ;
- il peut créer des conflits dans `.git` ;
- les fichiers « disponibles à la demande » ne sont pas toujours présents sur le disque quand Docker ou le compilateur les lisent.

Vérification, à lancer dans PowerShell depuis le dossier du dépôt :

```powershell
$dossier = (Get-Location).Path
$racines = @($env:OneDrive, $env:OneDriveConsumer, $env:OneDriveCommercial) | Where-Object { $_ }
$dansOneDrive = $racines | Where-Object { ($dossier + '\').StartsWith($_.TrimEnd('\') + '\', [System.StringComparison]::OrdinalIgnoreCase) }
if ($dansOneDrive) { Write-Host "ATTENTION : dossier dans OneDrive" -ForegroundColor Red } else { Write-Host "OK : hors OneDrive" -ForegroundColor Green }
```

### 7.3 Fins de ligne

Comportement observé lors de tests avec des fichiers CRLF montés dans des conteneurs Linux :

| Fichier en CRLF | Résultat |
|---|---|
| Dockerfile | toléré |
| Script SQL simple | toléré |
| Texte sur plusieurs lignes dans un script SQL | le caractère `\r` est stocké dans la donnée, sans erreur |
| Script shell exécutable | échec avec un message trompeur : `no such file or directory` |

Deux fichiers évitent ces cas :

- **`.gitattributes`** : tous les fichiers texte sont stockés et extraits en LF, quelle que soit la configuration Git de la machine.
- **`.editorconfig`** : les éditeurs écrivent en LF et en UTF-8. Ce réglage est nécessaire en plus du précédent, car `.gitattributes` n'agit que lorsque Git écrit un fichier. Un fichier créé dans un éditeur Windows reste en CRLF sur le disque, donc dans le conteneur, tant que Git ne l'a pas réécrit.

L'UTF-8 compte aussi pour les données de démonstration : un fichier SQL accentué enregistré dans un autre encodage fait échouer l'initialisation de PostgreSQL.

Documentation : [gitattributes](https://git-scm.com/docs/gitattributes), [EditorConfig](https://editorconfig.org/)

### 7.4 Outils

| Outil | Version | Installation (winget) |
|---|---|---|
| Git | récente | `winget install --id Git.Git -e` |
| Go | 1.27.x | `winget install --id GoLang.Go -e` |
| SDK .NET | 10.0.x (LTS) | `winget install --id Microsoft.DotNet.SDK.10 -e` |
| Docker Desktop | récente, moteur WSL 2 | `winget install --id Docker.DockerDesktop -e` |
| PostgreSQL | 18 | aucune : image Docker |

Éditeur : VS Code avec les extensions `golang.go`, `ms-dotnettools.csharp` et `EditorConfig.EditorConfig`, ou Visual Studio / Rider (EditorConfig pris en charge nativement).

### 7.5 Développer l'API pendant que la pile tourne

Le conteneur `api` publie le port 8080, qui est aussi l'adresse par défaut de `go run .` : les deux ne peuvent pas tourner en même temps. Deux façons de travailler :

- itérations rapides : `docker compose stop api`, puis `go run .` dans `api/`. La base reste lancée ; l'interface du conteneur `web` affiche « API injoignable », car elle vise `http://api:8080` ;
- vérification dans les conteneurs : `docker compose up -d --build api`.

Une route nouvelle qui répond 404 signale presque toujours une image qui n'a pas encore été reconstruite.

Documentation : [winget](https://learn.microsoft.com/windows/package-manager/winget/), [WSL](https://learn.microsoft.com/windows/wsl/install), [Go dans VS Code](https://code.visualstudio.com/docs/languages/go)
