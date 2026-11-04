using Cairn.Web.Api;
using Cairn.Web.Components;
using Cairn.Web.State;

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
// Délai maximal d'un appel à l'API : 5 secondes par défaut, car le délai de
// HttpClient (100 secondes) est bien trop long sur une tablette. En ligne
// (Render, docs/PLAN.md étape 11), ApiTimeoutSeconds vaut 90 : une API
// endormie met environ une minute à se réveiller, et l'appel doit l'attendre.
var apiTimeout = TimeSpan.FromSeconds(builder.Configuration.GetValue("ApiTimeoutSeconds", 5));
builder.Services.AddHttpClient<CairnApiClient>(client =>
{
    client.BaseAddress = apiBaseUrl;
    client.Timeout = apiTimeout;
});

// Profil du préparateur choisi. « Scoped » : une instance par circuit Blazor,
// donc par onglet ouvert (State/PickerState.cs).
builder.Services.AddScoped<PickerState>();

var app = builder.Build();

// Au démarrage de l'interface, un appel à /health réveille l'API en même
// temps qu'elle : sur Render, les deux services s'endorment ensemble, et le
// premier préparateur attend ainsi un seul réveil au lieu de deux à la suite.
// L'appel part en arrière-plan (« _ = ») : le démarrage ne l'attend pas, et
// GetHealthAsync ne lève pas d'exception.
app.Lifetime.ApplicationStarted.Register(() =>
    _ = app.Services.GetRequiredService<CairnApiClient>().GetHealthAsync());

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
