import { useEffect, useMemo, useState } from "react";
import { ChevronDown, ChevronUp, Play } from "lucide-react";

const connectionNames = {
  api: "外部 API",
  bridge: "云桥账号",
  official: "官方账号",
};
const autoModel = "yunqiao-auto";

function backend() {
  return window.go?.main?.BridgeApp;
}

export function App() {
  const [advanced, setAdvanced] = useState(false);
  const [connection, setConnection] = useState("api");
  const [models, setModels] = useState([]);
  const [model, setModel] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState("正在读取本机配置…");
  const [notice, setNotice] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [apiUrl, setApiUrl] = useState("https://api.velyn65.com/v1");
  const [apiKey, setApiKey] = useState("");
  const [hasKey, setHasKey] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [cloud, setCloud] = useState({ loggedIn: false, providers: [], usage: [] });
  const [provider, setProvider] = useState("");
  const [version, setVersion] = useState("1.6.0");
  const connectionLabel = useMemo(() => connectionNames[connection] || "外部 API", [connection]);

  useEffect(() => {
    const unsubscribe = window.runtime?.EventsOn?.("bridge:status", (value) => setStatus(value));
    const app = backend();
    if (!app) {
      setStatus("客户端连接未就绪");
      return () => unsubscribe?.();
    }
    app.GetState().then((state) => {
      setApiUrl(state.baseURL || "https://api.velyn65.com/v1");
      setHasKey(Boolean(state.hasAPIKey));
      setModels(state.models || []);
      setModel(state.selectedModel || state.models?.[0] || "");
      setVersion(state.version || "1.6.0");
      setStatus(state.status || "准备就绪");
      setSaved(Boolean(state.hasAPIKey));
      setCloud(state.cloud || { loggedIn: false, providers: [], usage: [] });
      if (state.cloud?.loggedIn) setConnection("bridge");
    }).catch((error) => setStatus(error?.message || "读取本机配置失败"));
    return () => unsubscribe?.();
  }, []);

  function showError(error) {
    const message = error?.message || String(error || "操作失败");
    setNotice(message);
    setStatus(`操作失败：${message}`);
  }


  async function saveConnection() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    setBusy(true);
    setNotice("");
    try {
      const state = await app.SaveConnection(apiUrl, apiKey, model);
      setApiUrl(state.baseURL);
      setModels(state.models || []);
      setModel(state.selectedModel || "");
      setHasKey(Boolean(state.hasAPIKey));
      setApiKey("");
      setSaved(true);
      setStatus("配置已加密保存，模型已同步。API Key 只保存在本机。 ");
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  async function loginCloud() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    setBusy(true);
    setNotice("");
    try {
      const result = await app.LoginCloud(email, password);
      setCloud(result);
      setConnection("bridge");
      setPassword("");
      const first = result.providers?.[0] || "";
      setProvider(first);
      if (first) await selectProvider(first);
      setStatus("云桥账号已登录。");
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  async function selectProvider(value) {
    setProvider(value);
    setModels([]);
    setModel("");
    const app = backend();
    if (!app || !value) return;
    try {
      const result = await app.FetchProviderModels(value);
      setModels(result || []);
      setModel(result?.[0] || "");
    } catch (error) {
      showError(error);
    }
  }

  async function logoutCloud() {
    const app = backend();
    if (!app) return;
    const state = await app.LogoutCloud();
    setCloud(state);
    setProvider("");
    setModels([]);
    setModel("");
    setStatus("已退出云桥账号。");
  }

  async function launch() {
    const app = backend();
    if (!app) return showError(new Error("客户端服务尚未启动"));
    if (!window.confirm("为了应用连接设置，启动前会关闭已运行的 Codex 窗口并重新启动。继续吗？")) return;
    setBusy(true);
    setNotice("");
    setStatus("正在启动 Codex…");
    try {
      const result = await app.Launch(connection, provider, model, apiUrl, apiKey);
      setSaved(connection !== "official");
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
      if (!result.available) {
        setStatus(`云桥客户端 v${result.current} 已是最新版。`);
        return;
      }
      const message = `发现 v${result.latest} 更新（当前 v${result.current}）。\n\n${result.notes || ""}\n\n是否下载并安装？`;
      if (!window.confirm(message)) {
        setStatus("已取消更新。");
        return;
      }
      await app.InstallUpdate();
    } catch (error) {
      showError(error);
    } finally {
      setBusy(false);
    }
  }

  const modelOptions = models.length ? models : [];
  const accountVisible = connection === "bridge";

  return (
    <div className="desktop-shell">
      <header className="topbar">
        <div className="brand-lockup" aria-label="云桥 熙楠">
          <div className="brand-mark">云</div>
          <div className="brand-copy"><strong>云桥</strong><span>熙楠</span></div>
        </div>
        <div className="top-actions">
          <div className="connection-pill"><i className={saved ? "status-dot" : "status-dot pending"} /><span>{connectionLabel}</span><b>·</b><span>{saved ? "已配置" : "待配置"}</span></div>
          <button className="quiet-button" onClick={() => setAdvanced((value) => !value)}>{advanced ? "收起设置" : "设置"}</button>
        </div>
      </header>

      <main className="workspace">
        <section className="launch-panel" aria-labelledby="welcome-title">
          <h1 id="welcome-title">准备启动</h1>
          <p className="intro">选择模型，然后打开 Codex</p>

          {connection !== "official" && <div className="model-field">
            <label>启动模型</label>
            {modelOptions.length ? <div className="model-options" role="group" aria-label="选择启动模型">
              {modelOptions.map((item) => <button key={item} type="button" className={`model-option ${model === item ? "selected" : ""}`} aria-pressed={model === item} onClick={() => setModel(item)}>
                <span className="model-option-label">{item === autoModel ? "智能路由" : item}</span>
                <span className="model-option-detail">{item === autoModel ? "根据任务自动选择" : "手动指定模型"}</span>
              </button>)}
            </div> : <div className="empty-models">请在「连接与高级设置」中同步模型</div>}
            <div className="field-help">智能路由会根据任务自动选择合适模型；也可以直接点选模型</div>
          </div>}

          <button className="launch-button" onClick={launch} disabled={busy}><Play size={18} fill="currentColor" strokeWidth={1.5} aria-hidden="true" /><span>{busy ? "正在处理…" : "启动 Codex"}</span></button>
          <div className="launch-footnote"><span className="secure-dot" />启动前会关闭已运行的 Codex，并按所选连接重新启动</div>
        </section>

        <section className={`advanced-card ${advanced ? "is-open" : ""}`}>
          <button className="advanced-toggle" aria-expanded={advanced} onClick={() => setAdvanced(!advanced)}>
            <span className="toggle-copy"><strong>连接与高级设置</strong><small>连接方式、模型同步和本机选项</small></span>
            <span className="toggle-state">{advanced ? "收起" : "展开"}{advanced ? <ChevronUp size={16} strokeWidth={1.8} aria-hidden="true" /> : <ChevronDown size={16} strokeWidth={1.8} aria-hidden="true" />}</span>
          </button>
          {advanced && <div className="advanced-content">
            <div className="section-heading"><div><h2>连接方式</h2><p>选择 Codex 使用的账户或服务</p></div><span className="active-label">当前：{connectionNames[connection]}</span></div>
            <div className="connection-options" role="group" aria-label="连接方式">
              {Object.entries(connectionNames).map(([key, label]) => <button key={key} className={`connection-option ${connection === key ? "selected" : ""}`} onClick={() => { setConnection(key); setNotice(""); }} aria-pressed={connection === key}><span className="radio" /><span>{label}</span></button>)}
            </div>

            {connection === "api" && <div className="config-fields">
              <div className="field-row"><label htmlFor="api-url">接口地址</label><input id="api-url" name="api-url" type="url" autoComplete="off" value={apiUrl} onChange={(event) => { setApiUrl(event.target.value); setSaved(false); }} placeholder="https://api.velyn65.com/v1" /></div>
              <div className="field-row"><label htmlFor="api-key">API Key</label><div className="key-input"><input id="api-key" name="api-key" type={showKey ? "text" : "password"} autoComplete="off" spellCheck="false" value={apiKey} onChange={(event) => { setApiKey(event.target.value); setSaved(false); }} placeholder={hasKey ? "已加密保存；留空则继续使用现有 Key" : "输入 API Key"} /><button type="button" onClick={() => setShowKey(!showKey)}>{showKey ? "隐藏" : "显示"}</button></div></div>
              <p className="privacy-note"><span className="privacy-mark">本机</span> API Key 使用 Windows DPAPI 加密保存在本机，不会上传至云桥。</p>
            </div>}

            {accountVisible && <div className="account-config">
              {!cloud.loggedIn ? <div className="account-login">
                <div className="field-row"><label htmlFor="cloud-email">云桥账号</label><input id="cloud-email" type="email" autoComplete="username" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="邮箱" /></div>
                <div className="field-row"><label htmlFor="cloud-password">密码</label><input id="cloud-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="密码" /></div>
                <button className="outline-button" onClick={loginCloud} disabled={busy}>登录并读取订阅</button>
              </div> : <>
                <div className="account-row"><div><strong>{cloud.identity || "云桥账号"}</strong><p>{cloud.groups || "订阅已读取"}</p></div><button className="text-button" onClick={logoutCloud}>退出登录</button></div>
                <div className="provider-options" role="group" aria-label="选择上游">
                  {(cloud.providers || []).map((item) => <button key={item} className={`connection-option ${provider === item ? "selected" : ""}`} onClick={() => selectProvider(item)} aria-pressed={provider === item}><span className="radio" /><span>{item}</span></button>)}
                </div>
                <div className="usage-summary">{(cloud.usage || []).map((item) => <span key={item}>{item}</span>)}</div>
              </>}
            </div>}

            {connection === "official" && <div className="account-row"><div><strong>使用官方账号</strong><p>启动后在 Codex 中登录 ChatGPT 账号；本机中转配置会先备份并恢复官方设置。</p></div><span className="plain-status">单一启动按钮</span></div>}

            <div className="advanced-bottom">
              <div className="local-setting"><strong>本机连接</strong><span>由本机代理连接上游；关闭客户端会结束本机代理</span></div>
              <div className="advanced-actions">
                {connection === "api" && <button className="outline-button" onClick={saveConnection} disabled={busy}>保存并同步</button>}
                {connection === "bridge" && cloud.loggedIn && <button className="outline-button" onClick={() => selectProvider(provider)} disabled={busy}>同步所选上游</button>}
              </div>
            </div>
            <div className="update-row"><div><strong>客户端更新</strong><span>当前版本 v{version}</span></div><button className="text-button" onClick={checkUpdates} disabled={busy}>检查更新</button></div>
          </div>}
        </section>
      </main>

      <div className="version">云桥客户端 v{version}</div>
      <div className="demo-note">{status}</div>
      {notice && <div className="toast" role="status" aria-live="polite">{notice}</div>}
    </div>
  );
}
