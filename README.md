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
8. 提供“智能路由（Auto）”，按任务内容自动选择 Grok、Terra、Luna 或 Sol；Astra
   始终只允许手动选择。

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

如果需要使用 ChatGPT 官方账号，可点击“原生账号启动”。程序会先备份并移除云桥写入的
自定义供应商配置，保留官方登录文件和其他 Codex 设置，再直接启动官方客户端；未登录时
按官方页面提示完成 ChatGPT 账号登录即可。

Bridge 启动 Codex 时固定使用 `%USERPROFILE%` 作为中性的进程工作目录，不会把
Bridge EXE 所在目录（例如桌面的 `yunqiao` 文件夹）创建或选为默认项目。已经存在的
旧对话不会被删除；新对话不再继承 Bridge 所在文件夹。

## 工作原理

- 使用官方支持的 `~/.codex/config.toml` 自定义 `model_provider`，并将其指向仅监听
  `127.0.0.1` 的 Bridge 本机代理。
- Bridge 给上游请求加入 API Key，并透明转发到用户填写的中转地址。
- 使用本机 Chromium DevTools Protocol 启动官方 Codex。
- 在官方客户端页面内补全 `model/list`、app-server、Statsig 和 React 模型状态。
- 实际模型名称来自中转站 `/models` 返回值；当 Grok 4.6、Terra、Luna 和 Sol 全部
  可用时，Bridge 额外加入本地虚拟模型 `yunqiao-auto`，并将其显示为“智能路由（Auto）”。
- Auto 的规则来自云桥服务器 `routing-policy.json`，默认每 5 分钟自动刷新，并保留本机
  最近一次有效缓存。以后调整模型分工、推理强度、匹配关键词和模糊任务业务比例无需
  更新客户端；远程文件不可用或校验失败时会自动使用缓存或内置策略。
- Auto 针对工程标书工作优化：文件查找、复制粘贴、批量替换、格式调整和普通修改
  默认交给 Grok 4.6；表格、清单、字段提取和结构化转换交给 Luna；整个文件夹、
  多文档综合和超长上下文交给 Terra；施工组织设计、技术方案、评分点响应和疑难重写
  交给 Sol。最终合规、废标项、报价、签章等审核由人工完成，不因出现合同或合规词语
  自动调用 Sol。Astra 不参与自动分流，只保留手动选择。
- 明确任务先由规则快速分配；模糊或复合任务由 Luna `low` 只读取短任务说明和附件数量
  进行判断，低信心时交给 Terra。分类失败会退回规则，不会阻断请求。各执行模型的推理
  强度由远程策略配置；手动选择模型时仍保留用户自己的设置。
- 同一 Codex 会话使用 `prompt_cache_key` 记住已选模型，工具调用的后续请求不会在
  任务中途切换模型。路由日志只记录模型、原因、字符数和附件数，不记录文档正文。
- 启动参数和官方 `localeOverride` 都设置为 `zh-CN`；最长持续重试
  120 秒并监听延迟加载的多语言开关。若官方语言设置或 Statsig 服务未及时生效，
  Bridge 会对应用导航、按钮、设置项等界面控件启用严格的精确匹配中文兜底；对话正文、
  提示词、代码、编辑器、终端和模型输出不会被改写。
- “保存并启动 Codex”和“原生账号启动”都会启用中文界面；原生账号模式仍不经过云桥
  API 代理。首次写入官方中文设置时，Bridge 会自动完整重启一次 Codex，让 Electron
  主进程重新加载语言目录；随后读取页面端汉化状态再显示启动成功，不再把脚本无异常
  误报成汉化成功。顶部 File/Edit/View/Help 等 Electron 原生菜单通过本地主进程调试接口汉化。
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
- Grok 文本/推理模型原样透传 Codex 的 `/responses` 请求，保留 `custom`、
  `namespace` 和 `tool_search` 等工具定义；Gemini 文本模型仍自动转换为
  `/chat/completions`，Gemini 图片模型使用原生 `v1beta ...:generateContent`；
  `grok-imagine-image` 使用 `/images/generations`，`grok-imagine-video` 使用
  `/videos/generations`。GPT/OpenAI 模型仍直接使用 `/responses`。
- 同一 Bridge 内的 Gemini 请求会自动排队；遇到短暂的并发槽 429 会等待 3 秒重试
  一次，避免文字请求与图片请求互相占用账号并发。
- Grok 请求遇到账号并发 502 时会等待 3 秒自动重试一次；若仍失败，
  会在当前对话中显示中文原因，不会只留下无响应状态。
- Codex 执行请求时会显示 Bridge 本地进度，包括“正在分析”“正在等待首次响应”
  “正在返回结果”和“正在执行文件工具”；超过 45 秒仍未收到首段数据时会持续显示等待时长。
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
