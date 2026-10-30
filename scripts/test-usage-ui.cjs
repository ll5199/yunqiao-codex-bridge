// Drives the usage strip and detail dialog against frontend/dist with a mocked backend.
// Usage: (cd frontend && npm run build) && node scripts/test-usage-ui.cjs [frontend/dist] [screenshot dir]
const { chromium } = require('playwright'); const http = require('http'); const fs = require('fs'); const path = require('path');
const dist = process.argv[2] || path.join(__dirname, '..', 'frontend', 'dist'), out = process.argv[3] || require('os').tmpdir();
const server = http.createServer((q, r) => { const u = q.url.split('?')[0]; const f = path.join(dist, u === '/' ? 'index.html' : u);
  if (!fs.existsSync(f)) { r.statusCode = 404; return r.end(); } r.setHeader('content-type', f.endsWith('.js') ? 'text/javascript' : f.endsWith('.css') ? 'text/css' : 'text/html'); r.end(fs.readFileSync(f)); }).listen(0);
const day = (offset) => { const d = new Date(); d.setDate(d.getDate() - offset); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'); };
let failures = 0; const check = (ok, name) => { console.log((ok ? 'PASS ' : 'FAIL ') + name); if (!ok) failures++; };
(async () => { const b = await chromium.launch();
  for (const balance of [59.91, 1.62, null]) {
    const p = await b.newPage({ viewport: { width: 1000, height: 680 } });
    const usage = { hasBalance: balance !== null, balance: balance || 0, unit: 'USD', plan: '钱包余额', todayCost: 0.42, todayRequests: 3, totalCost: 13.91, totalRequests: 482, totalTokens: 36574257,
      daily: [{ date: day(5), requests: 16, cost: 1.61 }, { date: day(6), requests: 11, cost: 1.13 }, { date: day(29), requests: 4, cost: 0.2 }],
      models: [{ model: 'gpt-6-sol', requests: 178, cost: 7.42 }, { model: 'gpt-6-luna', requests: 132, cost: 2.4 }], updatedAt: Date.now() };
    await p.addInitScript((u) => { window.runtime = { EventsOn: () => () => {} }; window.go = { main: { BridgeApp: {
      GetState: async () => ({ version: '1.6.11', models: ['yunqiao-auto', 'gpt-6-sol'], hasAPIKey: true, status: '准备就绪', cloud: { loggedIn: false, providers: [], usage: [] } }),
      GetUsage: async () => u } } }; }, usage);
    await p.goto(`http://127.0.0.1:${server.address().port}/`); await p.waitForSelector('.usage-strip');
    const name = balance === null ? 'nobalance' : balance < 2 ? 'low' : 'normal';
    check(!(await p.isVisible('text=云桥账号')), name + ': 云桥账号 tab hidden');
    check(await p.isVisible('.connection-option >> text=官方账号'), name + ': other tabs shown');
    check((await p.$('.usage-strip.low')) !== null === (balance !== null && balance < 2), name + ': low-balance style only below $2');
    const launchBox = await p.locator('.launch-button, button:has-text("启动 Codex")').first().boundingBox();
    check(!!launchBox && launchBox.y + launchBox.height <= 680, name + ': launch button fully visible');
    await p.screenshot({ path: `${out}/usage-${name}.png` });
    await p.click('.usage-strip >> text=明细'); await p.waitForSelector('.usage-modal');
    check(await p.locator('.day-col').count() === 30, name + ': 30 day columns');
    check(await p.locator('.day-bar[style*="height"]').evaluateAll((bars) => bars.filter((bar) => parseFloat(bar.style.height) > 0).length) === 3, name + ': bars only on days with spend');
    await p.hover('.day-col:nth-child(25)'); check(await p.isVisible('.day-tip'), name + ': hover tooltip');
    await p.screenshot({ path: `${out}/usage-${name}-detail.png` });
    await p.close(); }
  await b.close(); server.close(); process.exit(failures ? 1 : 0); })();
