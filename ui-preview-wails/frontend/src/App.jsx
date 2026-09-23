import { useMemo, useState } from "react";
import { ChevronDown, ChevronUp, Play } from "lucide-react";

const connectionNames = {
  api: "外部 API",
  bridge: "云桥账号",
  official: "官方账号",
};

const models = [
  { id: "auto", label: "智能路由", detail: "Auto" },
  { id: "gpt5", label: "GPT-5", detail: "通用任务" },
  { id: "codex", label: "GPT-5 Codex", detail: "代码任务" },
  { id: "o3", label: "o3", detail: "推理任务" },
];

export function App() {
  const [advanced, setAdvanced] = useState(false);
  const [connection, setConnection] = useState("api");
  const [model, setModel] = useState("auto");
  const [saved, setSaved] = useState(true);
  const [notice, setNotice] = useState("");
  const [showKey, setShowKey] = useState(false);
  const [apiUrl, setApiUrl] = useState("https://api.openai.com/v1");
  const [apiKey, setApiKey] = useState("");
  const status = useMemo(() => saved ? "连接正常" : "有未保存的更改", [saved]);

  function chooseConnection(value) {
    setConnection(value);
    setSaved(false);
    setNotice("");
  }

  function saveConnection() {
    setSaved(true);
    setNotice("配置已保存 · 模型列表已同步");
  }

  function launch() {
    setNotice("样板预览：启动操作已触发（不会启动本机 Codex）");
  }

  return (
    <div className="desktop-shell">
      <header className="topbar">
        <div className="brand-lockup" aria-label="云桥 熙楠">
          <div className="brand-mark">云</div>
          <div className="brand-copy"><strong>云桥</strong><span>熙楠</span></div>
        </div>
        <div className="top-actions">
          <div className="connection-pill"><i className={saved ? "status-dot" : "status-dot pending"} /><span>{connectionNames[connection]}</span><b>·</b><span>{status}</span></div>
          <button className="quiet-button" onClick={() => { setAdvanced(true); setNotice("设置已展开"); }}>设置</button>
        </div>
      </header>

      <main className="workspace">
        <section className="launch-panel" aria-labelledby="welcome-title">
          <h1 id="welcome-title">准备启动</h1>
          <p className="intro">选择模型，然后打开 Codex</p>

          <div className="model-field">
            <label>启动模型</label>
            <div className="model-options" role="group" aria-label="选择启动模型">
              {models.map((item) => <button key={item.id} type="button" className={`model-option ${model === item.id ? "selected" : ""}`} aria-pressed={model === item.id} onClick={() => setModel(item.id)}>
                <span className="model-option-label">{item.label}</span>
                <span className="model-option-detail">{item.detail}</span>
              </button>)}
            </div>
            <div className="field-help">智能路由会根据任务自动选择合适模型</div>
          </div>

          <button className="launch-button" onClick={launch}><Play size={18} fill="currentColor" strokeWidth={1.5} aria-hidden="true" /><span>启动 Codex</span></button>
          <div className="launch-footnote"><span className="secure-dot" />使用当前连接启动 · 设置可随时调整</div>
        </section>

        <section className={`advanced-card ${advanced ? "is-open" : ""}`}>
          <button className="advanced-toggle" aria-expanded={advanced} onClick={() => setAdvanced(!advanced)}>
            <span className="toggle-copy"><strong>连接与高级设置</strong><small>管理连接方式、模型与本机选项</small></span>
            <span className="toggle-state">{advanced ? "收起" : "展开"}{advanced ? <ChevronUp size={16} strokeWidth={1.8} aria-hidden="true" /> : <ChevronDown size={16} strokeWidth={1.8} aria-hidden="true" />}</span>
          </button>
          {advanced && <div className="advanced-content">
            <div className="section-heading"><div><h2>连接方式</h2><p>选择 Codex 使用的账户或服务</p></div><span className="active-label">当前：{connectionNames[connection]}</span></div>
            <div className="connection-options" role="group" aria-label="连接方式">
              {Object.entries(connectionNames).map(([key, label]) => <button key={key} className={`connection-option ${connection === key ? "selected" : ""}`} onClick={() => chooseConnection(key)} aria-pressed={connection === key}><span className="radio" /><span>{label}</span></button>)}
            </div>
            {connection === "api" && <div className="config-fields">
              <div className="field-row"><label htmlFor="api-url">接口地址</label><input id="api-url" name="api-url" type="url" autoComplete="off" value={apiUrl} onChange={(e) => { setApiUrl(e.target.value); setSaved(false); }} placeholder="https://api.openai.com/v1…" /></div>
              <div className="field-row"><label htmlFor="api-key">API Key</label><div className="key-input"><input id="api-key" name="api-key" type={showKey ? "text" : "password"} autoComplete="off" spellCheck="false" value={apiKey} onChange={(e) => { setApiKey(e.target.value); setSaved(false); }} placeholder="输入 API Key…" /><button type="button" onClick={() => setShowKey(!showKey)}>{showKey ? "隐藏" : "显示"}</button></div></div>
              <p className="privacy-note"><span className="privacy-mark">本机</span> API Key 使用 Windows DPAPI 加密保存在本机，不会上传至云桥。此预览不会保存输入内容。</p>
            </div>}
            {connection === "bridge" && <div className="account-row"><div><strong>云桥账号</strong><p>登录后同步订阅与模型配置</p></div><button className="outline-button" onClick={() => setNotice("样板预览：账号登录暂未连接")}>登录云桥</button></div>}
            {connection === "official" && <div className="account-row"><div><strong>使用官方账号</strong><p>通过 Codex 官方账户进行连接</p></div><span className="plain-status">启动时登录</span></div>}

            <div className="advanced-bottom">
              <div className="local-setting"><strong>本机连接</strong><span>按系统代理设置连接网络</span></div>
              <div className="advanced-actions"><button className="text-button" onClick={() => setNotice("样板预览：模型同步已模拟完成")}>同步模型</button><button className="outline-button" onClick={saveConnection}>保存并同步</button></div>
            </div>
            <div className="update-row"><div><strong>客户端更新</strong><span>当前版本 v1.6.0</span></div><button className="text-button" onClick={() => setNotice("当前已是最新版本（样板状态）")}>检查更新</button></div>
          </div>}
        </section>
      </main>

      <div className="version">版本 1.6.0 · 样板预览</div>
      <div className="demo-note">仅供界面预览 · 不会连接网络或启动本机程序</div>
      {notice && <div className="toast" role="status" aria-live="polite">{notice}</div>}
    </div>
  );
}
