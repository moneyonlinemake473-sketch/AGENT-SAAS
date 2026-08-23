package models

import "time"

// Client représente un client de l'agence, stocké dans Firestore
// collection "clients", document ID = ClientID (slug unique, ex: "boulangerie-dupont")
type Client struct {
	ID              string    `firestore:"id"`
	Name            string    `firestore:"name"`
	PasswordHash    string    `firestore:"password_hash"` // bcrypt, jamais le mot de passe en clair
	SystemPrompt    string    `firestore:"system_prompt"`
	IsActive        bool      `firestore:"is_active"` // true/false — c'est le flag que tu bascules
	CreatedAt       time.Time `firestore:"created_at"`
	ReactivatedAt   time.Time `firestore:"reactivated_at"` // messages antérieurs à cette date = ignorés
	SpreadsheetID   string    `firestore:"spreadsheet_id"` // Google Sheet dédié à ce client (historique conversation)

	// Suivi de consommation Gemini
	TokenLimit      int64   `firestore:"token_limit"`       // limite fixée par toi (ex: 2 000 000)
	TokensUsed      int64   `firestore:"tokens_used"`       // cumulé depuis la dernière remise à zéro
	CostUsedUSD     float64 `firestore:"cost_used_usd"`     // coût cumulé estimé en dollars

	// Liste des contacts (numéros WhatsApp, format international sans +) que l'IA doit ignorer
	BlockedContacts []string `firestore:"blocked_contacts"`

	// Sauvegarde de la session WhatsApp (whatsmeow), en base64, pour survivre à un redeploy Render
	WhatsAppSessionBackup string `firestore:"wa_session_backup"`
}

// TokenUsageEvent est loggé pour garder une trace détaillée si besoin d'audit
type TokenUsageEvent struct {
	ClientID       string    `firestore:"client_id"`
	Timestamp      time.Time `firestore:"timestamp"`
	PromptTokens   int64     `firestore:"prompt_tokens"`
	CompletionTokens int64   `firestore:"completion_tokens"`
	CostUSD        float64   `firestore:"cost_usd"`
}
