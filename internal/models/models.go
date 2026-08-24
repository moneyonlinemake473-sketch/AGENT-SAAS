package models

import "time"

type Client struct {
	ID            string    `firestore:"id"`
	Name          string    `firestore:"name"`
	PasswordHash  string    `firestore:"password_hash"`
	SystemPrompt  string    `firestore:"system_prompt"`
	IsActive      bool      `firestore:"is_active"`
	CreatedAt     time.Time `firestore:"created_at"`
	ReactivatedAt time.Time `firestore:"reactivated_at"`
	SheetTabName  string    `firestore:"sheet_tab_name"`

	TokenLimit  int64   `firestore:"token_limit"`
	TokensUsed  int64   `firestore:"tokens_used"`
	CostUsedUSD float64 `firestore:"cost_used_usd"`

	BlockedContacts []string `firestore:"blocked_contacts"`

	WhatsAppSessionBackup string `firestore:"wa_session_backup"`
}

type TokenUsageEvent struct {
	ClientID         string    `firestore:"client_id"`
	Timestamp        time.Time `firestore:"timestamp"`
	PromptTokens     int64     `firestore:"prompt_tokens"`
	CompletionTokens int64     `firestore:"completion_tokens"`
	CostUSD          float64   `firestore:"cost_usd"`
}