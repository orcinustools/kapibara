import { ReactNode } from "react";
import { AlertCircle } from "lucide-react";
import { Card as CardBase, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

/** Card with an optional title + right-aligned actions row (legacy-friendly API). */
export function Card({ title, children, actions }: { title?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <CardBase className="mb-4">
      {(title || actions) && (
        <CardHeader>
          {title ? <CardTitle>{title}</CardTitle> : <span />}
          {actions}
        </CardHeader>
      )}
      <CardContent className={title ? undefined : "pt-5"}>{children}</CardContent>
    </CardBase>
  );
}

export function StatusPill({ status }: { status: string | boolean }) {
  const ok = ["success", "Running", "Ready"];
  const err = ["failed", "NotReady"];
  const run = ["running", "pending"];
  const s = String(status);
  const variant =
    status === true || (typeof status === "string" && ok.includes(status))
      ? "success"
      : typeof status === "string" && err.includes(status)
      ? "destructive"
      : typeof status === "string" && run.includes(status)
      ? "warning"
      : "outline";
  return (
    <Badge variant={variant}>
      <span className="size-1.5 rounded-full bg-current" />
      {s}
    </Badge>
  );
}

export function ErrorBox({ error }: { error?: string | null }) {
  if (!error) return null;
  return (
    <div className="mb-3 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
      <AlertCircle className="mt-0.5 size-4 shrink-0" />
      <span>{error}</span>
    </div>
  );
}

export function Empty({ text }: { text: string }) {
  return <div className="py-6 text-center text-sm text-muted-foreground">{text}</div>;
}
