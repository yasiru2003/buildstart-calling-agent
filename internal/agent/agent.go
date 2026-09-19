package agent

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type AgentState string

const (
	StateIdle      AgentState = "idle"
	StateListening AgentState = "listening"
	StateThinking  AgentState = "thinking"
	StateSpeaking  AgentState = "speaking"
)

type TranscriptMessage struct {
	Role      string `json:"role"` // "user" or "assistant" or "system"
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
}

type AIAgent struct {
	openRouter *OpenRouterClient
	stt        STTClient
	tts        TTSClient
	vad        *VAD
	log        *slog.Logger

	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	state      AgentState
	enabled    atomic.Bool
	isSpeaking atomic.Bool
	peerFrames atomic.Int64
	speechLock sync.Mutex
	streamMu   sync.Mutex

	// Pre-synthesized greeting audio — populated during ring delay so it plays instantly
	prewarmPCM     []float32
	prewarmMu      sync.Mutex
	customGreeting string

	// Output callback to inject 16 kHz Float32 PCM back into WhatsApp CallManager
	FeedAudio func(pcm []float32)
	// Event callbacks
	OnTranscript  func(msg TranscriptMessage)
	OnStateChange func(state AgentState)
}

func NewAIAgent(openRouterKey, model, systemPrompt string, log *slog.Logger) *AIAgent {
	if log == nil {
		log = slog.Default()
	}
	orClient := NewOpenRouterClient(openRouterKey, model, systemPrompt)
	ttsClient := NewMultiTTS("gemini-aoede")
	sttClient := NewWhisperSTT("", "")
	vad := NewVAD(DefaultVADConfig())

	ctx, cancel := context.WithCancel(context.Background())
	a := &AIAgent{
		openRouter: orClient,
		stt:        sttClient,
		tts:        ttsClient,
		vad:        vad,
		log:        log.With("component", "ai_agent"),
		ctx:        ctx,
		cancel:     cancel,
		state:      StateIdle,
	}
	a.enabled.Store(true)

	a.setupVAD()
	return a
}

func (a *AIAgent) setupVAD() {
	a.vad.OnSpeechStart = func() {
		if !a.enabled.Load() || a.isSpeaking.Load() {
			return
		}
		a.setState(StateListening)
	}

	a.vad.OnSpeechEnd = func(pcm []float32, wav []byte) {
		if !a.enabled.Load() || a.isSpeaking.Load() || a.GetState() == StateThinking || a.GetState() == StateSpeaking {
			return
		}
		a.log.Info("caller finished speaking, generating AI response", "samples", len(pcm), "wavBytes", len(wav))
		go a.handleCallerSpeech(pcm, wav)
	}
}

func (a *AIAgent) handleCallerSpeech(pcm []float32, wav []byte) {
	a.speechLock.Lock()
	defer a.speechLock.Unlock()

	a.setState(StateThinking)

	// Concurrently query intelligence
	type chatResult struct {
		transcription string
		replyText     string
		err           error
	}
	resChan := make(chan chatResult, 1)

	go func() {
		t, r, err := a.openRouter.ChatWithAudio(a.ctx, wav)
		if err != nil {
			a.log.Warn("multimodal chat retry with text", "err", err)
			r, err = a.openRouter.Chat(a.ctx, "The caller just finished speaking on the phone. Answer warmly in 1-2 brief spoken sentences in Sinhala.")
		}
		resChan <- chatResult{transcription: t, replyText: r, err: err}
	}()

	// Wait for the intelligence result
	res := <-resChan
	transcription := res.transcription
	replyText := res.replyText
	err := res.err

	if strings.TrimSpace(transcription) != "" {
		a.emitTranscript("user", transcription)
	} else {
		a.emitTranscript("user", "🎤 [Caller speaking]")
	}

	if err != nil || replyText == "" {
		a.log.Error("OpenRouter response empty", "err", err)
		replyText = "මම අසා සිටිමි, කියන්නකො."
	}

	a.log.Info("AI response generated", "transcription", transcription, "reply", replyText)
	a.emitTranscript("assistant", replyText)

	// Step 2: High-Fidelity Text to Speech (Google AI Studio Gemini Live Voice)
	a.setState(StateSpeaking)
	outPCM, err := a.tts.Synthesize(a.ctx, replyText)
	if err != nil || len(outPCM) == 0 {
		a.log.Error("TTS synthesis failed", "err", err)
		a.setState(StateIdle)
		return
	}

	// Stream synthesized audio to WhatsApp VoIP
	a.streamAudioToCall(outPCM)
	a.setState(StateIdle)
}

func (a *AIAgent) SetCustomGreeting(text string) {
	a.prewarmMu.Lock()
	defer a.prewarmMu.Unlock()
	a.customGreeting = text
}

// PrewarmGreeting synthesizes the greeting text in the background during the ring
// delay so the audio is ready to play the instant the call goes active, eliminating
// the TTS latency that would otherwise cause 2-3 seconds of silence.
func (a *AIAgent) PrewarmGreeting() {
	if !a.enabled.Load() {
		return
	}
	greeting := "හෙලෝ, ආයුබෝවන්! කියන්නකො, මම කොහොමද ඔයාට උදව් කරන්න ඕනෙ?"
	if strings.HasPrefix(a.tts.GetVoice(), "en-") {
		greeting = "Hello! How can I help you today?"
	}
	a.prewarmMu.Lock()
	if a.customGreeting != "" {
		greeting = a.customGreeting
	}
	a.prewarmMu.Unlock()

	go func() {
		pcm, err := a.tts.Synthesize(a.ctx, greeting)
		if err != nil || len(pcm) == 0 {
			a.log.Warn("prewarm greeting synthesis failed", "err", err)
			return
		}
		a.prewarmMu.Lock()
		a.prewarmPCM = pcm
		a.prewarmMu.Unlock()
		a.log.Info("greeting pre-synthesized and ready", "samples", len(pcm))
	}()
}

func (a *AIAgent) GreetCaller() {
	if !a.enabled.Load() {
		return
	}
	go func() {
		// Margin to let WhatsApp media path fully establish before sending audio
		time.Sleep(750 * time.Millisecond)
		if !a.enabled.Load() || a.isSpeaking.Load() {
			return
		}
		a.speechLock.Lock()
		defer a.speechLock.Unlock()

		greeting := "හෙලෝ, ආයුබෝවන්! කියන්නකො, මම කොහොමද ඔයාට උදව් කරන්න ඕනෙ?"
		if strings.HasPrefix(a.tts.GetVoice(), "en-") {
			greeting = "Hello! How can I help you today?"
		}
		a.prewarmMu.Lock()
		if a.customGreeting != "" {
			greeting = a.customGreeting
		}
		outPCM := a.prewarmPCM
		a.prewarmPCM = nil
		a.prewarmMu.Unlock()

		if len(outPCM) == 0 {
			a.log.Info("prewarm not ready, synthesizing greeting now")
			var err error
			outPCM, err = a.tts.Synthesize(a.ctx, greeting)
			if err != nil || len(outPCM) == 0 {
				a.log.Error("TTS greeting synthesis failed", "err", err)
				a.setState(StateIdle)
				return
			}
		}

		a.emitTranscript("assistant", greeting)
		if a.openRouter != nil {
			a.openRouter.AddAssistantMessage(greeting)
		}
		a.setState(StateSpeaking)
		a.streamAudioToCall(outPCM)
		a.setState(StateIdle)
	}()
}

func (a *AIAgent) streamAudioToCall(pcm []float32) {
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	a.isSpeaking.Store(true)
	defer func() {
		// Cooldown period after speech ends to prevent speaker echo triggering VAD
		time.Sleep(350 * time.Millisecond)
		a.vad.Reset()
		a.isSpeaking.Store(false)
	}()

	chunkSize := 960 // 60ms frames at 16 kHz
	ticker := time.NewTicker(60 * time.Millisecond)
	defer ticker.Stop()

	for offset := 0; offset < len(pcm); offset += chunkSize {
		if !a.enabled.Load() || a.ctx.Err() != nil {
			break
		}
		end := min(offset+chunkSize, len(pcm))
		frame := pcm[offset:end]
		if len(frame) < chunkSize {
			padded := make([]float32, chunkSize)
			copy(padded, frame)
			frame = padded
		}
		if a.FeedAudio != nil {
			a.FeedAudio(frame)
		}
		<-ticker.C
	}
}

// FeedPeerAudio feeds incoming WhatsApp audio into VAD with full echo gating
func (a *AIAgent) FeedPeerAudio(pcm []float32) {
	a.peerFrames.Add(1)
	if !a.enabled.Load() || a.isSpeaking.Load() {
		// Gated while AI is speaking or cooling down
		return
	}
	a.vad.ProcessFrame(pcm)
}

func (a *AIAgent) HasSpokenWithPeer() bool {
	// A real conversation has at least 50 incoming audio frames (~1 second of audio)
	return a.peerFrames.Load() >= 50
}

func (a *AIAgent) SetEnabled(enabled bool) {
	a.enabled.Store(enabled)
	if !enabled {
		a.isSpeaking.Store(false)
		a.vad.Reset()
		a.setState(StateIdle)
	}
}

func (a *AIAgent) IsEnabled() bool {
	return a.enabled.Load()
}

func (a *AIAgent) SetVoice(v string) {
	if a.tts != nil {
		a.tts.SetVoice(v)
	}
}

func (a *AIAgent) SetAzureTTS(key, region string) {
	if a.tts != nil {
		a.tts.SetAzureConfig(key, region)
	}
}

func (a *AIAgent) SetGoogleCloudTTS(key string) {
	if a.tts != nil {
		a.tts.SetGoogleCloudConfig(key)
	}
}

func (a *AIAgent) SetHuggingFaceTTS(token string) {
	if a.tts != nil {
		a.tts.SetHuggingFaceToken(token)
	}
}

func (a *AIAgent) SetOpenAITTS(url, key string) {
	if a.tts != nil {
		a.tts.SetOpenAIConfig(url, key)
	}
}

func (a *AIAgent) GetState() AgentState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

func (a *AIAgent) setState(s AgentState) {
	a.mu.Lock()
	a.state = s
	a.mu.Unlock()
	if a.OnStateChange != nil {
		a.OnStateChange(s)
	}
}

func (a *AIAgent) emitTranscript(role, text string) {
	msg := TranscriptMessage{
		Role:      role,
		Text:      text,
		Timestamp: time.Now().UnixMilli(),
	}
	if a.OnTranscript != nil {
		a.OnTranscript(msg)
	}
}

func (a *AIAgent) Close() {
	a.cancel()
	a.SetEnabled(false)
}
