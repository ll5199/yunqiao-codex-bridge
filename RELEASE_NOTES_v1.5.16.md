# 云桥（熙楠）v1.5.16

修复中转模式下启动器覆盖浏览器认证兼容配置的问题。

- 检测到文件中的 ChatGPT 登录时启用本机桥接的 OpenAI 认证；保留用户已设置的 requires_openai_auth = true，支持系统凭据库登录。
- 遵循已有的绝对路径 CODEX_HOME，配置和客户端使用相同目录；不修改 auth.json。
- 向 Sub2API 转发模型请求时只使用中转 Key，并删除 ChatGPT 账号、Cookie 等认证信息。
- 防止读取配置失败时覆盖现有配置；认证只允许用于启动器本机桥接地址。

验证：全部 Go 测试、模拟上游模型回答与认证隔离、配置重复生成及认证文件保持原样、Windows x64 构建通过。

实机验证状态：真实 Sub2API 回答、Chrome/Edge 扩展列出标签页、重启后的浏览器与模型联合验收尚未完成，不将自动测试等同于实机验收。

系统凭据库用户：如果没有 auth.json，请在有效 CODEX_HOME/config.toml 的 [model_providers.yunqiao_bridge] 下设置 requires_openai_auth = true；启动器后续启动会保留该值。
