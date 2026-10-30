// Renders the website's tutorial screenshots and the walkthrough video from the
// real client UI (frontend/dist) with a mocked backend, so the site can be
// refreshed whenever the UI changes.
// Usage: (cd frontend && npm run build) && node scripts/render-site-media.cjs [out dir]
// Output: <out>/*.png screenshots, <out>/yunqiao-tutorial.mp4 and its poster.
const { chromium } = require('playwright');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const http = require('node:http');
const path = require('node:path');

const root = path.join(__dirname, '..');
const dist = path.join(root, 'frontend', 'dist');
const out = path.resolve(process.argv[2] || path.join(root, 'docs', 'website', 'media'));
const version = /appVersion\s*=\s*"([^"]+)"/.exec(fs.readFileSync(path.join(root, 'config.go'), 'utf8'))[1];
fs.mkdirSync(out, { recursive: true });

const types = { '.js': 'text/javascript', '.css': 'text/css', '.html': 'text/html', '.svg': 'image/svg+xml' };
const server = http.createServer((request, response) => {
  const file = path.join(dist, request.url.split('?')[0] === '/' ? 'index.html' : request.url.split('?')[0]);
  if (!file.startsWith(dist) || !fs.existsSync(file)) { response.statusCode = 404; return response.end(); }
  response.setHeader('content-type', types[path.extname(file)] || 'application/octet-stream');
  response.end(fs.readFileSync(file));
}).listen(0, '127.0.0.1');

// Demo data only; no real key or account is shown.
function mockBackend(appVersion) {
  const day = (offset) => { const d = new Date(); d.setDate(d.getDate() - offset); return d.toISOString().slice(0, 10); };
  const state = { baseURL: 'https://api.velyn65.com/v1', hasAPIKey: false, models: [], selectedModel: 'yunqiao-auto', version: appVersion, status: '准备就绪', cloud: { loggedIn: false, providers: [], usage: [] }, running: false, proxyRunning: false };
  const usage = { hasBalance: true, balance: 25.6, unit: 'USD', plan: '钱包余额', todayCost: 0.36, todayRequests: 4, totalCost: 8.42, totalRequests: 216, totalTokens: 15800000, updatedAt: Date.now(),
    daily: [[1, 4, 0.36], [2, 12, 0.92], [4, 9, 0.61], [6, 21, 1.48], [9, 7, 0.44], [12, 15, 1.06], [16, 30, 1.92], [21, 8, 0.52], [25, 6, 0.41]].map(([o, r, c]) => ({ date: day(o), requests: r, cost: c })),
    models: [{ model: 'gpt-6-sol', requests: 120, cost: 5.71 }, { model: 'gpt-6-luna', requests: 88, cost: 2.38 }, { model: 'gpt-6.1-sol', requests: 8, cost: 0.33 }] };
  window.runtime = { EventsOn: () => () => {} };
  window.go = { main: { BridgeApp: {
    GetState: async () => ({ ...state }),
    SaveConnection: async () => { Object.assign(state, { hasAPIKey: true, models: ['yunqiao-auto', 'gpt-6-sol', 'gpt-6-luna', 'gpt-6.1-sol'], status: '配置已加密保存，已同步 3 个模型。' }); return { ...state }; },
    GetUsage: async () => { if (!state.hasAPIKey) throw new Error('尚未保存 API Key'); return usage; },
    Launch: async () => { Object.assign(state, { running: true, proxyRunning: true, status: 'Codex 已启动，本机代理运行中。' }); return state.status; },
    CheckUpdates: async () => ({ available: false, current: appVersion, latest: appVersion }),
    ExportDiagnostics: async () => '', Close: async () => {},
  } } };
}

// Caption bar and a visible pointer for the video (Playwright records no cursor).
function overlay() {
  const style = document.createElement('style');
  style.textContent = '#tour-caption{position:fixed;left:50%;bottom:4px;transform:translateX(-50%);z-index:2147483647;max-width:94%;padding:7px 18px;border-radius:10px;background:rgba(17,40,32,.94);color:#fff;white-space:nowrap;font:600 15px/1.5 system-ui,"Microsoft YaHei",sans-serif;text-align:center;box-shadow:0 8px 24px rgba(0,0,0,.25);transition:opacity .25s}#tour-cursor{position:fixed;z-index:2147483647;width:22px;height:22px;margin:-4px 0 0 -4px;pointer-events:none;transition:left .6s ease,top .6s ease}#tour-cursor.click::after{content:"";position:absolute;left:-9px;top:-9px;width:36px;height:36px;border-radius:50%;background:rgba(29,122,79,.35);animation:tour-pulse .45s ease-out}@keyframes tour-pulse{from{transform:scale(.3);opacity:1}to{transform:scale(1.2);opacity:0}}';
  document.head.appendChild(style);
  const caption = Object.assign(document.createElement('div'), { id: 'tour-caption' });
  caption.style.opacity = '0';
  const cursor = Object.assign(document.createElement('div'), { id: 'tour-cursor' });
  cursor.innerHTML = '<svg width="22" height="22" viewBox="0 0 24 24"><path d="M4 2l16 9-7 2-3 7z" fill="#fff" stroke="#133a29" stroke-width="1.6" stroke-linejoin="round"/></svg>';
  cursor.style.left = '520px'; cursor.style.top = '420px';
  document.body.append(caption, cursor);
}

(async () => {
  const browser = await chromium.launch();
  const url = `http://127.0.0.1:${server.address().port}/`;
  const errors = [];
  async function app(options = {}) {
    const page = await browser.newPage({ viewport: { width: 1000, height: 680 }, deviceScaleFactor: 1.5, ...options });
    page.on('pageerror', (error) => errors.push(error.message));
    await page.addInitScript(mockBackend, version);
    await page.goto(url);
    await page.getByRole('button', { name: '外部 API', exact: true }).waitFor();
    if (await page.getByRole('button', { name: '云桥账号', exact: true }).count()) throw new Error('云桥账号 should be hidden');
    return page;
  }

  // Tutorial screenshots, named after the figures they replace.
  let page = await app();
  await page.getByLabel('API Key', { exact: true }).fill('sk-example-not-a-real-key');
  await page.screenshot({ path: `${out}/api-config.png` });
  await page.getByRole('button', { name: '保存并同步模型', exact: true }).click();
  await page.locator('.usage-strip').waitFor();
  await page.screenshot({ path: `${out}/api-saved.png` });
  await page.locator('.usage-strip').getByRole('button', { name: '明细' }).click();
  await page.locator('.usage-modal').waitFor();
  await page.locator('.day-col').nth(23).hover();
  await page.screenshot({ path: `${out}/usage-detail.png` });
  await page.getByRole('button', { name: '关闭明细' }).click();
  await page.getByRole('button', { name: '启动 Codex', exact: true }).click();
  await page.getByRole('button', { name: 'Codex 已启动', exact: true }).waitFor();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: `${out}/api-running.png` });
  await page.getByRole('button', { name: '设置', exact: true }).click();
  await page.getByRole('dialog').waitFor();
  await page.screenshot({ path: `${out}/settings.png` });
  await page.close();
  page = await app();
  await page.getByRole('button', { name: '官方账号', exact: true }).click();
  await page.screenshot({ path: `${out}/official.png` });
  await page.close();

  // Walkthrough video.
  const videoDir = fs.mkdtempSync(path.join(require('node:os').tmpdir(), 'yunqiao-video-'));
  const context = await browser.newContext({ viewport: { width: 1000, height: 680 }, recordVideo: { dir: videoDir, size: { width: 1000, height: 680 } } });
  page = await context.newPage();
  page.on('pageerror', (error) => errors.push(error.message));
  await page.addInitScript(mockBackend, version);
  await page.goto(url);
  await page.getByRole('button', { name: '外部 API', exact: true }).waitFor();
  await page.evaluate(overlay);
  const say = (text, ms) => page.evaluate(([t]) => { const c = document.getElementById('tour-caption'); c.textContent = t; c.style.opacity = t ? '1' : '0'; }, [text]).then(() => page.waitForTimeout(ms));
  const point = async (locator, click = true) => {
    const box = await locator.boundingBox();
    await page.evaluate(([x, y]) => { const c = document.getElementById('tour-cursor'); c.style.left = x + 'px'; c.style.top = y + 'px'; }, [box.x + box.width / 2, box.y + box.height / 2]);
    await page.waitForTimeout(750);
    if (click) {
      await page.evaluate(() => { const c = document.getElementById('tour-cursor'); c.classList.remove('click'); void c.offsetWidth; c.classList.add('click'); });
      await locator.click();
    }
  };
  await say(`云桥 v${version} 使用演示`, 1800);
  await say('① 连接类型选择「外部 API」', 600);
  await point(page.getByRole('button', { name: '外部 API', exact: true }));
  await page.waitForTimeout(1200);
  await say('② 接口地址保持默认，填写你的 API Key', 600);
  const key = page.getByLabel('API Key', { exact: true });
  await point(key);
  await key.pressSequentially('sk-example-not-a-real-key', { delay: 45 });
  await page.waitForTimeout(700);
  await say('③ 点击「保存并同步模型」，自动同步模型并选中智能模型', 600);
  await point(page.getByRole('button', { name: '保存并同步模型', exact: true }));
  await page.locator('.usage-strip').waitFor();
  await page.waitForTimeout(2400);
  await say('④ 下方显示余额、今日和累计消耗，余额低于 $2 时橙色提醒', 600);
  await point(page.locator('.usage-strip > div').first(), false);
  await page.waitForTimeout(2400);
  await say('⑤ 点击「明细」，查看每日消耗和各模型花费', 600);
  await point(page.locator('.usage-strip').getByRole('button', { name: '明细' }));
  await page.locator('.usage-modal').waitFor();
  await point(page.locator('.day-col').nth(23), false);
  await page.locator('.day-col').nth(23).hover();
  await page.waitForTimeout(2600);
  await point(page.getByRole('button', { name: '关闭明细' }));
  await page.waitForTimeout(500);
  await say('⑥ 默认智能模型会按任务自动选择，也可以点选指定模型', 600);
  await point(page.getByRole('button', { name: 'gpt-6-sol', exact: true }), false);
  await page.waitForTimeout(2400);
  await say('⑦ 点击「启动 Codex」，官方 Codex 自动打开并使用此配置', 600);
  await point(page.getByRole('button', { name: '启动 Codex', exact: true }));
  await page.getByRole('button', { name: 'Codex 已启动', exact: true }).waitFor();
  await page.waitForTimeout(2600);
  await say('⑧ 右上角设置：检查更新、导出诊断日志', 600);
  await point(page.getByRole('button', { name: '设置', exact: true }));
  await page.getByRole('dialog').waitFor();
  await page.waitForTimeout(1200);
  await point(page.getByRole('button', { name: '检查更新' }));
  await page.waitForTimeout(2400);
  await say('使用外部 API 时请保持云桥运行 · 更多说明见图文教程', 3000);
  const video = page.video();
  await context.close();
  const webm = await video.path();
  const mp4 = `${out}/yunqiao-tutorial.mp4`;
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', webm, '-c:v', 'libx264', '-preset', 'slow', '-crf', '26', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', '-an', mp4]);
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-ss', '9', '-i', mp4, '-frames:v', '1', '-q:v', '3', `${out}/yunqiao-tutorial-poster.jpg`]);
  fs.rmSync(videoDir, { recursive: true, force: true });

  await browser.close();
  server.close();
  if (errors.length) throw new Error(errors.join('\n'));
  for (const name of fs.readdirSync(out)) console.log(`${name}  ${fs.statSync(path.join(out, name)).size} bytes`);
})().catch((error) => { console.error(error); server.close(); process.exitCode = 1; });
