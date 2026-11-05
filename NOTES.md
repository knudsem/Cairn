# Journal d'apprentissage

Ce journal suit ma prise en main de Go, de C# / .NET et de Blazor pendant la réalisation de Cairn.
## Go

### Ce que j'ai compris de son fonctionnement

Go est un langage compilé et typé. On compile le code et on obtient un seul exécutable qui contient tout. Pas besoin d'installer Go ni de dépendances sur le serveur pour le faire tourner.

L'API de Cairn utilise surtout la bibliothèque standard. Le paquet `net/http` suffit pour créer un serveur et déclarer les routes. Depuis Go 1.22, une route peut préciser sa méthode, par exemple `GET /api/pickers` ou `POST /api/demo/reset`. Chaque requête est traitée dans sa propre goroutine, une tâche légère que Go gère tout seul.

Pour la base, Cairn utilise `pgx` avec un pool de connexions (`pgxpool`). Les requêtes SQL sont écrites à la main : il n'y a pas d'ORM comme dans Django.

Les erreurs ne sont pas des exceptions. Une fonction renvoie son résultat et une erreur, et on vérifie l'erreur juste après avec `if err != nil`.

Le paramètre `ctx` (le contexte) passe d'une fonction à l'autre. Il permet d'arrêter une requête en base si le client abandonne ou si un délai est dépassé.

Les données de démo sont embarquées dans l'exécutable avec `go:embed`. C'est ce qui permet au bouton « Réinitialiser la démo » de remettre la base à zéro sans fichier externe.

### Principaux usages et avantages

- Les API et les services qui reçoivent beaucoup de requêtes en même temps.
- Les outils en ligne de commande et les petits services internes.
- Un exécutable unique, léger, rapide à démarrer et simple à déployer. Dans Cairn, l'API utilise environ 15 Mo de mémoire.
- Un langage volontairement simple : il y a souvent une seule façon d'écrire les choses, donc le code d'une autre personne se lit vite.
- Des outils fournis avec le langage : `go test`, `go vet`, `gofmt`.

### Différences avec ce que je connais (Python, Django, JavaScript, React)

- Par rapport à Python : Go est compilé et typé, donc beaucoup d'erreurs apparaissent à la compilation plutôt qu'en production.
- Pas d'exceptions : il faut vérifier chaque erreur explicitement. Ça demande plus de code que ce dont j'ai l'habitude en Python, mais on voit tout de suite où une erreur peut arriver.
- Par rapport à Django : Django fournit beaucoup de choses toutes prêtes (ORM, migrations, admin, authentification). En Go, on part de la bibliothèque standard et on choisit ses outils. Il y a moins de magie, mais plus de choses à écrire soi-même.
- Par rapport à JavaScript : JavaScript gère l'asynchrone avec `async` / `await` sur un seul fil d'exécution. Go lance des goroutines, qui peuvent vraiment tourner en parallèle.
- La mise en forme est imposée par `gofmt`, un peu comme Prettier, mais sans réglages à choisir.

### Difficultés rencontrées

- Écrire le SQL à la main, alors que j'ai l'habitude de l'ORM de Django.
- Gérer le stock quand deux personnes valident en même temps. J'ai dû comprendre les transactions, le verrouillage avec `SELECT ... FOR UPDATE` et les mises à jour conditionnelles (`WHERE stock_qty >= quantité`).
- Les `if err != nil` à chaque ligne au début. Avec le temps, j'ai compris l'intérêt : chaque erreur est traitée là où elle arrive.
- Écrire des tests qui tournent sur une vraie base PostgreSQL, séparée de la base de démo.

### Intérêt dans un environnement logistique

Je comprends pourquoi Go est utilisé pour ce genre d'applications. Dans un entrepôt, beaucoup de choses arrivent en même temps : plusieurs préparateurs, des terminaux, des échanges avec d'autres logiciels. Go gère bien ces accès simultanés, et ses exécutables légers tournent facilement sur un serveur ou une machine virtuelle. La gestion explicite des erreurs aide aussi : une erreur de stock ou de commande ne doit pas passer inaperçue.

## C# / .NET

### Ce que j'ai compris de son fonctionnement

.NET est une plateforme complète : un langage (C#), un environnement d'exécution et une grande bibliothèque standard. Cairn utilise .NET 10, une version à support long (LTS).

Le point de départ d'une application ASP.NET Core est `Program.cs`. On y crée l'application et on déclare les services dont elle a besoin. C'est l'injection de dépendances : au lieu de créer les objets soi-même, on les déclare au démarrage, et .NET les fournit aux composants qui en ont besoin.

Chaque service a une durée de vie :

- `Singleton` : un seul objet pour toute l'application.
- `Scoped` : un objet par requête. En Blazor Server, c'est un objet par connexion d'utilisateur (par circuit).
- `Transient` : un nouvel objet à chaque fois.

Dans Cairn, l'interface ne parle pas directement à la base. Elle appelle l'API Go en HTTP, à travers un client dédié (`CairnApiClient`) configuré au démarrage.

La configuration vient de `appsettings.json` et des variables d'environnement. Ça permet d'avoir des réglages différents en local et sur Render.

### Principaux usages et avantages

- Les applications d'entreprise : API, sites web, services en arrière-plan, applications Windows.
- Un typage fort et des outils très complets (compilateur, débogueur, autocomplétion) qui aident sur les gros projets.
- Des versions à support long, pratiques pour des applications qui doivent durer plusieurs années.
- Un seul langage pour le serveur et, avec Blazor, pour l'interface.
- De bonnes performances pour un environnement aussi complet.

### Différences avec ce que je connais (Python, Django, JavaScript, React)

- Par rapport à Django : Django fournit beaucoup de choses toutes prêtes. Avec ASP.NET Core, on choisit et on déclare soi-même, au démarrage, les services dont l'appli a besoin.
- Par rapport à TypeScript : C# m'a semblé familier (types, classes, génériques, `async` / `await`). C# et TypeScript ont d'ailleurs été conçus par la même personne, Anders Hejlsberg.
- Par rapport à Python : tout est typé et compilé, et l'éditeur signale beaucoup d'erreurs avant même de lancer le code.
- Un projet est plus cadré qu'en Python : un fichier `.csproj`, des espaces de noms, des classes.

### Difficultés rencontrées

- Comprendre la durée de vie des services, surtout en Blazor Server, où `Scoped` veut dire « une connexion d'utilisateur » et non « une requête ».
- Régler les délais d'attente des appels HTTP vers l'API, notamment pendant les démarrages à froid sur Render.
- Beaucoup de notions nouvelles d'un coup : l'hôte, les middlewares, l'injection de dépendances, la configuration.

### Intérêt dans un environnement logistique

.NET est très présent en entreprise, et beaucoup d'outils métier sont écrits avec. Pour des applications d'entrepôt ou de transport qui vivent longtemps et que plusieurs développeurs maintiennent, le typage fort et les versions à support long sont rassurants. Avoir le serveur et l'interface dans le même langage simplifie aussi le travail d'une petite équipe.

## Blazor

### Ce que j'ai compris de son fonctionnement

Blazor permet d'écrire des interfaces web en C#. Un écran est un composant `.razor` : du HTML, et un bloc `@code` en C# pour la logique.

Cairn utilise le mode Interactive Server. Les composants tournent sur le serveur, et le navigateur reste connecté en permanence avec SignalR (on parle de circuit). Quand l'utilisateur clique, l'événement part au serveur, le composant se met à jour, et le serveur renvoie au navigateur seulement les changements à afficher.

Chaque composant peut avoir son propre fichier CSS, par exemple `Pickers.razor.css`. Ces styles ne s'appliquent qu'au composant.

Si la connexion coupe, une fenêtre de reconnexion s'affiche. Et si une erreur n'est pas rattrapée dans un composant, le circuit s'arrête et tout l'écran ne répond plus.

### Principaux usages et avantages

- Les outils internes et les applications de gestion, utilisés sur le réseau de l'entreprise.
- Pas de JavaScript à écrire pour la plupart des écrans.
- Le même langage et les mêmes modèles de données côté serveur et côté interface.
- En mode serveur, le code et les données restent sur le serveur : le navigateur ne reçoit que l'affichage.
- Le serveur peut mettre l'écran à jour à tout moment, ce qui permettrait d'afficher des changements en direct. Je ne l'ai pas fait dans Cairn (il y a un bouton « Actualiser »), mais c'est une piste.

### Différences avec ce que je connais (Python, Django, JavaScript, React)

- Par rapport à React : on retrouve les composants, les paramètres (l'équivalent des props) et l'affichage qui se met à jour quand l'état change. Mais en React, tout tourne dans le navigateur. En Blazor Server, l'état vit sur le serveur.
- Chaque clic fait un aller-retour avec le serveur. Sur un bon réseau, on ne le sent pas, mais l'appli dépend de la qualité de la connexion.
- Par rapport aux templates Django : la page n'est pas rechargée à chaque action, seules les parties qui changent sont mises à jour.
- Je n'ai presque pas écrit de JavaScript.

### Difficultés rencontrées

- Prévoir les coupures de connexion, par exemple quand l'hébergement gratuit se met en veille.
- Éviter qu'une seule erreur bloque tout l'écran : il faut rattraper les erreurs dans les composants et afficher un message clair.
- Le double appui : avec des gants, un préparateur peut toucher deux fois le même bouton. Un deuxième appui trop rapide est maintenant ignoré.

### Intérêt dans un environnement logistique

Pour des écrans utilisés par des préparateurs, Blazor permet de construire vite des interfaces métier en C#, avec la même équipe que pour le reste. Le point à surveiller, c'est le réseau : dans un entrepôt, le Wi-Fi n'est pas toujours parfait, donc les reconnexions doivent être bien gérées. Je ne connais pas encore les contraintes réelles de Menhir et Dolmen sur ce point, et c'est une des premières choses que j'aimerais comprendre.

## Ressources consultées

| Ressource | Technologie | Ce que j'en ai retenu |
|---|---|---|
| Les explications de Claude Code, fichier par fichier | Go, C# / .NET, Blazor | Comprendre chaque fichier avant de passer à l'étape suivante. |
| Les logs de Render | Hébergement | Les erreurs 502 venaient d'une API qui ne démarrait pas : un service gratuit n'en réveille pas un autre. |
| La console du navigateur | Blazor, hébergement | Mon bloqueur de publicité bloquait les appels de réveil (`ERR_BLOCKED_BY_CLIENT`). |

## Expérimentations

| Étape | Expérience | Résultat | Ce que j'en retiens |
|---|---|---|---|
| Gestion du stock | 20 validations lancées en même temps sur la dernière unité en stock | Une seule est acceptée, le stock reste correct | Le verrouillage et les mises à jour conditionnelles font leur travail. |
| Tests | J'ai cassé exprès une condition de stock, puis une suppression | Les tests ont échoué, comme prévu | Il faut vérifier qu'un test sait échouer. |
| Données de démo | CMD-0002 et CMD-0003 demandent chacune 4 rouleaux de papier bulle, pour 5 en stock | La première validée passe, l'autre est refusée avec un message | Un refus doit dire quoi faire. |
| Mise en ligne | L'API et l'interface dans deux services Render séparés | Erreurs 502 après la mise en veille | Lire les logs avant de chercher une solution. |
| Mise en ligne | Réveiller l'API depuis le navigateur | Bloqué par mon propre bloqueur de publicité | Tester dans les conditions d'un vrai visiteur. |
| Mise en ligne | L'API et l'interface regroupées dans un seul conteneur | Une visite suffit à tout démarrer, même avec un bloqueur actif | Simplifier l'architecture a réglé le problème. |
| Mise en ligne | Mesure de la mémoire sur l'offre gratuite | Environ 15 Mo pour l'API et 145 Mo pour l'interface, sur 512 Mo | Il reste de la marge pour les faire tourner ensemble. |
| Interface | Parcours automatiques sur ordinateur, tablette et téléphone | Les écrans s'adaptent aux trois tailles | Tester plusieurs tailles d'écran dès le début. |
| Interface | Ignorer un deuxième appui trop rapide | Un double appui ne valide qu'une fois | Penser au terrain, pas seulement à l'écran. |
| Remise à zéro | Bouton « Réinitialiser la démo », activé par la variable `DEMO_RESET_ENABLED` | La démo revient à son état de départ en un clic | Une fonction sensible doit pouvoir se couper par un simple réglage. |