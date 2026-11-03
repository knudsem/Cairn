namespace Cairn.Web.Api;

/// <summary>État de l'API, tel que l'interface le voit.</summary>
public enum ApiHealth
{
    /// <summary>L'API et la base de données répondent (200).</summary>
    Ok,

    /// <summary>L'API répond, mais pas la base de données (503).</summary>
    DatabaseUnavailable,

    /// <summary>L'API ne répond pas, ou répond de façon inattendue.</summary>
    Unreachable,
}
