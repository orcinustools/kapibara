import { useState } from "react";
import { useAuth } from "../auth";
import { ApiError } from "../api";
import { ErrorBox } from "../ui";

export function AuthPage() {
  const { login, register } = useAuth();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [totp, setTotp] = useState("");
  const [needTotp, setNeedTotp] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      if (mode === "register") await register(email, password, name);
      else await login(email, password, totp || undefined);
    } catch (e) {
      if (e instanceof ApiError && e.body?.totpRequired) {
        setNeedTotp(true);
        setErr("Two-factor code required");
      } else {
        setErr((e as Error).message);
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="center">
      <form className="card authcard" onSubmit={submit}>
        <div className="brand" style={{ fontSize: "1.4rem", marginBottom: ".5rem" }}>🦫 Kapibara</div>
        <p className="muted" style={{ marginTop: 0 }}>Self-hosted PaaS on the Orcinus engine</p>
        <ErrorBox error={err} />
        {mode === "register" && (
          <>
            <label>Name</label>
            <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Your name" />
          </>
        )}
        <label>Email</label>
        <input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" />
        <label>Password</label>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="min 8 chars" />
        {needTotp && (
          <>
            <label>Two-factor code</label>
            <input value={totp} onChange={(e) => setTotp(e.target.value)} placeholder="123456" />
          </>
        )}
        <div className="row" style={{ marginTop: "1rem" }}>
          <button disabled={busy} type="submit">
            {mode === "login" ? "Login" : "Register"}
          </button>
          <button
            type="button"
            className="sec"
            onClick={() => { setMode(mode === "login" ? "register" : "login"); setErr(null); }}
          >
            {mode === "login" ? "Need an account?" : "Have an account?"}
          </button>
        </div>
      </form>
    </div>
  );
}
