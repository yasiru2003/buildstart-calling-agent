import { useState } from "react";
import { 
  PhoneIncoming, 
  PhoneOutgoing, 
  Zap, 
  Clock, 
  MessageSquare, 
  ChevronRight, 
  FileText, 
  RotateCcw,
  Sparkles
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { useHistory } from "@/hooks/useHistory";
import { CallDetailDialog } from "./CallDetailDialog";
import type { HistoryRow } from "@/types/history";

export const RecentCallsSection = ({ sid }: { sid: string }) => {
  const { data: rows = [], refetch, isFetching } = useHistory(sid, true);
  const [selectedCall, setSelectedCall] = useState<HistoryRow | null>(null);
  const [filter, setFilter] = useState<"all" | "inbound" | "callback" | "outbound">("all");

  const filteredRows = rows.filter((r) => {
    if (filter === "inbound") return r.direction === "inbound";
    if (filter === "callback") return r.direction === "auto-callback" || r.triggerReason?.includes("callback");
    if (filter === "outbound") return r.direction === "outbound" && !r.triggerReason?.includes("callback");
    return true;
  });

  const recentCalls = filteredRows.slice(0, 10);

  return (
    <Card className="border shadow-sm">
      <CardHeader className="pb-3 border-b bg-muted/10 flex flex-row items-center justify-between">
        <div>
          <CardTitle className="text-base font-semibold flex items-center gap-2">
            <Sparkles className="h-4 w-4 text-primary" />
            Recent Call Activity & Logs
          </CardTitle>
          <p className="text-xs text-muted-foreground mt-0.5">
            Audit trail of each WhatsApp call, AI speech transcripts, and auto-callbacks
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex bg-muted p-0.5 rounded-lg text-xs">
            <button
              onClick={() => setFilter("all")}
              className={`px-2.5 py-1 rounded-md transition-colors ${
                filter === "all" ? "bg-background text-foreground shadow-xs font-medium" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              All
            </button>
            <button
              onClick={() => setFilter("callback")}
              className={`px-2.5 py-1 rounded-md transition-colors flex items-center gap-1 ${
                filter === "callback" ? "bg-background text-amber-600 dark:text-amber-400 shadow-xs font-medium" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              <Zap className="h-3 w-3" /> Auto-Callback
            </button>
            <button
              onClick={() => setFilter("inbound")}
              className={`px-2.5 py-1 rounded-md transition-colors ${
                filter === "inbound" ? "bg-background text-blue-600 dark:text-blue-400 shadow-xs font-medium" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              Inbound
            </button>
            <button
              onClick={() => setFilter("outbound")}
              className={`px-2.5 py-1 rounded-md transition-colors ${
                filter === "outbound" ? "bg-background text-emerald-600 dark:text-emerald-400 shadow-xs font-medium" : "text-muted-foreground hover:text-foreground"
              }`}
            >
              Outbound
            </button>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 text-muted-foreground"
            onClick={() => refetch()}
            disabled={isFetching}
            title="Refresh logs"
          >
            <RotateCcw className={`h-3.5 w-3.5 ${isFetching ? "animate-spin" : ""}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {recentCalls.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <Clock className="h-8 w-8 mx-auto mb-2 opacity-30" />
            <p className="text-sm font-medium">No calls recorded yet</p>
            <p className="text-xs mt-1 text-muted-foreground max-w-xs mx-auto">
              Inbound and outbound calls, along with autonomous auto-callbacks, will appear here with full logs.
            </p>
          </div>
        ) : (
          <div className="divide-y">
            {recentCalls.map((call) => {
              const isCallback = call.direction === "auto-callback" || call.triggerReason?.includes("callback");
              const isInbound = call.direction === "inbound";
              const durSec = call.durationSeconds ?? (call.endedAt && call.startedAt ? Math.round((call.endedAt - call.startedAt) / 1000) : 0);
              const displayName = call.peerNumber || call.peer;
              const hasTranscripts = call.transcripts && call.transcripts.length > 0;
              const lastTranscript = hasTranscripts ? call.transcripts![call.transcripts!.length - 1] : null;

              return (
                <div
                  key={call.callId}
                  onClick={() => setSelectedCall(call)}
                  className="p-4 hover:bg-muted/40 transition-colors cursor-pointer flex items-center justify-between gap-4 group"
                >
                  <div className="flex items-start gap-3 min-w-0">
                    <div className="mt-0.5">
                      {isCallback ? (
                        <div className="h-8 w-8 rounded-full bg-amber-500/10 text-amber-600 dark:text-amber-400 flex items-center justify-center">
                          <Zap className="h-4 w-4" />
                        </div>
                      ) : isInbound ? (
                        <div className="h-8 w-8 rounded-full bg-blue-500/10 text-blue-600 dark:text-blue-400 flex items-center justify-center">
                          <PhoneIncoming className="h-4 w-4" />
                        </div>
                      ) : (
                        <div className="h-8 w-8 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
                          <PhoneOutgoing className="h-4 w-4" />
                        </div>
                      )}
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-sm truncate">{displayName}</span>
                        {isCallback ? (
                          <Badge variant="outline" className="text-[10px] px-1.5 py-0 border-amber-500/30 text-amber-600 dark:text-amber-400 bg-amber-500/5">
                            Auto-Callback
                          </Badge>
                        ) : isInbound ? (
                          <Badge variant="outline" className="text-[10px] px-1.5 py-0 border-blue-500/30 text-blue-600 dark:text-blue-400 bg-blue-500/5">
                            Inbound
                          </Badge>
                        ) : (
                          <Badge variant="outline" className="text-[10px] px-1.5 py-0 border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-500/5">
                            Outbound
                          </Badge>
                        )}
                        {call.outcome && (
                          <span className="text-[11px] font-medium text-muted-foreground hidden sm:inline">
                            • {call.outcome}
                          </span>
                        )}
                      </div>

                      {/* Snippet / Transcript Preview */}
                      {lastTranscript ? (
                        <p className="text-xs text-muted-foreground mt-1 truncate max-w-md flex items-center gap-1.5">
                          <MessageSquare className="h-3 w-3 shrink-0 text-primary" />
                          <span className="font-medium text-foreground/80">{lastTranscript.role === "agent" ? "AI:" : "Caller:"}</span>
                          <span>"{lastTranscript.text}"</span>
                        </p>
                      ) : (
                        <p className="text-xs text-muted-foreground mt-0.5">
                          {call.endReason ? `Status: ${call.endReason}` : "Call logged"}
                        </p>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-3 shrink-0 text-right">
                    <div className="text-xs">
                      <div className="font-medium text-foreground flex items-center justify-end gap-1">
                        <Clock className="h-3 w-3 text-muted-foreground" />
                        {durSec > 0 ? `${durSec}s` : "Brief"}
                      </div>
                      <div className="text-[11px] text-muted-foreground mt-0.5">
                        {new Date(call.startedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                      </div>
                    </div>
                    <Button variant="ghost" size="sm" className="h-8 gap-1 text-xs opacity-80 group-hover:opacity-100">
                      <FileText className="h-3.5 w-3.5 text-primary" />
                      <span className="hidden sm:inline">Log</span>
                      <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>

      <CallDetailDialog
        call={selectedCall}
        open={!!selectedCall}
        onOpenChange={(open) => {
          if (!open) setSelectedCall(null);
        }}
      />
    </Card>
  );
};
