import { useState } from "react";
import { Loader2 } from "lucide-react";
import { useAuth } from "../auth";
import { ApiError } from "../api";
import { ErrorBox } from "../ui";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/components/ui/sonner";
import { Card, CardContent } from "@/components/ui/card";

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
      if (mode === "register") {
        await register(email, password, name);
        toast.success("Account created — welcome to Kapibara");
      } else {
        await login(email, password, totp || undefined);
        toast.success("Signed in");
      }
    } catch (e) {
      if (e instanceof ApiError && e.body?.totpRequired) {
        setNeedTotp(true);
        setErr("Two-factor code required");
        toast.message("Two-factor code required");
      } else {
        setErr((e as Error).message);
        toast.error((e as Error).message);
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid min-h-screen place-items-center bg-background p-4">
      <Card className="w-[380px] max-w-[92vw]">
        <CardContent className="pt-6">
          <div className="mb-1 flex items-center gap-2 text-2xl font-bold">
            <span>🦫</span> Kapibara
          </div>
          <p className="mb-4 text-sm text-muted-foreground">Self-hosted PaaS on the Orcinus engine</p>
          <form onSubmit={submit}>
            <ErrorBox error={err} />
            {mode === "register" && (
              <>
                <Label>Name</Label>
                <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Your name" />
              </>
            )}
            <Label>Email</Label>
            <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" />
            <Label>Password</Label>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="min 8 chars"
            />
            {needTotp && (
              <>
                <Label>Two-factor code</Label>
                <Input value={totp} onChange={(e) => setTotp(e.target.value)} placeholder="123456" />
              </>
            )}
            <div className="mt-5 flex items-center gap-2">
              <Button disabled={busy} type="submit">
                {busy && <Loader2 className="size-4 animate-spin" aria-hidden />}
                {mode === "login" ? "Login" : "Register"}
              </Button>
              <Button
                type="button"
                variant="ghost"
                disabled={busy}
                onClick={() => {
                  setMode(mode === "login" ? "register" : "login");
                  setErr(null);
                }}
              >
                {mode === "login" ? "Need an account?" : "Have an account?"}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
