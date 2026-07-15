# WeChat Connector

[English](README.md)

WeChat Connector 是将 xAgent 接入微信 IM 场景的官方参考实现。它负责微信 iLink 协议、扫码登录态、入站消息队列、媒体缓存和 connector 侧工具执行。

## 源码结构

```text
connectors/wechat/
  main.go
  internal/services/
  releases.json
```

共享的 xAgent connector wire model 从 [`../protocol`](../protocol) 导入。

本目录是独立 Go module。仓库根目录的 `go.work` 用于本地开发时从仓库根目录同时测试协议包和 WeChat module。

## 构建和测试

```bash
go test ./connectors/protocol
go test ./connectors/wechat/...
cd connectors/wechat
go build -trimpath -o dist/xagent-wechat-connector .
```

本地运行：

```bash
./connectors/wechat/dist/xagent-wechat-connector --addr 127.0.0.1:19090 --api-key test-api
```

## Release Tag

当前连接器发布使用仓库级 tag：

```text
v0.0.4
```

同一个 Release 同时包含 Telegram、微信和飞书附件。WeChat 附件使用
`xagent-wechat-connector` 前缀区分。

## 发布附件

常见附件示例：

```text
xagent-wechat-connector-v0.0.4-linux-amd64.tar.gz
xagent-wechat-connector-v0.0.4-linux-arm64.tar.gz
xagent-wechat-connector-v0.0.4-darwin-amd64.tar.gz
xagent-wechat-connector-v0.0.4-darwin-arm64.tar.gz
SHA256SUMS
```

## 发布索引

连接器发布索引见 [`releases.json`](releases.json)。
