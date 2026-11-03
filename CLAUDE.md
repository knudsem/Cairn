# Règles de travail pour Claude Code

## Le projet
Cairn est un prototype de suivi des préparations de commandes en entrepôt : API en Go (`net/http`, `pgx`), interface Blazor (.NET 10, Interactive Server), PostgreSQL, Docker Compose. Le scope fait référence : `SCOPE.md`. Le plan détaillé est dans `docs/PLAN.md`.

Je découvre Go, C# et Blazor. Je connais bien Python, Django, JavaScript, TypeScript et React.

## Comment travailler
1. Avant chaque étape, enquête dans le code et propose un plan. N'écris le code qu'après ma validation.
2. Avance par petites étapes. Une étape = une vérification = un commit (que je fais moi-même).
3. Explique en français simple ce que fait chaque fichier et chaque bloc important, pour que je puisse l'expliquer ligne par ligne. Compare avec Python, Django, JavaScript ou React quand ça aide.
4. Vérifie ton travail toi-même : compile, lance les tests et la stack avant de me dire qu'une étape est finie.
5. Quand je dois vérifier quelque chose dans le navigateur, donne l'URL exacte, quoi regarder et le résultat attendu.
6. À la fin de chaque étape, rappelle-moi de compléter `NOTES.md` avec mes propres mots.
7. Si une idée est mauvaise ou si on prend du retard, dis-le franchement. En cas de retard, on coupe les bonus, jamais les tests, le README ni la préparation de la présentation.

## Règles strictes
- Ne fais jamais de commit, de push ni de pull request. Je committe moi-même.
- Je travaille sous Windows : donne les commandes en PowerShell (une commande par ligne, `curl.exe` et non `curl`, `$env:VAR="valeur"` pour les variables d'environnement).
- Les fichiers texte sont en UTF-8 avec des fins de ligne LF (voir `.editorconfig` et `.gitattributes`).
- Ne touche jamais à la logique de validation et de stock sans test qui la couvre.
- Garde le code simple : pas de couche, de librairie ou d'abstraction qui n'est pas justifiée par un besoin réel.
- N'utilise jamais de tiret long (cadratin) dans les textes, le code ou les commentaires.
