#!/bin/bash
# Démarrage du conteneur Render (docs/PLAN.md, étape 11 ter) : l'API Go et
# l'interface Blazor tournent dans le même conteneur. L'interface appelle
# l'API en local, et Render réveille les deux en même temps.
#
# Si l'un des deux programmes s'arrête, le script arrête l'autre et se
# termine en erreur. Le port public se ferme : la sonde de Render échoue, et
# Render redémarre le conteneur entier (au plus 60 s d'échec, d'après sa page
# « health checks »). Un conteneur à moitié en marche, lui, garderait le port
# de l'interface ouvert et ne serait jamais redémarré.

# Une variable non définie est une erreur, pas une chaîne vide.
set -u

# Sans base, l'API démarrerait, mais chaque appel échouerait. Le conteneur
# refuse donc de démarrer : sur Render, le déploiement échoue et la version
# précédente reste en ligne.
if [ -z "${DATABASE_URL:-}" ]; then
    echo "start.sh : la variable DATABASE_URL est manquante" >&2
    exit 1
fi

# API : seulement sur 127.0.0.1, donc jamais joignable hors du conteneur.
# Elle lit DATABASE_URL dans l'environnement. « & » lance le programme en
# arrière-plan, et $! donne son numéro de processus.
API_ADDR=127.0.0.1:9000 api &
api_pid=$!

# Interface : sur le port donné par Render (PORT), et ces réglages écrits ici
# plutôt que dans render.yaml, car ils vont avec l'adresse de l'API :
# - ApiBaseUrl remplace toute ancienne valeur du tableau de bord (Render
#   garde une variable retirée de render.yaml) ;
# - pas de réveil par le navigateur : 127.0.0.1 désignerait l'ordinateur du
#   visiteur, et l'API démarre avec l'interface.
# DOTNET_SHUTDOWNTIMEOUTSECONDS : à l'arrêt, l'interface attend au plus 5 s
# que les onglets ouverts se déconnectent, au lieu de 30 s par défaut (autant
# que le délai de Render avant SIGKILL). Les onglets se reconnectent seuls.
# env -u retire DATABASE_URL : l'interface n'a pas besoin du mot de passe.
# C'est de l'hygiène, pas une barrière : les deux programmes tournent sous
# le même utilisateur, qui peut lire l'environnement de l'API.
env -u DATABASE_URL \
    ASPNETCORE_HTTP_PORTS="${PORT:-10000}" \
    ApiBaseUrl=http://127.0.0.1:9000/ \
    ApiWakeUpFromBrowser=false \
    DOTNET_SHUTDOWNTIMEOUTSECONDS=5 \
    dotnet Cairn.Web.dll &
web_pid=$!

# Arrêt demandé (SIGTERM de Render ou de docker stop, Ctrl+C) : transmis
# d'abord à l'interface, qui dépend de l'API (voir la fin du script). Sans
# ce trap, le script, premier processus du conteneur, ignorerait le signal.
stopping=false
trap 'stopping=true; kill -TERM "$web_pid" 2>/dev/null' TERM INT

# wait -n rend la main dès que le premier des deux programmes s'arrête, ou
# à la réception d'un signal ; -p donne le numéro du programme arrêté.
wait -n -p stopped_pid
status=$?

if [ "$stopping" = false ]; then
    if [ "$stopped_pid" = "$api_pid" ]; then
        name="l'API"
    else
        name="l'interface"
    fi
    echo "start.sh : $name s'est arrêtée (code $status), arrêt du conteneur" >&2
    kill -TERM "$api_pid" "$web_pid" 2>/dev/null
    wait
    exit 1
fi

# Arrêt demandé : l'interface termine ses requêtes et ferme les onglets
# pendant que l'API répond encore ; ensuite seulement, l'API s'arrête. Le
# conteneur ne tourne donc jamais avec une interface privée de son API.
wait "$web_pid"
kill -TERM "$api_pid" 2>/dev/null
wait "$api_pid"
echo "start.sh : arrêt terminé" >&2
exit 0
