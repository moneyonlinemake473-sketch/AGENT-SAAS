package whatsapp

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"agent-saas/internal/ai"
	"agent-saas/internal/models"
	"agent-saas/internal/sheets"
	"agent-saas/internal/store"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	_ "modernc.org/sqlite"
)

// ClientHandle regroupe tout ce qu'il faut pour un client WhatsApp connecté
type ClientHandle struct {
	WAClient   *whatsmeow.Client
	ClientID   string
	dbPath     string
	QRChan     chan string // les codes QR successifs sont poussés ici pour l'interface web
}

type Manager struct {
	mu      sync.Mutex
	clients map[string]*ClientHandle

	fsStore    *store.Store
	sheetsMgr  *sheets.Manager
	geminiCli  *ai.Client
}

func NewManager(fsStore *store.Store, sheetsMgr *sheets.Manager, geminiCli *ai.Client) *Manager {
	return &Manager{
		clients:   make(map[string]*ClientHandle),
		fsStore:   fsStore,
		sheetsMgr: sheetsMgr,
		geminiCli: geminiCli,
	}
}

// StartClient (re)connecte le bot WhatsApp d'un client.
// Si une session sauvegardée existe dans Firestore, elle est restaurée d'abord
// (c'est ce qui évite de rescanner le QR code après chaque redeploy Render).
// Sinon, un nouveau QR code est généré et envoyé sur qrOut.
func (m *Manager) StartClient(ctx context.Context, clientID string, qrOut chan<- string) error {
	m.mu.Lock()
	if _, exists := m.clients[clientID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("client %s déjà démarré", clientID)
	}
	m.mu.Unlock()

	c, err := m.fsStore.GetClient(ctx, clientID)
	if err != nil {
		return fmt.Errorf("client introuvable: %w", err)
	}

	dbPath := fmt.Sprintf("/tmp/wa-session-%s.db", clientID)

	// Restauration de la session depuis Firestore si elle existe
	if c.WhatsAppSessionBackup != "" {
		raw, err := base64.StdEncoding.DecodeString(c.WhatsAppSessionBackup)
		if err == nil {
			_ = os.WriteFile(dbPath, raw, 0600)
		}
	}

	container, err := sqlstore.New(ctx, "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)", waLog.Noop)
	if err != nil {
		return fmt.Errorf("ouverture store sqlite: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("récupération device: %w", err)
	}

	waClient := whatsmeow.NewClient(device, waLog.Noop)

	handle := &ClientHandle{
		WAClient: waClient,
		ClientID: clientID,
		dbPath:   dbPath,
		QRChan:   make(chan string, 4),
	}

	waClient.AddEventHandler(func(evt interface{}) {
		m.handleEvent(ctx, clientID, waClient, evt)
	})

	if waClient.Store.ID == nil {
		// Pas encore associé à un compte WhatsApp -> générer le QR code
		qrChan, _ := waClient.GetQRChannel(ctx)
		if err := waClient.Connect(); err != nil {
			return fmt.Errorf("connexion whatsapp: %w", err)
		}
		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					qrOut <- evt.Code // à convertir en image QR côté handler HTTP (voir handlers/client.go)
				}
				if evt.Event == "success" {
					m.backupSession(ctx, clientID, dbPath)
				}
			}
		}()
	} else {
		if err := waClient.Connect(); err != nil {
			return fmt.Errorf("connexion whatsapp: %w", err)
		}
	}

	m.mu.Lock()
	m.clients[clientID] = handle
	m.mu.Unlock()

	return nil
}

func (m *Manager) StopClient(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.clients[clientID]; ok {
		h.WAClient.Disconnect()
		delete(m.clients, clientID)
	}
}

// backupSession relit le fichier sqlite local et le sauvegarde en base64 dans Firestore.
// Appelé après un pairing réussi et périodiquement (voir cmd/server/main.go) pour
// survivre à un redeploy Render qui efface le disque.
func (m *Manager) backupSession(ctx context.Context, clientID, dbPath string) {
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		return
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	_ = m.fsStore.SaveWhatsAppSession(ctx, clientID, encoded)
}

// handleEvent traite les messages entrants : vérifie le statut actif, la liste noire,
// le quota de tokens, puis appelle Gemini et répond avec simulation anti-ban.
func (m *Manager) handleEvent(ctx context.Context, clientID string, waClient *whatsmeow.Client, evt interface{}) {
	msgEvt, ok := evt.(*events.Message)
	if !ok || msgEvt.Info.IsFromMe {
		return
	}

	c, err := m.fsStore.GetClient(ctx, clientID)
	if err != nil {
		return
	}

	// 1. Bot désactivé -> on ignore totalement
	if !c.IsActive {
		return
	}

	// 2. Message reçu pendant une période où le bot était désactivé -> on l'ignore aussi
	if msgEvt.Info.Timestamp.Before(c.ReactivatedAt) {
		return
	}

	sender := msgEvt.Info.Sender.User // numéro sans le @s.whatsapp.net

	// 3. Contact dans la liste noire du client -> on ignore
	for _, blocked := range c.BlockedContacts {
		if blocked == sender {
			return
		}
	}

	// 4. Quota de tokens dépassé -> message d'info, pas d'appel Gemini
	if c.TokenLimit > 0 && c.TokensUsed >= c.TokenLimit {
		m.sendWithAntiBan(ctx, waClient, msgEvt.Info.Chat,
			"⚠️ Vous avez atteint votre limite d'utilisation. Contactez-nous pour ajouter des crédits et continuer.")
		return
	}

	userText := extractText(msgEvt)
	if userText == "" {
		return
	}

	// Historique récent depuis Google Sheets pour garder le contexte
	var history []ai.HistoryTurn
	if c.SpreadsheetID != "" {
		rows, err := m.sheetsMgr.ReadRecentHistory(ctx, c.SpreadsheetID, 20)
		if err == nil {
			for _, r := range rows {
				if len(r) < 3 {
					continue
				}
				role := "user"
				if fmt.Sprint(r[1]) == "assistant" {
					role = "model"
				}
				history = append(history, ai.HistoryTurn{Role: role, Text: fmt.Sprint(r[2])})
			}
		}
	}

	reply, err := m.geminiCli.Generate(ctx, c.SystemPrompt, history, userText)
	if err != nil {
		return
	}

	// Enregistrement de l'usage (tokens + coût) dans Firestore
	_ = m.fsStore.RecordUsage(ctx, clientID, reply.PromptTokens+reply.CompletionTokens, reply.CostUSD)

	// Log dans Google Sheets (message user + réponse assistant)
	if c.SpreadsheetID != "" {
		now := time.Now().Format(time.RFC3339)
		_ = m.sheetsMgr.AppendMessage(ctx, c.SpreadsheetID, now, "user", userText)
		_ = m.sheetsMgr.AppendMessage(ctx, c.SpreadsheetID, now, "assistant", reply.Text)
	}

	m.sendWithAntiBan(ctx, waClient, msgEvt.Info.Chat, reply.Text)
}

// sendWithAntiBan simule "en train d'écrire" puis attend un délai variable
// avant d'envoyer, pour éviter un comportement trop robotique (risque de ban WhatsApp).
func (m *Manager) sendWithAntiBan(ctx context.Context, waClient *whatsmeow.Client, chat types.JID, text string) {
	_ = waClient.SendChatPresence(chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)

	// délai proportionnel à la longueur du message + aléa, entre ~2 et ~8 secondes
	base := 2000 + len(text)*20
	jitter := rand.Intn(3000)
	delay := time.Duration(base+jitter) * time.Millisecond
	if delay > 8*time.Second {
		delay = 8 * time.Second
	}
	time.Sleep(delay)

	_ = waClient.SendChatPresence(chat, types.ChatPresencePaused, types.ChatPresenceMediaText)

	_, _ = waClient.SendMessage(ctx, chat, &waProto.Message{
		Conversation: &text,
	})
}

func extractText(evt *events.Message) string {
	if evt.Message.GetConversation() != "" {
		return evt.Message.GetConversation()
	}
	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}

// périodique : à appeler depuis un ticker dans main.go pour re-sauvegarder
// toutes les sessions actives, au cas où whatsmeow aurait mis à jour des clés
// de chiffrement sans déclencher l'event "success" (rotation normale du protocole)
func (m *Manager) BackupAllSessions(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, h := range m.clients {
		m.backupSession(ctx, id, h.dbPath)
	}
}
