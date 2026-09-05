# 云桥（熙楠）v1.4.3

- Grok 文本与推理模型改为原样透传 Codex 原生 `/v1/responses` 请求，不再降级为 Chat Completions。
- 保留 Codex 的 `custom`、`namespace` 和 `tool_search` 工具定义及工具调用事件，修复生图、文件检索等操作只返回文字说明的问题。
- 保持 `grok-imagine-image` 与 `grok-imagine-video` 的现有图片、视频兼容接口不变。
- 新增 Grok Responses 请求与工具事件的完整代理链路回归测试。
