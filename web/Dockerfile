# Image de l'interface Blazor, construite en deux étapes.

# Étape 1 : compilation avec le SDK .NET complet.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
# Restauration des paquets d'abord : cette couche reste en cache tant que le
# fichier projet ne change pas.
COPY Cairn.Web.csproj ./
RUN dotnet restore
COPY . ./
RUN dotnet publish --configuration Release --no-restore --output /app
# Sans le script de Blazor, les pages resteraient vides, sans erreur côté
# serveur : mieux vaut faire échouer la construction de l'image
# (voir Cairn.Web.csproj).
RUN test -f /app/wwwroot/_framework/blazor.web.js

# Étape 2 : exécution avec le seul runtime ASP.NET Core, bien plus léger.
FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app ./
# Les images .NET écoutent sur le port 8080 et fournissent un utilisateur
# sans privilèges, dont l'identifiant est dans APP_UID.
USER $APP_UID
EXPOSE 8080
ENTRYPOINT ["dotnet", "Cairn.Web.dll"]
