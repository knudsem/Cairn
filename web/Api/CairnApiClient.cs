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

    /// <summary>
    /// Interroge GET /health et traduit la réponse en <see cref="ApiHealth"/>.
    /// Une API injoignable n'est pas une exception : c'est un état à afficher.
    /// </summary>
    public async Task<ApiHealth> GetHealthAsync()
    {
        try
        {
            using var response = await http.GetAsync("health");
            if (response.StatusCode == HttpStatusCode.OK)
            {
                return ApiHealth.Ok;
            }
            if (response.StatusCode == HttpStatusCode.ServiceUnavailable)
            {
                return ApiHealth.DatabaseUnavailable;
            }
            logger.LogWarning("GET /health : réponse inattendue {StatusCode}", (int)response.StatusCode);
            return ApiHealth.Unreachable;
        }
        catch (HttpRequestException ex)
        {
            // Connexion impossible : API arrêtée, nom inconnu, port fermé.
            logger.LogWarning("GET /health : API injoignable ({Message})", ex.Message);
            return ApiHealth.Unreachable;
        }
        catch (TaskCanceledException)
        {
            // Délai maximal de HttpClient dépassé (voir Program.cs).
            logger.LogWarning("GET /health : pas de réponse dans le délai");
            return ApiHealth.Unreachable;
        }
    }

    /// <summary>GET /api/pickers : les profils, déjà triés par nom par l'API.</summary>
    public Task<ApiResult<IReadOnlyList<Picker>>> GetPickersAsync() =>
        SendAsync<IReadOnlyList<Picker>>(HttpMethod.Get, "api/pickers", null, HttpStatusCode.OK);

    /// <summary>
    /// POST /api/pickers : crée le profil name. En cas de succès, la valeur est
    /// le profil créé, avec le nom tel que l'API l'a enregistré (espaces en
    /// trop retirées). Refus prévus (Rejected) : 409 profil_existant, 409
    /// trop_de_profils, 400 requete_invalide.
    /// </summary>
    public Task<ApiResult<Picker>> CreatePickerAsync(string name) =>
        SendAsync<Picker>(HttpMethod.Post, "api/pickers", JsonContent.Create(new { name }, options: Json), HttpStatusCode.Created);

    /// <summary>
    /// Envoie la requête et lit la réponse : la valeur JSON si le code HTTP est
    /// expected, sinon la raison de l'échec. Aucune exception ne sort de cette
    /// méthode pour une API absente ou une réponse inattendue : dans un
    /// composant Blazor, une exception non rattrapée couperait le circuit, et
    /// la page cesserait de répondre.
    /// </summary>
    private async Task<ApiResult<T>> SendAsync<T>(HttpMethod method, string path, HttpContent? content, HttpStatusCode expected)
    {
        using var request = new HttpRequestMessage(method, path) { Content = content };
        var call = $"{method} {path}";
        int? status = null;
        try
        {
            // SendAsync lit tout le corps de la réponse avant de rendre la
            // main : une coupure en cours de lecture arrive ici, sous forme
            // de HttpRequestException.
            using var response = await http.SendAsync(request);
            status = (int)response.StatusCode;
            if (response.StatusCode != expected)
            {
                return ApiResult<T>.Failure(await ReadErrorAsync(response, call));
            }
            if (!IsJson(response))
            {
                logger.LogWarning("{Call} : réponse {Status} qui n'est pas du JSON", call, status);
                return ApiResult<T>.Failure(new ApiError(ApiErrorKind.Unexpected, status));
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
            // Délai maximal de HttpClient dépassé (voir Program.cs).
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
    /// Traduit une réponse d'erreur. Son JSON n'est lu que si elle annonce du
    /// JSON : c'est alors le format d'erreur unique de l'API. Le routeur de Go
    /// (404 ou 405 en texte brut) ou un intermédiaire (page HTML) peuvent
    /// répondre autrement : c'est une réponse inattendue.
    /// </summary>
    private async Task<ApiError> ReadErrorAsync(HttpResponseMessage response, string call)
    {
        var status = (int)response.StatusCode;
        if (IsJson(response))
        {
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
        }
        logger.LogWarning("{Call} : réponse inattendue {Status}", call, status);
        return new ApiError(ApiErrorKind.Unexpected, status);
    }

    /// <summary>Vrai si la réponse annonce du JSON (en-tête Content-Type).</summary>
    private static bool IsJson(HttpResponseMessage response) =>
        string.Equals(response.Content.Headers.ContentType?.MediaType, "application/json", StringComparison.OrdinalIgnoreCase);
}
