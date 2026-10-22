const assert = require('node:assert/strict');
const { spawn } = require('node:child_process');
const { chromium } = require('playwright');
const fs = require('node:fs');
const server = spawn(process.execPath, ['-e', `const http=require('http'),fs=require('fs'),path=require('path');http.createServer((q,r)=>{let p=path.join(process.cwd(),'frontend/dist',q.url.split('?')[0]==='/'?'index.html':q.url.split('?')[0]);fs.readFile(p,(e,b)=>{if(e){r.statusCode=404;return r.end()}r.setHeader('Content-Type',p.endsWith('.js')?'text/javascript':p.endsWith('.css')?'text/css':'text/html');r.end(b)})}).listen(8125,'127.0.0.1')`], { stdio: 'ignore' });
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const connection of ['api', 'official']) {
      for (const scenario of ['success', 'failure', 'unconfirmed']) {
        const page = await browser.newPage({ viewport: { width: 1000, height: 680 } });
        const pageErrors = [];
        page.on('pageerror', error => pageErrors.push(error.message));
        await page.addInitScript(({ scenario }) => {
          const state = { baseURL: 'https://api.example.com/v1', hasAPIKey: true, models: ['yunqiao-auto', 'example-model'], selectedModel: 'yunqiao-auto', version: '1.6.5', status: '准备就绪', cloud: { loggedIn: false, providers: [], usage: [] }, running: false, proxyRunning: false };
          window.testState = state;
          window.launchCalls = [];
          window.go = { main: { BridgeApp: {
            GetState: async () => ({ ...state }),
            Launch: async (...args) => {
              window.launchCalls.push(args);
              await new Promise(resolve => setTimeout(resolve, 200));
              if (scenario === 'failure') throw new Error('应用入口启动失败');
              state.running = scenario === 'success';
              state.proxyRunning = state.running && args[0] === 'api';
              return '启动操作完成';
            },
          } } };
        }, { scenario });
        await page.goto('http://127.0.0.1:8125', { waitUntil: 'networkidle' });
        await page.getByRole('button', { name: connection === 'api' ? '外部 API' : '官方账号', exact: true }).click();
        const launch = page.getByRole('button', { name: '启动 Codex', exact: true });
        await launch.waitFor();
        assert.equal(await launch.isEnabled(), true);
        await launch.click();
        if (scenario === 'success') {
          const running = page.getByRole('button', { name: 'Codex 已启动', exact: true });
          await running.waitFor();
          assert.equal(await running.isDisabled(), true);
          await page.evaluate(() => document.querySelector('.launch-button').click());
          assert.equal(await page.evaluate(() => window.launchCalls.length), 1, 'duplicate launch while running');
          await page.evaluate(() => { window.testState.running = false; window.testState.proxyRunning = false; });
          await page.waitForFunction(() => !document.querySelector('.launch-button').disabled, undefined, { timeout: 6000 });
        } else {
          await page.waitForFunction(() => !document.querySelector('.launch-button').disabled, undefined, { timeout: 6000 });
          assert.equal(await page.getByRole('button', { name: '启动 Codex', exact: true }).isEnabled(), true);
          assert.equal(await page.evaluate(() => window.launchCalls.length), 1);
        }
        assert.equal(await page.evaluate(() => window.launchCalls[0][0]), connection);
        assert.deepEqual(pageErrors, []);
        console.log(`PASS ${connection}: ${scenario}`);
        fs.mkdirSync('build/launch-state-qa', { recursive: true });
        await page.screenshot({ path: `build/launch-state-qa/${connection}-${scenario}.png` });
        await page.close();
      }
    }
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => server.kill());
