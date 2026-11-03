# Image de l'API Go, construite en deux étapes.

# Étape 1 : compilation avec l'image officielle de Go.
FROM golang:1.27 AS build
WORKDIR /src
# Dépendances d'abord : cette couche reste en cache tant que go.mod et go.sum
# ne changent pas.
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
# CGO_ENABLED=0 produit un exécutable autonome, qui tourne sur Alpine sans la
# bibliothèque C de l'image de compilation.
RUN CGO_ENABLED=0 go build -o /out/api .

# Étape 2 : exécution sur Alpine, une image Linux minimale.
FROM alpine:3.24
COPY --from=build /out/api /usr/local/bin/api
# Dans un conteneur, l'API écoute sur toutes les interfaces : le trafic arrive
# par le réseau Docker, pas par 127.0.0.1.
ENV API_ADDR=:8080
EXPOSE 8080
# Utilisateur sans privilèges, fourni par Alpine.
USER nobody
ENTRYPOINT ["api"]
