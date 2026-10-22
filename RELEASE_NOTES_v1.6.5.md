# 云桥（熙楠）v1.6.5

修复官方 Codex 应用更新后启动时报 Access is denied 或安装路径失效的问题。

- 动态读取已注册应用的 AppUserModelID，经 Windows 应用入口启动；原生账号、外部 API 和云桥账号共用同一启动方式。
- 删除直接执行 WindowsApps 中 ChatGPT.exe / Codex.exe 的回退，不再依赖版本目录；自定义 CODEX_HOME 也走应用入口。
- Windows 激活后验证主程序进程和官方包身份，确认运行后才锁定启动按钮；失败恢复可点击，退出后自动解锁。
- 调试端口或汉化注入未就绪时，不再退回直接执行 EXE。

验证包含 Windows PowerShell 多入口清单、应用 ID 变化、开始菜单入口回退、进程退出与启动失败测试，以及原生和中转模式按钮交互测试。
