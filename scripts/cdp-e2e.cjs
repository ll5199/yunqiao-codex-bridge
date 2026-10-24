// End-to-end check of image delivery and saving over DevTools: opens the mock
// Codex page with a debugging port, then the Go test drives the real bridge loop.
// Usage: node scripts/cdp-e2e.cjs   (runs `go test -run TestCDPImageDeliveryAndDownload`)
const { chromium } = require('playwright');
const { spawn } = require('child_process');
const fs = require('fs'), path = require('path'), http = require('http');
const root = path.join(__dirname, '..');
const port = 9333;
(async () => {
  const html = fs.readFileSync(path.join(__dirname, 'image-card', 'mock-codex.html'), 'utf8');
  const server = http.createServer((q, r) => { r.setHeader('content-type', 'text/html'); r.end(html); }).listen(0);
  const browser = await chromium.launch({ args: [`--remote-debugging-port=${port}`, `--remote-allow-origins=http://127.0.0.1:${port}`] });
  const page = await browser.newPage();
  await page.goto(`http://127.0.0.1:${server.address().port}/`);
  await page.addScriptTag({ content: fs.readFileSync(path.join(root, 'inject.js'), 'utf8') });
  const go = spawn('go', ['test', '-count=1', '-v', '-run', 'TestCDPImageDeliveryAndDownload', '.'],
    { cwd: root, env: { ...process.env, YUNQIAO_CDP_E2E_PORT: String(port) }, stdio: 'inherit' });
  const goExit = new Promise((resolve) => go.on('exit', resolve));
  let failed = false;
  try {
    await page.waitForSelector('[data-yunqiao-image-key] img', { timeout: 60000 });
    console.log('PASS image delivered over DevTools and shown');
    await page.mouse.move(0, 0);
    await page.click('text=下载原图');
    await page.waitForSelector('text=/已保存到“下载”/', { timeout: 30000 });
    console.log('PASS button reports the saved file:', await page.textContent('text=/已保存到“下载”/'));
  } catch (error) { failed = true; console.log('FAIL', error.message); }
  const code = await goExit;
  await Promise.race([browser.close(), new Promise((resolve) => setTimeout(resolve, 3000))]);
  server.close();
  process.exit(failed || code ? 1 : 0);
})();
