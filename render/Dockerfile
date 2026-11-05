# Image de Cairn pour Render seulement (docs/PLAN.md, étape 11 ter) : l'API Go
# et l'interface Blazor dans un seul conteneur, lancées par render/start.sh.
# En local, compose.yaml utilise toujours api/Dockerfile et web/Dockerfile.
#
# Construite depuis la racine du dépôt (dockerContext: . dans render.yaml),
# car elle compile api/ et web/. Le fichier .dockerignore de la racine ne
# laisse passer que ces dossiers et render/start.sh. Les deux étapes de
# compilation reprennent celles de api/Dockerfile et de web/Dockerfile.

# Étape 1 : compilation de l'API avec l'image officielle de Go.
FROM golang:1.27 AS api
WORKDIR /src
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
# Exécutable autonome (CGO_ENABLED=0) : il tourne sur l'image .NET, sans la
# bibliothèque C de l'image de compilation.
RUN CGO_ENABLED=0 go build -o /out/api .

# Étape 2 : compilation de l'interface avec le SDK .NET complet.
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS web
WORKDIR /src
COPY web/Cairn.Web.csproj ./
RUN dotnet restore
COPY web/ ./
RUN dotnet publish --configuration Release --no-restore --output /app
# Sans le script de Blazor, les pages resteraient vides (voir web/Dockerfile).
RUN test -f /app/wwwroot/_framework/blazor.web.js

# Étape 3 : exécution sur l'image ASP.NET Core (Ubuntu), qui fournit bash pour
# render/start.sh. L'API n'a besoin de rien d'autre que son exécutable.
FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=web /app ./
COPY --from=api /out/api /usr/local/bin/api
COPY render/start.sh ./
# Utilisateur sans privilèges de l'image .NET, pour les deux programmes.
USER $APP_UID
# Port public par défaut de Render ; start.sh suit la variable PORT.
EXPOSE 10000
# bash lance le script : pas besoin du droit d'exécution sur le fichier, que
# Git sous Windows ne conserve pas.
ENTRYPOINT ["bash", "/app/start.sh"]
