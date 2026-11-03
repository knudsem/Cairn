using Cairn.Web.Api;
using Cairn.Web.Components;

var builder = WebApplication.CreateBuilder(args);

// Composants Razor avec rendu interactif côté serveur (Blazor Server).
builder.Services.AddRazorComponents()
    .AddInteractiveServerComponents();

// Client de l'API Go. Son adresse vient de la configuration : appsettings.json
// en local, variable d'environnement ApiBaseUrl dans Docker (compose.yaml).
// new Uri vérifie l'adresse dès le démarrage : une valeur absente, vide ou mal
// formée arrête l'application avec un message clair, au lieu d'une page blanche.
var apiBaseUrl = new Uri(builder.Configuration["ApiBaseUrl"]
    ?? throw new InvalidOperationException("La configuration ApiBaseUrl est manquante."));
builder.Services.AddHttpClient<CairnApiClient>(client =>
{
    client.BaseAddress = apiBaseUrl;
    // Le délai par défaut est de 100 secondes : bien trop long sur une tablette.
    client.Timeout = TimeSpan.FromSeconds(5);
});

var app = builder.Build();

// Traitement des requêtes HTTP : page d'erreur hors développement, page 404,
// protection antifalsification, fichiers statiques, puis composants Razor.
if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Error", createScopeForErrors: true);
}
app.UseStatusCodePagesWithReExecute("/not-found", createScopeForStatusCodePages: true);
app.UseAntiforgery();

app.MapStaticAssets();
app.MapRazorComponents<App>()
    .AddInteractiveServerRenderMode();

app.Run();
