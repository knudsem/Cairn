using Microsoft.JSInterop;

namespace Cairn.Web.Api;

/// <summary>
/// Fait réveiller l'API par le navigateur du préparateur (docs/PLAN.md,
/// étape 11 bis).
/// </summary>
/// <remarks>
/// Sur Render, un service gratuit endormi ne se réveille que sur un appel venu
/// de l'extérieur. Ceux de l'interface, hébergée chez Render, reçoivent un 502
/// et ne le réveillent pas (journaux du 4 novembre). Le navigateur, lui, est à
/// l'extérieur : il envoie un GET /health dont la réponse est ignorée (module
/// wwwroot/js/api-wake-up.js). Les lectures de CairnApiClient, réessayées
/// pendant ce temps, aboutissent une fois l'API réveillée.
///
/// Appelé au premier affichage (MainLayout), puis avant chaque nouvel essai
/// d'une lecture (API qui démarre ou ne répond pas) et après une écriture
/// qui reçoit « l'API démarre », au plus une fois toutes les 10 secondes.
/// Enregistré en « scoped » (Program.cs) : une instance par circuit, donc par
/// onglet ; l'adresse de l'API vient de la configuration ApiBaseUrl. Si
/// healthUrl est null (réveil coupé par ApiWakeUpFromBrowser, dans Compose),
/// WakeUpAsync ne fait rien.
/// </remarks>
public class ApiWakeUp(IJSRuntime js, Uri? healthUrl, ILogger<ApiWakeUp> logger)
{
    private static readonly TimeSpan MinInterval = TimeSpan.FromSeconds(10);

    // Attente maximale d'un appel au navigateur. Par défaut, une minute : un
    // appel perdu pendant une coupure du réseau bloquerait la lecture qui
    // attend l'API, au-delà de son propre délai.
    private static readonly TimeSpan JsTimeout = TimeSpan.FromSeconds(5);

    // Heure du dernier appel, et module JavaScript, chargé au premier appel.
    private DateTime lastCall = DateTime.MinValue;
    private IJSObjectReference? module;

    /// <summary>
    /// Demande au navigateur d'appeler GET /health de l'API, sauf si un appel
    /// date de moins de 10 secondes. Ne lève pas d'exception : au pire, l'API
    /// n'est pas réveillée, et la page affiche l'erreur après le délai.
    /// </summary>
    public async Task WakeUpAsync()
    {
        if (healthUrl is null || DateTime.UtcNow - lastCall < MinInterval)
        {
            return;
        }
        lastCall = DateTime.UtcNow;
        try
        {
            // import() charge le module une seule fois par onglet, comme un
            // import dynamique en JavaScript.
            module ??= await js.InvokeAsync<IJSObjectReference>("import", JsTimeout, "./js/api-wake-up.js");
            await module.InvokeVoidAsync("wakeUp", JsTimeout, healthUrl.ToString());
            logger.LogInformation("Réveil de l'API demandé au navigateur");
        }
        catch (Exception ex) when (ex is JSException or JSDisconnectedException or OperationCanceledException or InvalidOperationException)
        {
            // Erreur dans le navigateur, onglet fermé ou circuit coupé : sans
            // effet sur la page.
            logger.LogInformation("Réveil de l'API par le navigateur impossible ({Message})", ex.Message);
        }
    }
}
