package sheets

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type Manager struct {
	svc *sheets.Service
}

func New(ctx context.Context, credentialsJSON string) (*Manager, error) {
	svc, err := sheets.NewService(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))
	if err != nil {
		return nil, fmt.Errorf("connexion google sheets: %w", err)
	}
	return &Manager{svc: svc}, nil
}

// CreateClientSheet crée un nouveau classeur Google Sheets dédié à un client,
// avec un onglet "Conversation" et les en-têtes de colonnes.
// Le compte de service doit avoir le rôle Editor sur Google Drive (ou passer par
// l'API Drive pour le partager avec ton compte perso ensuite, voir README).
func (m *Manager) CreateClientSheet(ctx context.Context, clientName string) (spreadsheetID string, err error) {
	spreadsheet := &sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{
			Title: fmt.Sprintf("Conversation - %s", clientName),
		},
		Sheets: []*sheets.Sheet{
			{Properties: &sheets.SheetProperties{Title: "Conversation"}},
		},
	}
	created, err := m.svc.Spreadsheets.Create(spreadsheet).Context(ctx).Do()
	if err != nil {
		return "", err
	}

	header := &sheets.ValueRange{
		Values: [][]interface{}{{"Horodatage", "Expéditeur", "Message"}},
	}
	_, err = m.svc.Spreadsheets.Values.Update(created.SpreadsheetId, "Conversation!A1:C1", header).
		ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return "", err
	}

	return created.SpreadsheetId, nil
}

// AppendMessage ajoute une ligne à la conversation (utilisé à chaque message entrant/sortant
// pour que l'IA garde le fil — on relit les dernières lignes avant chaque appel Gemini)
func (m *Manager) AppendMessage(ctx context.Context, spreadsheetID, timestamp, sender, message string) error {
	row := &sheets.ValueRange{
		Values: [][]interface{}{{timestamp, sender, message}},
	}
	_, err := m.svc.Spreadsheets.Values.Append(spreadsheetID, "Conversation!A:C", row).
		ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// ReadRecentHistory relit les N dernières lignes pour reconstituer le contexte
// avant d'appeler Gemini (ainsi l'IA ne perd pas le fil même après un redeploy)
func (m *Manager) ReadRecentHistory(ctx context.Context, spreadsheetID string, maxRows int) ([][]interface{}, error) {
	resp, err := m.svc.Spreadsheets.Values.Get(spreadsheetID, "Conversation!A2:C10000").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	rows := resp.Values
	if len(rows) > maxRows {
		rows = rows[len(rows)-maxRows:]
	}
	return rows, nil
}

// ClearConversation vide l'historique (appelé soit par ton propre cron Go,
// soit par l'Apps Script fourni dans /scripts/apps-script-daily-clear.gs
// si tu préfères que ce soit Google qui déclenche à minuit sans dépendre de Render)
func (m *Manager) ClearConversation(ctx context.Context, spreadsheetID string) error {
	_, err := m.svc.Spreadsheets.Values.Clear(spreadsheetID, "Conversation!A2:C10000", &sheets.ClearValuesRequest{}).
		Context(ctx).Do()
	return err
}
