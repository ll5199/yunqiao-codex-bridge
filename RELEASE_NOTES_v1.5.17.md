# 云桥（熙楠）v1.5.17

修复 v1.5.16 在 ChatGPT 登录令牌保存在 Windows 系统凭据库时仍写回 `requires_openai_auth = false` 的问题。

- 启动器增加默认勾选的“保留 ChatGPT 登录（浏览器扩展）”，已有用户更新后点击“保存并启动 Codex”即可将本机桥接供应商配置改为 `true`。
- 选项保存在启动器 `config.json` 的 `preserve_chatgpt_auth` 字段，重启后保持；只有中转 Key、没有原生登录的用户可关闭。
- Codex 的 `auth.json` 保持原样，遵循 `CODEX_HOME`；发送至 Sub2API 的模型请求仅使用用户的中转 Key，清除 ChatGPT 认证相关请求头。

已通过配置重写/重启模拟、代理隔离及模拟模型回答测试，完成 Windows GUI 构建。真实 Chrome/Edge 列出标签页仍需用户的已登录 Windows 客户端验收。
