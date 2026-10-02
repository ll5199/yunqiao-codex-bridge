# 中转模式与浏览器登录兼容

中转模型请求仍由本机 127.0.0.1:9230 桥接使用启动器保存的中转 Key 访问 Sub2API。

启动器读取有效 CODEX_HOME（未设置时为用户目录 .codex），不写入、删除或复制 auth.json，不更换认证目录。CODEX_HOME 必须是绝对路径。设置 CODEX_HOME 时直接启动可执行文件，使客户端继承相同环境，而不是依赖应用包激活环境。

检测 auth.json 中的 ChatGPT access_token 后，为本机桥接开启 requires_openai_auth。已有的 true 值始终保留，包括保存在系统凭据库的登录。纯中转 Key 用户无登录时仍使用 false。若登录只保存在系统凭据库，应在有效 CODEX_HOME/config.toml 的 [model_providers.yunqiao_bridge] 中设置 requires_openai_auth = true；此值重启后不会被覆盖。此开关只允许用于启动器的本机桥接地址。

代理覆盖 Authorization，并删除 Cookie、ChatGPT-Account-ID、OpenAI-Organization、OpenAI-Project、Proxy-Authorization、X-API-Key、Sec-WebSocket-Protocol，避免把客户端登录信息转发给模型服务。

## 验收状态

自动测试：配置重复写入与重读、认证文件保持原样、CODEX_HOME 一致性、模拟上游返回模型文本、模型服务仅收到中转 Key。Windows 构建检查。

尚须 Windows 实机完成以下一次性验收，完成前不得称为正式修复已验证：

1. 保持原生账号的 ChatGPT 登录，切换中转模式，向真实 Sub2API 模型提问并得到完整回答。
2. 分别使用 Chrome 和 Edge，要求客户端列出已有标签页，确认没有 Codex auth token is unavailable。
3. 退出并重新启动启动器和 Codex，重复模型问答与两个浏览器列出标签页，检查配置 requires_openai_auth 仍为 true。
4. 上游只收到中转 Key；不要输出或上传 auth.json、令牌或完整请求认证头。

没有可访问的 Windows 实机，本次不能替代上述验收。
