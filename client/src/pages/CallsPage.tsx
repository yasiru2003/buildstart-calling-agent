import { PhoneCall } from "lucide-react";
import { Dialer } from "@/components/domain/call/Dialer";
import { CallCard } from "@/components/domain/call/CallCard";
import { HistoryDrawer } from "@/components/domain/history/HistoryDrawer";
import { RecentCallsSection } from "@/components/domain/history/RecentCallsSection";
import { EmptyState } from "@/components/shared/EmptyState";
import { useCalls } from "@/stores/calls";

export const CallsPage = ({ sid }: { sid: string }) => {
  const calls = useCalls((s) => s.calls);

  const sessionCalls = calls.filter((c) => c.sessionId === sid && c.status !== "ended");

  return (
    <div className="mx-auto max-w-3xl space-y-6 pb-12">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-medium text-muted-foreground">
          {sessionCalls.length} active call{sessionCalls.length === 1 ? "" : "s"}
        </h2>
        <HistoryDrawer sid={sid} />
      </div>
      <Dialer sid={sid} />
      {sessionCalls.length > 0 ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {sessionCalls.map((c) => (
            <CallCard key={c.callId} call={c} />
          ))}
        </div>
      ) : (
        <EmptyState
          icon={<PhoneCall className="h-6 w-6" />}
          title="No active calls"
          description="Dial a number above to start a call, or wait for an incoming call."
        />
      )}

      {/* Persistent Call Audit Log & History on the Dashboard */}
      <RecentCallsSection sid={sid} />
    </div>
  );
};
