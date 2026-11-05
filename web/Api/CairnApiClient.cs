using System.Diagnostics;
using System.Net;
using System.Net.Http.Json;
using System.Text.Json;

namespace Cairn.Web.Api;

/// <summary>
/// Client de l'API Go : le seul endroit de l'interface qui connaît les URL de
/// l'API. Les pages appellent ses méthodes, jamais HttpClient directement.
/// </summary>
/// <remarks>
/// Enregistré dans Program.cs avec AddHttpClient : ASP.NET Core lui fournit un
/// HttpClient déjà réglé (adresse de base, délai maximal). Les chemins sont
/// relatifs, sans barre oblique au début (« api/pickers ») : ils s'ajoutent à
/// l'adresse de base, quelle qu'elle soit (Docker Compose ou Render).
///
/// Démarrage de l'API (docs/PLAN.md, étape 11 bis) : une lecture (GET) est
/// réessayée tant que l'API démarre, dans la limite du délai configuré ; une
/// écriture (POST, DELETE) n'est jamais renvoyée sans action du préparateur.
/// </remarks>
public class CairnApiClient(HttpClient http, ILogger<CairnApiClient> logger)
{
    /// <summary>
    /// Réglages JSON. JsonSerializerDefaults.Web associe les noms camelCase du
    /// JSON (createdAt) aux propriétés C# (CreatedAt), comme les méthodes JSON
    /// de HttpClient. Les deux options suivantes font respecter les types
    /// déclarés dans Models.cs : un champ absent, ou null là où le type ne
    /// l'accepte pas, fait échouer la lecture (JsonException), au lieu de
    /// donner un objet à moitié rempli qui planterait plus loin. Deux limites
    /// connues : un nombre écrit en chaîne (« "id": "1" ») est accepté, et un
    /// élément null dans une liste aussi. L'API Go n'envoie ni l'un ni l'autre.
    /// </summary>
    private static readonly JsonSerializerOptions Json = new(JsonSerializerDefaults.Web)
    {
        RespectNullableAnnotations = true,
        RespectRequiredConstructorParameters = true,
    };

    /// <summary>Attente entre deux essais d'une lecture, pendant que l'API démarre.</summary>
    private static readonly TimeSpan RetryDelay = TimeSpan.FromSeconds(3);

    /// <summary>
    /// GET /api/pickers : les profils, déjà triés par nom par l'API. Pour
    /// onStarting, voir <see cref="GetAsync{T}"/>.
    /// </summary>
    public Task<ApiResult<IReadOnlyList<Picker>>> GetPickersAsync(Func<Task>? onStarting = null) =>
        GetAsync<IReadOnlyList<Picker>>("api/pickers", onStarting);

    /// <summary>
    /// POST /api/pickers : crée le profil name. En cas de succès, la valeur est
    /// le profil créé, avec le nom tel que l'API l'a enregistré (espaces en
    /// trop retirées). Refus prévus (Rejected) : 409 profil_existant, 409
    /// trop_de_profils, 400 requete_invalide.
    /// </summary>
    public Task<ApiResult<Picker>> CreatePickerAsync(string name) =>
        SendAsync<Picker>(HttpMethod.Post, "api/pickers", JsonContent.Create(new { name }, options: Json), HttpStatusCode.Created);

    /// <summary>
    /// DELETE /api/pickers/{id} : supprime le profil (étape 13). Succès : 204,
    /// sans corps. Refus prévus (Rejected) : 404 profil_introuvable (déjà
    /// supprimé ailleurs), 400. Un seul envoi, comme toute écriture.
    /// </summary>
    public Task<ApiResult<NoContent>> DeletePickerAsync(long id) =>
        SendAsync<NoContent>(HttpMethod.Delete, $"api/pickers/{id}", null, HttpStatusCode.NoContent);

    /// <summary>
    /// Vrai si le profil existe encore, d'après GET /api/pickers (20 profils
    /// au plus) : un profil choisi dans cet onglet a pu être supprimé depuis un
    /// autre (étape 13), ou effacé par la remise à zéro de la démo (étape 15).
    /// L'identifiant et le nom doivent correspondre : après une remise à zéro,
    /// les identifiants repartent de 1, et un nouveau profil peut reprendre
    /// celui d'un profil disparu. Lecture réessayée pendant le démarrage de
    /// l'API, comme les autres ; en cas d'échec, l'erreur de la lecture.
    /// </summary>
    public async Task<ApiResult<bool>> PickerExistsAsync(Picker picker, Func<Task>? onStarting = null)
    {
        var result = await GetPickersAsync(onStarting);
        return result.IsSuccess
            ? ApiResult<bool>.Success(result.Value.Any(p => p.Id == picker.Id && p.Name == picker.Name))
            : ApiResult<bool>.Failure(result.Error);
    }

    /// <summary>
    /// POST /api/demo/reset : remet les commandes, le stock et les profils à
    /// leur état de départ (étape 15). Succès : 204, sans corps. La route
    /// n'existe que si l'API a DEMO_RESET_ENABLED=true ; « Qui prépare ? »
    /// n'affiche le bouton qu'avec la même variable. Un seul envoi, comme
    /// toute écriture ; la recommencer est sans risque (même résultat).
    /// </summary>
    public Task<ApiResult<NoContent>> ResetDemoAsync() =>
        SendAsync<NoContent>(HttpMethod.Post, "api/demo/reset", null, HttpStatusCode.NoContent);

    /// <summary>GET /api/orders : les commandes, la plus ancienne en premier.</summary>
    public Task<ApiResult<IReadOnlyList<OrderSummary>>> GetOrdersAsync(Func<Task>? onStarting = null) =>
        GetAsync<IReadOnlyList<OrderSummary>>("api/orders", onStarting);

    /// <summary>
    /// GET /api/orders/{id} : la commande et ses lignes. Refus prévu
    /// (Rejected) : 404 commande_introuvable.
    /// </summary>
    public Task<ApiResult<Order>> GetOrderAsync(long id, Func<Task>? onStarting = null) =>
        GetAsync<Order>($"api/orders/{id}", onStarting);

    /// <summary>
    /// POST /api/orders/{orderId}/lines/{lineId}/pick : valide la ligne au nom
    /// de picker. En cas de succès, la valeur est la commande mise à jour.
    /// Refus prévus (Rejected) : 409 stock_insuffisant, 409 deja_preparee,
    /// 404, 400. Sans réponse, la validation a pu être enregistrée : la page
    /// relit alors la commande. Un seul envoi, même si l'API démarre.
    /// </summary>
    public Task<ApiResult<Order>> PickLineAsync(long orderId, long lineId, string picker) =>
        SendAsync<Order>(HttpMethod.Post, $"api/orders/{orderId}/lines/{lineId}/pick",
            JsonContent.Create(new { picker }, options: Json), HttpStatusCode.OK);

    /// <summary>
    /// Lecture (GET), réessayée tant que l'API démarre (Starting) ou ne répond
    /// pas (Unreachable), toutes les <see cref="RetryDelay"/>. Le délai
    /// configuré (http.Timeout : 30 s sur Render, 5 s en local) borne la
    /// lecture entière, essais et attentes compris : passé ce délai, le
    /// dernier échec est rendu à la page. Avant chaque attente, onStarting
    /// prévient la page : elle affiche « Cairn démarre » et demande le réveil
    /// par le navigateur (ApiWakeUp), sans effet s'il est coupé, comme sur
    /// Render et dans Compose. Une lecture peut être renvoyée
    /// sans risque : elle ne change rien. Un essai coupé par le délai rend
    /// l'échec de l'essai précédent, plus parlant qu'une simple annulation.
    /// </summary>
    private async Task<ApiResult<T>> GetAsync<T>(string path, Func<Task>? onStarting)
    {
        var clock = Stopwatch.StartNew();
        // Annule l'essai en cours une fois le délai configuré écoulé depuis
        // le premier essai.
        using var cancel = new CancellationTokenSource(http.Timeout);
        var attempts = 1;
        ApiResult<T>? previous = null;
        while (true)
        {
            var result = await SendAsync<T>(HttpMethod.Get, path, null, HttpStatusCode.OK, cancel.Token);
            if (cancel.IsCancellationRequested && previous is not null)
            {
                result = previous;
            }
            if (result.IsSuccess || result.Error.Kind is not (ApiErrorKind.Starting or ApiErrorKind.Unreachable))
            {
                if (attempts > 1)
                {
                    // Sur Render, donne la durée réelle du réveil de l'API.
                    logger.LogInformation("GET {Path} : réponse après {Attempts} essais, en {Seconds:0} s",
                        path, attempts, clock.Elapsed.TotalSeconds);
                }
                return result;
            }
            if (clock.Elapsed + RetryDelay >= http.Timeout)
            {
                logger.LogWarning("GET {Path} : l'API ne répond toujours pas après {Attempts} essais, en {Seconds:0} s",
                    path, attempts, clock.Elapsed.TotalSeconds);
                return result;
            }
            if (onStarting is not null)
            {
                await onStarting();
            }
            await Task.Delay(RetryDelay);
            previous = result;
            attempts++;
        }
    }

    /// <summary>
    /// Envoie la requête une seule fois et lit la réponse : la valeur JSON si
    /// le code HTTP est expected, sinon la raison de l'échec. Le délai de
    /// HttpClient s'applique, et cancellationToken peut l'abréger (lecture
    /// réessayée, voir GetAsync). Aucune exception ne sort de cette
    /// méthode pour une API absente ou une réponse inattendue : dans un
    /// composant Blazor, une exception non rattrapée couperait le circuit, et
    /// la page cesserait de répondre.
    /// </summary>
    private async Task<ApiResult<T>> SendAsync<T>(HttpMethod method, string path, HttpContent? content,
        HttpStatusCode expected, CancellationToken cancellationToken = default)
    {
        using var request = new HttpRequestMessage(method, path) { Content = content };
        var call = $"{method} {path}";
        int? status = null;
        try
        {
            // SendAsync lit tout le corps de la réponse avant de rendre la
            // main : une coupure en cours de lecture arrive ici, sous forme
            // de HttpRequestException.
            using var response = await http.SendAsync(request, cancellationToken);
            status = (int)response.StatusCode;
            // Succès sans corps (204, suppression d'un profil) : rien à lire.
            // Ce test passe avant IsStarting, qui prendrait cette réponse sans
            // type pour la page d'un intermédiaire.
            if (expected == HttpStatusCode.NoContent && response.StatusCode == expected)
            {
                return ApiResult<T>.Success(default!);
            }
            if (IsStarting(response))
            {
                // Une ligne par essai, avec le code et le type reçus : de quoi
                // savoir, dans les journaux de Render, ce qui a répondu.
                logger.LogInformation("{Call} : l'API démarre (réponse {Status} {ContentType})",
                    call, status, response.Content.Headers.ContentType?.MediaType ?? "sans type");
                return ApiResult<T>.Failure(new ApiError(ApiErrorKind.Starting, status));
            }
            if (response.StatusCode != expected)
            {
                return ApiResult<T>.Failure(await ReadErrorAsync(response, call));
            }
            var value = await response.Content.ReadFromJsonAsync<T>(Json);
            if (value is null)
            {
                logger.LogWarning("{Call} : réponse JSON vide (null)", call);
                return ApiResult<T>.Failure(new ApiError(ApiErrorKind.Unexpected, status));
            }
            return ApiResult<T>.Success(value);
        }
        catch (HttpRequestException ex)
        {
            // Connexion impossible ou coupée : API arrêtée, nom inconnu, port fermé.
            logger.LogWarning("{Call} : API injoignable ({Message})", call, ex.Message);
            return ApiResult<T>.Failure(new ApiError(ApiErrorKind.Unreachable));
        }
        catch (TaskCanceledException)
        {
            // Délai dépassé : celui de HttpClient (voir Program.cs), ou celui
            // d'une lecture réessayée (GetAsync).
            logger.LogWarning("{Call} : pas de réponse dans le délai", call);
            return ApiResult<T>.Failure(new ApiError(ApiErrorKind.Unreachable));
        }
        catch (Exception ex) when (ex is JsonException or InvalidOperationException)
        {
            // JSON illisible, qui ne respecte pas les types de Models.cs, ou
            // annoncé dans un jeu de caractères inconnu (InvalidOperationException).
            logger.LogWarning("{Call} : JSON inattendu ({Message})", call, ex.Message);
            return ApiResult<T>.Failure(new ApiError(ApiErrorKind.Unexpected, status));
        }
    }

    /// <summary>
    /// Vrai si la réponse vient d'un intermédiaire pendant le démarrage de
    /// l'API, et non de l'API : un code 502, 503 ou 504, ou une réponse qui
    /// n'est pas du JSON (page HTML d'attente ou d'erreur). L'API répond
    /// toujours en JSON, sauf son routeur pour une route inconnue (404 ou 405
    /// en texte brut), ce que le client ne demande jamais, et la suppression
    /// d'un profil (204 sans corps), reconnue avant ce test (SendAsync).
    /// </summary>
    private static bool IsStarting(HttpResponseMessage response) =>
        response.StatusCode is HttpStatusCode.BadGateway or HttpStatusCode.ServiceUnavailable or HttpStatusCode.GatewayTimeout
        || !string.Equals(response.Content.Headers.ContentType?.MediaType, "application/json", StringComparison.OrdinalIgnoreCase);

    /// <summary>
    /// Traduit une réponse d'erreur en JSON : c'est normalement le format
    /// d'erreur unique de l'API (Rejected). Un JSON d'une autre forme est une
    /// réponse inattendue.
    /// </summary>
    private async Task<ApiError> ReadErrorAsync(HttpResponseMessage response, string call)
    {
        var status = (int)response.StatusCode;
        try
        {
            var body = await response.Content.ReadFromJsonAsync<ApiErrorBody>(Json);
            if (body is not null)
            {
                // Un refus prévu (409, 400) est normal ; une erreur 500 mérite l'attention.
                logger.Log(status >= 500 ? LogLevel.Warning : LogLevel.Information,
                    "{Call} : {Status} {Code}", call, status, body.Error);
                return new ApiError(ApiErrorKind.Rejected, status, body.Error, body.Message);
            }
        }
        catch (Exception ex) when (ex is JsonException or InvalidOperationException)
        {
            // Corps illisible : traité plus bas comme une réponse inattendue.
        }
        logger.LogWarning("{Call} : réponse inattendue {Status}", call, status);
        return new ApiError(ApiErrorKind.Unexpected, status);
    }
}
