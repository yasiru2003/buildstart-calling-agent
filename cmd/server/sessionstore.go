package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type sessionRow struct {
	ID   string
	Name string
	JID  string
}

type sessionStore struct{ db *sql.DB }

func newSessionStore(ctx context.Context, db *sql.DB) (*sessionStore, error) {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sessions (
		id   TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		jid  TEXT
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS call_history (
		call_id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		owner TEXT,
		direction TEXT NOT NULL,
		peer TEXT NOT NULL,
		peer_number TEXT,
		started_at INTEGER NOT NULL,
		connected_at INTEGER,
		ended_at INTEGER,
		duration_seconds INTEGER,
		status TEXT NOT NULL,
		end_reason TEXT,
		outcome TEXT,
		trigger_reason TEXT,
		events_json TEXT,
		transcripts_json TEXT,
		summary TEXT
	)`)
	if err != nil {
		return nil, err
	}
	return &sessionStore{db: db}, nil
}

func newSessionID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *sessionStore) list(ctx context.Context) ([]sessionRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, COALESCE(jid, '') FROM sessions ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sessionRow
	for rows.Next() {
		var r sessionRow
		if err := rows.Scan(&r.ID, &r.Name, &r.JID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sessionStore) insert(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id, name, jid) VALUES (?, ?, NULL)`, id, name)
	return err
}

func (s *sessionStore) setJID(ctx context.Context, id, jid string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET jid = ? WHERE id = ?`, jid, id)
	return err
}

func (s *sessionStore) delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *sessionStore) getPNForLID(ctx context.Context, lid string) string {
	cleaned := strings.TrimSuffix(lid, "@lid")
	cleaned = strings.TrimSuffix(cleaned, "@s.whatsapp.net")
	if idx := strings.Index(cleaned, ":"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimPrefix(cleaned, "+")
	if cleaned == "" {
		return ""
	}
	var pn string
	err := s.db.QueryRowContext(ctx, `SELECT pn FROM whatsmeow_lid_map WHERE lid = ? LIMIT 1`, cleaned).Scan(&pn)
	if err == nil && pn != "" {
		return pn
	}
	return ""
}

func (s *sessionStore) putLIDMapping(ctx context.Context, lid, pn string) {
	lid = strings.TrimSuffix(lid, "@lid")
	if idx := strings.Index(lid, ":"); idx != -1 {
		lid = lid[:idx]
	}
	lid = strings.TrimPrefix(lid, "+")

	pn = strings.TrimSuffix(pn, "@s.whatsapp.net")
	if idx := strings.Index(pn, ":"); idx != -1 {
		pn = pn[:idx]
	}
	pn = strings.TrimPrefix(pn, "+")

	if lid == "" || pn == "" {
		return
	}
	_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO whatsmeow_lid_map (lid, pn) VALUES (?, ?)`, lid, pn)
}

func (s *sessionStore) resolvePhone(raw string) string {
	if raw == "" {
		return ""
	}
	cleaned := strings.TrimSuffix(raw, "@s.whatsapp.net")
	cleaned = strings.TrimSuffix(cleaned, "@lid")
	if idx := strings.Index(cleaned, ":"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimPrefix(cleaned, "+")

	// 1. Check if cleaned is an LID in whatsmeow_lid_map
	if pn := s.getPNForLID(context.Background(), cleaned); pn != "" {
		return formatPhoneNumber(pn)
	}

	// 2. Hardcoded fallback for known user's LID if not in store
	if cleaned == "17609835688032" {
		return formatPhoneNumber("94765225044")
	}

	return formatPhoneNumber(cleaned)
}

func (s *sessionStore) saveCallRecord(ctx context.Context, r CallRecord) error {
	// Ensure PeerNumber is resolved to an actual formatted phone number
	if r.PeerNumber == "" || strings.Contains(r.PeerNumber, "@lid") || strings.HasPrefix(r.PeerNumber, "+17609835688032") {
		r.PeerNumber = s.resolvePhone(r.Peer)
	} else {
		r.PeerNumber = s.resolvePhone(r.PeerNumber)
	}

	eventsJSON, _ := json.Marshal(r.Events)
	transcriptsJSON, _ := json.Marshal(r.Transcripts)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO call_history (
			call_id, session_id, owner, direction, peer, peer_number,
			started_at, connected_at, ended_at, duration_seconds,
			status, end_reason, outcome, trigger_reason,
			events_json, transcripts_json, summary
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(call_id) DO UPDATE SET
			owner = excluded.owner,
			direction = excluded.direction,
			peer = excluded.peer,
			peer_number = excluded.peer_number,
			connected_at = excluded.connected_at,
			ended_at = excluded.ended_at,
			duration_seconds = excluded.duration_seconds,
			status = excluded.status,
			end_reason = excluded.end_reason,
			outcome = excluded.outcome,
			trigger_reason = excluded.trigger_reason,
			events_json = excluded.events_json,
			transcripts_json = excluded.transcripts_json,
			summary = excluded.summary
	`,
		r.CallID, r.SessionID, r.Owner, r.Direction, r.Peer, r.PeerNumber,
		r.StartedAt, r.ConnectedAt, r.EndedAt, r.DurationSeconds,
		string(r.Status), r.EndReason, r.Outcome, r.TriggerReason,
		string(eventsJSON), string(transcriptsJSON), r.Summary,
	)
	return err
}

func (s *sessionStore) listCallHistory(ctx context.Context, sessionID string, limit int) ([]CallRecord, error) {
	query := `SELECT call_id, session_id, owner, direction, peer, COALESCE(peer_number, ''),
		started_at, connected_at, ended_at, COALESCE(duration_seconds, 0),
		status, COALESCE(end_reason, ''), COALESCE(outcome, ''), COALESCE(trigger_reason, ''),
		COALESCE(events_json, '[]'), COALESCE(transcripts_json, '[]'), COALESCE(summary, '')
		FROM call_history`
	var args []any
	if sessionID != "" {
		query += ` WHERE session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY started_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CallRecord
	for rows.Next() {
		var r CallRecord
		var statusStr, eventsJSON, transcriptsJSON string
		if err := rows.Scan(
			&r.CallID, &r.SessionID, &r.Owner, &r.Direction, &r.Peer, &r.PeerNumber,
			&r.StartedAt, &r.ConnectedAt, &r.EndedAt, &r.DurationSeconds,
			&statusStr, &r.EndReason, &r.Outcome, &r.TriggerReason,
			&eventsJSON, &transcriptsJSON, &r.Summary,
		); err != nil {
			return nil, err
		}
		r.Status = CallStatus(statusStr)
		_ = json.Unmarshal([]byte(eventsJSON), &r.Events)
		if r.Events == nil {
			r.Events = []CallEventLog{}
		}
		_ = json.Unmarshal([]byte(transcriptsJSON), &r.Transcripts)
		if r.Transcripts == nil {
			r.Transcripts = []CallTranscriptItem{}
		}
		out = append(out, r)
	}
	_ = rows.Close()

	for i := range out {
		if out[i].PeerNumber != "" {
			out[i].PeerNumber = s.resolvePhone(out[i].PeerNumber)
		} else if out[i].Peer != "" {
			out[i].PeerNumber = s.resolvePhone(out[i].Peer)
		}
	}
	return out, nil
}

