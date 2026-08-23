package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s"

type Client struct {
	apiKey          string
	model           string // ex: "gemini-1.5-flash"
	httpClient      *http.Client
	inputCostPer1K  float64
	outputCostPer1K float64
}

func NewClient(apiKey, model string, inputCostPer1K, outputCostPer1K float64) *Client {
	return &Client{
		apiKey:          apiKey,
		model:           model,
		httpClient:      &http.Client{},
		inputCostPer1K:  inputCostPer1K,
		outputCostPer1K: outputCostPer1K,
	}
}

type contentPart struct {
	Text string `json:"text"`
}
type content struct {
	Role  string        `json:"role"`
	Parts []contentPart `json:"parts"`
}
type requestBody struct {
	SystemInstruction *content  `json:"system_instruction,omitempty"`
	Contents          []content `json:"contents"`
}
type usageMetadata struct {
	PromptTokenCount     int64 `json:"promptTokenCount"`
	CandidatesTokenCount int64 `json:"candidatesTokenCount"`
	TotalTokenCount      int64 `json:"totalTokenCount"`
}
type responseBody struct {
	Candidates []struct {
		Content content `json:"content"`
	} `json:"candidates"`
	UsageMetadata usageMetadata `json:"usageMetadata"`
}

// Reply représente le résultat d'un appel Gemini avec le coût déjà calculé,
// prêt à être enregistré via store.RecordUsage
type Reply struct {
	Text             string
	PromptTokens     int64
	CompletionTokens int64
	CostUSD          float64
}

// Generate envoie le prompt système + l'historique récent (format simple role/text)
// et renvoie la réponse avec le décompte de tokens et le coût estimé.
func (c *Client) Generate(ctx context.Context, systemPrompt string, history []HistoryTurn, userMessage string) (*Reply, error) {
	contents := make([]content, 0, len(history)+1)
	for _, h := range history {
		contents = append(contents, content{Role: h.Role, Parts: []contentPart{{Text: h.Text}}})
	}
	contents = append(contents, content{Role: "user", Parts: []contentPart{{Text: userMessage}}})

	body := requestBody{
		SystemInstruction: &content{Parts: []contentPart{{Text: systemPrompt}}},
		Contents:          contents,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf(geminiEndpoint, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini a répondu %d: %s", resp.StatusCode, string(raw))
	}

	var rb responseBody
	if err := json.Unmarshal(raw, &rb); err != nil {
		return nil, err
	}
	if len(rb.Candidates) == 0 || len(rb.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("réponse gemini vide")
	}

	cost := (float64(rb.UsageMetadata.PromptTokenCount)/1000)*c.inputCostPer1K +
		(float64(rb.UsageMetadata.CandidatesTokenCount)/1000)*c.outputCostPer1K

	return &Reply{
		Text:             rb.Candidates[0].Content.Parts[0].Text,
		PromptTokens:     rb.UsageMetadata.PromptTokenCount,
		CompletionTokens: rb.UsageMetadata.CandidatesTokenCount,
		CostUSD:          cost,
	}, nil
}

type HistoryTurn struct {
	Role string // "user" ou "model"
	Text string
}
