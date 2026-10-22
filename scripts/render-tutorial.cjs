const { chromium } = require('playwright');
const fs = require('node:fs');
const { spawn } = require('node:child_process');
const root = process.cwd();
const site = `${root}/docs/website`;
const shots = `${site}/images`;
fs.mkdirSync(shots, { recursive: true });
const appServer = spawn('python3', ['-m', 'http.server', '8123', '--bind', '127.0.0.1', '--directory', 'frontend/dist'], {stdio:'ignore'});
const siteServer = spawn('python3', ['-m', 'http.server', '8124', '--bind', '127.0.0.1', '--directory', 'docs/website'], {stdio:'ignore'});
(async () => {
 const browser = await chromium.launch({headless:true});
 const errors = [];
 async function newApp() {
  const page = await browser.newPage({viewport:{width:1000,height:680},deviceScaleFactor:1.5});
  page.on('pageerror',e=>errors.push(e.message));
  await page.addInitScript(() => {
   const state={baseURL:'https://api.velyn65.com/v1',hasAPIKey:false,models:[],selectedModel:'yunqiao-auto',version:'1.6.4',status:'准备就绪',cloud:{loggedIn:false,providers:[],usage:[]},running:false,proxyRunning:false};
   window.go={main:{BridgeApp:{
    GetState:async()=>({...state}),
    SaveConnection:async()=>{Object.assign(state,{hasAPIKey:true,models:['yunqiao-auto','gpt-6-luna','gpt-6-sol'],status:'配置已加密保存，已同步 2 个模型。'});return {...state}},
    Launch:async()=>{Object.assign(state,{running:true,proxyRunning:true,status:'Codex 已启动，本机代理运行中。'});return state.status},
    CheckUpdates:async()=>({available:false,current:'1.6.4',latest:'1.6.4'}),Close:async()=>{}
   }}};
  });
  await page.goto('http://127.0.0.1:8123',{waitUntil:'networkidle'});
  await page.getByRole('button',{name:'外部 API',exact:true}).waitFor();
  return page;
 }
 let page=await newApp();
 await page.getByLabel('API Key',{exact:true}).fill('example-key-not-valid');
 await page.screenshot({path:`${shots}/api-config.png`});
 await page.getByRole('button',{name:'保存并同步模型',exact:true}).click();
 await page.getByRole('button',{name:'智能模型',exact:true}).waitFor();
 if(await page.getByRole('button',{name:'智能模型',exact:true}).getAttribute('aria-pressed')!=='true')throw Error('Smart model not selected');
 await page.screenshot({path:`${shots}/api-saved.png`});
 await page.getByRole('button',{name:'启动 Codex',exact:true}).click();
 await page.getByRole('button',{name:'Codex 已启动',exact:true}).waitFor();
 if(!await page.getByRole('button',{name:'Codex 已启动',exact:true}).isDisabled())throw Error('Launch button not disabled');
 await page.waitForTimeout(2200);
 await page.screenshot({path:`${shots}/api-running.png`});
 await page.getByRole('button',{name:'设置',exact:true}).click();
 await page.getByRole('dialog').waitFor();
 await page.screenshot({path:`${shots}/settings.png`});
 await page.close();
 page=await newApp();await page.getByRole('button',{name:'云桥账号',exact:true}).click();
 await page.screenshot({path:`${shots}/cloud.png`});await page.close();
 page=await newApp();await page.getByRole('button',{name:'官方账号',exact:true}).click();
 await page.screenshot({path:`${shots}/official.png`});await page.close();
 for(const [template,out] of [['tutorial.template.html','tutorial.html'],['index.template.html','index.html']]) {
  let html=fs.readFileSync(`${site}/${template}`,'utf8');
  html=html.replace(/@@IMG:([a-z-]+)@@/g,(_,name)=>`data:image/png;base64,${fs.readFileSync(`${shots}/${name}.png`).toString('base64')}`);
  if(html.includes('@@IMG:'))throw Error('Unresolved image placeholder');
  fs.writeFileSync(`${site}/${out}`,html);
 }
 fs.mkdirSync(`${site}/qa`,{recursive:true});
 for(const [label,width,height] of [['desktop',1440,1000],['mobile',390,844]]) {
  page=await browser.newPage({viewport:{width,height}});page.on('pageerror',e=>errors.push(e.message));
  await page.goto('http://127.0.0.1:8124/tutorial.html',{waitUntil:'networkidle'});
  await page.locator('img').evaluateAll(imgs=>Promise.all(imgs.map(i=>i.decode())));
  const stats=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,broken:[...document.images].filter(i=>!i.naturalWidth).length,links:[...document.querySelectorAll('a[href^="#"]')].filter(a=>!document.querySelector(a.getAttribute('href'))).length}));
  if(stats.scroll>stats.width+1||stats.broken||stats.links)throw Error(`${label}: ${JSON.stringify(stats)}`);
  await page.screenshot({path:`${site}/qa/tutorial-${label}.png`});
  await page.locator('#steps').scrollIntoViewIfNeeded();await page.screenshot({path:`${site}/qa/steps-${label}.png`});
  await page.goto('http://127.0.0.1:8124/index.html',{waitUntil:'networkidle'});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1))throw Error(`Homepage overflow ${label}`);
  await page.screenshot({path:`${site}/qa/home-${label}.png`});await page.close();
  console.log(`${label}: images, anchors and layout checks passed`);
 }
 if(errors.length)throw Error(errors.join('\n'));
 await browser.close();
 console.log('Tutorial and homepage built and verified');
})().catch(e=>{console.error(e);process.exitCode=1}).finally(()=>{appServer.kill();siteServer.kill()});
