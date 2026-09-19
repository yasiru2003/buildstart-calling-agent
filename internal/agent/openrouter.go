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
	DefaultSystemPrompt    = `ඔබ සජීවී WhatsApp දුරකථන ඇමතුමකට පිළිතුරු දෙන මිත්‍රශීලී, කාරුණික සහ ඉතා ස්වාභාවික මිනිස් හඬ සහායකයෙකි.

අතිශය වැදගත් උපදෙස්:
1. කතා කරන බස (Spoken Sinhala): කිසිවිටෙකත් ලියන/පොත් බසින් කතා නොකරන්න. සැබෑ මිනිසුන් දුරකථනයෙන් කතා කරන සරල, ජීවමාන සිංහලෙන් කතා කරන්න.
2. සංවාදශීලී බව: පෙර කියූ දේම නැවත නැවත නොකියා, අමතන්නාගේ ප්‍රශ්නයට හෝ අදහසට සෘජුව සහ උණුසුම්ව පිළිතුරු දෙන්න. කලින් ආයුබෝවන් කිව්වා නම් නැවත ආයුබෝවන් නොකියන්න.
3. ස්වාභාවික හැඟීම්: 'ආ හරි...', 'ඔව්...', 'හ්ම්...', 'ඇත්තටම...', 'අනිවාර්යයෙන්ම...' වැනි ස්වාභාවික වචන මුලට යොදාගෙන පිළිතුරු දෙන්න.
4. කෙටි සහ පැහැදිලි: දුරකථන ඇමතුමක් බැවින් එක් වරකට වාක්‍ය 1-2 කින් පමණක් කෙටියෙන් පිළිතුරු දෙන්න.
5. අංක කියවීම: ඕනෑම අංකයක් හෝ මිලක් කියවීමේදී අකුරෙන් ලියන්න (උදා: 077 නොව 'බිංදුවයි හතයි හත...', 500 නොව 'පන්සීයක්').
6. කිසිදු markdown, තරු ලකුණු (*), bullet points හෝ emojis භාවිත නොකරන්න.`
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

	audioPrompt := "Listen carefully to what the caller said in this live phone call audio.\n" +
		"Return JSON only with this exact structure:\n" +
		"{\n" +
		"  \"transcription\": \"exact words the caller said in Sinhala or English (leave empty if unintelligible or pure silence)\",\n" +
		"  \"reply\": \"warm, natural, spoken conversational response in colloquial everyday Sinhala (1-2 brief sentences, talk like a real caring human friend on a phone, no robotic or bookish phrases, directly respond to what they asked/said, do not repeat yourself, no emojis, no asterisks, no bullet points)\"\n" +
		"}"

	c.mu.Lock()
	// Build request messages without permanently storing the large audio payload in history
	audioMsg := ChatMessage{
		Role: "user",
		Content: []ContentPart{
			{Type: "text", Text: audioPrompt},
			{Type: "input_audio", InputAudio: &InputAudioPart{Data: b64Audio, Format: "wav"}},
		},
	}
	msgs := make([]ChatMessage, len(c.history)+1)
	copy(msgs, c.history)
	msgs[len(c.history)] = audioMsg
	apiKey := c.apiKey
	model := c.model
	c.mu.Unlock()

	rawText, err := c.sendRequest(ctx, apiKey, model, msgs, true)
	if err != nil {
		return "", "", err
	}

	// Clean code fence blocks or surrounding text if any
	clean := strings.TrimSpace(rawText)
	firstBrace := strings.Index(clean, "{")
	lastBrace := strings.LastIndex(clean, "}")
	if firstBrace != -1 && lastBrace > firstBrace {
		clean = clean[firstBrace : lastBrace+1]
	}

	var parsed AudioTurnResult
	if jsonErr := json.Unmarshal([]byte(clean), &parsed); jsonErr == nil && parsed.Reply != "" {
		transcription = strings.TrimSpace(parsed.Transcription)
		reply = strings.TrimSpace(parsed.Reply)
	} else {
		reply = rawText
	}

	c.mu.Lock()
	// Store the compact text version in history so future turns do not re-send audio blobs
	userHistoryText := transcription
	if userHistoryText == "" {
		userHistoryText = "[Caller spoken audio]"
	}
	c.history = append(c.history, ChatMessage{
		Role:    "user",
		Content: userHistoryText,
	})
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
