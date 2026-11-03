// Commande api : le serveur HTTP de Cairn.
//
// main.go s'occupe du démarrage : configuration, connexion à la base, serveur
// HTTP et arrêt propre. Les routes sont dans internal/httpapi, l'accès à la
// base dans internal/store (docs/PLAN.md, section 2.3).
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/knudsem/cairn/api/internal/httpapi"
	"github.com/knudsem/cairn/api/internal/store"
)

// Valeurs par défaut pour le développement sous Windows, remplacées par les
// variables d'environnement DATABASE_URL et API_ADDR quand elles existent.
// 127.0.0.1 et non localhost : voir docs/PLAN.md, section 2.1.
const (
	// Base lancée par compose.yaml. Elle ne propose pas de chiffrement TLS :
	// sslmode=disable évite que pgx essaie d'abord une connexion chiffrée.
	defaultDatabaseURL = "postgres://cairn:cairn@127.0.0.1:5432/cairn?sslmode=disable"

	// En local, l'API n'écoute que sur ce poste : elle n'est pas exposée au
	// réseau, et le pare-feu de Windows ne pose pas de question à chaque
	// go run. Dans un conteneur, API_ADDR vaut ":8080" (toutes les interfaces).
	defaultAddr = "127.0.0.1:8080"
)

// shutdownTimeout laisse aux requêtes en cours le temps de se terminer quand
// l'API s'arrête.
const shutdownTimeout = 5 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run démarre l'API et rend la main à son arrêt. Séparer run de main permet
// aux defer de s'exécuter : log.Fatal arrête le programme sans les exécuter.
func run() error {
	// stopped est annulé à la réception de Ctrl+C (os.Interrupt) ou de SIGTERM,
	// le signal envoyé par docker compose stop.
	stopped, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := getenv("DATABASE_URL", defaultDatabaseURL)
	addr := getenv("API_ADDR", defaultAddr)

	// Le pool ouvre ses connexions à la demande : sa création réussit même si
	// la base est arrêtée. Seule une URL mal formée provoque une erreur ici.
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("configuration de la base : %w", err)
	}
	defer pool.Close()

	server := &http.Server{
		Addr:    addr,
		Handler: httpapi.NewHandler(store.New(pool)),
		// Délai maximal pour recevoir les en-têtes d'une requête : protège
		// contre les clients qui envoient leur requête très lentement.
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Le port est ouvert avant d'annoncer l'écoute : s'il est déjà pris (par
	// le conteneur api, par exemple), l'erreur arrive tout de suite.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("écoute sur %s : %w", addr, err)
	}
	db := pool.Config().ConnConfig
	log.Printf("API Cairn à l'écoute sur %s (base %s:%d/%s)", addr, db.Host, db.Port, db.Database)

	// Le serveur tourne dans une goroutine, une fonction exécutée en parallèle.
	// run attend la première de ces deux choses : une erreur du serveur, ou
	// une demande d'arrêt.
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-stopped.Done():
		// Rétablit le comportement par défaut : un second Ctrl+C arrête le
		// programme sur-le-champ, sans attendre la fin de l'arrêt propre.
		stop()
	}

	// Arrêt propre : le serveur n'accepte plus de connexion et laisse les
	// requêtes en cours se terminer, pendant shutdownTimeout au plus. Au-delà,
	// server.Close coupe les connexions restantes : les requêtes en cours sont
	// annulées, et le pool de connexions peut se fermer à son tour.
	log.Print("arrêt demandé : fin des requêtes en cours")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		server.Close()
		return fmt.Errorf("arrêt du serveur, requêtes interrompues : %w", err)
	}
	log.Print("API arrêtée")
	return nil
}

// getenv renvoie la variable d'environnement name, ou fallback si elle est
// absente ou vide.
func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
