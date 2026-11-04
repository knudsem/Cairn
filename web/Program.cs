using Cairn.Web.Api;
using Cairn.Web.Components;
using Cairn.Web.State;
using Microsoft.JSInterop;

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
// (Render, docs/PLAN.md étapes 11 et 11 bis), ApiTimeoutSeconds vaut 90 : une
// API endormie met du temps à se réveiller. Pour une lecture, ce délai couvre
// tous les essais (CairnApiClient.GetAsync).
var apiTimeout = TimeSpan.FromSeconds(builder.Configuration.GetValue("ApiTimeoutSeconds", 5));
builder.Services.AddHttpClient<CairnApiClient>(client =>
{
    client.BaseAddress = apiBaseUrl;
    client.Timeout = apiTimeout;
});

// Réveil de l'API par le navigateur (Api/ApiWakeUp.cs), avec l'adresse de
// /health tirée d'ApiBaseUrl. « Scoped » : une instance par circuit. Actif
// par défaut ; compose.yaml le coupe (ApiWakeUpFromBrowser=false) : le
// navigateur ne peut pas joindre http://api:8080/, nom connu seulement à
// l'intérieur de Docker, et chaque appel laisserait une erreur dans sa
// console. En local, l'API ne s'endort pas.
var wakeUpUrl = builder.Configuration.GetValue("ApiWakeUpFromBrowser", true) ? new Uri(apiBaseUrl, "health") : null;
builder.Services.AddScoped(services => new ApiWakeUp(
    services.GetRequiredService<IJSRuntime>(),
    wakeUpUrl,
    services.GetRequiredService<ILogger<ApiWakeUp>>()));

// Profil du préparateur choisi. « Scoped » : une instance par circuit Blazor,
// donc par onglet ouvert (State/PickerState.cs).
builder.Services.AddScoped<PickerState>();

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
