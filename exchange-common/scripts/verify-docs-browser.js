const { chromium } = require("playwright");

const url = process.argv[2];
if (!url) {
  console.error("Usage: verify-docs-browser.js <url>");
  process.exit(1);
}

const hasSemver = (text) => /v\d+\.\d+\.\d+/.test(text || "");

async function run() {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  const consoleErrors = [];

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push(msg.text());
    }
  });

  page.on("pageerror", (err) => {
    consoleErrors.push(err.message);
  });

  await page.goto(url, { waitUntil: "networkidle" });

  const currentVersion = await page.textContent("main section .card h3");
  const historyCount = await page.$$eval(
    "table.version-table tbody tr",
    (rows) => rows.length
  );
  const errorLinkCount = await page.$$eval(
    "a.link-pill[href$=\"errors.md\"]",
    (links) => links.length
  );

  await browser.close();

  const failures = [];
  if (!hasSemver(currentVersion)) {
    failures.push("current version missing");
  }
  if (historyCount < 1) {
    failures.push("no historical versions listed");
  }
  if (errorLinkCount < 1) {
    failures.push("error code link missing");
  }
  if (consoleErrors.length) {
    failures.push(`console errors: ${consoleErrors.join("; ")}`);
  }

  if (failures.length) {
    console.error(`Browser verification failed: ${failures.join(" | ")}`);
    process.exit(1);
  }

  console.log("Browser verification passed.");
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
