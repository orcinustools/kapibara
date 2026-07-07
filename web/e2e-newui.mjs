// Supplementary browser e2e for the features added in fix/improvement:
// RBAC members (M1), Git providers (M3), and the metrics history sparkline (M6).
// Runs against a live kapibara. Reuses the ui-e2e project created by e2e.mjs.
import { chromium } from "playwright";

const BASE = process.env.KAPIBARA_URL || "http://localhost:9000";
const EMAIL = "ibnu@biznetgio.com";
const PASS = "supersecret";
const TEAMMATE = "teammate@biznetgio.com";

const log = (...a) => console.log("[newui]", ...a);
let pass = 0, fail = 0;
const ok = (m) => { console.log("  PASS:", m); pass++; };
const bad = (m) => { console.error("  FAIL:", m); fail++; };

const browser = await chromium.launch();
const page = await browser.newPage();
page.on("pageerror", (e) => console.error("PAGE ERROR:", e.message));

async function section(name, fn) {
  log(name);
  try { await fn(); } catch (e) { bad(`${name}: ${e.message}`); }
}

try {
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.getByPlaceholder("you@example.com").fill(EMAIL);
  await page.getByPlaceholder("min 8 chars").fill(PASS);
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await page.getByText("Your projects").waitFor({ timeout: 8000 });
  log("logged in ✓");

  await page.goto(BASE + "/settings", { waitUntil: "networkidle" });

  await section("M1: organization members card", async () => {
    await page.getByText("Organization members").waitFor({ timeout: 8000 });
    ok("members card renders");
    // Self row shows owner.
    await page.getByRole("cell", { name: new RegExp(EMAIL) }).first().waitFor({ timeout: 8000 });
    ok("self listed as member");
    // Add teammate by email.
    await page.getByPlaceholder("user@example.com").fill(TEAMMATE);
    // Two "Add" buttons exist (Members + Notifications cards); the members
    // card is rendered first.
    await page.getByRole("button", { name: "Add", exact: true }).first().click();
    await page.getByRole("cell", { name: new RegExp(TEAMMATE) }).waitFor({ timeout: 8000 });
    ok("added teammate — row appears in table");
  });

  await section("M3: git providers card", async () => {
    await page.getByText("Git providers", { exact: true }).waitFor({ timeout: 8000 });
    ok("git providers card renders");
    await page.getByRole("button", { name: "Connect with token" }).waitFor({ timeout: 5000 });
    await page.getByRole("button", { name: "Connect with OAuth" }).waitFor({ timeout: 5000 });
    ok("connect (PAT + OAuth) buttons present");
    // Attempt connect with an invalid token → expect a rejection toast.
    await page.getByPlaceholder("ghp_… / glpat-…").fill("ghp_invalid000000000000000000000000000000");
    await page.getByRole("button", { name: "Connect with token" }).click();
    await page.getByText(/could not validate token/i).waitFor({ timeout: 8000 });
    ok("invalid token rejected with error toast");
  });

  await section("M6: metrics history sparkline", async () => {
    // Open the ui-e2e project and its full management → Pods & metrics overview.
    await page.goto(BASE, { waitUntil: "networkidle" });
    await page.getByRole("link", { name: "ui-e2e" }).click();
    await page.getByText("No services yet").waitFor({ timeout: 8000 }).catch(() => {});
    // Manage ▸ Pods & metrics opens the project-total resource usage card.
    await page.getByRole("button", { name: /Manage/ }).click();
    await page.getByRole("menuitem", { name: /Pods & metrics/ }).click();
    await page.getByText("Resource usage (project total)").waitFor({ timeout: 10000 });
    ok("metrics history card renders on overview");
    await page.getByText(/CPU \(millicores\)/).waitFor({ timeout: 5000 });
    await page.getByText(/Memory \(Mi\)/).waitFor({ timeout: 5000 });
    ok("CPU + Memory series labels present");
  });

  console.log(`\n==== NEW-UI E2E: ${pass} passed, ${fail} failed ====`);
} catch (e) {
  console.error("[newui] FATAL:", e.message);
  await page.screenshot({ path: "/tmp/kapibara-newui-fail.png" }).catch(() => {});
  fail++;
} finally {
  await browser.close();
  process.exit(fail ? 1 : 0);
}
