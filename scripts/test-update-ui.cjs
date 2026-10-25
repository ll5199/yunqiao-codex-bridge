// Drives the settings update panel and banner against frontend/dist with a mocked backend.
// Usage: (cd frontend && npm run build) && node scripts/test-update-ui.cjs [frontend/dist] [screenshot dir]
const { chromium } = require('playwright'); const http = require('http'); const fs = require('fs'); const path = require('path');
const dist = process.argv[2] || path.join(__dirname, '..', 'frontend', 'dist'), out = process.argv[3] || require('os').tmpdir();
const server = http.createServer((q, r) => { let f = path.join(dist, q.url.split('?')[0] === '/' ? 'index.html' : q.url.split('?')[0]);
  if (!fs.existsSync(f)) { r.statusCode = 404; return r.end(); } r.setHeader('content-type', f.endsWith('.js') ? 'text/javascript' : f.endsWith('.css') ? 'text/css' : 'text/html'); r.end(fs.readFileSync(f)); }).listen(0);
(async () => {
  const b = await chromium.launch(); const p = await b.newPage({ viewport: { width: 1000, height: 680 } });
  await p.addInitScript(() => {
    const handlers = {}; window.__emit = (n, v) => (handlers[n] || []).forEach((h) => h(v));
    window.runtime = { EventsOn: (n, h) => { (handlers[n] ||= []).push(h); return () => {}; } };
    window.go = { main: { BridgeApp: {
      GetState: async () => ({ version: '1.6.8', models: ['yunqiao-auto', 'gpt-5.6'], hasAPIKey: true, status: '准备就绪', cloud: { loggedIn: false, providers: [], usage: [] } }),
      CheckUpdates: async () => ({ current: '1.6.8', latest: '1.6.9', available: true, notes: '修复若干问题。' }),
      InstallUpdate: () => new Promise(() => { window.__installing = true; }),
    } } };
  });
  await p.goto(`http://127.0.0.1:${server.address().port}/`); await p.waitForTimeout(500);
  await p.click('[aria-label="设置"]'); await p.click('text=检查更新'); await p.waitForSelector('text=立即更新到 v1.6.9');
  await p.screenshot({ path: out + '/ui-available.png' });
  await p.click('text=立即更新到 v1.6.9');
  await p.evaluate(() => window.__emit('bridge:update', { stage: 'downloading', percent: 41, message: '正在下载 v1.6.9…' }));
  await p.waitForTimeout(400); await p.screenshot({ path: out + '/ui-downloading.png' });
  const bar = await p.$eval('.settings-modal [role=progressbar]', (e) => e.getAttribute('aria-valuenow'));
  await p.click('[aria-label="关闭设置"]'); await p.waitForTimeout(200);
  const banner = await p.isVisible('.update-banner'); await p.screenshot({ path: out + '/ui-banner.png' });
  await p.evaluate(() => window.__emit('bridge:update', { stage: 'error', percent: 0, message: '下载更新失败：HTTP 404' }));
  await p.waitForTimeout(200); const bannerGone = !(await p.isVisible('.update-banner'));
  await p.click('[aria-label="设置"]'); const retry = await p.isVisible('text=重试');
  await p.screenshot({ path: out + '/ui-error.png' });
  console.log(JSON.stringify({ progressInModal: bar, bannerWhenModalClosed: banner, bannerHiddenOnError: bannerGone, retryShown: retry }));
  await b.close(); server.close();
})();
