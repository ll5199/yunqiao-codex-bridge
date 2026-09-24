import { useEffect, useState } from "react";
import { Check, Eye, EyeOff, Link2, Minus, Play, Settings2, ShieldCheck, Square, UserRound, X } from "lucide-react";

const connections = { api: "外部 API", bridge: "云桥账号", official: "官方账号" };
const autoModel = "yunqiao-auto";
const emptyCloud = { loggedIn: false, providers: [], usage: [] };
const backend = () => window.go?.main?.BridgeApp;

export function App() {
  const [connection, setConnection] = useState("api");
  const [apiModels, setApiModels] = useState([]);
  const [providerModels, setProviderModels] = useState([]);
  const [model, setModel] = useState(autoModel);
  const [showAllModels, setShowAllModels] = useState(false);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("正在读取本机配置…");
  const [notice, setNotice] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [apiUrl, setApiUrl] = useState("https://api.velyn65.com/v1");
  const [apiKey, setApiKey] = useState("");
  const [hasKey, setHasKey] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [cloud, setCloud] = useState(emptyCloud);
  const [provider, setProvider] = useState("");
  const [version, setVersion] = useState("1.6.0");

  function showError(error) {
    const message = error?.message || String(error || "操作失败");
    setNotice(message);
    setStatus("操作失败：" + message);
  }

  useEffect(() => {
    const unsubscribe = window.runtime?.EventsOn?.("bridge:status", setStatus);
    const app = backend();
    if (!app) {
      setStatus("客户端服务尚未就绪");
      return () => unsubscribe?.();
    }
    app.GetState().then((state) => {
      const found = state.models || [];
      setApiUrl(state.baseURL || "https://api.velyn65.com/v1");
      setHasKey(Boolean(state.hasAPIKey));
      setSaved(Boolean(state.hasAPIKey));
      setApiModels(found);
      setModel(found.includes(autoModel) ? autoModel : state.selectedModel || autoModel);
      setVersion(state.version || "1.6.0");
      setStatus(state.status || "准备就绪");
      setCloud(state.cloud || emptyCloud);
    }).catch(showError);
    return () => unsubscribe?.();
  }, []);

  function chooseConnection(next) {
    setConnection(next);
    setNotice("");
    setShowAllModels(false);
    const found = next === "api" ? apiModels : next === "bridge" ? providerModels : [];
    setModel(found.includes(autoModel) ? autoModel : found[0] || autoModel);
  }

  async function saveConnection() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    setBusy(true);
    setNotice("");
    try {
      const state = await app.SaveConnection(apiUrl, apiKey, autoModel);
      const found = state.models || [];
      setApiUrl(state.baseURL);
      setApiModels(found);
      setModel(found.includes(autoModel) ? autoModel : state.selectedModel || autoModel);
      setHasKey(Boolean(state.hasAPIKey));
      setApiKey("");
      setSaved(true);
      setStatus("Key 已加密保存，已同步 " + found.filter((item) => item !== autoModel).length + " 个模型。");
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  async function selectProvider(value) {
    setProvider(value);
    setProviderModels([]);
    setModel(autoModel);
    setShowAllModels(false);
    const app = backend();
    if (!app || !value) return;
    const found = await app.FetchProviderModels(value);
    setProviderModels(found || []);
    setModel((found || []).includes(autoModel) ? autoModel : found?.[0] || autoModel);
  }

  async function loginCloud() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    setBusy(true);
    setNotice("");
    try {
      const result = await app.LoginCloud(email, password);
      setCloud(result);
      setPassword("");
      await selectProvider(result.providers?.[0] || "");
      setStatus("云桥账号已登录，模型已同步。");
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  async function logoutCloud() {
    const app = backend();
    if (!app) return;
    try {
      setCloud(await app.LogoutCloud());
      setProvider("");
      setProviderModels([]);
      setModel(autoModel);
      setStatus("已退出云桥账号。");
    } catch (error) {
      showError(error);
    }
  }

  async function launch() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    if (!window.confirm("启动前会关闭已运行的 Codex 窗口并重新启动。继续吗？")) return;
    setBusy(true);
    setNotice("");
    setStatus("正在启动 Codex…");
    try {
      const result = await app.Launch(connection, provider, model, apiUrl, apiKey);
      setStatus(result || "Codex 已启动。");
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  async function checkUpdates() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    setBusy(true);
    setNotice("");
    try {
      const result = await app.CheckUpdates();
      if (!result.available) return setStatus("云桥客户端 v" + result.current + " 已是最新版。");
      if (window.confirm("发现 v" + result.latest + " 更新（当前 v" + result.current + "）。\n\n" + (result.notes || "") + "\n\n是否下载并安装？")) {
        await app.InstallUpdate();
      } else {
        setStatus("已取消更新。");
      }
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  const fetched = connection === "api" ? apiModels : connection === "bridge" ? providerModels : [];
  const modelOptions = fetched.length ? [autoModel, ...fetched.filter((item) => item !== autoModel)] : [];
  const visibleModels = showAllModels ? modelOptions : modelOptions.slice(0, 5);
  const ready = connection === "official" || (connection === "api" ? saved && Boolean(fetched.length) : cloud.loggedIn && Boolean(provider) && Boolean(fetched.length));
  const fetchedCount = fetched.filter((item) => item !== autoModel).length;

  return <div className="desktop-shell">
    <header className="topbar">
      <div className="brand-lockup"><div className="brand-mark"><Link2 size={24} strokeWidth={2.8} /></div><strong>云桥</strong><span>Codex Bridge</span><i /><small>熙楠</small></div>
      <div className="top-actions"><span className="connection-state"><span className={"status-dot " + (ready ? "" : "pending")} />{ready ? "连接正常" : "等待连接"}</span><button type="button" className="icon-button" aria-label="设置" title="设置" onClick={() => setShowSettings(true)}><Settings2 size={21} /></button><div className="window-controls"><button type="button" className="window-button" aria-label="最小化" onClick={() => window.runtime?.WindowMinimise?.()}><Minus size={19} /></button><button type="button" className="window-button" aria-label="最大化或还原" onClick={() => window.runtime?.WindowToggleMaximise?.()}><Square size={15} /></button><button type="button" className="window-button close" aria-label="关闭" onClick={() => window.runtime?.Quit?.()}><X size={18} /></button></div></div>
    </header>

    <main className="workspace">
      <h1>准备启动 Codex</h1>
      <section className="work-section" aria-labelledby="connection-heading">
        <h2 id="connection-heading">连接类型</h2>
        <div className="connection-options" role="group" aria-label="连接类型">
          {Object.entries(connections).map(([key, label]) => <button key={key} type="button" className={"connection-option " + (connection === key ? "selected" : "")} aria-pressed={connection === key} onClick={() => chooseConnection(key)}>{key === "api" ? <Link2 size={20} /> : key === "bridge" ? <UserRound size={20} /> : <ShieldCheck size={20} />}{label}</button>)}
        </div>
      </section>

      <section className="connection-config" aria-label="连接配置">
        {connection === "api" && <div className="api-fields">
          <div className="field-row"><label htmlFor="api-url">接口地址</label><input id="api-url" type="url" autoComplete="off" value={apiUrl} onChange={(event) => { setApiUrl(event.target.value); setSaved(false); }} placeholder="https://api.velyn65.com/v1" /></div>
          <div className="field-row"><label htmlFor="api-key">API Key</label><div className="key-input"><input id="api-key" type={showKey ? "text" : "password"} autoComplete="off" spellCheck="false" value={apiKey} onChange={(event) => { setApiKey(event.target.value); setSaved(false); }} placeholder={hasKey ? "已加密保存，留空继续使用" : "输入 API Key"} /><button type="button" className="eye-button" aria-label={showKey ? "隐藏 Key" : "显示 Key"} onClick={() => setShowKey(!showKey)}>{showKey ? <EyeOff size={17} /> : <Eye size={17} />}</button></div></div>
          <button type="button" className="save-button" onClick={saveConnection} disabled={busy}>{busy ? "正在同步…" : "保存并同步模型"}</button>
        </div>}
        {connection === "bridge" && (!cloud.loggedIn ? <div className="cloud-fields"><div className="field-row"><label htmlFor="cloud-email">云桥账号</label><input id="cloud-email" type="email" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="邮箱" /></div><div className="field-row"><label htmlFor="cloud-password">密码</label><input id="cloud-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="密码" /></div><button className="save-button" onClick={loginCloud} disabled={busy}>{busy ? "正在登录…" : "登录并同步模型"}</button></div> : <div className="cloud-connected"><div><strong>{cloud.identity || "云桥账号已连接"}</strong><span>{cloud.groups || "已读取订阅"}</span></div><div className="provider-options" role="group" aria-label="选择上游">{(cloud.providers || []).map((item) => <button key={item} className={"provider " + (provider === item ? "selected" : "")} aria-pressed={provider === item} onClick={() => selectProvider(item)}>{item}</button>)}</div><button className="text-button" onClick={logoutCloud}>退出登录</button></div>)}
        {connection === "official" && <div className="official-info"><ShieldCheck size={21} /><span>使用官方账号启动；请在 Codex 中登录 ChatGPT。</span></div>}
      </section>

      {connection !== "official" && <section className="model-section" aria-labelledby="model-heading">
        <div className="model-heading"><h2 id="model-heading">可用模型</h2><span>{fetchedCount ? "已同步 " + fetchedCount + " 个" : "保存或登录后自动同步"}</span></div>
        {modelOptions.length ? <div className="model-options" role="group" aria-label="选择启动模型">
          {visibleModels.map((item) => <button key={item} type="button" className={"model-option " + (model === item ? "selected" : "")} aria-pressed={model === item} onClick={() => setModel(item)}>{model === item && <Check size={17} strokeWidth={2.7} />}{item === autoModel ? "智能模型" : item}</button>)}
          {modelOptions.length > 5 && <button type="button" className="text-button more-button" aria-expanded={showAllModels} onClick={() => setShowAllModels(!showAllModels)}>{showAllModels ? "收起" : "查看更多"}</button>}
        </div> : <div className="empty-models"><span className="model-option selected"><Check size={17} />智能模型</span><span>模型同步后即可选择</span></div>}
        <p className="model-hint"><span className="status-dot" />智能模型会根据任务自动路由；也可以手动选择已同步的模型</p>
      </section>}

      <button type="button" className="launch-button" onClick={launch} disabled={busy || !ready}><Play size={22} fill="currentColor" strokeWidth={1.5} />{busy ? "正在处理…" : "启动 Codex"}</button>
      <p className="launch-footnote">{ready ? "已准备就绪，将使用所选配置启动 Codex CLI。" : "请先完成连接配置"}</p>
    </main>

    <footer className="status-line" role="status" aria-live="polite"><span>{status}</span><span>云桥客户端 v{version}</span></footer>
    {showSettings && <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) setShowSettings(false); }}><section className="settings-modal" role="dialog" aria-modal="true" aria-labelledby="settings-heading"><div className="modal-heading"><h2 id="settings-heading">客户端设置</h2><button className="icon-button" aria-label="关闭设置" onClick={() => setShowSettings(false)}><X size={20} /></button></div><div className="setting-item"><div><strong>客户端版本</strong><span>当前版本 v{version}</span></div><button className="text-button" onClick={checkUpdates} disabled={busy}>检查更新</button></div><div className="setting-item"><div><strong>本机连接</strong><span>代理仅在本机运行，退出客户端后停止</span></div></div><div className="setting-item"><div><strong>API Key 保护</strong><span>使用 Windows DPAPI 加密保存在本机</span></div></div></section></div>}
    {notice && <div className="toast" role="alert">{notice}<button type="button" aria-label="关闭提示" onClick={() => setNotice("")}><X size={15} /></button></div>}
  </div>;
}
