<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/cairn-logo-white.svg">
    <img src="docs/cairn-logo.svg" alt="Cairn" width="300">
  </picture>
</h1>

<p align="center">
  Un prototype qui guide le préparateur de commandes dans l'entrepôt :<br>
  où aller, quoi prendre, combien, avec le stock mis à jour à chaque produit pris.
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.27-1B2A41?logo=go&logoColor=white" alt="Go 1.27">
  <img src="https://img.shields.io/badge/.NET-10-1B2A41?logo=dotnet&logoColor=white" alt=".NET 10">
  <img src="https://img.shields.io/badge/Blazor-Interactive%20Server-1B2A41?logo=blazor&logoColor=white" alt="Blazor Interactive Server">
  <img src="https://img.shields.io/badge/PostgreSQL-18-1B2A41?logo=postgresql&logoColor=white" alt="PostgreSQL 18">
</p>

<p align="center">
  <a href="https://cairn-knudsem-web.onrender.com"><strong>Essayer la démo en ligne</strong></a><br>
  <sub>Le premier chargement peut prendre environ une minute (offre gratuite de Render).</sub>
</p>

![Une commande ouverte sur ordinateur : la liste des commandes à gauche, la carte « À faire maintenant » au centre, « À prendre ensuite » et « Déjà pris » à droite](docs/images/ordinateur.png)

<table>
  <tr>
    <td align="center"><img src="docs/images/tablette.png" alt="La même commande sur tablette, en une colonne" width="375"></td>
    <td align="center"><img src="docs/images/telephone.png" alt="La même commande sur téléphone" width="250"></td>
  </tr>
  <tr>
    <td align="center">Tablette</td>
    <td align="center">Téléphone</td>
  </tr>
</table>

**Sommaire** : [À quoi sert Cairn](#à-quoi-sert-cairn) · [Essayer la démo](#essayer-la-démo) · [Fonctionnalités](#fonctionnalités) · [Architecture](#architecture) · [La mise à jour du stock](#la-mise-à-jour-du-stock) · [Lancer le projet en local](#lancer-le-projet-en-local) · [Lancer les tests](#lancer-les-tests) · [Structure du dépôt](#structure-du-dépôt) · [Variables d'environnement](#variables-denvironnement) · [Déploiement](#déploiement) · [Choix techniques et limites](#choix-techniques-et-limites) · [Pistes pour la suite](#pistes-pour-la-suite) · [Comment ce projet a été construit](#comment-ce-projet-a-été-construit)

## À quoi sert Cairn

J'ai imaginé un préparateur de commandes debout dans les allées, parfois avec des gants, une tablette à la main. Entre deux cartons, il doit savoir tout de suite où aller, quoi prendre et combien. Il n'a pas le temps de lire un long écran, et le stock doit rester juste.

Cairn lui donne une consigne à la fois : « Allez à B-07-2 », « Prenez 3 », le nom du produit et la référence à vérifier sur l'étiquette. Il touche « C'est pris » : le stock baisse, le produit garde son nom, et la commande passe à « prête » quand tout est pris. Si le stock ne suffit pas, la validation est refusée avec un message clair, et rien n'est enregistré.

C'est un prototype, fait en quelques jours pour apprendre Go, C# et Blazor. Le but est une fonctionnalité complète et testée, de l'écran jusqu'à la base de données, plutôt qu'un produit entier.

## Essayer la démo

Lien : **https://cairn-knudsem-web.onrender.com**

Le service est sur l'offre gratuite de Render. Il s'endort après 15 minutes sans visite, et le premier chargement peut alors prendre environ une minute. Render affiche d'abord sa propre page d'attente : c'est normal.

1. Sur « Qui prépare ? », choisissez **Claire**, **Romain** ou **Virginie**, ou créez votre profil avec « Ajouter un profil ». Il n'y a pas de mot de passe.
2. Ouvrez une commande et touchez « C'est pris » pour chaque produit.

Chaque commande de démonstration montre un cas précis :

| Commande | Client | Ce qu'elle montre |
|---|---|---|
| CMD-0001 | Boulangerie Martin | Le parcours complet : 4 produits à prendre, jusqu'à « prête ». |
| CMD-0002 et CMD-0003 | Pharmacie du Centre, Fromagerie Lefèvre | Chacune demande 4 « Papier bulle 1 x 50 m », et il n'y en a que 5 en stock. La première servie passe, l'autre reçoit « Stock insuffisant : 1 disponible, 4 demandés. ». Essayez-les dans deux onglets, ou à deux. |
| CMD-0004 | Épicerie Moreau | Toujours refusée : 5 dévidoirs de ruban adhésif demandés, 2 en stock. Le message se termine par « Prévenez votre responsable. ». |
| CMD-0005 | Librairie Dumas | Une commande commencée : 1 produit sur 3 déjà pris par Romain. |
| CMD-0006 | Café des Halles | Une commande prête, préparée par Virginie (onglet « Prêtes »). |

Les données sont partagées entre tous les visiteurs. Si quelqu'un est passé avant vous, le bouton **« Réinitialiser la démo »**, en bas de « Qui prépare ? », remet les commandes, le stock et les profils à leur état de départ, après une confirmation. Les profils créés sont alors supprimés.

Un onglet resté ouvert garde le service éveillé et consomme les heures gratuites de Render : merci de le fermer après l'essai.

<table>
  <tr>
    <td align="center"><img src="docs/images/qui-prepare.png" alt="L'écran « Qui prépare ? » avec les profils Claire, Romain et Virginie, et « Réinitialiser la démo » en bas à droite" width="420"></td>
    <td align="center"><img src="docs/images/commandes.png" alt="La liste des commandes à préparer, avec leur statut et leur avancement" width="420"></td>
  </tr>
  <tr>
    <td align="center">Qui prépare ?</td>
    <td align="center">Les commandes</td>
  </tr>
</table>

## Fonctionnalités

- **Profils.** On choisit son profil, on en crée un (nom unique sans tenir compte des majuscules, 20 profils au plus), ou on en supprime un avec « Gérer les profils ». Les produits déjà pris gardent le nom du préparateur.
- **Commandes.** La liste commence par la plus ancienne. Chaque commande affiche son statut en couleur et en toutes lettres (« À préparer », « En cours », « Prête ») et son avancement (« 1 / 3 produits pris »).
- **Préparation.** La carte « À faire maintenant » se lit de haut en bas : « Allez à » et l'emplacement en grand, avec sa traduction (« Allée B, rack 7, niveau 2 »), « Prenez » et la quantité, le produit, la référence à vérifier, puis le bouton « C'est pris ». Viennent ensuite « À prendre ensuite » et « Déjà pris ». Les produits suivent l'ordre des emplacements.
- **Stock.** Chaque « C'est pris » baisse le stock dans une transaction. Un stock insuffisant, ou un produit déjà pris par quelqu'un d'autre, donne un message clair, et rien n'est enregistré.
- **Commande prête.** Le statut passe à « prête » tout seul quand le dernier produit est pris, et « Commande suivante » ouvre la commande d'après.
- **Trois formats.** Boutons de 44 px à la souris, de 64 px sur tablette pour les doigts gantés, et de 48 px sur téléphone. Sur un écran large, la liste reste à gauche de la commande ouverte.
- **Coupures et réveil.** Si la connexion tombe, une fenêtre de reconnexion s'affiche. Pendant que l'API démarre, les lectures sont réessayées et l'écran affiche « Cairn démarre. Cela peut prendre une minute. ».
- **Réinitialiser la démo.** Un bouton, avec confirmation, remet les données de démonstration à leur état de départ.

## Architecture

Cairn compte trois programmes : l'interface Blazor, l'API Go et la base PostgreSQL. Avec Blazor en mode Interactive Server, le code C# tourne sur le serveur. Le navigateur lui envoie les clics par une connexion permanente (SignalR, sur WebSocket), et c'est le serveur de l'interface qui appelle l'API Go en HTTP et JSON. Le navigateur ne parle pas directement à l'API.

**En local**, avec Docker Compose, chaque programme a son conteneur. Les ports ne sont publiés que sur `127.0.0.1` : rien n'est joignable depuis le réseau.

```mermaid
flowchart LR
    nav["Navigateur"] -->|"SignalR (WebSocket)"| web["web : interface Blazor<br/>.NET 10, localhost:8081"]
    web -->|"HTTP + JSON"| api["api : Go, net/http + pgx<br/>localhost:8080"]
    api -->|"SQL"| db[("db : PostgreSQL 18<br/>127.0.0.1:5432")]
    tests["go test"] -->|"SQL"| dbtest[("db-test : PostgreSQL 18<br/>127.0.0.1:5433, données en mémoire")]
```

**En ligne**, un seul service Render fait tourner l'interface et l'API dans le même conteneur. L'API n'écoute qu'à l'intérieur du conteneur : seule l'interface est publique. La base est chez Neon.

```mermaid
flowchart LR
    nav["Navigateur"] -->|"HTTPS"| web
    subgraph render["Render : service cairn-knudsem-web, un seul conteneur"]
        web["Interface Blazor<br/>port 10000"] -->|"HTTP + JSON"| api["API Go<br/>127.0.0.1:9000"]
    end
    api -->|"TLS"| neon[("Neon : PostgreSQL 18<br/>Francfort")]
```

<details>
<summary><strong>Les routes de l'API</strong></summary>

| Méthode | Route | Rôle |
|---|---|---|
| `GET` | `/health` | Vérifie que l'API et la base répondent : 200, ou 503 si la base ne répond pas. |
| `GET` | `/api/orders?status=a_preparer` | Liste les commandes, la plus ancienne en premier. Le filtre est optionnel. |
| `GET` | `/api/orders/{id}` | Une commande et ses lignes, triées par emplacement. |
| `POST` | `/api/orders/{id}/lines/{lineId}/pick` | Valide une ligne. Corps : `{ "picker": "Claire" }`. Renvoie la commande à jour. |
| `GET` | `/api/products` | Les produits et leur stock. |
| `GET` | `/api/pickers` | Les profils, triés par nom. |
| `POST` | `/api/pickers` | Crée un profil. Corps : `{ "name": "Test" }`. Réponse 201. |
| `DELETE` | `/api/pickers/{id}` | Supprime un profil. Réponse 204. Les lignes déjà préparées gardent son nom. |
| `POST` | `/api/demo/reset` | Remet la démo à son état de départ. Réponse 204. La route n'existe que si `DEMO_RESET_ENABLED` vaut `true`. |

Toutes les erreurs ont le même format JSON :

```json
{ "error": "stock_insuffisant", "message": "Stock insuffisant : 2 disponibles, 5 demandés." }
```

| Code HTTP | `error` |
|---|---|
| 400 | `requete_invalide` |
| 404 | `commande_introuvable`, `ligne_introuvable`, `profil_introuvable` |
| 409 | `deja_preparee`, `stock_insuffisant`, `profil_existant`, `trop_de_profils` |
| 500 | `erreur_interne` (le détail reste dans les journaux du serveur) |
| 503 | `base_indisponible` (`/health` seulement) |

Le détail des réponses est dans [docs/PLAN.md](docs/PLAN.md), section 4.

</details>

## La mise à jour du stock

C'est la règle centrale du projet : le stock ne doit pas devenir négatif, même si deux préparateurs touchent « C'est pris » au même moment. Trois protections se complètent : une contrainte en base, une mise à jour conditionnelle du stock et un verrou sur la commande.

### La contrainte en base

Dans [`db/init/001_schema.sql`](db/init/001_schema.sql) :

```sql
stock_qty  integer NOT NULL CHECK (stock_qty >= 0)
```

PostgreSQL refuse toute écriture qui ferait passer le stock sous zéro, qu'elle vienne de l'API ou d'une requête faite à la main. C'est le filet de sécurité. Seule, elle donnerait une erreur technique (code `23514`) au lieu d'un message clair : d'où les deux protections suivantes.

### La transaction en quatre temps

La validation d'une ligne (`PickLine`, dans [`api/internal/store/pick.go`](api/internal/store/pick.go)) se fait dans une seule transaction : soit tout est enregistré, soit rien. Voici ses quatre requêtes, reprises du code et mises en page pour la lecture :

```sql
-- 1. Verrouiller la commande et vérifier qu'elle existe
SELECT status FROM orders WHERE id = $1 FOR UPDATE

-- 2. Marquer la ligne, seulement si elle n'est pas déjà prise
UPDATE order_lines SET picked_at = now(), picked_by = $3
WHERE id = $2 AND order_id = $1 AND picked_at IS NULL
RETURNING product_id, quantity

-- 3. Baisser le stock, seulement s'il suffit ($1 : le produit, $2 : la quantité)
UPDATE products SET stock_qty = stock_qty - $2
WHERE id = $1 AND stock_qty >= $2

-- 4. Recalculer le statut de la commande
UPDATE orders SET status = CASE
    WHEN EXISTS (SELECT 1 FROM order_lines
                 WHERE order_id = $1 AND picked_at IS NULL)
    THEN 'en_cours' ELSE 'prete' END
WHERE id = $1
```

À l'étape 3, la vérification et la baisse du stock tiennent en une seule instruction. Si aucune ligne n'est modifiée, le stock ne suffit pas, et la fonction s'arrête. Le `Rollback`, prévu dès le début, annule alors aussi l'étape 2 :

```go
defer tx.Rollback(ctx)

// ...

if tag.RowsAffected() == 0 {
	// ...
	return &InsufficientStockError{Available: available, Requested: quantity}
}
```

### Deux préparateurs en même temps

Quand une transaction modifie une ligne de table, elle la verrouille jusqu'à la fin. Une seconde transaction qui veut modifier le même produit attend. À sa reprise, PostgreSQL relit le stock à jour et réévalue la condition `stock_qty >= $2`. C'est le comportement documenté du niveau d'isolation par défaut, `READ COMMITTED`.

Exemple avec le papier bulle de CMD-0002 et CMD-0003 : 5 en stock, et deux validations de 4.

| Temps | Validation A | Validation B |
|---|---|---|
| t1 | `UPDATE ... WHERE stock_qty >= 4` : 5 ≥ 4, le stock passe à 1, le produit est verrouillé | |
| t2 | | même `UPDATE` : attend le verrou |
| t3 | `COMMIT` | |
| t4 | | relit le stock : 1 ≥ 4 est faux, aucune ligne modifiée, refus |

Le verrou sur la commande (étape 1) sert au recalcul du statut. Chaque instruction SQL lit une image de la base prise à son début. Sans ce verrou, deux préparateurs qui prennent en même temps les deux derniers produits d'une commande peuvent chacun voir l'autre produit comme « pas encore pris », et la commande reste « en cours ». Mesuré dans ce projet : 200 commandes sur 200 restées « en cours » sans le verrou, 0 sur 100 avec.

Toutes les validations prennent leurs verrous dans le même ordre (commande, ligne, produit) : deux validations ne peuvent pas s'attendre l'une l'autre en boucle.

**Ce qui a été écarté :**

- un `SELECT ... FOR UPDATE` sur le produit, puis un contrôle en Go : cela marche aussi, mais demande deux requêtes au lieu d'une ;
- un verrou en Go (`sync.Mutex`) : il ne protège qu'une instance de l'API, pas la base ;
- le niveau d'isolation `SERIALIZABLE` : il oblige à rejouer les transactions refusées (erreur `40001`).

**Ce que les tests vérifient :** 10 validations simultanées sur un produit dont le stock n'en couvre que 3 donnent exactement 3 succès et 7 refus (scénario répété 10 fois) ; les deux dernières lignes d'une commande validées en parallèle donnent toujours « prête » (répété 50 fois) ; une même ligne validée deux fois en parallèle n'est prise qu'une fois ; une validation interrompue après la baisse du stock n'enregistre rien. Pour contrôler ces tests, la condition `AND stock_qty >= $2` a été retirée : le test des 10 validations échoue alors sur la contrainte `CHECK`. Sans le `FOR UPDATE`, le test des deux dernières lignes échoue.

## Lancer le projet en local

**Prérequis :** [Docker Desktop](https://docs.docker.com/desktop/) (avec Docker Compose) et Git. Go 1.27 est utile pour lancer les tests, mais pas obligatoire (voir plus bas). Le SDK .NET 10 ne sert qu'à travailler sur l'interface hors de Docker.

Les commandes sont écrites pour PowerShell. Elles ont été vérifiées sous Windows 11, avec Docker Compose 2.34 et Go 1.27.3.

```powershell
git clone https://github.com/knudsem/Cairn.git
cd Cairn
docker compose up --build
```

La première construction télécharge les images de Go, de .NET et de PostgreSQL, ce qui peut prendre plusieurs minutes. Au premier démarrage, PostgreSQL crée les tables et charge les données de démonstration.

Ensuite :

| Adresse | Ce qu'on y trouve |
|---|---|
| http://localhost:8081 | L'interface. Commencez par « Qui prépare ? ». |
| http://localhost:8080/health | L'API. Elle répond `{"status":"ok"}` quand la base répond aussi. |

Pour arrêter : Ctrl+C dans le terminal, ou `docker compose down` depuis un autre terminal. Les données restent dans le volume `db-data` jusqu'au prochain démarrage.

Pour remettre les données de démonstration à leur état de départ : le bouton « Réinitialiser la démo », en bas de « Qui prépare ? ». Il est activé dans `compose.yaml`.

## Lancer les tests

Les tests Go tournent sur une vraie base PostgreSQL, le service `db-test` de `compose.yaml`. Elle est jetable : ses données sont en mémoire et disparaissent à l'arrêt. Elle ne reçoit que le schéma, et chaque test vide les tables puis insère ses propres données. Par sécurité, les tests refusent de tourner sur une base dont le nom ne se termine pas par `_test`.

Avec Go installé, depuis la racine du dépôt :

```powershell
docker compose up -d --wait db-test
cd api
go test -count=1 ./...
cd ..
docker compose stop db-test
```

`-count=1` désactive le cache des résultats, qui n'a pas de sens pour des tests qui dépendent d'une base.

Sans Go, avec Docker seulement, une fois `db-test` lancée et depuis la racine du dépôt :

```powershell
docker run --rm --network cairn_default -v "${PWD}\api:/src:ro" -w /src -e TEST_DATABASE_URL=postgres://cairn:cairn@db-test:5432/cairn_test?sslmode=disable golang:1.27 go test -race -count=1 ./...
```

Cette adresse contient les identifiants locaux de développement de `db-test`, déjà écrits dans `compose.yaml`. Ce ne sont pas des secrets : la base n'est joignable que depuis le poste et ne contient que des données de test. `cairn_default` est le réseau créé par Compose ; son nom vient du dossier du dépôt (ici `Cairn`). `-race` active le détecteur d'accès concurrents de Go, qui demande un compilateur C, présent dans l'image `golang:1.27`.

**Ce que couvrent les tests** (59 fonctions de test et 133 sous-tests) :

- `internal/store`, la couche SQL : la validation d'une ligne (stock baissé de la bonne quantité, refus sans aucun changement, double validation, passage à « en cours » puis à « prête », validations simultanées, interruption en pleine transaction), les lectures (tris, avancement, dates en UTC), les profils (nom unique sans tenir compte des majuscules, limite de 20 même avec des créations simultanées, suppression qui garde l'historique) et la remise à zéro de la démo ;
- `internal/httpapi`, la couche HTTP, avec `net/http/httptest` : chaque route, ses codes de réponse et le format des erreurs, les corps refusés (taille, type, champ inconnu, JSON invalide), les noms refusés, une panne de base sans détail technique dans la réponse ;
- `internal/testdb` : le garde-fou sur le nom de la base de test.

Pour vérifier que les tests détectent vraiment les erreurs, des erreurs plausibles ont été introduites une à une dans une copie du code (contrôle par mutations). Sur la couche SQL, 66 erreurs : 16 passaient, les tests ont été renforcés, et il en reste 3, sans conséquence ou écartées pour des raisons expliquées dans [docs/PLAN.md](docs/PLAN.md) (étape 6). Sur la couche HTTP, 58 erreurs, et sur les profils, 43 : toutes détectées.

L'interface n'a pas de tests automatiques dans le dépôt. Chaque étape a été vérifiée dans le navigateur, à la main et avec des scripts Playwright qui ne sont pas dans le dépôt.

## Structure du dépôt

```
compose.yaml                  services db, api et web, plus db-test pour les tests
render.yaml                   le service Render (Blueprint)
SCOPE.md                      le périmètre du prototype
NOTES.md                      mon journal d'apprentissage
CLAUDE.md                     les règles de travail données à Claude Code
api/                          l'API Go
  main.go                     démarrage : configuration, connexion, serveur HTTP, arrêt propre
  Dockerfile
  internal/
    store/                    SQL et règles métier (PickLine, profils, remise à zéro), avec leurs tests
    httpapi/                  routes, lecture des corps JSON, traduction des erreurs en codes HTTP, avec leurs tests
    demo/                     002_seed.sql, les données de démonstration, embarquées dans l'exécutable
    testdb/                   accès à la base de test : garde-fou, verrou entre paquets, remise à zéro
web/                          l'interface Blazor (.NET 10)
  Program.cs                  configuration et services
  Dockerfile
  Api/                        CairnApiClient, le seul endroit qui appelle l'API
  State/                      PickerState, le profil choisi dans l'onglet
  Components/                 les écrans : Qui prépare ?, Commandes, préparation d'une commande
  wwwroot/                    app.css (tailles et couleurs en variables), logo, un module JavaScript
db/
  init/001_schema.sql         tables et contraintes
  reset.sql                   recrée les tables et les données sur une base hors Docker (Neon)
render/
  Dockerfile                  l'image unique de Render : API et interface
  start.sh                    lance les deux programmes et les arrête ensemble
docs/
  PLAN.md                     conception et déroulé du projet, étape par étape
  maquettes/                  maquettes des écrans
  images/                     captures de ce README
```

## Variables d'environnement

Aucune variable n'est à créer pour lancer le projet en local : `compose.yaml` règle tout.

<details>
<summary><strong>Le détail des variables</strong></summary>

| Variable | Lue par | Rôle |
|---|---|---|
| `DATABASE_URL` | API | Adresse de la base PostgreSQL. Sans elle, l'API vise la base locale de Compose. Sur Render, c'est la chaîne de connexion de Neon, saisie dans le tableau de bord ; `render/start.sh` refuse de démarrer sans elle, et la retire de l'environnement de l'interface. |
| `API_ADDR` | API | Adresse d'écoute de l'API. Elle diffère selon l'endroit : poste, conteneur Compose ou conteneur Render. |
| `DEMO_RESET_ENABLED` | API et interface | Quand elle vaut `true`, l'API déclare `POST /api/demo/reset` et « Qui prépare ? » affiche « Réinitialiser la démo ». Sinon, ni route ni bouton. Activée dans `compose.yaml` et `render.yaml`. |
| `TEST_DATABASE_URL` | tests Go | Base de test à utiliser à la place de `db-test`. Son nom doit se terminer par `_test`. |
| `ApiBaseUrl` | interface | Adresse de l'API Go : `appsettings.json` hors Docker, `compose.yaml` dans Docker, `render/start.sh` sur Render. |
| `ApiTimeoutSeconds` | interface | Délai maximal d'un appel à l'API, nouveaux essais des lectures compris. Plus long sur Render, pour couvrir le démarrage de l'API et le réveil de Neon. |
| `ApiWakeUpFromBrowser` | interface | Reste du premier correctif du démarrage à froid, quand l'API et l'interface étaient deux services Render : le navigateur réveillait l'API. Depuis le passage à un seul conteneur, elle est coupée dans Compose et sur Render, et ne sert plus. Le code correspondant est à retirer. |
| `PORT` | `render/start.sh` | Port public donné par Render. `start.sh` le transmet à l'interface. |

`render/start.sh` règle aussi deux variables standard de .NET : `ASPNETCORE_HTTP_PORTS` (le port de l'interface) et `DOTNET_SHUTDOWNTIMEOUTSECONDS` (l'attente des onglets ouverts à l'arrêt).

</details>

## Déploiement

La démo en ligne repose sur deux offres gratuites, toutes deux dans la région de Francfort.

**Render** fait tourner un seul service web Docker, `cairn-knudsem-web`, décrit dans [`render.yaml`](render.yaml). L'image [`render/Dockerfile`](render/Dockerfile) se construit en trois étapes : compilation de l'API Go, compilation de l'interface .NET, puis exécution sur l'image ASP.NET Core. Au démarrage, [`render/start.sh`](render/start.sh) lance l'API sur `127.0.0.1:9000`, joignable seulement dans le conteneur, et l'interface sur le port public. Si l'un des deux programmes s'arrête, le script arrête l'autre, et Render redémarre le conteneur.

Au début, l'API et l'interface étaient deux services Render. Mais un appel de l'interface hébergée chez Render ne réveillait pas l'API endormie (réponse 502), et le réveil par le navigateur pouvait être bloqué par une extension. Avec un seul service, l'API démarre en même temps que l'interface. Le détail est dans [docs/PLAN.md](docs/PLAN.md), étapes 11 bis et 11 ter.

**Neon** héberge la base PostgreSQL 18. L'API s'y connecte avec la chaîne de connexion directe, en TLS. Cette chaîne contient le mot de passe : elle est saisie dans le tableau de bord de Render (`sync: false` dans `render.yaml`) et n'est pas dans le dépôt.

<details>
<summary><strong>Créer ou remettre à zéro la base Neon</strong></summary>

[`db/reset.sql`](db/reset.sql) supprime les quatre tables, puis rejoue le schéma et les données de démonstration, en une seule transaction. `psql` vient de l'image `postgres:18` : rien à installer. Depuis la racine du dépôt :

```powershell
$env:DEMO_DATABASE_URL = "<chaîne de connexion directe de Neon>"
docker run --rm -v "${PWD}:/cairn:ro" postgres:18 psql $env:DEMO_DATABASE_URL -v ON_ERROR_STOP=1 -1 -f /cairn/db/reset.sql
```

Pour remettre seulement les données à zéro, le bouton « Réinitialiser la démo » suffit. Ce script sert surtout à créer les tables.

</details>

**Les limites de l'offre gratuite :**

- le service s'endort après 15 minutes sans trafic, et met environ une minute à se réveiller ;
- Render accorde 750 heures par mois pour tout le compte. Un onglet ouvert garde le service éveillé : Blazor envoie un message toutes les 15 secondes ;
- 0,1 CPU et 512 Mo de mémoire. Sur une image construite comme celle de Render et lancée avec ces limites, les deux programmes occupaient environ 160 Mo après des parcours complets, et le démarrage à froid prenait de 9 à 15 secondes ;
- Neon endort la base après 5 minutes sans requête. Elle se réveille en quelques centaines de millisecondes, et le pool de connexions de pgx recrée les connexions.

## Choix techniques et limites

**Go avec `net/http` et `pgx`, sans framework.** Depuis Go 1.22, le routeur de la bibliothèque standard gère les méthodes et les paramètres de chemin (`POST /api/orders/{id}/lines/{lineId}/pick`) : les neuf routes tiennent dans [`routes.go`](api/internal/httpapi/routes.go). La seule dépendance directe est `pgx`, le pilote PostgreSQL. Le SQL est écrit à la main, sans ORM : la garantie de stock se lit directement dans les requêtes. Le code tient en deux paquets principaux : `store` parle SQL et ignore HTTP, `httpapi` parle HTTP et ignore SQL, et les erreurs métier font le lien. Il n'y a pas de couche « service » : la règle métier est une transaction, et une couche qui ne ferait que transmettre n'apporterait rien ici.

**Blazor en mode Interactive Server.** Le code C# tourne sur le serveur, dans un seul projet. Le navigateur ne parle pas à l'API Go : il n'y a pas de CORS à régler, et la tablette ne télécharge pas le runtime .NET, comme ce serait le cas avec Blazor WebAssembly. La contrepartie est la connexion permanente : si le Wi-Fi coupe, Blazor affiche une fenêtre de reconnexion. Le pré-rendu est désactivé, sinon chaque page appellerait l'API deux fois. Le style est une feuille CSS sans librairie, avec les tailles et les couleurs en variables.

**Cette API en Go, comparée à la même API en FastAPI.** Je n'ai pas écrit la version FastAPI : cette comparaison vient de mon expérience de Python et de ce projet.

| Sujet | Cette API en Go | La même API en FastAPI |
|---|---|---|
| Typage | Vérifié à la compilation : une erreur de type empêche de construire l'exécutable. | Annotations de type vérifiées à l'exécution par Pydantic pour les entrées ; le reste dépend d'un outil comme mypy. |
| Validation des entrées | Écrite à la main (taille du corps, champ inconnu, nom du préparateur) : plus de code, mais chaque règle est visible. | Déclarée avec des modèles Pydantic, plus courte, avec une documentation OpenAPI générée. Les erreurs 422 par défaut seraient à adapter au format `error` et `message`. |
| Gestion des erreurs | Les erreurs sont des valeurs (`if err != nil`) ; un `switch` sur `errors.Is` traduit les erreurs métier en codes HTTP. | Des exceptions, traduites par des gestionnaires : moins de code, mais le chemin d'une erreur se lit moins directement. |
| Concurrence | Chaque requête a sa goroutine ; les tests lancent les validations simultanées avec des goroutines. | `async` et `await` avec un pilote asynchrone. Dans les deux cas, la garantie de stock reste dans la transaction SQL. |
| Accès à la base | `pgx` et du SQL écrit à la main. | Le même SQL avec asyncpg ou psycopg ; SQLAlchemy est courant, mais ajoute une couche. |
| Déploiement | Un exécutable autonome, sans rien d'autre à installer dans l'image. | Un interpréteur Python, les dépendances et un serveur ASGI (Uvicorn) dans l'image. |

**Ce qui manque encore :**

- **Pas d'authentification.** N'importe qui peut choisir, créer ou supprimer un profil, valider un produit ou réinitialiser la démo. C'est un choix assumé pour ce prototype, à revoir avant tout usage réel.
- La validation envoie seulement le nom du préparateur : l'API ne vérifie pas que le profil existe encore.
- Pas d'outil de migration : les scripts de `db/init` ne s'exécutent que sur un volume de base vide.
- Une validation faite par erreur ne peut pas être annulée.
- Pas d'intégration continue, et pas de tests de l'interface dans le dépôt.
- En ligne, le réveil prend environ une minute, et les données sont partagées entre tous les visiteurs.

## Pistes pour la suite

- **Scanner les étiquettes** au lieu de vérifier la référence à l'œil.
- **Un ordre de prélèvement qui suit le chemin dans l'entrepôt.** Aujourd'hui, les produits sont triés par code d'emplacement.
- **Signaler un produit manquant** depuis la carte, au lieu de seulement prévenir le responsable.
- **Annuler une validation faite par erreur**, avec remise en stock et un test qui la couvre.
- **Voir en direct qui prépare quelle commande.**
- **L'historique des mouvements de stock** : qui, quoi, quand.
- **Une alerte de stock bas.**
- **L'intégration continue**, qui lance les tests Go à chaque push.
- **Les tests navigateur dans le dépôt**, pour que les vérifications de l'interface puissent être rejouées par tous.
- **Un code PIN par préparateur**, pour qu'un profil ne puisse pas être choisi par n'importe qui.

## Comment ce projet a été construit

Claude Code a écrit la plus grande partie du code, étape par étape. J'ai défini le besoin et les règles, relu ses propositions, testé chaque étape dans le navigateur et fait les commits moi-même.

Les documents de travail sont dans le dépôt :

- [SCOPE.md](SCOPE.md) : le besoin, le périmètre et ce qui en est exclu ;
- [CLAUDE.md](CLAUDE.md) : les règles de travail données à Claude Code (un plan avant chaque étape, vérifier avant de dire qu'une étape est finie, ne pas toucher au stock sans un test qui le couvre, aucun commit fait par l'IA) ;
- [docs/PLAN.md](docs/PLAN.md) : la conception, puis chaque étape avec ce qui a été fait et vérifié ;
- [NOTES.md](NOTES.md) : mon journal d'apprentissage de Go, de C# et de Blazor.

---

Mathias Knudsen
