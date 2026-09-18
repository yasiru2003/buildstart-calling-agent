package signaling

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"wacalls/internal/voip/core"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

var encryptedCallTags = map[string]bool{"preaccept": true, "accept": true}

func NeedsDecryption(tag string) bool { return encryptedCallTags[tag] }

func GenerateCallID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func GenerateCallStanzaID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func padRandomMax16(msg []byte) []byte {
	var r [1]byte
	rand.Read(r[:])
	padLen := int(r[0]&0x0f) + 1
	out := make([]byte, len(msg)+padLen)
	copy(out, msg)
	for i := len(msg); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
}

func unpadRandomMax16(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("unpad given empty bytes")
	}
	pad := int(b[len(b)-1])
	if pad > len(b) {
		return nil, fmt.Errorf("unpad given %d bytes, but pad is %d", len(b), pad)
	}
	return b[:len(b)-pad], nil
}

func EncodeCallKeyMessage(callKey []byte) ([]byte, error) {
	msg := &waE2E.Message{Call: &waE2E.Call{CallKey: callKey}}
	return proto.Marshal(msg)
}

func DecryptCallKeyInNode(ctx context.Context, sock core.VoipSocket, inner *waBinary.Node, peerJid types.JID) ([]byte, error) {
	encNode := findEncNode(inner)
	if encNode == nil {
		return nil, nil
	}
	return sock.DecryptCallKey(ctx, peerJid, encNode)
}

func DecodeCallKeyPlaintext(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 32 {
		return plaintext, nil
	}
	var msg waE2E.Message
	if err := proto.Unmarshal(plaintext, &msg); err == nil {
		if key := msg.GetCall().GetCallKey(); len(key) == 32 {
			return key, nil
		}
	}
	if unpadded, err := unpadRandomMax16(plaintext); err == nil {
		var msgPadded waE2E.Message
		if err := proto.Unmarshal(unpadded, &msgPadded); err == nil {
			if key := msgPadded.GetCall().GetCallKey(); len(key) == 32 {
				return key, nil
			}
		}
	}
	return nil, fmt.Errorf("failed to extract 32-byte callKey from plaintext (len %d)", len(plaintext))
}
