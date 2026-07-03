import { ReactNode } from "react";

export function Card({ title, children, actions }: { title?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="card">
      {(title || actions) && (
        <div className="row" style={{ justifyContent: "space-between" }}>
          {title && <h2 style={{ margin: 0 }}>{title}</h2>}
          {actions}
        </div>
      )}
      <div style={{ marginTop: title ? ".8rem" : 0 }}>{children}</div>
    </div>
  );
}

export function StatusPill({ status }: { status: string | boolean }) {
  const ok = ["success", "Running", "Ready"];
  const err = ["failed", "NotReady"];
  const run = ["running", "pending"];
  const cls =
    status === true || (typeof status === "string" && ok.includes(status))
      ? "ok"
      : typeof status === "string" && err.includes(status)
      ? "err"
      : typeof status === "string" && run.includes(status)
      ? "run"
      : "gray";
  return <span className={"pill " + cls}>{String(status)}</span>;
}

export function ErrorBox({ error }: { error?: string | null }) {
  if (!error) return null;
  return <div className="err-box">{error}</div>;
}

export function Empty({ text }: { text: string }) {
  return <div className="muted" style={{ padding: ".6rem 0" }}>{text}</div>;
}
