package sheets

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Manager gère UN SEUL classeur Google Sheets "maître" (créé et possédé par
// toi, un humain — pas par le compte de service). Chaque client obtient son
// propre onglet à l'intérieur de ce classeur. On procède ainsi car un compte
// de service Google n'a aucun stockage Drive propre : il ne peut jamais créer
// de nouveau fichier lui-même (même dans un dossier partagé, l'erreur
// "storageQuotaExceeded" persiste), mais il PEUT modifier un fichier
// existant qui lui a été partagé en tant qu'Éditeur — ce qui inclut ajouter
// de nouveaux onglets. Ça évite complètement le problème de quota.
type Manager struct {
	svc                 *sheets.Service
	masterSpreadsheetID string
}

func New(ctx context.Context, credentialsJSON string, masterSpreadsheetID string) (*Manager, error) {
	svc, err := sheets.NewService(ctx,
		option.WithCredentialsJSON([]byte(credentialsJSON)),
		option.WithScopes("https://www.googleapis.com/auth/spreadsheets"),
	)
	if err != nil {
		return nil, fmt.Errorf("connexion google sheets: %w", err)
	}
	return &Manager{svc: svc, masterSpreadsheetID: masterSpreadsheetID}, nil
}

// CreateClientTab ajoute un nouvel onglet au classeur maître pour un client,
// avec les en-têtes de colonnes. Retourne le nom de l'onglet créé (unique),
// à conserver dans le document Firestore du client (champ SheetTabName).
func (m *Manager) CreateClientTab(ctx context.Context, tabName string) error {
	if m.masterSpreadsheetID == "" {
		return fmt.Errorf("GOOGLE_MASTER_SPREADSHEET_ID non configuré")
	}

	_, err := m.svc.Spreadsheets.BatchUpdate(m.masterSpreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: tabName,
					},
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("création de l'onglet %q: %w", tabName, err)
	}

	header := &sheets.ValueRange{
		Values: [][]interface{}{{"Horodatage", "Expéditeur", "Message"}},
	}
	_, err = m.svc.Spreadsheets.Values.Update(m.masterSpreadsheetID, tabName+"!A1:C1", header).
		ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("écriture des en-têtes: %w", err)
	}

	return nil
}

// AppendMessage ajoute une ligne à l'onglet du client (utilisé à chaque
// message entrant/sortant pour que l'IA garde le fil)
func (m *Manager) AppendMessage(ctx context.Context, tabName, timestamp, sender, message string) error {
	row := &sheets.ValueRange{
		Values: [][]interface{}{{timestamp, sender, message}},
	}
	_, err := m.svc.Spreadsheets.Values.Append(m.masterSpreadsheetID, tabName+"!A:C", row).
		ValueInputOption("RAW").Context(ctx).Do()
	return err
}

// ReadRecentHistory relit les N dernières lignes de l'onglet du client pour
// reconstituer le contexte avant d'appeler Gemini
func (m *Manager) ReadRecentHistory(ctx context.Context, tabName string, maxRows int) ([][]interface{}, error) {
	resp, err := m.svc.Spreadsheets.Values.Get(m.masterSpreadsheetID, tabName+"!A2:C10000").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	rows := resp.Values
	if len(rows) > maxRows {
		rows = rows[len(rows)-maxRows:]
	}
	return rows, nil
}

// ClearConversation vide l'historique de l'onglet du client
func (m *Manager) ClearConversation(ctx context.Context, tabName string) error {
	_, err := m.svc.Spreadsheets.Values.Clear(m.masterSpreadsheetID, tabName+"!A2:C10000", &sheets.ClearValuesRequest{}).
		Context(ctx).Do()
	return err
}