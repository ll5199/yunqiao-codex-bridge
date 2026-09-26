# v1.5.12

修复 Sub2API 上游 HTTP/2 流的 PROTOCOL_ERROR：云桥出站使用 HTTP/1.1，普通 Responses 请求体可重放。保留流式返回，并通过独立测试验证 TLS 上游确实使用 HTTP/1.1。
