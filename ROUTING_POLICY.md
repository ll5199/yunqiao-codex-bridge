# 智能路由远程配置

正式配置文件位于服务器：

`/www/wwwroot/index.velyn65.com/download/routing-policy.json`

客户端通过以下地址读取：

`https://index.velyn65.com/download/routing-policy.json`

逐项中文注释模板位于同一目录：

`routing-policy-cn-commented.jsonc`

注释模板只供阅读和修改参考，客户端不会读取。标准 JSON 不允许注释，因此不能把
`.jsonc` 文件直接改名覆盖正式配置；应根据注释修改 `routing-policy.json` 中对应值。

Bridge 启动后读取一次，此后按 `refresh_seconds` 定期更新。下载成功且校验通过后，
会缓存到 `%LOCALAPPDATA%\YunqiaoCodexBridge\routing-policy.json`。远程文件不可用或
内容无效时，继续使用上一次有效缓存；缓存也不可用时使用程序内置策略。

## 常用调整

- `default`：没有命中明确规则时的默认执行模型和推理强度。
- `rules`：关键词、文件数和文本长度规则。`priority` 越大越优先。
- `classifier`：模糊任务的判断模型。当前使用 Luna `low`，只发送最多 4000 个字符的
  任务说明和附件数量，不发送文件正文。
- `classifier.low_confidence`：分类信心不足时使用的模型。
- `ambiguous_distribution.weights`：多个选择都合理时的业务偏好比例。默认关闭；开启后
  仅作用于模糊任务，并根据会话标识稳定分桶，不会在任务执行途中随机切换。

允许的执行模型只有 `grok-4.6`、`gpt-5.6-terra`、`gpt-5.6-luna` 和
`gpt-5.6-sol`。Astra 不允许通过远程策略自动调用。推理强度允许 `low`、`medium`、
`high`；只有 Grok 允许 `xhigh`。

每次修改都要同步递增 `policy_version`。配置写错时客户端会拒绝更新，因此不会影响
上一次可用策略。修改后最多等待 `refresh_seconds` 即可生效；重启 Bridge 可更快拉取。

## 修改步骤

1. 打开 `routing-policy-cn-commented.jsonc` 查找需要调整的字段和中文说明。
2. 在正式 `routing-policy.json` 中修改对应值，不要加入 `//` 注释。
3. 将 `policy_version` 改成新值并保存。
4. 使用 JSON 校验工具确认没有漏逗号、中文引号或多余字段。
5. 等待自动刷新，或重启 Bridge 后查看 `bridge.log` 中的 `router.policy_updated`。
