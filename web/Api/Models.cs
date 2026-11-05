using System.Diagnostics.CodeAnalysis;
using System.Globalization;

namespace Cairn.Web.Api;

/// <summary>
/// Profil de préparateur, tel que l'API l'envoie :
/// {"id": 1, "name": "Mathias", "createdAt": "2026-11-04T07:00:00Z"}.
/// </summary>
/// <remarks>
/// Un record est une classe de données : ses propriétés sont fixées à la
/// création, et deux records aux mêmes valeurs sont égaux, comme une
/// dataclass(frozen=True) en Python. Les noms JSON en camelCase (createdAt)
/// sont associés aux propriétés (CreatedAt) par les réglages JSON de
/// CairnApiClient. La date est un DateTimeOffset, qui garde le décalage
/// horaire, et jamais un DateTime.
/// </remarks>
public record Picker(long Id, string Name, DateTimeOffset CreatedAt)
{
    /// <summary>
    /// Initiale du nom, en majuscule, pour la pastille ronde du profil.
    /// GetNextTextElement prend le premier caractère tel qu'on le voit : « É »
    /// écrit avec un accent combinant, ou un émoji, reste entier.
    /// </summary>
    public string Initial => StringInfo.GetNextTextElement(Name).ToUpperInvariant();
}

/// <summary>Statuts d'une commande, tels que l'API les écrit.</summary>
public static class OrderStatus
{
    public const string ToPrepare = "a_preparer";
    public const string InProgress = "en_cours";
    public const string Ready = "prete";
}

/// <summary>
/// Commande dans la liste (GET /api/orders), avec son avancement : lineCount
/// lignes, dont pickedCount préparées.
/// </summary>
public record OrderSummary(
    long Id,
    string Reference,
    string Customer,
    string Status,
    DateTimeOffset CreatedAt,
    int LineCount,
    int PickedCount);

/// <summary>
/// Commande complète (GET /api/orders/{id}, et réponse d'une validation),
/// avec ses lignes triées par emplacement par l'API.
/// </summary>
public record Order(
    long Id,
    string Reference,
    string Customer,
    string Status,
    DateTimeOffset CreatedAt,
    IReadOnlyList<OrderLine> Lines)
{
    /// <summary>Nombre de lignes déjà préparées.</summary>
    public int PickedCount => Lines.Count(line => line.IsPicked);
}

/// <summary>
/// Ligne d'une commande : un produit, son emplacement, la quantité demandée
/// et le stock actuel. PickedAt et PickedBy sont null tant que la ligne n'est
/// pas préparée.
/// </summary>
public record OrderLine(
    long Id,
    long ProductId,
    string Sku,
    string ProductName,
    string Location,
    int Quantity,
    int StockQty,
    DateTimeOffset? PickedAt,
    string? PickedBy)
{
    /// <summary>Vrai si la ligne est préparée.</summary>
    public bool IsPicked => PickedAt is not null;
}

/// <summary>
/// Corps d'une réponse d'erreur de l'API, au format unique
/// {"error": "code", "message": "..."} (docs/PLAN.md, section 4.2).
/// </summary>
public record ApiErrorBody(string Error, string Message);

/// <summary>Raison d'un échec d'appel à l'API.</summary>
public enum ApiErrorKind
{
    /// <summary>Pas de réponse : API arrêtée, réseau coupé, délai dépassé.</summary>
    Unreachable,

    /// <summary>
    /// L'API démarre : un intermédiaire répond à sa place, par un code 502,
    /// 503 ou 504, ou par une réponse qui n'est pas du JSON (page HTML). Avec
    /// deux services sur Render, l'interface recevait un 502 tant que l'API
    /// dormait (docs/PLAN.md, étapes 11 bis et 11 ter).
    /// </summary>
    Starting,

    /// <summary>L'API a répondu par une erreur au format prévu (Code et Message renseignés).</summary>
    Rejected,

    /// <summary>
    /// L'API a répondu en JSON, mais pas comme prévu : JSON illisible ou
    /// incomplet, ou erreur qui n'est pas au format prévu.
    /// </summary>
    Unexpected,
}

/// <summary>
/// Échec d'un appel à l'API. Status est le code HTTP reçu (null sans réponse) ;
/// Code et Message sont ceux de l'API pour Rejected (par exemple
/// « profil_existant »), null sinon.
/// </summary>
public record ApiError(ApiErrorKind Kind, int? Status = null, string? Code = null, string? Message = null)
{
    /// <summary>
    /// Message affiché pendant qu'une lecture attend le démarrage de l'API : à
    /// la place de « Chargement... », ou en bas de l'écran si la page reste
    /// affichée (Actualiser, relecture après une validation).
    /// </summary>
    public const string StartingWaitMessage = "Cairn démarre. Cela peut prendre une minute.";

    /// <summary>
    /// Message à afficher quand la page n'en prévoit pas de plus précis pour ce
    /// code d'erreur. Les messages de l'API sont écrits pour le préparateur :
    /// ils sont repris tels quels.
    /// </summary>
    public string UserMessage => this switch
    {
        { Kind: ApiErrorKind.Rejected, Message: { } message } => message,
        { Kind: ApiErrorKind.Unreachable } => "Le serveur ne répond pas. Réessayez dans un instant.",
        { Kind: ApiErrorKind.Starting } => "Cairn démarre. Réessayez dans un instant.",
        _ => "Le serveur a envoyé une réponse inattendue. Réessayez dans un instant.",
    };
}

/// <summary>
/// Résultat d'un appel à l'API : la valeur reçue (Value) ou la raison de
/// l'échec (Error), jamais les deux.
/// </summary>
/// <remarks>
/// Une API absente ou une réponse inattendue n'est pas une exception, mais un
/// état que la page affiche. Usage :
/// <code>
/// var result = await Api.GetPickersAsync();
/// if (result.IsSuccess) { ... result.Value ... } else { ... result.Error ... }
/// </code>
/// Les attributs MemberNotNullWhen disent au compilateur que Value n'est pas
/// null quand IsSuccess est vrai, et Error quand il est faux : il n'avertit
/// donc pas d'un null possible dans chaque branche.
/// </remarks>
public sealed record ApiResult<T>
{
    private ApiResult(T? value, ApiError? error)
    {
        Value = value;
        Error = error;
    }

    /// <summary>La valeur reçue, si l'appel a réussi.</summary>
    public T? Value { get; }

    /// <summary>La raison de l'échec, si l'appel a échoué.</summary>
    public ApiError? Error { get; }

    /// <summary>Vrai si l'appel a réussi.</summary>
    [MemberNotNullWhen(true, nameof(Value))]
    [MemberNotNullWhen(false, nameof(Error))]
    public bool IsSuccess => Error is null;

    /// <summary>Résultat d'un appel réussi.</summary>
    public static ApiResult<T> Success(T value) => new(value, null);

    /// <summary>Résultat d'un appel échoué.</summary>
    public static ApiResult<T> Failure(ApiError error) => new(default, error);
}
