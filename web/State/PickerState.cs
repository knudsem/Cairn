using Cairn.Web.Api;

namespace Cairn.Web.State;

/// <summary>
/// Profil du préparateur choisi pour la session (docs/PLAN.md, étape 7 bis).
/// </summary>
/// <remarks>
/// Enregistré en « scoped » dans Program.cs. Avec Blazor Server, cela fait une
/// instance par circuit, c'est-à-dire par onglet ouvert : chaque tablette
/// garde son propre profil. C'est un peu une session Django, mais propre à
/// l'onglet et perdue au rechargement : le préparateur choisit alors à
/// nouveau son profil.
/// </remarks>
public class PickerState
{
    /// <summary>Le profil choisi, ou null tant qu'aucun ne l'est.</summary>
    public Picker? Current { get; private set; }

    /// <summary>
    /// Message à afficher une fois sur « Qui prépare ? », ou null : le profil
    /// choisi a été supprimé depuis un autre onglet (étape 13).
    /// </summary>
    public string? Notice { get; private set; }

    /// <summary>Retient picker pour la suite de la session.</summary>
    public void Choose(Picker picker) => Current = picker;

    /// <summary>Oublie le profil choisi (bouton « Changer »).</summary>
    public void Clear() => Current = null;

    /// <summary>
    /// Oublie le profil choisi, supprimé depuis un autre onglet, et prépare le
    /// message qui l'explique sur « Qui prépare ? ».
    /// </summary>
    public void ClearDeleted()
    {
        if (Current is { } picker)
        {
            Notice = $"Le profil « {picker.Name} » a été supprimé. Choisissez un autre profil, ou créez le vôtre.";
        }
        Current = null;
    }

    /// <summary>Rend le message à afficher, puis l'oublie : il ne s'affiche qu'une fois.</summary>
    public string? TakeNotice()
    {
        var notice = Notice;
        Notice = null;
        return notice;
    }
}
