import { useState } from "react";
import { 
  Dialog, 
  DialogContent, 
  DialogHeader, 
  DialogTitle, 
  DialogDescription 
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import { 
  PhoneIncoming, 
  PhoneOutgoing, 
  Zap, 
  Clock, 
  Bot, 
  User, 
  FileText, 
  Activity, 
  Info 
} from "lucide-react";
import type { HistoryRow } from "@/types/history";

interface CallDetailDialogProps {
  call: HistoryRow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const CallDetailDialog = ({ call, open, onOpenChange }: CallDetailDialogProps) => {
  const [activeTab, setActiveTab] = useState<"transcript" | "events" | "details">("transcript");

  if (!call) return null;

  const isAutoCallback = call.direction === "auto-callback" || call.triggerReason?.includes("callback");
  const isInbound = call.direction === "inbound";
  const durationText = call.durationSeconds 
    ? `${Math.floor(call.durationSeconds / 60)}m ${call.durationSeconds % 60}s`
    : call.endedAt && call.startedAt
      ? `${Math.max(0, Math.round((call.endedAt - call.startedAt) / 1000))}s`
      : "In progress / Brief";

  const displayName = call.peerNumber || call.peer;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] flex flex-col p-0 overflow-hidden">
        <DialogHeader className="p-6 pb-4 border-b bg-muted/20">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              {isAutoCallback ? (
                <div className="h-9 w-9 rounded-full bg-amber-500/10 text-amber-600 dark:text-amber-400 flex items-center justify-center font-bold">
                  <Zap className="h-5 w-5" />
                </div>
              ) : isInbound ? (
                <div className="h-9 w-9 rounded-full bg-blue-500/10 text-blue-600 dark:text-blue-400 flex items-center justify-center">
                  <PhoneIncoming className="h-5 w-5" />
                </div>
              ) : (
                <div className="h-9 w-9 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
                  <PhoneOutgoing className="h-5 w-5" />
                </div>
              )}
              <div>
                <DialogTitle className="text-lg font-semibold flex items-center gap-2">
                  <span>{displayName}</span>
                  {isAutoCallback ? (
                    <Badge variant="outline" className="border-amber-500/30 text-amber-600 dark:text-amber-400 bg-amber-500/5">
                      Auto-Callback
                    </Badge>
                  ) : isInbound ? (
                    <Badge variant="outline" className="border-blue-500/30 text-blue-600 dark:text-blue-400 bg-blue-500/5">
                      Inbound Call
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/5">
                      Outbound Call
                    </Badge>
                  )}
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground flex items-center gap-3 mt-1">
                  <span>{new Date(call.startedAt).toLocaleString()}</span>
                  <span>•</span>
                  <span className="flex items-center gap-1">
                    <Clock className="h-3 w-3" />
                    {durationText}
                  </span>
                  {call.outcome && (
                    <>
                      <span>•</span>
                      <span className="font-medium text-foreground">{call.outcome}</span>
                    </>
                  )}
                </DialogDescription>
              </div>
            </div>
          </div>

          {/* Navigation Tabs */}
          <div className="flex items-center gap-2 mt-4 pt-2">
            <Button
              variant={activeTab === "transcript" ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setActiveTab("transcript")}
              className="text-xs gap-1.5 h-8"
            >
              <FileText className="h-3.5 w-3.5" />
              Conversation Transcript
              {call.transcripts && call.transcripts.length > 0 && (
                <span className="ml-1 px-1.5 py-0.2 rounded-full bg-primary/10 text-[10px] font-semibold">
                  {call.transcripts.length}
                </span>
              )}
            </Button>
            <Button
              variant={activeTab === "events" ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setActiveTab("events")}
              className="text-xs gap-1.5 h-8"
            >
              <Activity className="h-3.5 w-3.5" />
              Event Timeline (What Happened)
              {call.events && call.events.length > 0 && (
                <span className="ml-1 px-1.5 py-0.2 rounded-full bg-primary/10 text-[10px] font-semibold">
                  {call.events.length}
                </span>
              )}
            </Button>
            <Button
              variant={activeTab === "details" ? "secondary" : "ghost"}
              size="sm"
              onClick={() => setActiveTab("details")}
              className="text-xs gap-1.5 h-8"
            >
              <Info className="h-3.5 w-3.5" />
              Technical Info
            </Button>
          </div>
        </DialogHeader>

        {/* Tab Content */}
        <ScrollArea className="flex-1 p-6 max-h-[55vh]">
          {activeTab === "transcript" && (
            <div className="space-y-4">
              {(!call.transcripts || call.transcripts.length === 0) ? (
                <div className="text-center py-10 text-muted-foreground">
                  <Bot className="h-10 w-10 mx-auto mb-2 opacity-40" />
                  <p className="font-medium text-sm">No spoken conversation recorded</p>
                  <p className="text-xs mt-1 max-w-sm mx-auto">
                    {call.outcome?.includes("Missed") 
                      ? "The call ended before a conversation started (e.g. caller hung up after ringing)."
                      : "Voice transcripts are recorded automatically when the caller and AI exchange speech."}
                  </p>
                </div>
              ) : (
                <div className="space-y-3">
                  {call.transcripts.map((t, idx) => {
                    const isAgent = t.role === "agent";
                    return (
                      <div
                        key={idx}
                        className={`flex gap-3 text-sm ${isAgent ? "flex-row-reverse" : "flex-row"}`}
                      >
                        <div
                          className={`h-7 w-7 rounded-full flex items-center justify-center shrink-0 text-xs font-semibold ${
                            isAgent
                              ? "bg-primary text-primary-foreground"
                              : "bg-muted text-muted-foreground border"
                          }`}
                        >
                          {isAgent ? <Bot className="h-4 w-4" /> : <User className="h-4 w-4" />}
                        </div>
                        <div
                          className={`rounded-2xl px-4 py-2.5 max-w-[80%] shadow-sm ${
                            isAgent
                              ? "bg-primary text-primary-foreground rounded-tr-xs"
                              : "bg-muted/80 text-foreground border rounded-tl-xs"
                          }`}
                        >
                          <div className="flex items-center justify-between gap-4 mb-1 text-[11px] opacity-75">
                            <span className="font-medium">{isAgent ? "AI Voice Assistant" : "Caller"}</span>
                            <span>{new Date(t.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
                          </div>
                          <p className="leading-relaxed whitespace-pre-wrap">{t.text}</p>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {activeTab === "events" && (
            <div className="space-y-4">
              {(!call.events || call.events.length === 0) ? (
                <div className="text-center py-10 text-muted-foreground">
                  <Activity className="h-10 w-10 mx-auto mb-2 opacity-40" />
                  <p className="font-medium text-sm">No timeline events recorded</p>
                </div>
              ) : (
                <div className="relative pl-6 space-y-6 before:absolute before:left-2.5 before:top-2 before:bottom-2 before:w-0.5 before:bg-border">
                  {call.events.map((ev, idx) => {
                    return (
                      <div key={idx} className="relative group">
                        <div className="absolute -left-6 top-1 h-3 w-3 rounded-full border-2 border-background bg-primary" />
                        <div className="space-y-0.5">
                          <div className="flex items-center justify-between text-xs text-muted-foreground">
                            <span className="font-medium text-foreground text-sm">{ev.message}</span>
                            <span>{new Date(ev.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
                          </div>
                          {ev.details && (
                            <p className="text-xs text-muted-foreground bg-muted/40 p-2 rounded border mt-1 font-mono break-all">
                              {ev.details}
                            </p>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {activeTab === "details" && (
            <div className="space-y-4 text-xs">
              <div className="grid grid-cols-2 gap-3">
                <div className="p-3 rounded-lg border bg-muted/20">
                  <span className="text-muted-foreground block mb-1">Call ID</span>
                  <span className="font-mono font-medium text-foreground break-all">{call.callId}</span>
                </div>
                <div className="p-3 rounded-lg border bg-muted/20">
                  <span className="text-muted-foreground block mb-1">Peer WhatsApp JID</span>
                  <span className="font-mono font-medium text-foreground break-all">{call.peer}</span>
                </div>
                <div className="p-3 rounded-lg border bg-muted/20">
                  <span className="text-muted-foreground block mb-1">Direction / Trigger</span>
                  <span className="font-medium text-foreground">{call.direction} {call.triggerReason ? `(${call.triggerReason})` : ""}</span>
                </div>
                <div className="p-3 rounded-lg border bg-muted/20">
                  <span className="text-muted-foreground block mb-1">End Reason</span>
                  <span className="font-medium text-foreground">{call.endReason || "Normal termination"}</span>
                </div>
                <div className="p-3 rounded-lg border bg-muted/20">
                  <span className="text-muted-foreground block mb-1">Audio Codec</span>
                  <span className="font-medium text-foreground">Opus 16 kHz (WhatsApp Standard)</span>
                </div>
                <div className="p-3 rounded-lg border bg-muted/20">
                  <span className="text-muted-foreground block mb-1">AI Voice Model</span>
                  <span className="font-medium text-foreground">Piper TTS (Sinhala Thilini) / Gemini</span>
                </div>
              </div>
            </div>
          )}
        </ScrollArea>
      </DialogContent>
    </Dialog>
  );
};
