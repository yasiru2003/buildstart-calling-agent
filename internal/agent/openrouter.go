package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	DefaultOpenRouterURL   = "https://openrouter.ai/api/v1/chat/completions"
	DefaultOpenRouterModel = "google/gemini-2.5-flash"
	DefaultSystemPrompt    = "ඔබ ඉතා මිත්‍රශීලී, උණුසුම් සහ ස්වාභාවික මිනිසෙකු මෙන් කතා කරන සිංහල AI සහායකයෙකි. ඔබ සජීවී WhatsApp දුරකථන ඇමතුමකට පිළිතුරු දෙයි.\n\nකතා කරන ආකාරය:\n1. සැබෑ මිනිසෙකු සේ ස්වාභාවිකව 'හ්ම්...', 'ආ...', 'හරි...', 'ඔව්...', 'එහෙමද...' වැනි වචන සුළු වශයෙන් යොදාගනිමින් කතාබහ කරන්න.\n2. පොත් බසින් නොව සාමාන්‍ය කතාබහ කරන ජීවමාන සිංහලෙන් (Colloquial Spoken Sinhala) කෙටියෙන් (වාක්‍ය 1-2 කින්) පිළිතුරු දෙන්න.\n3. ඉලක්කම් ලිවීමේදී ශබ්ද නගා කියවිය හැකි ලෙස සිංහල අකුරෙන් ලියන්න (උදා: 'දහය', 'දෙසීයක්').\n4. කිසිවිටෙකත් markdown, තරු ලකුණු (*), bullet points හෝ emojis භාවිතා නොකරන්න."
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentPart
}

type ContentPart struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	InputAudio *InputAudioPart `json:"input_audio,omitempty"`
	ImageURL   *ImageURLPart   `json:"image_url,omitempty"`
}

type InputAudioPart struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type ImageURLPart struct {
	URL string `json:"url"` // data:audio/wav;base64,...
}

type OpenRouterClient struct {
	apiKey       string
	model        string
	systemPrompt string
	temperature  float64
	maxTokens    int
	client       *http.Client
	mu           sync.Mutex
	history      []ChatMessage
}

func NewOpenRouterClient(apiKey, model, systemPrompt string) *OpenRouterClient {
	if model == "" || model == "openrouter/auto" || strings.Contains(model, "gemini-2.0-flash") {
		model = DefaultOpenRouterModel
	}
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	c := &OpenRouterClient{
		apiKey:       apiKey,
		model:        model,
		systemPrompt: systemPrompt,
		temperature:  0.7,
		maxTokens:    350, // Adequate tokens for Sinhala Unicode script & JSON
		client:       &http.Client{Timeout: 20 * time.Second},
		history:      make([]ChatMessage, 0),
	}
	c.Reset()
	return c
}

type AudioTurnResult struct {
	Transcription string `json:"transcription"`
	Reply         string `json:"reply"`
}

func (c *OpenRouterClient) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = []ChatMessage{
		{Role: "system", Content: c.systemPrompt},
	}
}

func (c *OpenRouterClient) SetSystemPrompt(prompt string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.systemPrompt = prompt
	if len(c.history) > 0 && c.history[0].Role == "system" {
		c.history[0].Content = prompt
	}
}

func (c *OpenRouterClient) SetModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if model == "" || model == "openrouter/auto" || strings.Contains(model, "gemini-2.0-flash") {
		model = DefaultOpenRouterModel
	}
	c.model = model
}

func (c *OpenRouterClient) SetAPIKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.apiKey = key
}

func (c *OpenRouterClient) Chat(ctx context.Context, userText string) (string, error) {
	c.mu.Lock()
	c.history = append(c.history, ChatMessage{
		Role:    "user",
		Content: userText,
	})
	msgs := make([]ChatMessage, len(c.history))
	copy(msgs, c.history)
	apiKey := c.apiKey
	model := c.model
	c.mu.Unlock()

	respText, err := c.sendRequest(ctx, apiKey, model, msgs, false)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.history = append(c.history, ChatMessage{
		Role:    "assistant",
		Content: respText,
	})
	if len(c.history) > 15 {
		c.history = append([]ChatMessage{c.history[0]}, c.history[len(c.history)-14:]...)
	}
	c.mu.Unlock()

	return respText, nil
}

func (c *OpenRouterClient) ChatWithAudio(ctx context.Context, wavData []byte) (transcription string, reply string, err error) {
	b64Audio := base64.StdEncoding.EncodeToString(wavData)

	audioPrompt := "Analyze the caller's spoken audio from this live WhatsApp telephone call.\n" +
		"Return JSON only with this exact structure:\n" +
		"{\n" +
		"  \"transcription\": \"exact words the caller said in Sinhala or English (leave empty if unintelligible or pure silence)\",\n" +
		"  \"reply\": \"warm, natural, spoken conversational response in colloquial Sinhala (1-2 brief sentences, no emojis, no asterisks, no bullet points)\"\n" +
		"}"

	c.mu.Lock()
	c.history = append(c.history, ChatMessage{
		Role: "user",
		Content: []ContentPart{
			{Type: "text", Text: audioPrompt},
			{Type: "input_audio", InputAudio: &InputAudioPart{Data: b64Audio, Format: "wav"}},
		},
	})
	msgs := make([]ChatMessage, len(c.history))
	copy(msgs, c.history)
	apiKey := c.apiKey
	model := c.model
	c.mu.Unlock()

	rawText, err := c.sendRequest(ctx, apiKey, model, msgs, true)
	if err != nil {
		return "", "", err
	}

	// Clean code fence blocks if any
	clean := strings.TrimSpace(rawText)
	if strings.HasPrefix(clean, "```json") {
		clean = strings.TrimPrefix(clean, "```json")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	} else if strings.HasPrefix(clean, "```") {
		clean = strings.TrimPrefix(clean, "```")
		clean = strings.TrimSuffix(clean, "```")
		clean = strings.TrimSpace(clean)
	}

	var parsed AudioTurnResult
	if jsonErr := json.Unmarshal([]byte(clean), &parsed); jsonErr == nil && parsed.Reply != "" {
		transcription = strings.TrimSpace(parsed.Transcription)
		reply = strings.TrimSpace(parsed.Reply)
	} else {
		reply = clean
	}

	c.mu.Lock()
	c.history = append(c.history, ChatMessage{
		Role:    "assistant",
		Content: reply,
	})
	if len(c.history) > 15 {
		c.history = append([]ChatMessage{c.history[0]}, c.history[len(c.history)-14:]...)
	}
	c.mu.Unlock()

	return transcription, reply, nil
}

func (c *OpenRouterClient) sendRequest(ctx context.Context, apiKey, model string, msgs []ChatMessage, jsonFormat bool) (string, error) {
	reqBody := map[string]any{
		"model":       model,
		"messages":    msgs,
		"temperature": c.temperature,
		"max_tokens":  c.maxTokens,
	}
	if jsonFormat {
		reqBody["response_format"] = map[string]string{"type": "json_object"}
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, DefaultOpenRouterURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://github.com/JotaDev66/WaCalls")
	httpReq.Header.Set("X-Title", "WaCalls Voice AI")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OpenRouter API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse OpenRouter response: %w", err)
	}

	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("OpenRouter error: %s", parsed.Error.Message)
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from OpenRouter")
	}

	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
