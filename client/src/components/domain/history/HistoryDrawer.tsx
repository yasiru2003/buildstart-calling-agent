import { useState } from "react";
import { History, PhoneIncoming, PhoneOutgoing, Zap, Clock, ChevronRight } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import { EmptyState } from "@/components/shared/EmptyState";
import { useHistory } from "@/hooks/useHistory";
import { CallDetailDialog } from "./CallDetailDialog";
import type { HistoryRow } from "@/types/history";

export const HistoryDrawer = ({ sid }: { sid: string }) => {
  const [open, setOpen] = useState(false);
  const [selectedCall, setSelectedCall] = useState<HistoryRow | null>(null);
  const { data: rows = [] } = useHistory(sid, open);

  return (
    <>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>
          <Button variant="outline" size="sm" className="gap-1.5">
            <History className="h-4 w-4 text-primary" />
            History & Logs
          </Button>
        </SheetTrigger>
        <SheetContent side="right" className="w-full p-0 sm:max-w-lg flex flex-col">
          <SheetHeader className="p-6 pb-4 border-b bg-muted/10">
            <SheetTitle className="text-base font-semibold flex items-center justify-between">
              <span>Call History & Audit Logs</span>
              <Badge variant="secondary" className="text-xs">
                {rows.length} call{rows.length === 1 ? "" : "s"}
              </Badge>
            </SheetTitle>
          </SheetHeader>
          <ScrollArea className="flex-1 px-6 py-4">
            {rows.length === 0 ? (
              <EmptyState title="No past calls" description="Calls you make or receive will appear here." />
            ) : (
              <ul className="space-y-2.5">
                {rows.map((r) => {
                  const isCallback = r.direction === "auto-callback" || r.triggerReason?.includes("callback");
                  const isInbound = r.direction === "inbound";
                  const durSec = r.durationSeconds ?? (r.endedAt && r.startedAt ? Math.round((r.endedAt - r.startedAt) / 1000) : 0);
                  const displayName = r.peerNumber || r.peer;

                  return (
                    <li
                      key={r.callId}
                      onClick={() => setSelectedCall(r)}
                      className="rounded-xl border p-3.5 hover:bg-muted/40 transition-colors cursor-pointer group flex items-center justify-between gap-3 shadow-2xs"
                    >
                      <div className="flex items-start gap-3 min-w-0">
                        <div className="mt-0.5">
                          {isCallback ? (
                            <div className="h-7 w-7 rounded-full bg-amber-500/10 text-amber-600 dark:text-amber-400 flex items-center justify-center">
                              <Zap className="h-3.5 w-3.5" />
                            </div>
                          ) : isInbound ? (
                            <div className="h-7 w-7 rounded-full bg-blue-500/10 text-blue-600 dark:text-blue-400 flex items-center justify-center">
                              <PhoneIncoming className="h-3.5 w-3.5" />
                            </div>
                          ) : (
                            <div className="h-7 w-7 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
                              <PhoneOutgoing className="h-3.5 w-3.5" />
                            </div>
                          )}
                        </div>
                        <div className="min-w-0">
                          <div className="flex items-center gap-1.5 flex-wrap">
                            <span className="font-semibold text-sm truncate">{displayName}</span>
                            {isCallback ? (
                              <Badge variant="outline" className="text-[10px] px-1 py-0 border-amber-500/30 text-amber-600 dark:text-amber-400">
                                Auto-Callback
                              </Badge>
                            ) : isInbound ? (
                              <Badge variant="outline" className="text-[10px] px-1 py-0 border-blue-500/30 text-blue-600 dark:text-blue-400">
                                Inbound
                              </Badge>
                            ) : (
                              <Badge variant="outline" className="text-[10px] px-1 py-0 border-emerald-500/30 text-emerald-600 dark:text-emerald-400">
                                Outbound
                              </Badge>
                            )}
                          </div>
                          <div className="text-xs text-muted-foreground flex items-center gap-2 mt-1">
                            <span>{new Date(r.startedAt).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}</span>
                            <span>•</span>
                            <span className="flex items-center gap-1">
                              <Clock className="h-3 w-3" />
                              {durSec > 0 ? `${durSec}s` : "Brief"}
                            </span>
                          </div>
                          {r.outcome && (
                            <p className="text-[11px] text-foreground/80 mt-1 font-medium">
                              {r.outcome}
                            </p>
                          )}
                        </div>
                      </div>
                      <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0 opacity-60 group-hover:opacity-100 group-hover:translate-x-0.5 transition-transform" />
                    </li>
                  );
                })}
              </ul>
            )}
          </ScrollArea>
        </SheetContent>
      </Sheet>

      <CallDetailDialog
        call={selectedCall}
        open={!!selectedCall}
        onOpenChange={(open) => {
          if (!open) setSelectedCall(null);
        }}
      />
    </>
  );
};

