# WeChat Connector

[简体中文](README.zh-CN.md)

WeChat Connector is the official reference implementation used to connect
xAgent with WeChat IM scenarios. It owns the WeChat iLink protocol, QR login
state, inbound message queue, media cache, and connector-side tool execution.

## Source Layout

```text
connectors/wechat/
  main.go
  internal/services/
  releases.json
```

The shared xAgent connector wire models are imported from
[`../protocol`](../protocol).

This directory is a separate Go module. The repository root `go.work` lets
local development test both modules from the repository root.

## Build And Test

```bash
go test ./connectors/protocol
go test ./connectors/wechat/...
cd connectors/wechat
go build -trimpath -o dist/xagent-wechat-connector .
```

Run locally:

```bash
./connectors/wechat/dist/xagent-wechat-connector --addr 127.0.0.1:19090 --api-key test-api
```

## Release Tag

The current connector release uses the repository-level tag:

```text
v0.0.4
```

The same release includes Telegram, WeChat, and Feishu assets. WeChat assets use the
`xagent-wechat-connector` prefix.

## Assets

Typical release assets:

```text
xagent-wechat-connector-v0.0.4-linux-amd64.tar.gz
xagent-wechat-connector-v0.0.4-linux-arm64.tar.gz
xagent-wechat-connector-v0.0.4-darwin-amd64.tar.gz
xagent-wechat-connector-v0.0.4-darwin-arm64.tar.gz
SHA256SUMS
```

## Manifest

See [`releases.json`](releases.json) for the connector release index.
