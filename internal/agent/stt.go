package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type STTClient interface {
	Transcribe(ctx context.Context, wavData []byte) (string, error)
}

// WhisperSTT implements STT via OpenAI or Groq Whisper endpoints.
type WhisperSTT struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewWhisperSTT(endpoint, apiKey string) *WhisperSTT {
	if endpoint == "" {
		endpoint = "https://api.groq.com/openai/v1/audio/transcriptions"
	}
	return &WhisperSTT{
		endpoint: endpoint,
		apiKey:   apiKey,
		client:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *WhisperSTT) Transcribe(ctx context.Context, wavData []byte) (string, error) {
	if len(wavData) == 0 {
		return "", nil
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wavData); err != nil {
		return "", err
	}

	_ = writer.WriteField("model", "whisper-1")
	_ = writer.WriteField("language", "en")
	_ = writer.WriteField("response_format", "json")
	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("transcription error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Text  string `json:"text"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("transcription failed: %s", result.Error.Message)
	}

	return strings.TrimSpace(result.Text), nil
}
