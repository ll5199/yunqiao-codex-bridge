// Drives inject.js's generated-image card in a mock Codex page (Chromium).
// Usage: node scripts/test-image-card.cjs [inject.js] [#late]
const { chromium } = require('playwright');
const fs = require('fs'), path = require('path'), http = require('http');
const inject = fs.readFileSync(process.argv[2] || path.join(__dirname, '..', 'inject.js'), 'utf8');
const html = fs.readFileSync(path.join(__dirname, 'image-card', 'mock-codex.html'), 'utf8');
const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64');
let failures = 0; const check = (ok, name) => { console.log((ok ? 'PASS ' : 'FAIL ') + name); if (!ok) failures++; };
(async () => {
  const server = http.createServer((q, r) => { r.setHeader('content-type', 'text/html'); r.end(html); }).listen(0);
  const browser = await chromium.launch();
  const page = await browser.newPage({ acceptDownloads: true });
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  await page.goto(`http://127.0.0.1:${server.address().port}/${process.argv[3]||''}`);
  await page.addScriptTag({ content: inject });
  await page.waitForTimeout(300);
  const dataURL = 'data:image/png;base64,' + png.toString('base64');
  await page.evaluate(d => window.__yunqiaoAcceptProxyImages([{ id: 'img1', source: 'http://127.0.0.1:9230/yunqiao/image/img1', download_url: 'http://127.0.0.1:9230/yunqiao/download/img1', file_name: 'dog.png', mime_type: 'image/png', created_at: Date.now(), data_url: d }]), dataURL);
  await page.mouse.move(0, 0); await page.waitForTimeout(1500);
  const card = () => page.$('[data-yunqiao-image-key]');
  const visible = async () => page.evaluate(() => { const c = document.querySelector('[data-yunqiao-image-key]'); if (!c || c.hidden) return false;
    for (let n = c; n; n = n.parentElement) { const s = getComputedStyle(n); if (s.opacity === '0' || s.visibility === 'hidden' || s.display === 'none') return false; }
    const img = c.querySelector('img'); return img.complete && img.naturalWidth > 0; });
  if (process.argv[3] === '#late') {
    check(await page.evaluate(() => !!document.querySelector('[data-yunqiao-image-key]')), 'card mounted before reply actions exist');
    await page.evaluate(() => finishReply()); await page.mouse.move(0, 0); await page.waitForTimeout(1500);
  }
  check(await visible(), 'image card visible without hover in thread a');
  check(await page.evaluate(() => { const c = document.querySelector('[data-yunqiao-image-key]'); return !!c && !c.closest('.user') && !!c.closest('.turn'); }), 'card sits in the assistant reply, not the user message');
  check(await page.evaluate(() => { const c = document.querySelector('[data-yunqiao-image-key]'); return !!c && !!c.closest('main') && !c.closest('form') && !c.closest('.actions'); }), 'card placed in conversation, outside composer and hover row');
  await page.click('[data-app-action-sidebar-thread-id="b"]'); await page.mouse.move(0, 0); await page.waitForTimeout(1500);
  check(!(await visible()), 'card not shown in thread b');
  await page.click('[data-app-action-sidebar-thread-id="a"]'); await page.mouse.move(0, 0); await page.waitForTimeout(1500);
  check(await visible(), 'card shown again after switching back to thread a');
  const [download] = await Promise.all([page.waitForEvent('download', { timeout: 5000 }).catch(() => null), page.click('text=下载原图')]);
  check(!!download && download.suggestedFilename() === 'dog.png', 'download button saves dog.png');
  const [download2] = await Promise.all([page.waitForEvent('download', { timeout: 5000 }).catch(() => null), page.click('[data-yunqiao-image-key] img', { button: 'right' })]);
  check(!!download2, 'right-click saves the image');
  check(errors.length === 0, 'no page errors ' + JSON.stringify(errors));
  await browser.close(); server.close(); process.exit(failures ? 1 : 0);
})();
