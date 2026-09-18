package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"wacalls/internal/agent"
	"wacalls/internal/voip/call"
	"wacalls/internal/voip/core"
	"wacalls/internal/voip/signaling"
	"wacalls/internal/voip/wanode"
	"wacalls/internal/wa"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

type Session struct {
	id   string
	name string
	mgr  *SessionManager
	log  *slog.Logger

	client *whatsmeow.Client
	reg    *callRegistry

	mu   sync.Mutex
	auth AuthSnapshot

	lastCallbackMu sync.Mutex
	lastCallbacks  map[string]time.Time
}

func newSession(mgr *SessionManager, id, name string, client *whatsmeow.Client) *Session {
	s := &Session{
		id:            id,
		name:          name,
		mgr:           mgr,
		log:           mgr.log.With("session", id),
		client:        client,
		auth:          AuthSnapshot{State: "connecting"},
		reg:           newCallRegistry(),
		lastCallbacks: make(map[string]time.Time),
	}
	client.AddEventHandler(s.handleEvent)
	return s
}

func (s *Session) createCall(callID string) *call.CallManager {
	cm := call.NewCallManager(wa.NewSocket(s.client), s.log)
	ac := &activeCall{cm: cm}

	if cfg := s.mgr.agentConfig; cfg != nil {
		ag := agent.NewAIAgent(cfg.RawKey(), cfg.CurrentModel(), cfg.CurrentPrompt(), s.log.With("call_id", callID))
		if azKey, azReg := cfg.AzureConfig(); azKey != "" {
			ag.SetAzureTTS(azKey, azReg)
		}
		if gKey := cfg.GoogleConfig(); gKey != "" {
			ag.SetGoogleCloudTTS(gKey)
		}
		if hfTok := cfg.HuggingFaceToken(); hfTok != "" {
			ag.SetHuggingFaceTTS(hfTok)
		}
		if cURL, cKey := cfg.CustomTts(); cURL != "" {
			ag.SetOpenAITTS(cURL, cKey)
		}
		if voice := cfg.CurrentVoice(); voice != "" {
			ag.SetVoice(voice)
		}
		ag.SetEnabled(cfg.IsEnabled())
		ag.FeedAudio = func(pcm []float32) {
			cm.FeedCapturedPCM(pcm)
		}
		ag.OnTranscript = func(msg agent.TranscriptMessage) {
			s.mgr.broker.emitAgentTranscript(s.id, callID, msg.Role, msg.Text, msg.Timestamp)
		}
		ag.OnStateChange = func(state agent.AgentState) {
			s.mgr.broker.emitAgentStatus(s.id, callID, ag.IsEnabled(), string(state))
		}
		ac.agent = ag
	}

	s.wireCall(cm, callID)
	s.reg.add(callID, ac)
	return cm
}

func (s *Session) wireCall(cm *call.CallManager, callID string) {
	cm.OnIncoming = func(c *call.CallInfo) {
		s.mgr.broker.upsertCall(CallRecord{
			SessionID: s.id, CallID: c.CallID, Direction: "inbound", Peer: c.PeerJid,
			StartedAt: time.Now().UnixMilli(), Status: StatusRinging,
		})
		s.mgr.broker.emitIncoming(s.id, c.CallID, c.PeerJid)
	}
	cm.OnStateChange = func(c *call.CallInfo) {
		if c.IsEnded() {
			if ac, ok := s.reg.get(c.CallID); ok && ac.agent != nil {
				ac.agent.Close()
			}
			s.removeCall(c.CallID)
			s.mgr.broker.endCall(c.CallID, string(c.StateData.EndReason))
			return
		}
		dir := "outbound"
		if c.Direction == core.CallDirectionIncoming {
			dir = "inbound"
		}
		existing, _ := s.mgr.broker.getCall(c.CallID)
		rec := CallRecord{
			SessionID: s.id, CallID: c.CallID, Direction: dir, Peer: c.PeerJid,
			StartedAt: time.Now().UnixMilli(), Status: mapStatus(c.StateData.State),
		}
		if existing != nil {
			rec.Owner = existing.Owner
			rec.StartedAt = existing.StartedAt
		}
		if c.StateData.State == core.CallStateActive {
			if ac, ok := s.reg.get(c.CallID); ok && ac.agent != nil && ac.agent.IsEnabled() {
				ac.agent.GreetCaller()
			}
		}
		s.mgr.broker.upsertCall(rec)
	}
	cm.OnEnded = func(c *call.CallInfo) {
		var callbackJID types.JID
		var hadConversation bool
		if ac, ok := s.reg.get(c.CallID); ok {
			callbackJID = ac.callbackJID
			if ac.agent != nil {
				hadConversation = ac.agent.HasSpokenWithPeer()
				ac.agent.Close()
			}
		}
		s.removeCall(c.CallID)
		s.mgr.broker.endCall(c.CallID, string(c.StateData.EndReason))

		// If inbound call was missed/unanswered, OR dropped/ended quickly (<15s) without any conversation:
		isShortOrDropped := c.StateData.ConnectedAt == nil || time.Since(*c.StateData.ConnectedAt) < 15*time.Second || !hadConversation
		if c.Direction == core.CallDirectionIncoming && isShortOrDropped {
			peer := callbackJID
			if peer.IsEmpty() {
				peer, _ = types.ParseJID(c.PeerJid)
			}
			if !peer.IsEmpty() {
				s.scheduleAutoCallback(peer.ToNonAD(), "inbound_missed_or_dropped: "+string(c.StateData.EndReason))
			}
		}
	}
	cm.OnPeerAudio = func(pcm16 []float32) {
		ac, ok := s.reg.get(callID)
		if !ok {
			return
		}
		if ac.bridge != nil {
			_ = ac.bridge.WritePCM(pcm16)
		}
		if ac.agent != nil && ac.agent.IsEnabled() {
			ac.agent.FeedPeerAudio(pcm16)
		}
	}
}

func (s *Session) startOutgoingWithGreeting(ctx context.Context, peer types.JID, isVideo bool, customGreeting string) (string, error) {
	callID := signaling.GenerateCallID()
	cm := s.createCall(callID)
	if ac, ok := s.reg.get(callID); ok && ac.agent != nil {
		if customGreeting != "" {
			ac.agent.SetCustomGreeting(customGreeting)
		}
		ac.agent.PrewarmGreeting()
	}
	if err := cm.StartCall(ctx, callID, peer, isVideo); err != nil {
		s.removeCall(callID)
		return "", err
	}
	return callID, nil
}

func (s *Session) startOutgoing(ctx context.Context, peer types.JID, isVideo bool) (string, error) {
	return s.startOutgoingWithGreeting(ctx, peer, isVideo, "")
}

func (s *Session) callForEvent(from types.JID, data *waBinary.Node) (*activeCall, bool) {
	callID := callIDFromNode(wrapCall(from, data))
	if callID == "" {
		return nil, false
	}
	return s.reg.get(callID)
}

func (s *Session) onIncomingOffer(ctx context.Context, evt *events.CallOffer) {
	node := wrapCall(evt.From, evt.Data)
	callID := callIDFromNode(node)
	if callID == "" {
		return
	}
	if max := s.mgr.maxCalls; max > 0 && s.reg.count() >= max {
		s.rejectOffer(ctx, node, evt.From)
		return
	}
	cm := s.createCall(callID)
	// Prioritize phone number JID (CallCreatorAlt) for direct mobile callback
	callbackJID := evt.CallCreatorAlt
	if callbackJID.IsEmpty() {
		callbackJID = evt.CallCreator
	}
	if callbackJID.IsEmpty() {
		callbackJID = evt.From
	}
	if ac, ok := s.reg.get(callID); ok {
		ac.callbackJID = callbackJID
	}
	cm.HandleCallOffer(ctx, node, evt.From)

	if s.mgr.agentConfig != nil && s.mgr.agentConfig.IsAutoAnswer() {
		s.log.Info("auto-answering incoming WhatsApp call after 1.2s ring with AI Agent", "call_id", callID)
		go func() {
			// Pre-synthesize the greeting during the ring delay so it plays instantly on connect
			if ac, ok := s.reg.get(callID); ok && ac.agent != nil {
				ac.agent.PrewarmGreeting()
			}
			time.Sleep(1200 * time.Millisecond)
			s.mgr.broker.emitIncomingClaimed(s.id, callID, "ai-agent")
			if err := cm.AcceptCall(context.Background(), callID); err != nil {
				s.log.Error("failed to auto-answer incoming call", "call_id", callID, "err", err)
			}
		}()
	}
}

func (s *Session) rejectOffer(ctx context.Context, node *waBinary.Node, from types.JID) {
	info := signaling.ExtractNodeInfo(node)
	if info == nil {
		return
	}
	creator := wanode.AttrString(info.InnerNode.Attrs, "call-creator")
	if creator == "" {
		creator = from.String()
	}
	reject := signaling.BuildRejectStanza(from, info.CallID, wanode.MustJID(creator))
	_ = wa.NewSocket(s.client).SendNode(ctx, reject)
	s.log.Info("inbound call rejected: session at capacity", "call_id", info.CallID)
}

func (s *Session) handleEvent(rawEvt any) {
	ctx := context.Background()
	switch evt := rawEvt.(type) {
	case *events.Connected:
		if id := s.client.Store.ID; id != nil {
			_ = s.mgr.store.setJID(s.mgr.appCtx, s.id, id.String())
		}
		s.setAuth(AuthSnapshot{State: "open", Paired: true})
	case *events.LoggedOut:
		s.setAuth(AuthSnapshot{State: "logged_out", Paired: false})
	case *events.CallOffer:
		s.onIncomingOffer(ctx, evt)
	case *events.CallOfferNotice:
		s.log.Info("call offer notice received (offline/missed call)", "from", evt.From, "creator", evt.CallCreator)
		peer := evt.CallCreator
		if peer.IsEmpty() {
			peer = evt.From
		}
		s.scheduleAutoCallback(peer, "call_offer_notice")
	case *events.Message:
		s.handleIncomingMessage(evt)
	case *events.CallAccept:
		if ac, ok := s.callForEvent(evt.From, evt.Data); ok {
			ac.cm.HandleCallAccept(ctx, wrapCall(evt.From, evt.Data), evt.From)
		}
	case *events.CallTransport:
		if ac, ok := s.callForEvent(evt.From, evt.Data); ok {
			ac.cm.HandleCallTransport(ctx, wrapCall(evt.From, evt.Data), evt.From)
		}
	case *events.CallTerminate:
		if ac, ok := s.callForEvent(evt.From, evt.Data); ok {
			ac.cm.HandleCallTerminate(wrapCall(evt.From, evt.Data), evt.From)
		}
	case *events.CallReject:
		if ac, ok := s.callForEvent(evt.From, evt.Data); ok {
			ac.cm.HandleCallTerminate(wrapCall(evt.From, evt.Data), evt.From)
		}
	}
}

func (s *Session) handleIncomingMessage(evt *events.Message) {
	// 1. Check for Missed Call stub message from WhatsApp WebMessageInfo
	if evt.SourceWebMsg != nil {
		stub := evt.SourceWebMsg.GetMessageStubType()
		if stub == waWeb.WebMessageInfo_CALL_MISSED_VOICE ||
			stub == waWeb.WebMessageInfo_CALL_MISSED_VIDEO ||
			stub == waWeb.WebMessageInfo_CALL_MISSED_GROUP_VOICE ||
			stub == waWeb.WebMessageInfo_CALL_MISSED_GROUP_VIDEO {
			s.log.Info("missed call message detected from WhatsApp", "sender", evt.Info.Sender, "stub", stub)
			s.scheduleAutoCallback(evt.Info.Sender, "missed_call_stub")
			return
		}
	}

	// 2. Check for E2E CallLogMessage if present
	if evt.Message != nil && evt.Message.GetCallLogMesssage() != nil {
		cl := evt.Message.GetCallLogMesssage()
		if cl.GetCallOutcome() == waE2E.CallLogMessage_MISSED {
			s.log.Info("call log message missed detected", "sender", evt.Info.Sender)
			s.scheduleAutoCallback(evt.Info.Sender, "call_log_missed")
			return
		}
	}

	// 3. Check for text triggers like "call", "call me", "call back", "කතා කරන්න"
	if evt.Message != nil {
		text := strings.TrimSpace(strings.ToLower(evt.Message.GetConversation()))
		if text == "" && evt.Message.GetExtendedTextMessage() != nil {
			text = strings.TrimSpace(strings.ToLower(evt.Message.GetExtendedTextMessage().GetText()))
		}
		if text != "" {
			if text == "call" || text == "call me" || text == "callback" || text == "call back" ||
				strings.Contains(text, "call me") || strings.Contains(text, "කතා කරන්න") || strings.Contains(text, "call කරන්න") {
				s.log.Info("call request keyword received in chat", "sender", evt.Info.Sender, "text", text)
				s.scheduleAutoCallback(evt.Info.Sender, "text_trigger: "+text)
			}
		}
	}
}

func (s *Session) scheduleAutoCallback(peer types.JID, reason string) {
	if s.mgr.agentConfig == nil || !s.mgr.agentConfig.IsEnabled() {
		return
	}
	if peer.IsEmpty() {
		return
	}
	// Don't call our own number
	if s.client.Store.ID != nil && peer.User == s.client.Store.ID.User {
		return
	}

	peerStr := peer.ToNonAD().String()
	s.lastCallbackMu.Lock()
	if s.lastCallbacks == nil {
		s.lastCallbacks = make(map[string]time.Time)
	}
	last, exists := s.lastCallbacks[peerStr]
	if exists && time.Since(last) < 15*time.Second {
		s.lastCallbackMu.Unlock()
		s.log.Debug("auto-callback debounced (called recently)", "peer", peerStr, "reason", reason)
		return
	}
	s.lastCallbacks[peerStr] = time.Now()
	s.lastCallbackMu.Unlock()

	s.log.Info("scheduling instant AI auto-callback", "peer", peerStr, "reason", reason)
	go func() {
		// Wait 2.0s so the caller's phone returns to idle state from the previous attempt
		time.Sleep(2000 * time.Millisecond)

		// Check if we already have an active call with this peer
		for _, ac := range s.reg.all() {
			if ac.cm != nil {
				c := ac.cm.CurrentCall()
				if c != nil && !c.IsEnded() && c.IsActive() && strings.Contains(c.PeerJid, peer.User) {
					s.log.Info("skipping auto-callback, call already active with peer", "peer", peerStr)
					return
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		callbackGreeting := "ආයුබෝවන්! ඔබ මීට සුළු මොහොතකට පෙර අප අමතන්නට උත්සාහ කළ බව දුටුවෙමි. මම ඔබගේ AI හඬ සහායකයා. ඔබට අද මම කොහොමද උදවු කරන්නේ?"
		if strings.HasPrefix(s.mgr.agentConfig.CurrentVoice(), "en-") {
			callbackGreeting = "Hello! I saw that you just tried calling our WhatsApp line. I am your AI voice assistant. How can I help you today?"
		}

		callID, err := s.startOutgoingWithGreeting(ctx, peer, false, callbackGreeting)
		if err != nil {
			s.log.Error("failed to start auto-callback", "peer", peerStr, "err", err)
			return
		}
		s.log.Info("auto-callback placed successfully", "call_id", callID, "peer", peerStr)
	}()
}

func (s *Session) connect(ctx context.Context) error {
	if s.client.Store.ID != nil {
		return s.client.Connect()
	}
	return s.startPairing(ctx)
}

func (s *Session) startPairing(ctx context.Context) error {
	qrChan, err := s.client.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err := s.client.Connect(); err != nil {
		return err
	}
	go func() {
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				s.log.Info("scan the QR code to pair this session")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				s.setAuth(AuthSnapshot{State: "qr", QR: evt.Code})
				s.mgr.broker.emitSessionQR(s.id, evt.Code)
			case "success":
				if id := s.client.Store.ID; id != nil {
					_ = s.mgr.store.setJID(s.mgr.appCtx, s.id, id.String())
				}
				s.setAuth(AuthSnapshot{State: "open", Paired: true})
			case "timeout":
				s.setAuth(AuthSnapshot{State: "logged_out", Paired: false})
			}
		}
	}()
	return nil
}

func (s *Session) setAuth(a AuthSnapshot) {
	s.mu.Lock()
	s.auth = a
	s.mu.Unlock()
	s.mgr.broker.emitAuthState(s.id, a)
	s.mgr.broker.emitSessionList(s.mgr.infos())
}

func (s *Session) info() SessionInfo {
	s.mu.Lock()
	a := s.auth
	s.mu.Unlock()
	jid := ""
	if id := s.client.Store.ID; id != nil {
		jid = id.String()
	}
	return SessionInfo{ID: s.id, Name: s.name, JID: jid, State: a.State, Paired: a.Paired || jid != ""}
}

func (s *Session) setBridge(callID string, b *Bridge) {
	oldB, found := s.reg.setBridge(callID, b)
	if !found {
		b.Close()
		return
	}
	if oldB != nil {
		oldB.Close()
	}
}

func (s *Session) removeCall(callID string) {
	ac, ok := s.reg.remove(callID)
	if !ok {
		return
	}
	if ac.bridge != nil {
		ac.bridge.Close()
	}
}

func (s *Session) terminateCall(callID string, reason core.EndCallReason) {
	ac, ok := s.reg.get(callID)
	if !ok {
		return
	}
	_ = ac.cm.EndCall(context.Background(), reason)
}

func (s *Session) teardownAllCalls() {
	for _, ac := range s.reg.drain() {
		_ = ac.cm.EndCall(context.Background(), core.EndCallReasonUserEnded)
		if ac.bridge != nil {
			ac.bridge.Close()
		}
	}
}

func (s *Session) replaceClient(client *whatsmeow.Client) {
	s.teardownAllCalls()
	s.client.Disconnect()
	s.client = client
	client.AddEventHandler(s.handleEvent)
}

func (s *Session) shutdown() {
	s.teardownAllCalls()
	s.client.Disconnect()
}

func mapStatus(state core.CallState) CallStatus {
	switch state {
	case core.CallStateActive, core.CallStateConnecting:
		return StatusConnected
	case core.CallStateEnded:
		return StatusEnded
	case core.CallStateInitiating:
		return StatusStarting
	default:
		return StatusRinging
	}
}
