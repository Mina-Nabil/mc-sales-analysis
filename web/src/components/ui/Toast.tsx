import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { cn } from "@/lib/cn";
import type { StatusVariant } from "@/lib/status";

interface ToastItem {
  id: number;
  message: string;
  variant: StatusVariant;
}

interface ToastContextValue {
  show: (message: string, variant?: StatusVariant) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const variantClasses: Record<StatusVariant, string> = {
  success: "border-l-ok",
  warning: "border-l-warn",
  danger: "border-l-bad",
  info: "border-l-info",
  accent: "border-l-acc",
  neutral: "border-l-t2",
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const show = useCallback((message: string, variant: StatusVariant = "info") => {
    const id = Date.now();
    setToasts((t) => [...t, { id, message, variant }]);
    setTimeout(() => setToasts((t) => t.filter((toast) => toast.id !== id)), 3500);
  }, []);

  return (
    <ToastContext.Provider value={{ show }}>
      {children}
      {createPortal(
        <div className="fixed bottom-4 right-4 z-[200] flex w-[calc(100%-2rem)] max-w-sm flex-col gap-2">
          {toasts.map((toast) => (
            <div
              key={toast.id}
              className={cn(
                "rounded-[var(--radius-vela-md)] border-l-4 bg-bg-2 border border-line px-4 py-3 text-[13px] font-medium text-t0 shadow-[var(--shadow-vela)] animate-vela-pop",
                variantClasses[toast.variant],
              )}
            >
              {toast.message}
            </div>
          ))}
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within a ToastProvider");
  return ctx;
}
