// Réveil de l'API par le navigateur (docs/PLAN.md, étape 11 bis), appelé par
// Blazor (Api/ApiWakeUp.cs). Sur Render, une API endormie ne se réveille que
// sur un appel venu de l'extérieur : le navigateur en est un, le serveur de
// l'interface non.
//
// mode "no-cors" : l'API est sur un autre site ; la requête part sans
// demande d'autorisation préalable (CORS), et sa réponse reste illisible.
// cache "no-store" : la requête part vraiment, au lieu d'une réponse gardée
// en cache. La réponse et une éventuelle erreur réseau sont ignorées : seul
// l'appel compte.
export function wakeUp(url) {
    fetch(url, { mode: "no-cors", cache: "no-store" }).catch(() => { });
}
