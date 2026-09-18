import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  public state: State = {
    hasError: false,
    error: null,
  };

  public static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  public componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("Uncaught runtime error:", error, errorInfo);
  }

  public render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center p-6 text-center bg-background text-foreground">
          <div className="p-3 rounded-full bg-destructive/10 text-destructive mb-4">
            <AlertTriangle className="h-10 w-10" />
          </div>
          <h2 className="text-xl font-bold mb-2">Something went wrong</h2>
          <p className="text-sm text-muted-foreground max-w-md mb-6">
            {this.state.error?.message || "An unexpected error occurred while rendering the page."}
          </p>
          <Button
            onClick={() => {
              window.location.reload();
            }}
            className="gap-2"
          >
            <RefreshCw className="h-4 w-4" />
            Reload Application
          </Button>
        </div>
      );
    }

    return this.props.children;
  }
}
