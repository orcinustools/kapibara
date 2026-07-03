// Real browser e2e for the kapibara SPA (Playwright + Chromium).
import { chromium } from "playwright";

const BASE = process.env.KAPIBARA_URL || "http://localhost:9000";
const EMAIL = "ibnu@biznetgio.com";
const PASS = "supersecret";

function log(...a) { console.log("[e2e]", ...a); }

const browser = await chromium.launch();
const page = await browser.newPage();
page.on("pageerror", (e) => console.error("PAGE ERROR:", e.message));
let failed = false;

try {
  await page.goto(BASE, { waitUntil: "networkidle" });

  // Login (user already registered via API in the harness).
  log("login…");
  await page.getByPlaceholder("you@example.com").fill(EMAIL);
  await page.getByPlaceholder("min 8 chars").fill(PASS);
  await page.getByRole("button", { name: "Login", exact: true }).click();

  // Projects page.
  await page.getByText("Your projects").waitFor({ timeout: 8000 });
  log("logged in, on Projects page ✓");

  // Create a project.
  const pname = "ui-e2e";
  log("creating project", pname);
  await page.getByPlaceholder("new project name").fill(pname);
  await page.getByRole("button", { name: "Create project" }).click();
  await page.getByRole("link", { name: pname }).waitFor({ timeout: 8000 });
  log("project created ✓");

  // Open project → Compose tab → deploy.
  await page.getByRole("link", { name: pname }).click();
  await page.getByRole("link", { name: "Compose" }).click();
  await page.getByRole("button", { name: "Deploy", exact: true }).click();
  log("compose deploy triggered, waiting…");
  await page.getByText(/Applied \d+ objects/).waitFor({ timeout: 60000 });
  const applied = await page.getByText(/Applied \d+ objects/).textContent();
  log("deploy result:", applied.trim(), "✓");

  // Overview tab → expect a pod row.
  await page.getByRole("link", { name: "Overview" }).click();
  await page.getByText("web-", { exact: false }).first().waitFor({ timeout: 30000 });
  log("pod visible in Overview ✓");

  log("ALL UI E2E CHECKS PASSED ✅");
} catch (e) {
  failed = true;
  console.error("[e2e] FAILED:", e.message);
  await page.screenshot({ path: "/tmp/kapibara-e2e-fail.png" }).catch(() => {});
} finally {
  await browser.close();
  process.exit(failed ? 1 : 0);
}
