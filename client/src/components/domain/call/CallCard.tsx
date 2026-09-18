import { useEffect, useRef, useState } from "react";
import { PhoneOff, Bot, Sparkles, MessageSquare, Volume2, Mic } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { AudioMeter } from "@/lib/audio-meter";
import { useCalls } from "@/stores/calls";
import { useDevices } from "@/stores/devices";
import { useAgentStore } from "@/stores/agent";
import { useEndCall } from "@/hooks/useEndCall";
import { formatCallDuration } from "@/utils/format";
import type { CallStatus, CallSummary } from "@/types/call";

const statusVariant: Record<CallStatus, "success" | "secondary" | "muted"> = {
  connected: "success",
  ringing: "secondary",
  starting: "secondary",
  ended: "muted",
};

const EMPTY_TRANSCRIPTS: any[] = [];
const DEFAULT_AGENT_STATE = { enabled: true, state: "idle" as const };

export const CallCard = ({ call }: { call: CallSummary }) => {
  const conn = useCalls((s) => s.ownConnections.get(call.callId));
  const outDeviceId = useDevices((s) => s.outId);
  const endCall = useEndCall();
  const [, force] = useState(0);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const audioRef = useRef<HTMLAudioElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);

  const rawTranscripts = useAgentStore((s) => s.transcripts[call.callId]);
  const transcripts = rawTranscripts ?? EMPTY_TRANSCRIPTS;
  const rawAgentState = useAgentStore((s) => s.callStates[call.callId]);
  const agentState = rawAgentState ?? DEFAULT_AGENT_STATE;
  const toggleCallAgent = useAgentStore((s) => s.toggleCallAgent);

  useEffect(() => {
    const t = setInterval(() => force((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [transcripts.length]);

  useEffect(() => {
    if (!conn) {
      setRemoteStream(null);
      return;
    }
    if (conn.remoteStream) {
      setRemoteStream(conn.remoteStream);
      if (audioRef.current && audioRef.current.srcObject !== conn.remoteStream) {
        audioRef.current.srcObject = conn.remoteStream;
        audioRef.current.play().catch(() => {});
      }
      return;
    }
    const checkInterval = setInterval(() => {
      if (conn.remoteStream) {
        setRemoteStream(conn.remoteStream);
        if (audioRef.current && audioRef.current.srcObject !== conn.remoteStream) {
          audioRef.current.srcObject = conn.remoteStream;
          audioRef.current.play().catch(() => {});
        }
        clearInterval(checkInterval);
      }
    }, 250);
    return () => clearInterval(checkInterval);
  }, [conn]);

  useEffect(() => {
    const el = audioRef.current as (HTMLAudioElement & { setSinkId?: (id: string) => Promise<void> }) | null;
    if (!el || !outDeviceId || typeof el.setSinkId !== "function") return;
    el.setSinkId(outDeviceId).catch(() => {});
  }, [outDeviceId, conn]);


  const getAgentBadge = () => {
    if (!agentState.enabled) {
      return (
        <Badge variant="secondary" className="text-[10px] bg-muted text-muted-foreground">
          AI Off
        </Badge>
      );
    }
    switch (agentState.state) {
      case "listening":
        return (
          <Badge variant="secondary" className="text-[10px] bg-amber-500/10 text-amber-500 border-amber-500/30 animate-pulse">
            <Mic className="h-3 w-3 mr-1" /> Caller Speaking...
          </Badge>
        );
      case "thinking":
        return (
          <Badge variant="secondary" className="text-[10px] bg-blue-500/10 text-blue-500 border-blue-500/30 animate-pulse">
            <Sparkles className="h-3 w-3 mr-1" /> AI Thinking...
          </Badge>
        );
      case "speaking":
        return (
          <Badge variant="secondary" className="text-[10px] bg-emerald-500/10 text-emerald-500 border-emerald-500/30 animate-pulse">
            <Volume2 className="h-3 w-3 mr-1" /> AI Speaking...
          </Badge>
        );
      default:
        return (
          <Badge variant="secondary" className="text-[10px] bg-emerald-500/10 text-emerald-500 border-emerald-500/30">
            <Bot className="h-3 w-3 mr-1" /> AI Active
          </Badge>
        );
    }
  };

  return (
    <Card className="border-border/60 shadow-sm overflow-hidden">
      <CardContent className="space-y-4 p-4">
        {/* Header */}
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate font-semibold text-base">{call.peer}</p>
            <div className="flex items-center gap-2 mt-1">
              <Badge variant={statusVariant[call.status]}>
                {formatCallDuration(call.startedAt, call.status)}
              </Badge>
              {getAgentBadge()}
            </div>
          </div>
          <div className="flex items-center gap-1.5">
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant={agentState.enabled ? "default" : "outline"}
                  size="icon"
                  className={agentState.enabled ? "bg-emerald-600 hover:bg-emerald-700 text-white" : ""}
                  onClick={() => toggleCallAgent(call.sessionId, call.callId, !agentState.enabled)}
                  aria-label="Toggle AI Agent"
                >
                  <Bot className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{agentState.enabled ? "Disable AI Auto-Pilot" : "Enable AI Auto-Pilot"}</TooltipContent>
            </Tooltip>

            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="destructive"
                  size="icon"
                  onClick={() => endCall.mutate({ sid: call.sessionId, callId: call.callId })}
                  aria-label="End call"
                >
                  <PhoneOff className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>End call</TooltipContent>
            </Tooltip>
          </div>
        </div>

        {/* Audio Meters */}
        <div className="grid grid-cols-2 gap-3 p-2.5 rounded-lg bg-muted/40">
          <AudioMeter label="Operator Mic" stream={conn?.micStream} />
          <AudioMeter label="Caller Voice" stream={remoteStream} />
        </div>

        {/* Live AI Conversation Transcript */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5 font-medium">
              <MessageSquare className="h-3.5 w-3.5 text-primary" />
              Live AI Transcript
            </span>
            <span className="text-[10px]">{transcripts.length} turns</span>
          </div>

          <div
            ref={scrollRef}
            className="h-36 overflow-y-auto rounded-lg border border-border/60 bg-muted/20 p-2.5 space-y-2 text-xs"
          >
            {transcripts.length === 0 ? (
              <div className="h-full flex items-center justify-center text-muted-foreground text-[11px] italic">
                Waiting for caller speech...
              </div>
            ) : (
              transcripts.map((t, idx) => (
                <div
                  key={idx}
                  className={`flex flex-col ${
                    t.role === "assistant" ? "items-start" : "items-end"
                  }`}
                >
                  <div className="text-[9px] text-muted-foreground mb-0.5 px-1 font-medium">
                    {t.role === "assistant" ? "AI Voice Agent" : "Caller"}
                  </div>
                  <div
                    className={`p-2 rounded-lg max-w-[88%] ${
                      t.role === "assistant"
                        ? "bg-emerald-500/10 text-emerald-950 dark:text-emerald-200 border border-emerald-500/20"
                        : "bg-primary/10 text-foreground border border-primary/20"
                    }`}
                  >
                    {t.text}
                  </div>
                </div>
              ))
            )}
          </div>
        </div>

        <audio ref={audioRef} autoPlay />
      </CardContent>
    </Card>
  );
};

