// Drives the prototype in a real browser and checks that the rules it claims
// to enforce actually fire, in both themes.
const { chromium } = require("playwright-core");
const path = require("path");

const FILE = "file://" + path.resolve(__dirname, "proto/index.html");
let pass = 0, fail = 0;
const check = (label, ok, detail = "") => {
  if (ok) { console.log("  PASS  " + label); pass++; }
  else { console.log("  FAIL  " + label + (detail ? "  → " + detail : "")); fail++; }
};

(async () => {
  const browser = await chromium.launch({ executablePath: "/opt/pw-browsers/chromium-1194/chrome-linux/chrome" });

  for (const scheme of ["light", "dark"]) {
    const ctx = await browser.newContext({ colorScheme: scheme, viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    const errors = [];
    page.on("pageerror", e => errors.push(String(e)));
    page.on("console", m => { if (m.type() === "error") errors.push(m.text()); });

    await page.goto(FILE);
    console.log("\n=== " + scheme + " ===");

    // Contrast sanity: body text must not match the body background.
    const colours = await page.evaluate(() => {
      const s = getComputedStyle(document.body);
      return { bg: s.backgroundColor, fg: s.color };
    });
    check("body paints its own background", colours.bg !== "rgba(0, 0, 0, 0)" && colours.bg !== "transparent", colours.bg);
    check("text is not the same colour as the ground", colours.bg !== colours.fg, `${colours.fg} on ${colours.bg}`);

    // Sign in as planner — the per-company role case.
    await page.click('[data-signin="planner"]');
    await page.waitForSelector(".shell");
    await page.click('[data-screen="planning"]');
    await page.waitForSelector(".shell");
    check("planner signs in", await page.isVisible(".companybar"));

    const roleChip = await page.textContent(".companybar .chip");
    check("role shown for company 1000 is PLANNER", roleChip.trim() === "PLANNER", roleChip);

    // The grid opens on every product the version plans.
    const grids = await page.$$eval("[data-grid]", els => els.map(e => e.dataset.grid));
    check("the matrix shows every planned product at once", grids.length >= 5, grids.join(" / "));

    // Matrix is editable for the draft forecast.
    const firstCell = page.locator("[data-cell]").first();
    check("matrix cells are editable on a draft version", await firstCell.isEnabled());

    // Totals recompute as you type.
    const before = await page.textContent("[data-grand]");
    await firstCell.fill("9999");
    await page.waitForTimeout(60);
    const after = await page.textContent("[data-grand]");
    check("totals recompute while typing", before !== after, `${before} → ${after}`);

    // Column totals must be right on first paint, not only after typing.
    await page.reload();
    await page.click('[data-signin="planner"]');
    await page.click('[data-screen="planning"]');
    await page.waitForSelector(".matrix");
    await page.selectOption('[data-msel="material"]', "RAW_SUGAR");
    const colTotals = await page.$$eval("[data-coltotal]", els => els.map(e => e.textContent.trim()));
    check("line totals are correct on first paint", colTotals.every(t => t !== "0"), colTotals.join(" / "));
    const grand = await page.textContent("[data-grand]");
    const colSum = colTotals.reduce((t, v) => t + Number(v.replace(/[^0-9]/g, "")), 0);
    check("line totals add up to the grand total", colSum === Number(grand.replace(/[^0-9]/g, "")),
          `${colSum} vs ${grand}`);

    // Switch to the approved budget: the grid must lock.
    await page.selectOption('[data-msel="versionId"]', "1");
    await page.waitForTimeout(80);
    check("approved version locks the grid", await page.locator("[data-cell]").first().isDisabled());
    check("and says why", (await page.textContent(".msg.info")).includes("APPROVED"));

    // Switch company to 2000 where the same user is display-only.
    await page.selectOption('[data-company="planning"]', "2");
    await page.waitForTimeout(80);
    const role2 = await page.textContent(".companybar .chip");
    check("same user is VIEWER in company 2000", role2.trim() === "VIEWER", role2);
    check("and loses the ability to edit", await page.locator("[data-cell]").first().isDisabled());

    // --- cane supply ---------------------------------------------------
    await page.click("[data-signout]");
    await page.click('[data-signin="planner"]');
    await page.click('[data-screen="cane"]');
    await page.waitForSelector("[data-hcell]");
    check("the harvest grid is editable on a draft version", await page.locator("[data-hcell]").first().isEnabled());

    const purchasedHeadings = await page.$$eval(".matrix th", els =>
      els.filter(e => e.textContent.includes("bought")).length);
    check("purchased growers are marked apart from own estate", purchasedHeadings === 4, String(purchasedHeadings));

    await page.click('[data-canetab="deliveries"]');
    await page.waitForSelector("[data-post-ticket]");
    const ticketRow = await page.$eval("tbody tr", r => r.textContent);
    check("a ticket shows a derived net weight", /\d/.test(ticketRow), ticketRow.slice(0, 60));

    const naValues = await page.$$eval(".na", els => els.length);
    check("own-estate loads report no value at all", naValues > 0, String(naValues));

    await page.click('[data-canetab="report"]');
    await page.waitForSelector("table");
    const caneReport = await page.textContent(".main");
    check("the cane report splits purchased from own estate",
      caneReport.includes("Purchased cane") && caneReport.includes("Own estate"));

    // A planner may plan cane but not weigh it in.
    await page.click('[data-canetab="deliveries"]');
    check("a planner may not record a delivery", await page.locator("[data-new-ticket]").isDisabled());

    // --- master data -----------------------------------------------------
    await page.click('[data-screen="master"]');
    await page.waitForSelector("[data-mastertab]");
    await page.click('[data-mastertab="tank"]');
    const tankRows = await page.$$eval("tbody tr td:nth-child(3)", els => els.map(e => e.textContent.trim()));
    check("the tank list holds only tanks", tankRows.length > 0 && tankRows.every(t => t === "TANK"), tankRows.join(","));

    await page.click('[data-mastertab="warehouse"]');
    const unlimited = await page.textContent(".main");
    check("an unmaintained capacity reads unlimited, not zero", unlimited.includes("unlimited"));

    await page.click("[data-signout]");
    await page.click('[data-signin="planner"]');

    // Posting rules.
    await page.click('[data-screen="actual"]');
    await page.waitForSelector("[data-do-post]");
    check("planner has no posting authority", await page.locator("[data-do-post]").isDisabled());

    await page.click("[data-signout]");
    await page.click('[data-signin="operator"]');
    await page.click('[data-screen="actual"]');
    await page.waitForSelector("[data-do-post]");
    check("operator may post", await page.locator("[data-do-post]").isEnabled());

    // Default form is white sugar into the silo — must be refused.
    await page.click("[data-do-post]");
    await page.waitForSelector(".msg.bad");
    const refusal = await page.textContent(".msg.bad .code");
    check("white sugar into the silo is refused", ["E-INV-015", "E-PROD-021"].includes(refusal.trim()), refusal);

    // Refined direct into a finished-goods warehouse must be refused.
    await page.selectOption('[data-post="material"]', "REFINED");
    await page.selectOption('[data-post="warehouse"]', "7");
    await page.selectOption('[data-post="process"]', "4");
    await page.click("[data-do-post]");
    await page.waitForTimeout(80);
    const cond = await page.textContent(".msg.bad .code");
    check("conditioned sugar cannot skip the silo", cond.trim() === "E-PROD-022", cond);

    // Capacity.
    await page.selectOption('[data-post="material"]', "REFINED");
    await page.selectOption('[data-post="warehouse"]', "6");
    await page.fill('[data-post="qty"]', "2500");
    await page.click("[data-do-post]");
    await page.waitForTimeout(80);
    const cap = await page.textContent(".msg.bad .code");
    check("silo overflow is refused", cap.trim() === "E-INV-012", cap);

    // Electricity is produced but never stock.
    await page.selectOption('[data-post="material"]', "ELECTRICITY");
    await page.selectOption('[data-post="warehouse"]', "4");
    await page.selectOption('[data-post="process"]', "2");
    await page.fill('[data-post="qty"]', "10");
    await page.click("[data-do-post]");
    await page.waitForTimeout(80);
    const elec = await page.textContent(".msg.bad .code");
    check("electricity cannot be stocked", elec.trim() === "E-INV-014", elec);

    // A legitimate posting must succeed and move stock.
    await page.selectOption('[data-post="material"]', "RAW_SUGAR");
    await page.selectOption('[data-post="warehouse"]', "5");
    await page.selectOption('[data-post="process"]', "1");
    await page.fill('[data-post="qty"]', "500");
    await page.click("[data-do-post]");
    await page.waitForTimeout(80);
    check("a valid posting succeeds", await page.isVisible(".msg.good"));

    await page.click('[data-screen="inventory"]');
    await page.waitForSelector(".meter");
    const bodyText = await page.textContent(".main");
    check("unlimited location reports no percentage", bodyText.includes("unlimited"));
    check("RW2 now holds the posted stock", bodyText.includes("500"));

    // Four eyes.
    await page.click("[data-signout]");
    await page.click('[data-signin="planner"]');
    await page.click('[data-screen="versions"]');
    await page.waitForSelector("[data-transition]");
    await page.click('[data-transition="2|SUBMITTED"]');
    await page.waitForTimeout(80);
    const approveBtn = page.locator('[data-transition="2|APPROVED"]');
    check("planner cannot approve their own version", await approveBtn.isDisabled());

    await page.click("[data-signout]");
    await page.click('[data-signin="approver"]');
    await page.click('[data-screen="versions"]');
    await page.waitForTimeout(80);
    check("approver can approve it", await page.locator('[data-transition="2|APPROVED"]').isEnabled());

    // Report: the n/a rule.
    await page.click('[data-screen="report"]');
    await page.waitForSelector("table");
    const report = await page.textContent(".main");
    check("a material with no plan reads n/a", report.includes("n/a"));

    // Horizontal overflow must be contained.
    const overflow = await page.evaluate(() =>
      document.documentElement.scrollWidth > document.documentElement.clientWidth + 1);
    check("page body does not scroll sideways", !overflow);

    check("no console or page errors", errors.length === 0, errors.slice(0, 2).join(" | "));

    await page.screenshot({ path: `shot-${scheme}.png`, fullPage: false });
    await ctx.close();
  }

  await browser.close();
  console.log(`\n  passed ${pass}, failed ${fail}`);
  process.exit(fail ? 1 : 0);
})();
