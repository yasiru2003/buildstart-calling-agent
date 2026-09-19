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
	DefaultOpenRouterModel = "google/gemini-3.8-flash"
	DefaultSystemPrompt    = `ඔබ Buildstart (බිල්ඩ්ස්ටාර්ට්) වෙනුවෙන් සජීවී WhatsApp දුරකථන ඇමතුමකට පිළිතුරු දෙන දක්ෂ, සුහදශීලී, සැබෑ ශ්‍රී ලාංකික AI කණ්ඩායම් සාමාජිකයෙකි (Buildstart Voice Agent).

[Buildstart පිළිබඳ මූලික දැනුම සහ විකුණුම් පිටපත (Script & Knowledge)]:
- අප කරන්නේ කුමක්ද?: බිල්ඩ්ස්ටාර්ට් (Buildstart) හරහා අපි ව්‍යාපාර වල සමස්ත ක්‍රියාවලියම WhatsApp ඔස්සේ ස්වයංක්‍රීය (automate) කර දෙනවා. මේක නිකන්ම චැට්බොට් එකක්වත්, ටෙම්ප්ලේට් එකක්වත් නෙවෙයි. අපි හදලා දෙන්නේ ඔයාගේ WhatsApp එක ඇතුළේ පැය විසිහතරෙම නොනවත්වා වැඩ කරන සැබෑ AI ටීම් මෙම්බර් කෙනෙක්ව.
- ස්ටාෆ් මෙම්බර් සංකල්පය: කිසිදා නොනිදන, කිසිදා විවේක නොගන්නා, කිසිදු මැසේජ් එකක් මග නොහරින කාර්ය මණ්ඩල සාමාජිකයෙක් බඳවා ගත්තා වගේ තමයි. කස්ටමර්ස්ලා එක්ක කතා කරනවා, ප්‍රශ්න වලට උත්තර දෙනවා, ලීඩ්ස් කොලිෆයි කරනවා, ඇපොයින්ට්මන්ට් බුක් කරනවා, රිමයින්ඩර්ස් යවනවා, ඒ වගේම ෆලෝ-අප් කරනවා — මේ හැමදේම සිද්ධ වෙන්නේ කස්ටමර්ස්ලා නිතරම ඉන්න WhatsApp එක ඇතුළෙමයි.
- කර්මාන්ත (Industries): පාසල් හා විශ්වවිද්‍යාල (ඇඩ්මිෂන්ස්), සාප්පු හා රීටේල් (ඕඩර්ස් හැසිරවීම), ඉඩම් හා දේපළ වෙළඳාම (ගැනුම්කරුවන් කොලිෆයි කිරීම), මානව සම්පත්/HR (අයදුම්කරුවන් ස්ක්‍රීන් කිරීම), හෝටල් (බුකින්ග්ස් හා අමුත්තන්ගේ අවශ්‍යතා), පාරිභෝගික සේවා (සපෝර්ට්). ව්‍යාපාරයේ ස්වභාවය අනුව සකස් කළ පුද්ගලාරෝපිත ඒජන්තයෙක් ලබා දෙන්නේ.
- ප්‍රතිඵලය: ඕනෑම වේලාවක ක්ෂණික ප්‍රතිචාර, වැඩිපුර ලීඩ්ස් ප්‍රමාණයක් අල්ලා ගැනීම, වැඩිපුර ඩීල්ස් ක්ලෝස් කරගැනීම, සාමාන්‍ය කණ්ඩායමකට කළ නොහැකි තරමේ පාරිභෝගික වර්ධනයක්.
- පැකේජ හා මිල ගණන් (Plans): සැලසුම් 3ක් තිබෙනවා — Starter, Business, Enterprise. මිල තීරණය වෙන්නේ සම්බන්ධ කරගන්නා කොන්ටැක්ට්ස් ප්‍රමාණය අනුවයි. මැසේජින් සම්පූර්ණයෙන්ම අන්ලිමිටඩ් (unlimited) නිසා අමතර සැඟවුණු ගාස්තු කිසිවක් නැහැ. (දුරකථනයෙන් ඩොලර් හෝ රුපියල් නිශ්චිත ගණන් නොකියන්න; එය පෙන්වීමට ඩෙමෝ එක බලන්න යෝජනා කරන්න).
- ඉදිරි පියවර (Call to Action): අපේ WhatsApp ඩෙමෝ එකට දැන්ම මැසේජ් එකක් දාලා මේක ලයිව් අත්හදා බලන්න පුළුවන්, නැත්නම් බිල්ඩ්ස්ටාර්ට් කණ්ඩායම සම්බන්ධ කරගන්න පුළුවන්.

[අතිශය වැදගත් නීති]:
1. 100% කතා කරන සිංහල බස (Pure Spoken Sinhala):
   - අමතන්නාට 'ඔයා', 'ඔයාට', 'ඔයාගෙ' කියා පමණක් අමතන්න. කිසි විටෙකත් 'ඔබ', 'ඔබට', 'ඔබගේ' නොකියන්න.
   - 'කියන්නකො', 'පුළුවන්ද', 'පුළුවන්', 'ආයෙත්', 'අහන්න', 'ඕනෙ' වැනි සැබෑ කතා කරන වචන යොදන්න.
   - පොත් වචන ('පවසන්න', 'හැකියි', 'නැවත', 'විමසන්න', 'කාරුණිකව') සම්පූර්ණයෙන්ම තහනම්ය.
2. ලාංකීය කතා විලාසය හා ව්‍යාකරණ (SOV):
   - ක්‍රියා පදය වාක්‍ය අගට තබන්න (උදා: 'අපිට පුළුවන් ඒක කරන්න' නොව 'අපිට ඒක කරලා දෙන්න පුළුවන්').
   - 'ආ හරි...', 'ඔව් අනිවාර්යයෙන්ම...', 'හරි බලමු...', 'ඒක තමයි...' වැනි ස්වාභාවික ලාංකීය ආරම්භක යෙදුම් යොදන්න.
3. කිසිවිටෙකත් එකම ප්‍රශ්නය හෝ ආයුබෝවන් නැවත නොකියන්න:
   - ඇමතුම පටන් ගන්නා විටම පෙර පටිගත කළ සුබපැතුම අමතන්නාට ඇසී අවසන්ය. එම නිසා නැවත 'හෙලෝ, ආයුබෝවන්!' නොකියන්න.
   - 'මොනවද දැනගන්න ඕනෙ?' හෝ 'මම කොහොමද උදව් කරන්න ඕනෙ?' කියා නැවත නැවත අසන්න එපා!
   - අමතන්නා යමක් ඇසූ විට, සෘජුවම Buildstart විසඳුම හෝ විස්තරය පැහැදිලි කර, ඔවුන්ගේ ව්‍යාපාරික ක්ෂේත්‍රය කුමක්දැයි අසන්න, නැතහොත් WhatsApp ඩෙමෝ එකට මඟ පෙන්වන්න.
4. දුරකථන සංවාදයකට ගැළපෙන කෙටි වාක්‍ය (1-3 Sentences):
   - දුරකථන ඇමතුමක් බැවින් එක් වරකට වාක්‍ය 1-3ක් පමණක් සරලව කියන්න. දිගු දේශනා එපා.
5. Formatting තහනම්:
   - කිසිදු markdown, තරු ලකුණු (*), bullet points හෝ emojis නොයොදන්න. කටහඬින් කියවන සරල පාඨ පමණක් ලබා දෙන්න.`
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

func (c *OpenRouterClient) AddAssistantMessage(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.history = append(c.history, ChatMessage{
		Role:    "assistant",
		Content: text,
	})
	if len(c.history) > 15 {
		c.history = append([]ChatMessage{c.history[0]}, c.history[len(c.history)-14:]...)
	}
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

	var respText string
	var err error
	if isGoogleKey(apiKey) {
		respText, err = c.sendGoogleRequest(ctx, apiKey, model, msgs, false)
	} else {
		respText, err = c.sendRequest(ctx, apiKey, model, msgs, false)
	}
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
		"  \"reply\": \"warm, natural, spoken conversational response as the Buildstart AI team member in colloquial everyday Sinhala (1-2 brief sentences, natural spoken Sinhala using 'ඔයා', SOV verb at the end, directly answer their question about Buildstart or their business, never repeat greeting or questions like 'මොනවද දැනගන්න ඕනෙ' or 'කොහොමද උදව් කරන්න ඕනෙ', progress the conversation towards understanding their business or trying the WhatsApp demo, no emojis, no asterisks, no bullet points)\"\n" +
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

	var rawText string
	if isGoogleKey(apiKey) {
		rawText, err = c.sendGoogleAudioRequest(ctx, apiKey, model, wavData, audioPrompt)
	} else {
		rawText, err = c.sendRequest(ctx, apiKey, model, msgs, true)
	}
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
		"reasoning":   map[string]string{"effort": "low"},
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

func isGoogleKey(key string) bool {
	k := strings.TrimSpace(key)
	return strings.HasPrefix(k, "AQ.") || strings.HasPrefix(k, "AIza")
}

func (c *OpenRouterClient) sendGoogleRequest(ctx context.Context, apiKey, model string, msgs []ChatMessage, jsonFormat bool) (string, error) {
	modelName := "gemini-3.6-flash"
	m := strings.TrimPrefix(model, "google/")
	if strings.Contains(m, "gemini-") {
		modelName = m
	}
	if modelName == "gemini-2.0-flash" || modelName == "gemini-2.5-flash" {
		modelName = "gemini-3.6-flash"
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", modelName)

	type Part struct {
		Text string `json:"text"`
	}
	type Content struct {
		Role  string `json:"role"`
		Parts []Part `json:"parts"`
	}

	var contents []Content
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		txt, _ := m.Content.(string)
		if txt != "" {
			contents = append(contents, Content{
				Role:  role,
				Parts: []Part{{Text: txt}},
			})
		}
	}

	reqBody := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []Part{{Text: c.systemPrompt}},
		},
		"contents": contents,
		"generationConfig": map[string]any{
			"temperature":     c.temperature,
			"maxOutputTokens": c.maxTokens,
			"thinkingConfig": map[string]any{
				"thinkingBudget": 0,
			},
		},
	}
	if jsonFormat {
		reqBody["generationConfig"].(map[string]any)["responseMimeType"] = "application/json"
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("X-goog-api-key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Google Gemini API error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var gResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &gResp); err != nil {
		return "", fmt.Errorf("failed to parse Google response: %w", err)
	}
	if gResp.Error != nil && gResp.Error.Message != "" {
		return "", fmt.Errorf("Google Gemini error: %s", gResp.Error.Message)
	}
	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no candidates in Google response")
	}

	return strings.TrimSpace(gResp.Candidates[0].Content.Parts[0].Text), nil
}

func (c *OpenRouterClient) sendGoogleAudioRequest(ctx context.Context, apiKey, model string, wavData []byte, audioPrompt string) (string, error) {
	modelName := "gemini-3.6-flash"
	m := strings.TrimPrefix(model, "google/")
	if strings.Contains(m, "gemini-") {
		modelName = m
	}
	if modelName == "gemini-2.0-flash" || modelName == "gemini-2.5-flash" {
		modelName = "gemini-3.6-flash"
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", modelName)

	b64Audio := base64.StdEncoding.EncodeToString(wavData)

	reqBody := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": c.systemPrompt}},
		},
		"contents": []map[string]any{
			{
				"parts": []any{
					map[string]string{"text": audioPrompt},
					map[string]any{
						"inlineData": map[string]string{
							"mimeType": "audio/wav",
							"data":     b64Audio,
						},
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"temperature":     0.5,
			"maxOutputTokens": 350,
			"thinkingConfig": map[string]any{
				"thinkingBudget": 0,
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("X-goog-api-key", apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Google Gemini Audio API error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var gResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(bodyBytes, &gResp); err != nil {
		return "", fmt.Errorf("failed to parse Google response: %w", err)
	}
	if len(gResp.Candidates) == 0 || len(gResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response candidates from Google Gemini")
	}

	return strings.TrimSpace(gResp.Candidates[0].Content.Parts[0].Text), nil
}
