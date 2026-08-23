package main

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"agent-saas/internal/ai"
	"agent-saas/internal/config"
	"agent-saas/internal/handlers"
	"agent-saas/internal/sheets"
	"agent-saas/internal/store"
	"agent-saas/internal/whatsapp"
)

//go:embed ../../web/templates/*.html
var templatesFS embed.FS

func main() {
	cfg := config.Load()
	ctx := context.Background()

	fsStore, err := store.New(ctx, cfg.FirestoreProjectID, cfg.GoogleCredentialsJSON)
	if err != nil {
		log.Fatalf("firestore: %v", err)
	}

	sheetsMgr, err := sheets.New(ctx, cfg.GoogleCredentialsJSON)
	if err != nil {
		log.Fatalf("google sheets: %v", err)
	}

	geminiCli := ai.NewClient(cfg.GeminiAPIKey, "gemini-1.5-flash", cfg.GeminiInputCostPer1K, cfg.GeminiOutputCostPer1K)

	waManager := whatsapp.NewManager(fsStore, sheetsMgr, geminiCli)

	adminH := &handlers.AdminHandlers{Store: fsStore, SheetsMgr: sheetsMgr, WAManager: waManager, AdminKey: cfg.AdminAPIKey}
	clientH := handlers.NewClientHandlers(fsStore, waManager)

	mux := http.NewServeMux()

	// --- Routes admin (toi uniquement, protégées par X-Admin-Key) ---
	mux.HandleFunc("/admin/clients", adminH.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			adminH.CreateClient(w, r)
		case http.MethodGet:
			adminH.ListClients(w, r)
		default:
			http.Error(w, "méthode non supportée", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/admin/clients/", adminH.RequireAdmin(func(w http.ResponseWriter, r *http.Request) {
		// /admin/clients/{id}/active | /tokens | /connect
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/clients/"), "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		clientID, action := parts[0], parts[1]
		switch action {
		case "active":
			var body struct {
				Active bool `json:"active"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			adminH.SetActive(w, r, clientID, body.Active)
		case "tokens":
			adminH.AddTokens(w, r, clientID)
		case "connect":
			adminH.Connect(w, r, clientID)
		default:
			http.NotFound(w, r)
		}
	}))

	// --- Routes client (page HTML protégée par mot de passe + API pro) ---
	tmpl := template.Must(template.ParseFS(templatesFS, "../../web/templates/client.html"))

	mux.HandleFunc("/c/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/c/"), "/")
		clientID := parts[0]
		if clientID == "" {
			http.NotFound(w, r)
			return
		}

		if len(parts) == 1 {
			// page HTML (login + interface une fois connecté, géré côté JS avec le cookie)
			_ = tmpl.Execute(w, map[string]string{"ClientID": clientID})
			return
		}

		action := parts[1]
		switch action {
		case "login":
			clientH.Login(w, r, clientID)
		case "toggle":
			clientH.RequireClientSession(clientID, func(w http.ResponseWriter, r *http.Request) {
				clientH.ToggleBot(w, r, clientID)
			})(w, r)
		case "blocklist":
			clientH.RequireClientSession(clientID, func(w http.ResponseWriter, r *http.Request) {
				clientH.SetBlocklist(w, r, clientID)
			})(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Sauvegarde périodique de toutes les sessions WhatsApp actives vers Firestore,
	// pour limiter la perte en cas de crash/redeploy inattendu entre deux events "success"
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			waManager.BackupAllSessions(context.Background())
		}
	}()

	log.Printf("serveur démarré sur :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatal(err)
	}
}
