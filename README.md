# 云桥 Codex Bridge

这是一个独立的 Windows 图形程序，用来把 OpenAI 兼容中转站接入官方 Codex
Windows 客户端。

它不是 Codex++ 启动器，不包含、启动或依赖任何 Codex++ 可执行文件。程序只提供：

1. 填写 API 接口。
2. 填写 API Key。
3. 从 `GET /models` 获取全部模型。
4. 保存配置、启动官方 Codex，并把模型注入官方模型选择器。
5. 将官方 Codex 界面语言设置为简体中文。
6. 捕获并显示中转接口返回的生成图片，支持下载原图和按对话恢复。
7. 自动兼容 Gemini、Grok 常见的 Chat Completions / Images 接口。

## 使用

1. 安装并至少直接启动一次官方 Codex Windows 客户端。
2. 完全退出正在运行的官方 Codex。
3. 双击 `YunqiaoCodexBridge.exe`。
4. 填写 API 接口和 API Key。
5. 点击“获取模型”，确认列表正确。
6. 点击“保存并启动 Codex”。
7. 在官方 Codex 自己的模型菜单中选择中转站模型。

主界面的“检查云桥更新”按钮会读取云桥服务器上的更新清单。检测到新版后，
Bridge 会显示下载进度，验证 SHA-256，并启动内置更新助手。更新助手会等待
旧进程完全退出，备份当前版本、原子替换 EXE，然后自动启动新版。更新结果会
写入本机更新日志；API 配置、诊断日志和已保存图片不会被删除。

启动成功后请将 Bridge 保持运行或最小化。Codex 的请求会经过 Bridge 的本机代理，
关闭 Bridge 会中断当前 Codex 对话。以后需要使用中转模型时，应通过本程序的
“保存并启动 Codex”启动。

Bridge 启动 Codex 时固定使用 `%USERPROFILE%` 作为中性的进程工作目录，不会把
Bridge EXE 所在目录（例如桌面的 `yunqiao` 文件夹）创建或选为默认项目。已经存在的
旧对话不会被删除；新对话不再继承 Bridge 所在文件夹。

## 工作原理

- 使用官方支持的 `~/.codex/config.toml` 自定义 `model_provider`，并将其指向仅监听
  `127.0.0.1` 的 Bridge 本机代理。
- Bridge 给上游请求加入 API Key，并透明转发到用户填写的中转地址。
- 使用本机 Chromium DevTools Protocol 启动官方 Codex。
- 在官方客户端页面内补全 `model/list`、app-server、Statsig 和 React 模型状态。
- 模型名称全部来自中转站 `/models` 返回值，不在程序里写死。
- 启动参数和官方 `localeOverride` 都设置为 `zh-CN`；最长持续重试
  60 秒并监听延迟加载的多语言开关，因此英文版新装 Windows 也不会回退英文。
  顶部 File/Edit/View/Help 等 Electron 原生菜单通过本地主进程调试接口汉化。
- 识别 Responses API 的 `image_generation_call.result`、`b64_json`、
  `image_url`、Data URL 和常见图片 URL。Bridge 从真实 API 响应捕获图片，再通过
  页面轮询和 DevTools 主动推送两条通道显示在最新助手消息下方。
- 图片优先挂载到当前对话的助手消息或 conversation turn；若当前 Codex 版本没有
  这些页面标记，则安全挂载到输入框上方。任何情况下都不会回退覆盖整个主页面。
  图片卡片支持“下载原图”“折叠图片”和“继续对话”，预览最大高度为 480 像素。
- Base64 原图会保存到 `%LOCALAPPDATA%\YunqiaoCodexBridge\images`，并记录所属对话。
  关闭后重新打开同一个 Codex 对话，Bridge 会恢复该对话已经生成的图片。
- 页面结构变化时，图片会依次尝试挂载到助手消息、输入框上方和右下角固定图片面板；
  最近两小时内保存但未成功关联的最新图片会在当前对话自动恢复。
- 对 Responses SSE 中 Codex 不支持的 `image_generation_call` 输出进行兼容转换：
  图片数据由 Bridge 捕获显示，Codex 收到标准助手消息和完整结束事件，避免生图成功后
  出现 `Error submitting message`。
- Gemini、Grok 文本模型会把 Codex 的 `/responses` 请求自动转换为
  `/chat/completions`；Gemini 图片模型使用原生 `v1beta ...:generateContent`；
  `grok-imagine-image` 使用 `/images/generations`，`grok-imagine-video` 使用
  `/videos/generations`。GPT/OpenAI 模型仍直接使用 `/responses`。
- 同一 Bridge 内的 Gemini 请求会自动排队；遇到短暂的并发槽 429 会等待 3 秒重试
  一次，避免文字请求与图片请求互相占用账号并发。
- Gemini/Grok 上游错误会转换成当前对话内可读的中文说明，同时保留详细信息到
  `bridge.log`，不再只显示 `Error submitting message`。
- 图片推送成功或挂载脚本失败会分别记录为 `image.delivered` 和
  `image.delivery_error`，便于区分生成失败与显示失败。

程序不会修改官方 Codex 安装文件。

## 密钥与备份

- 本程序自己的配置位于：
  `%LOCALAPPDATA%\YunqiaoCodexBridge\config.json`
- 其中保存的 Key 使用当前 Windows 用户的 DPAPI 加密。
- Key 不写入 `config.toml`，也不再写入 Codex 的 `auth.json`。只有正在运行的 Bridge
  会解密 Key 并把它加入上游请求。
- 从 1.0.1 或 1.0.2 升级时，旧版可能已在 `auth.json` 中留下
  `OPENAI_API_KEY`；1.1.0 不再使用该字段，也不会自动删除用户现有认证文件。
- 修改前的 Codex 配置会备份到：
  `%LOCALAPPDATA%\YunqiaoCodexBridge\backups`
- 不含 API Key 的诊断日志位于：
  `%LOCALAPPDATA%\YunqiaoCodexBridge\bridge.log`
- 生成图片和对话关联索引位于：
  `%LOCALAPPDATA%\YunqiaoCodexBridge\images`

请不要把程序配置文件发给别人。排错时可以发送 `bridge.log`。

## 注意

- 支持 Windows 10/11 x64。
- 本机端口 `9229` 用于 Codex 页面桥接，`9329` 用于原生菜单汉化，`9230` 用于
  API 与图片代理；若端口被其他程序占用，Bridge 会显示错误并写入诊断日志。
- Gemini/Grok 的普通文字对话已提供协议兼容；不同中转站对工具调用、模型命名和生图
  参数的实现可能不同，遇到失败时请发送不含 Key 的 `bridge.log`。
- 若中转站只返回无法公开访问的临时图片 ID，而没有 Base64 或可访问 URL，客户端
  仍无法直接显示；需要中转接口返回标准图片数据。
- 本版本未进行 Microsoft 代码签名，Windows SmartScreen 可能显示未知发布者。

## 源码构建

项目只使用 Go 标准库，无第三方运行依赖：

```powershell
go test ./...
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"
go build -buildvcs=false -trimpath -ldflags "-s -w -H=windowsgui" -o YunqiaoCodexBridge.exe .
```

模型注入方式参考 CodexPlusPlus 的公开实现，项目按 AGPL-3.0-only 发布，详情见
`NOTICE.md` 和 `LICENSE`。
