using System.Globalization;
using Cairn.Web.Api;

namespace Cairn.Web.Components;

/// <summary>
/// Textes d'affichage communs aux écrans des commandes : heures, avancement.
/// </summary>
/// <remarks>
/// Les dates de l'API sont en UTC. Le conteneur web tourne aussi en UTC : sans
/// conversion, « 08:00 » s'afficherait « 07:00 » en hiver. Les heures sont
/// donc converties vers le fuseau de l'entrepôt, Europe/Paris, heure d'été
/// comprise (les données de fuseau sont dans l'image .NET).
/// </remarks>
public static class Format
{
    private static readonly TimeZoneInfo Paris = TimeZoneInfo.FindSystemTimeZoneById("Europe/Paris");

    /// <summary>L'instant at, à l'heure de Paris.</summary>
    private static DateTimeOffset InParis(DateTimeOffset at) => TimeZoneInfo.ConvertTime(at, Paris);

    /// <summary>Heure à Paris : « 09:20 ».</summary>
    public static string Time(DateTimeOffset at) =>
        InParis(at).ToString("HH:mm", CultureInfo.InvariantCulture);

    /// <summary>
    /// Date de création d'une commande : « Créée à 07:30 » si c'est
    /// aujourd'hui (à Paris), sinon « Créée le 03/11 à 07:30 ».
    /// </summary>
    public static string Created(DateTimeOffset at)
    {
        var local = InParis(at);
        var today = InParis(DateTimeOffset.UtcNow).Date;
        return local.Date == today
            ? $"Créée à {Time(at)}"
            : $"Créée le {local.ToString("dd/MM", CultureInfo.InvariantCulture)} à {Time(at)}";
    }

    /// <summary>
    /// Suffixe des classes CSS d'un statut, commun à la pastille
    /// (badge-todo...) et à la barre d'avancement (progress-todo...).
    /// </summary>
    public static string StatusClass(string status) => status switch
    {
        OrderStatus.InProgress => "progress",
        OrderStatus.Ready => "ready",
        _ => "todo",
    };

    /// <summary>Avancement : « 1 / 3 lignes préparées », « 0 / 1 ligne préparée ».</summary>
    public static string Progress(int picked, int total) =>
        $"{picked} / {total} {(total > 1 ? "lignes préparées" : "ligne préparée")}";

    /// <summary>Avancement en pourcentage, pour la largeur de la barre.</summary>
    public static string Percent(int picked, int total) =>
        (total == 0 ? 0 : picked * 100 / total).ToString(CultureInfo.InvariantCulture) + "%";
}
