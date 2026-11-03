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
| Verrouiller la commande au début de la transaction de validation | Sans ce verrou, deux validations simultanées des dernières lignes d'une commande peuvent la laisser au statut `en_cours` alors que tout est préparé. Mesure sur 200 essais : 25 erreurs sans verrou, 0 avec. Explication en section 3.3. |

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
        store.go               types (Order, Line, Product) et erreurs métier
        queries.go             lectures : liste, détail, produits
        pick.go                règle métier : PickLine, une transaction
        pick_test.go           tests sur une vraie base
      httpapi/
        routes.go              table des routes
        handlers.go            traduction HTTP ↔ store
        respond.go             réponses JSON et format d'erreur unique
  web/                         projet Blazor
    Cairn.Web.csproj
    Program.cs                 injection de dépendances, HttpClient
    Dockerfile
    appsettings.json, appsettings.Development.json
    Api/
      CairnApiClient.cs        seul endroit qui connaît les URL de l'API
      Models.cs                types échangés avec l'API : records du JSON, état ApiHealth
    State/
      PickerState.cs           nom du préparateur sélectionné
    Components/
      App.razor, Routes.razor, _Imports.razor, Layout/   (générés)
      Pages/
        Orders.razor           écran 1 : liste des commandes
        OrderDetail.razor      écran 2 : préparation d'une commande
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
| `PickerState` en service « scoped » | En Interactive Server, une instance par onglet connecté. Le nom est perdu au rechargement de la page, ce qui est acceptable sans authentification. |
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
| CMD-0005 | en cours | 1 ligne sur 3 déjà préparée |
| CMD-0006 | prête | toutes les lignes préparées |

Une dizaine de produits, avec des emplacements numérotés sur deux chiffres (`A-03-2`). Le tri alphabétique des emplacements suit alors l'ordre physique de l'allée : les lignes d'une commande sont présentées dans l'ordre du chemin de prélèvement.

### 3.5 Évolutions optionnelles

Chaque évolution ajoute un script `003_...sql`, justifié dans le README. Comme les scripts d'initialisation ne s'exécutent que sur un volume vide, leur application demande `docker compose down -v`.

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

// POST /api/orders/1/lines/3/pick, corps : { "picker": "Alice" }
// Réponse : la commande mise à jour, au même format.

// Toute erreur :
{ "error": "stock_insuffisant", "message": "Stock insuffisant : 3 disponibles, 4 demandés." }
```

La liste (`GET /api/orders`) inclut `lineCount` et `pickedCount` pour afficher l'avancement (« 2/5 »).

### 4.3 Codes d'erreur

| Code HTTP | `error` | Cas |
|---|---|---|
| 400 | `requete_invalide` | identifiant non numérique, JSON invalide, nom de préparateur vide, statut de filtre inconnu |
| 404 | `commande_introuvable` | commande inexistante |
| 404 | `ligne_introuvable` | ligne inexistante, ou n'appartenant pas à la commande |
| 409 | `deja_preparee` | ligne déjà validée |
| 409 | `stock_insuffisant` | stock inférieur à la quantité demandée |
| 500 | `erreur_interne` | message générique ; le détail reste dans les journaux du serveur |
| 503 | `base_indisponible` | `GET /health` : la base de données ne répond pas |

### 4.4 Points d'attention

- **Le POST renvoie la commande à jour** : l'interface remplace son état et voit immédiatement le passage à `prete`, sans second appel.
- **Noms JSON en camelCase** (`stockQty`) : les méthodes JSON de `HttpClient` les associent automatiquement aux propriétés C# `StockQty`.
- **Listes vides** : en Go, une slice non initialisée est encodée `null` et non `[]`. Toutes les listes renvoyées sont initialisées (`[]Line{}`), sinon le client C# reçoit une liste `null`.
- **Méthode HTTP incorrecte** : le routeur de `net/http` répond 405 automatiquement.

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
  - `/api/orders/abc` renvoie 400.
- **Documentation :** [encoding/json](https://pkg.go.dev/encoding/json), [pgx](https://pkg.go.dev/github.com/jackc/pgx/v5), [débuter avec pgx](https://github.com/jackc/pgx/wiki/Getting-started-with-pgx), [erreurs en Go](https://go.dev/blog/go1.13-errors)

### Étape 5 : règle métier `PickLine` (1 h 30)

- **Contenu :** la transaction de la section 3.3.
- **Notions Go :**
  - `pool.Begin` ;
  - `defer tx.Rollback(ctx)`, sans effet après un `Commit` ;
  - `RETURNING` avec `QueryRow` ;
  - `RowsAffected()` ;
  - erreurs déclarées une fois (`var ErrInsufficientStock = errors.New(...)`) ;
  - emballage d'erreur avec `%w`.
- **Vérification :** par les tests de l'étape 6.
- **Documentation :** [transactions en Go](https://go.dev/doc/database/execute-transactions), [isolation PostgreSQL](https://www.postgresql.org/docs/18/transaction-iso.html) (§13.2.1), [verrous](https://www.postgresql.org/docs/18/explicit-locking.html), [clause FOR UPDATE](https://www.postgresql.org/docs/18/sql-select.html)

### Étape 6 : tests Go (2 h)

**Base de test :** service `db-test` dans `compose.yaml`, avec `profiles: ["test"]` et ses données en mémoire (`tmpfs`). Lancement : `docker compose up -d --wait db-test`. Chaque test vide les tables puis insère ses propres données.

**Cas testés :**

1. La validation baisse le stock de la quantité exacte et enregistre le préparateur.
2. Avec un stock insuffisant, la validation est refusée, et ni le stock ni la ligne ne changent.
3. Une seconde validation de la même ligne est refusée, et le stock n'a baissé qu'une fois.
4. 10 validations simultanées sur un produit dont le stock n'en couvre que 3 donnent exactement 3 succès, 7 refus, et un stock final exact.
5. La première ligne validée fait passer la commande à `en_cours`, la dernière à `prete`.
6. Les deux dernières lignes d'une commande validées en parallèle, répété 50 fois, donnent toujours `prete`.
7. La même ligne validée deux fois en parallèle donne un succès et un refus.

**Contrôle de l'efficacité des tests :**

- retirer temporairement `AND stock_qty >= $2` doit faire échouer le test 4 (erreur `23514`) ;
- retirer le `FOR UPDATE` doit faire échouer le test 6 ;
- résultat à consigner dans NOTES.md, section Expérimentations.

**Notions Go :**

- `t.Fatal`, `t.Errorf`, `t.Helper` ;
- goroutines ;
- `sync.WaitGroup` et `wg.Go` (depuis Go 1.25) ;
- barrière de départ : un channel fermé pour libérer toutes les goroutines en même temps.

**Commande :** `go test -count=1 ./...`. L'option `-count=1` désactive le cache des résultats, inadapté à des tests qui dépendent d'une base.

**Arrêt complet, base de test comprise :** `docker compose --profile test down`. Un simple `docker compose down` laisse `db-test` tourner.

**Documentation :** [écrire un test](https://go.dev/doc/tutorial/add-a-test), [testing](https://pkg.go.dev/testing), [tests en table](https://go.dev/wiki/TableDrivenTests), [WaitGroup](https://pkg.go.dev/sync#WaitGroup), [race detector](https://go.dev/doc/articles/race_detector), [profils Compose](https://docs.docker.com/compose/how-tos/profiles/), [PgError](https://pkg.go.dev/github.com/jackc/pgx/v5/pgconn#PgError)

### Étape 7 : endpoint de validation (45 min)

- **Contenu :**
  - lecture du corps JSON ;
  - validation du nom du préparateur (non vide, 50 caractères au maximum) ;
  - traduction des erreurs métier en codes HTTP par un `switch` sur `errors.Is`.
- **Notions Go :** `json.Decoder`, `DisallowUnknownFields`, `http.MaxBytesReader`.
- **Vérification :** requêtes HTTP (commandes fournies pour PowerShell et bash) :
  - une première validation renvoie 200 ;
  - la même validation renvoie ensuite 409 `deja_preparee` ;
  - une ligne de CMD-0004 renvoie 409 `stock_insuffisant`.

### Étape 8 : écran liste des commandes (1 h 30)

- **Contenu :**
  - une carte par commande : référence, client, statut (couleur et libellé, jamais la couleur seule), avancement ;
  - sélection du préparateur par de grands boutons ;
  - dans `CairnApiClient`, des chemins relatifs sans barre oblique initiale (`api/orders`), pour fonctionner avec n'importe quelle adresse de base (Compose ou Render) ;
  - une API lente est un cas normal (réveil sur Render, étape 11) : état de chargement visible, puis message clair et bouton « Réessayer » si elle ne répond pas.
- **Notions C# et Blazor :**
  - `@foreach` ;
  - composants et `[Parameter]` ;
  - CSS isolé (`.razor.css`) ;
  - durée de vie « scoped » ;
  - `record` (données JSON des commandes) ;
  - expressions `switch`.
- **Vérification :** http://localhost:8081 affiche les commandes de la plus ancienne à la plus récente, avec trois couleurs de statut ; un appui sur une carte ouvre `/orders/{id}`.
- **Documentation :** [composants](https://learn.microsoft.com/aspnet/core/blazor/components/), [routage](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/routing), [injection de dépendances dans Blazor](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/dependency-injection), [CSS isolé](https://learn.microsoft.com/aspnet/core/blazor/components/css-isolation), [records](https://learn.microsoft.com/dotnet/csharp/fundamentals/types/records), [expression switch](https://learn.microsoft.com/dotnet/csharp/language-reference/operators/switch-expression)

### Étape 9 : écran de préparation (2 h)

- **Contenu :**
  - une carte par ligne : emplacement et quantité en très grand, nom du produit ;
  - bouton « Préparé » d'au moins 48 px de haut (64 px visés pour un usage avec des gants) ;
  - bouton désactivé pendant l'appel, et garde-fou côté serveur : un appui reçu pendant un appel en cours, ou sur une ligne déjà préparée, est ignoré (un double appui avec des gants ne doit pas produire un 409 pour une validation réussie) ;
  - en cas de 409, message clair choisi d'après le code d'erreur, puis rechargement de la commande ;
  - quand la commande est prête, un bandeau et un bouton de retour.
- **Notions C# et Blazor :**
  - paramètres de route (`{Id:long}`) ;
  - `@onclick` asynchrone ;
  - rendu conditionnel ;
  - lecture du code HTTP d'une réponse ;
  - `HttpRequestException`.
- **Vérification :**
  - sur http://localhost:8081/orders/1, chaque validation change l'état de la carte, puis le bandeau « Commande prête » apparaît ;
  - sur CMD-0004, le message de stock insuffisant s'affiche ;
  - deux fenêtres ouvertes sur CMD-0002 et CMD-0003 : la seconde validation est refusée. La preuve formelle de concurrence reste le test 4.
- **Documentation :** [événements](https://learn.microsoft.com/aspnet/core/blazor/components/event-handling), [cycle de vie](https://learn.microsoft.com/aspnet/core/blazor/components/lifecycle), [programmation asynchrone](https://learn.microsoft.com/dotnet/csharp/asynchronous-programming/), [types nullables](https://learn.microsoft.com/dotnet/csharp/nullable-references)

### Étape 10 : vérifications de bout en bout (1 h)

- Dans un clone neuf du dépôt, `docker compose up --build` doit suffire.
- Affichage tablette dans les outils de développement du navigateur.
- API coupée puis navigateur hors ligne : la fenêtre de reconnexion apparaît.
- **Documentation :** [SignalR et Blazor](https://learn.microsoft.com/aspnet/core/blazor/fundamentals/signalr)

### Étape 11 : déploiement en ligne (1 h)

**Objectif :** un lien public en HTTPS, que la dirigeante et le développeur ouvrent dans leur navigateur sans rien installer. **Priorité :** inférieure aux tests, au README et à la préparation de la présentation ; si l'étape déborde, elle passe en soirée, après la répétition.

**Cible, vérifiée le 3 novembre 2026 sur les documentations officielles :**

- **Render, offre gratuite**, pour `api` et `web` : deux services web Docker construits depuis le dépôt GitHub, région Frankfurt, 0,1 CPU et 512 Mo de mémoire chacun. Render accepte les WebSockets, sans durée maximale : Blazor Server fonctionne, sans affinité de session puisqu'il n'y a qu'une instance. HTTPS automatique (Render termine le TLS et transmet du HTTP au conteneur).
- **Neon, offre gratuite**, pour la base : PostgreSQL 18 (même version majeure qu'en local), région AWS Francfort, sans carte bancaire et sans expiration.
- **Base gratuite de Render écartée :** elle expire 30 jours après sa création (puis 14 jours de grâce, puis suppression), sans sauvegarde, une seule par compte.

**Contraintes de l'offre gratuite qui orientent les choix :**

- un service s'endort après 15 minutes sans trafic entrant (requêtes HTTP ou messages WebSocket) et met environ une minute à se réveiller ; le navigateur voit pendant ce temps une page d'attente de Render. Ce que reçoit un client qui n'est pas un navigateur (l'interface qui appelle l'API) n'est pas documenté : probablement un 503, à mesurer ;
- 750 heures gratuites par mois pour tout le compte, partagées entre les deux services ; épuisées, elles suspendent tout jusqu'au mois suivant. Deux services éveillés en permanence les épuisent vers le 16e jour ;
- un service gratuit ne reçoit pas de trafic du réseau privé : l'interface appelle l'API par son URL publique HTTPS, et non par `http://api:8080` comme dans Compose. L'API est donc joignable publiquement ;
- pas de commande « pre-deploy », de shell ni de tâche ponctuelle : le schéma et les données s'appliquent depuis le poste, avec psql ;
- Neon endort la base après 5 minutes sans requête (réveil en quelques centaines de millisecondes, connexions coupées puis recréées par pgxpool) et accorde 100 heures de calcul (CU-heures) par mois, soit environ 400 heures avec la plus petite taille ;
- sans `plan: free` explicite, Render crée un service payant ; sans `region: frankfurt`, il choisit l'Oregon, et la région ne change plus ensuite.

**Fichiers :**

- `render.yaml` (Blueprint), à la racine : deux services `type: web`, `runtime: docker`, `plan: free`, `region: frankfurt`, avec `dockerfilePath` et `dockerContext` (`./api` et `./web`), sans `rootDir` (la documentation se contredit sur son effet), et un `buildFilter` par dossier pour ne reconstruire que le service modifié. Pas de `healthCheckPath` : la sonde TCP par défaut suffit, alors qu'un contrôle sur `/health` interrogerait Neon toutes les quelques secondes. Variables :
  - `cairn-api` : `PORT=8080` (Render attend 10000 par défaut) ; `DATABASE_URL` avec `sync: false`.
  - `cairn-web` : `PORT=8080` ; `ApiBaseUrl`, l'URL publique de l'API (`fromService` avec `envVarKey: RENDER_EXTERNAL_URL` ; si Render ne la fournit pas, l'URL copiée depuis le tableau de bord) ; `ApiTimeoutSeconds=90`.
- `db/reset.sql` : supprime les trois tables, puis rejoue `init/001_schema.sql` et `init/002_seed.sql` (commande `\ir` de psql), le tout en une seule transaction (`psql -1`). Il sert à initialiser la base Neon et à remettre les données de démonstration à zéro, avec des dates de création recalculées.
- `web/Program.cs` : délai d'appel à l'API configurable (`ApiTimeoutSeconds`, 5 s par défaut, 90 s sur Render, plus que le réveil de l'API) ; appel à `/health` au démarrage de l'interface, pour réveiller l'API en même temps qu'elle ; si l'API répond par la page d'attente de Render plutôt que par son JSON, message « démarrage en cours » plutôt que « base indisponible ».

**Secrets :** l'URL de connexion Neon contient le mot de passe. Elle n'est jamais écrite dans le dépôt : avec `sync: false`, le tableau de bord de Render demande sa valeur à la création du Blueprint, et le script de remise à zéro la lit dans une variable d'environnement de la session PowerShell. On prend la chaîne directe de Neon (bouton « Connection pooling » désactivé : pgxpool regroupe déjà les connexions), avec `sslmode=require&channel_binding=require`. Neon refuse les connexions sans TLS, et pgx gère `channel_binding` depuis la version 5.9.

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
- parcours complet : CMD-0001 jusqu'à « prête », refus de stock insuffisant sur CMD-0004 ;
- après plus de 15 minutes sans visite : page d'attente de Render, puis l'application, sans « API injoignable » ; durée du réveil notée ;
- remise à zéro avec `db/reset.sql`, puis rechargement : données de démonstration d'origine ;
- journaux des deux services sans erreur dans le tableau de bord Render ; aucun secret dans le dépôt (`git grep neon.tech` ne trouve rien).

**Documentation :** [offre gratuite de Render](https://render.com/docs/free), [spécification Blueprint](https://render.com/docs/blueprint-spec), [Docker sur Render](https://render.com/docs/docker), [WebSockets sur Render](https://render.com/docs/websocket), [port des services web](https://render.com/docs/web-services), [health checks](https://render.com/docs/health-checks), [offres Neon](https://neon.com/docs/introduction/plans), [cycle de vie d'une compute Neon](https://neon.com/docs/introduction/compute-lifecycle), [choisir sa connexion Neon](https://neon.com/docs/connect/choose-connection), [connexion sécurisée à Neon](https://neon.com/docs/connect/connect-securely), [changelog de pgx](https://github.com/jackc/pgx/blob/master/CHANGELOG.md), [FAQ de cron-job.org](https://cron-job.org/en/faq/)

### Étape 12 : README, capture d'écran, vidéo de démonstration (1 h 30)

Contenu défini dans SCOPE.md, section 9, avec le lien public de l'étape 11 s'il existe.

### Étape 13 : évolutions optionnelles

Seulement s'il reste du temps. Dans l'ordre :

1. historique des mouvements de stock, avec un test ;
2. alerte de stock bas ;
3. annulation d'une validation, écartée par défaut car c'est la plus risquée.

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

### Priorités en cas de retard

- **Supprimés en premier :** les évolutions optionnelles, puis les finitions visuelles, puis le déploiement en ligne.
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

`go test -race` nécessite un compilateur C (cgo), généralement absent sous Windows : cette option est facultative.

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
