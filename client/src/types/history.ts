export type CallEventLog = {
  timestamp: number;
  type: string;
  message: string;
  details?: string;
};

export type CallTranscriptItem = {
  timestamp: number;
  role: "caller" | "agent";
  text: string;
};

export type HistoryRow = {
  sessionId?: string;
  callId: string;
  owner?: string | null;
  direction: "inbound" | "outbound" | "auto-callback" | string;
  peer: string;
  peerNumber?: string;
  startedAt: number;
  connectedAt?: number | null;
  endedAt?: number | null;
  durationSeconds?: number;
  status?: string;
  endReason?: string | null;
  outcome?: string | null;
  triggerReason?: string | null;
  events?: CallEventLog[];
  transcripts?: CallTranscriptItem[];
  summary?: string | null;
};

