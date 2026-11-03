// Commande api : le serveur HTTP de Cairn.
//
// Étape 2 du plan (docs/PLAN.md) : configuration, pool de connexions
// PostgreSQL et route GET /health. Les autres routes, et les paquets
// internal/store et internal/httpapi, arrivent à l'étape 4.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	// go run. Dans un conteneur, API_ADDR vaudra ":8080" (toutes les interfaces).
	defaultAddr = "127.0.0.1:8080"
)

// healthTimeout limite l'attente de la base dans GET /health.
const healthTimeout = 2 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run démarre l'API et ne rend la main qu'en cas d'erreur. Séparer run de
// main permet aux defer de s'exécuter : log.Fatal arrête le programme sans
// les exécuter.
func run() error {
	databaseURL := getenv("DATABASE_URL", defaultDatabaseURL)
	addr := getenv("API_ADDR", defaultAddr)

	// Le pool ouvre ses connexions à la demande : sa création réussit même si
	// la base est arrêtée. Seule une URL mal formée provoque une erreur ici.
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("configuration de la base : %w", err)
	}
	defer pool.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(pool))

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
		// Délai maximal pour recevoir les en-têtes d'une requête : protège
		// contre les clients qui envoient leur requête très lentement.
		ReadHeaderTimeout: 5 * time.Second,
	}

	db := pool.Config().ConnConfig
	log.Printf("API Cairn à l'écoute sur %s (base %s:%d/%s)", addr, db.Host, db.Port, db.Database)
	return server.ListenAndServe()
}

// getenv renvoie la variable d'environnement name, ou fallback si elle est
// absente ou vide.
func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// healthHandler renvoie le handler de GET /health : 200 {"status":"ok"} si la
// base répond, 503 sinon. Le handler garde accès au pool après le retour de
// healthHandler : c'est une fermeture (closure).
func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			log.Printf("GET /health : base injoignable : %v", err)
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{
				Error:   "base_indisponible",
				Message: "La base de données ne répond pas.",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// errorResponse est le format unique des erreurs de l'API
// (docs/PLAN.md, section 4.2).
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeJSON envoie value encodée en JSON, avec le code HTTP status. Les
// en-têtes doivent être fixés avant WriteHeader : ensuite, ils sont déjà partis.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("écriture de la réponse JSON : %v", err)
	}
}
