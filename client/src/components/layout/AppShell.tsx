import { useState, type ReactNode } from "react";
import { Menu, PhoneCall, Bot, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Sidebar } from "./Sidebar";
import { ThemeToggle } from "./ThemeToggle";
import { useAgentStore } from "@/stores/agent";
import { AIAgentSettingsModal } from "@/components/domain/agent/AIAgentSettingsModal";

export const AppShell = ({ children }: { children: ReactNode }) => {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [agentModalOpen, setAgentModalOpen] = useState(false);
  const config = useAgentStore((s) => s.config);

  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-30 flex items-center justify-between border-b bg-background/80 px-4 py-3 backdrop-blur sm:px-6">
        <div className="flex items-center gap-2">
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button variant="outline" size="icon" className="md:hidden" aria-label="Accounts">
                <Menu className="h-4 w-4" />
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="w-72 p-0">
              <SheetTitle className="px-3 pt-3">Accounts</SheetTitle>
              <Sidebar onNavigate={() => setMobileOpen(false)} />
            </SheetContent>
          </Sheet>
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-600 text-white shadow-sm">
            <PhoneCall className="h-4 w-4" />
          </span>
          <span className="text-lg font-semibold tracking-tight">WaCalls</span>
          <span className="hidden sm:inline-flex ml-2 items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-500/10 text-emerald-600 border border-emerald-500/20">
            <Sparkles className="h-3 w-3" />
            OpenRouter AI
          </span>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setAgentModalOpen(true)}
            className={`gap-1.5 font-medium border-emerald-500/30 ${
              config.enabled ? "bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20" : ""
            }`}
          >
            <Bot className="h-4 w-4 text-emerald-500" />
            <span>AI Voice Agent</span>
            {config.autoAnswer && (
              <Badge variant="secondary" className="text-[10px] px-1 py-0 h-4 bg-emerald-500 text-white">
                Auto
              </Badge>
            )}
          </Button>
          <ThemeToggle />
        </div>
      </header>
      <div className="flex flex-1">
        <aside className="hidden w-64 shrink-0 border-r md:block">
          <Sidebar />
        </aside>
        <main className="flex-1 px-4 py-6 sm:px-6">{children}</main>
      </div>
      <AIAgentSettingsModal open={agentModalOpen} onOpenChange={setAgentModalOpen} />
    </div>
  );
};
