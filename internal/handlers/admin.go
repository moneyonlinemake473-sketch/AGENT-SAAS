package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"agent-saas/internal/models"
	"agent-saas/internal/sheets"
	"agent-saas/internal/store"
	"agent-saas/internal/whatsapp"

	"golang.org/x/crypto/bcrypt"
)

type AdminHandlers struct {
	Store     *store.Store
	SheetsMgr *sheets.Manager
	WAManager *whatsapp.Manager
	AdminKey  string
}

// Middleware simple par clé API (header X-Admin-Key). À remplacer par un vrai
// système d'auth si tu ouvres cette route au-delà de toi-même.
func (h *AdminHandlers) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Admin-Key") != h.AdminKey {
			http.Error(w, "non autorisé", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type createClientRequest struct {
	Name         string `json:"name"`
	SystemPrompt string `json:"system_prompt"`
	TokenLimit   int64  `json:"token_limit"`
}

type createClientResponse struct {
	ClientID string `json:"client_id"`
	Password string `json:"password"` // généré une seule fois, à transmettre au client
	URL      string `json:"url"`
}

// CreateClient génère un nouveau client : slug unique, mot de passe aléatoire,
// feuille Google Sheets dédiée, document Firestore. Ne connecte pas encore
// WhatsApp — ça se fait via /admin/clients/{id}/connect pour scanner le QR.
func (h *AdminHandlers) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req createClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corps de requête invalide", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	id := slugify(req.Name) + "-" + randomHex(4)
	password := randomHex(6)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "erreur interne", http.StatusInternalServerError)
		return
	}

	spreadsheetID, err := h.SheetsMgr.CreateClientSheet(ctx, req.Name)
	if err != nil {
		http.Error(w, "erreur création feuille google sheets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	client := &models.Client{
		ID:            id,
		Name:          req.Name,
		PasswordHash:  string(hash),
		SystemPrompt:  req.SystemPrompt,
		IsActive:      true,
		TokenLimit:    req.TokenLimit,
		SpreadsheetID: spreadsheetID,
	}
	if err := h.Store.CreateClient(ctx, client); err != nil {
		http.Error(w, "erreur création client: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, createClientResponse{
		ClientID: id,
		Password: password,
		URL:      "/c/" + id,
	})
}

func (h *AdminHandlers) SetActive(w http.ResponseWriter, r *http.Request, clientID string, active bool) {
	if err := h.Store.SetActive(context.Background(), clientID, active); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !active {
		h.WAManager.StopClient(clientID)
	}
	writeJSON(w, map[string]bool{"is_active": active})
}

type addTokensRequest struct {
	Tokens int64 `json:"tokens"`
}

func (h *AdminHandlers) AddTokens(w http.ResponseWriter, r *http.Request, clientID string) {
	var req addTokensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corps invalide", http.StatusBadRequest)
		return
	}
	if err := h.Store.AddTokenQuota(context.Background(), clientID, req.Tokens); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]int64{"tokens_added": req.Tokens})
}

func (h *AdminHandlers) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.Store.ListClients(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, clients)
}

// Connect démarre (ou redémarre) la session WhatsApp d'un client. Le QR code
// est renvoyé en flux (SSE simplifié ici en polling simple, voir client.go).
func (h *AdminHandlers) Connect(w http.ResponseWriter, r *http.Request, clientID string) {
	qrOut := make(chan string, 1)
	go func() {
		_ = h.WAManager.StartClient(context.Background(), clientID, qrOut)
	}()
	select {
	case code := <-qrOut:
		writeJSON(w, map[string]string{"qr_code": code})
	case <-r.Context().Done():
		return
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
