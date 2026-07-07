// Real browser e2e for the kapibara SPA (Playwright + Chromium).
// Exercises the Railway-style canvas project view: login → create project →
// canvas empty state → New ▸ Compose → deploy → a service node appears →
// click node → detail panel.
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

  // Open project → canvas empty state.
  await page.getByRole("link", { name: pname }).click();
  await page.getByText("No services yet").waitFor({ timeout: 8000 });
  log("canvas empty state ✓");

  // New ▸ Compose → deploy the default compose stack.
  await page.getByRole("button", { name: "New" }).click();
  await page.getByRole("menuitem", { name: "Compose" }).click();
  await page.getByRole("button", { name: "Deploy", exact: true }).click();
  log("compose deploy triggered, waiting…");
  await page.getByText(/Applied \d+ objects/).waitFor({ timeout: 60000 });
  const applied = await page.getByText(/Applied \d+ objects/).textContent();
  log("deploy result:", applied.trim(), "✓");

  // Close the manage dialog; the compose unit should now be a canvas node.
  await page.keyboard.press("Escape");
  await page.getByText("docker-compose").first().waitFor({ timeout: 30000 });
  log("service node visible on canvas ✓");

  // Click the node → detail panel opens with per-unit actions.
  await page.getByText("docker-compose").first().click();
  await page.getByRole("button", { name: "Open full management" }).waitFor({ timeout: 8000 });
  log("node detail panel opened ✓");

  log("ALL UI E2E CHECKS PASSED ✅");
} catch (e) {
  failed = true;
  console.error("[e2e] FAILED:", e.message);
  await page.screenshot({ path: "/tmp/kapibara-e2e-fail.png" }).catch(() => {});
} finally {
  await browser.close();
  process.exit(failed ? 1 : 0);
}
