// Browser e2e: deploy from Git entirely through the dashboard (New ▸ From Git repo).
// Runs against the server-side kapibara (:9010) which can build git sources.
import { chromium } from "playwright";

const BASE = process.env.KAPIBARA_URL || "http://localhost:9010";
const EMAIL = "ibnu@biznetgio.com";
const PASS = "supersecret";
const REPO = "https://github.com/anak10thn/kapibara-git-e2e.git";
// Unique per run so the suite is repeatable against a persistent server.
const RUN = process.env.E2E_RUN || String(Date.now()).slice(-6);
const PROJECT = `uigit-${RUN}`;
const APP = `webgit${RUN}`;

const log = (...a) => console.log("[gitui]", ...a);
const browser = await chromium.launch();
const page = await browser.newPage();
page.on("pageerror", (e) => console.error("PAGE ERROR:", e.message));
let failed = false;

try {
  await page.goto(BASE, { waitUntil: "networkidle" });
  await page.getByPlaceholder("you@example.com").fill(EMAIL);
  await page.getByPlaceholder("min 8 chars").fill(PASS);
  await page.getByRole("button", { name: "Login", exact: true }).click();
  await page.getByText("Your projects").waitFor({ timeout: 8000 });
  log("logged in ✓");

  // Create (or reuse) a project.
  if (await page.getByRole("link", { name: PROJECT }).count() === 0) {
    await page.getByPlaceholder("new project name").fill(PROJECT);
    await page.getByRole("button", { name: "Create project" }).click();
  }
  await page.getByRole("link", { name: PROJECT }).click();
  await page.getByText("No services yet").waitFor({ timeout: 8000 }).catch(() => {});
  log("project open ✓");

  // New ▸ From Git repo → form defaults to a Dockerfile git build.
  await page.getByRole("button", { name: "New" }).click();
  await page.getByRole("menuitem", { name: "From Git repo" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByText("New application").waitFor({ timeout: 8000 });
  const build = await dialog.locator("select").first().inputValue();
  if (build === "dockerfile") log("build type preselected = dockerfile ✓");
  else { failed = true; console.error("  FAIL: build type not preselected:", build); }
  await dialog.getByPlaceholder("https://github.com/user/repo").waitFor({ timeout: 5000 });
  log("git repo field visible ✓");

  // Fill the form: name, repo, port.
  await dialog.getByRole("textbox").first().fill(APP);
  await dialog.getByPlaceholder("https://github.com/user/repo").fill(REPO);
  await dialog.getByRole("spinbutton").first().fill("8080");
  await dialog.getByRole("button", { name: "Create application" }).click();
  await page.getByText(APP).first().waitFor({ timeout: 8000 });
  log("git app created via dashboard ✓");

  // Deploy → build-log drawer opens; wait for a successful build.
  await page.getByRole("button", { name: "Deploy", exact: true }).first().click();
  log("deploy clicked, following build log…");
  await page.getByText(/succeeded|objects applied|success/i).first().waitFor({ timeout: 120000 });
  log("git build + deploy succeeded from the dashboard ✅");
} catch (e) {
  failed = true;
  console.error("[gitui] FAILED:", e.message);
  await page.screenshot({ path: "/tmp/kapibara-gitui-fail.png" }).catch(() => {});
} finally {
  await browser.close();
  process.exit(failed ? 1 : 0);
}
