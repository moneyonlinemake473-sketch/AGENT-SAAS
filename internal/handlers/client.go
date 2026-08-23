package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"agent-saas/internal/store"
	"agent-saas/internal/whatsapp"

	"golang.org/x/crypto/bcrypt"
)

type ClientHandlers struct {
	Store     *store.Store
	WAManager *whatsapp.Manager

	// sessions web très simples en mémoire : token -> (clientID, expiration)
	// Sur plusieurs instances Render il faudrait déplacer ça dans Firestore aussi,
	// mais pour un seul service ça suffit.
	mu       sync.Mutex
	sessions map[string]webSession
}

type webSession struct {
	ClientID string
	Expires  time.Time
}

func NewClientHandlers(s *store.Store, wa *whatsapp.Manager) *ClientHandlers {
	return &ClientHandlers{Store: s, WAManager: wa, sessions: make(map[string]webSession)}
}

type loginRequest struct {
	Password string `json:"password"`
}

// Login vérifie le mot de passe du client et pose un cookie de session.
// Le "QR code de reconnexion" côté client correspond à cette étape : si sa
// session web expire, il doit se reconnecter ici (mot de passe), puis peut
// déclencher un nouveau scan WhatsApp si besoin depuis son interface.
func (h *ClientHandlers) Login(w http.ResponseWriter, r *http.Request, clientID string) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corps invalide", http.StatusBadRequest)
		return
	}

	c, err := h.Store.GetClient(context.Background(), clientID)
	if err != nil {
		http.Error(w, "client introuvable", http.StatusNotFound)
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(req.Password)) != nil {
		http.Error(w, "mot de passe incorrect", http.StatusUnauthorized)
		return
	}

	token := randomHex(16)
	h.mu.Lock()
	h.sessions[token] = webSession{ClientID: clientID, Expires: time.Now().Add(12 * time.Hour)}
	h.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_" + clientID,
		Value:    token,
		Path:     "/c/" + clientID,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(12 * time.Hour),
	})
	writeJSON(w, map[string]bool{"ok": true})
}

// RequireClientSession protège les routes de l'interface pro du client
func (h *ClientHandlers) RequireClientSession(clientID string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_" + clientID)
		if err != nil {
			http.Error(w, "session expirée, merci de vous reconnecter", http.StatusUnauthorized)
			return
		}
		h.mu.Lock()
		sess, ok := h.sessions[cookie.Value]
		h.mu.Unlock()
		if !ok || sess.ClientID != clientID || time.Now().After(sess.Expires) {
			http.Error(w, "session expirée, merci de vous reconnecter", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type toggleRequest struct {
	Active bool `json:"active"`
}

// ToggleBot permet au client d'activer/désactiver lui-même son bot depuis
// son interface (en plus de ton contrôle admin côté agence)
func (h *ClientHandlers) ToggleBot(w http.ResponseWriter, r *http.Request, clientID string) {
	var req toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corps invalide", http.StatusBadRequest)
		return
	}
	if err := h.Store.SetActive(context.Background(), clientID, req.Active); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !req.Active {
		h.WAManager.StopClient(clientID)
	}
	writeJSON(w, map[string]bool{"active": req.Active})
}

type blocklistRequest struct {
	Contacts []string `json:"contacts"` // numéros WhatsApp sans le +, ex: "22997xxxxxx"
}

// SetBlocklist définit les contacts pour lesquels l'IA ne doit jamais répondre
func (h *ClientHandlers) SetBlocklist(w http.ResponseWriter, r *http.Request, clientID string) {
	var req blocklistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "corps invalide", http.StatusBadRequest)
		return
	}
	cleaned := make([]string, 0, len(req.Contacts))
	for _, c := range req.Contacts {
		c = strings.TrimSpace(strings.TrimPrefix(c, "+"))
		if c != "" {
			cleaned = append(cleaned, c)
		}
	}
	if err := h.Store.SetBlockedContacts(context.Background(), clientID, cleaned); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string][]string{"blocked_contacts": cleaned})
}
