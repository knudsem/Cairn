using System.Net;

namespace Cairn.Web.Api;

/// <summary>
/// Client de l'API Go : le seul endroit de l'interface qui connaît les URL de
/// l'API. Les pages appellent ses méthodes, jamais HttpClient directement.
/// </summary>
/// <remarks>
/// Enregistré dans Program.cs avec AddHttpClient : ASP.NET Core lui fournit un
/// HttpClient déjà réglé (adresse de base, délai maximal).
/// </remarks>
public class CairnApiClient(HttpClient http, ILogger<CairnApiClient> logger)
{
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
}
