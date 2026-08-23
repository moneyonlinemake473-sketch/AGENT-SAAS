package store

import (
	"context"
	"fmt"
	"time"

	"agent-saas/internal/models"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const clientsCollection = "clients"

type Store struct {
	client *firestore.Client
}

// New se connecte à Firestore en utilisant le JSON du compte de service
// (variable d'env GOOGLE_CREDENTIALS_JSON, voir README pour le créer)
func New(ctx context.Context, projectID string, credentialsJSON string) (*Store, error) {
	client, err := firestore.NewClient(ctx, projectID, option.WithCredentialsJSON([]byte(credentialsJSON)))
	if err != nil {
		return nil, fmt.Errorf("connexion firestore: %w", err)
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateClient(ctx context.Context, c *models.Client) error {
	c.CreatedAt = time.Now()
	c.ReactivatedAt = time.Now()
	_, err := s.client.Collection(clientsCollection).Doc(c.ID).Set(ctx, c)
	return err
}

func (s *Store) GetClient(ctx context.Context, id string) (*models.Client, error) {
	doc, err := s.client.Collection(clientsCollection).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	var c models.Client
	if err := doc.DataTo(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListClients(ctx context.Context) ([]*models.Client, error) {
	iter := s.client.Collection(clientsCollection).Documents(ctx)
	defer iter.Stop()
	var out []*models.Client
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var c models.Client
		if err := doc.DataTo(&c); err == nil {
			out = append(out, &c)
		}
	}
	return out, nil
}

// SetActive bascule le flag actif/inactif. Quand on réactive, on met à jour
// ReactivatedAt pour que les vieux messages en attente soient ignorés.
func (s *Store) SetActive(ctx context.Context, id string, active bool) error {
	updates := []firestore.Update{
		{Path: "is_active", Value: active},
	}
	if active {
		updates = append(updates, firestore.Update{Path: "reactivated_at", Value: time.Now()})
	}
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, updates)
	return err
}

// AddTokenQuota ajoute des tokens à la limite existante (rechargement après blocage)
func (s *Store) AddTokenQuota(ctx context.Context, id string, additionalTokens int64) error {
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "token_limit", Value: firestore.Increment(additionalTokens)},
	})
	return err
}

// RecordUsage incrémente atomiquement les tokens et le coût consommés
func (s *Store) RecordUsage(ctx context.Context, id string, tokens int64, costUSD float64) error {
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "tokens_used", Value: firestore.Increment(tokens)},
		{Path: "cost_used_usd", Value: firestore.Increment(costUSD)},
	})
	return err
}

func (s *Store) SetBlockedContacts(ctx context.Context, id string, contacts []string) error {
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "blocked_contacts", Value: contacts},
	})
	return err
}

// SaveWhatsAppSession sauvegarde le blob de session (base64) — voir internal/whatsapp
// pour comprendre pourquoi c'est indispensable sur Render (disque effacé au redeploy)
func (s *Store) SaveWhatsAppSession(ctx context.Context, id string, sessionB64 string) error {
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "wa_session_backup", Value: sessionB64},
	})
	return err
}

func (s *Store) UpdatePasswordHash(ctx context.Context, id, hash string) error {
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "password_hash", Value: hash},
	})
	return err
}

func (s *Store) UpdateSpreadsheetID(ctx context.Context, id, sheetID string) error {
	_, err := s.client.Collection(clientsCollection).Doc(id).Update(ctx, []firestore.Update{
		{Path: "spreadsheet_id", Value: sheetID},
	})
	return err
}
