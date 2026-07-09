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

## 构建和测试

```bash
go test ./connectors/wechat/...
make -C connectors/wechat build
```

本地运行：

```bash
./connectors/wechat/dist/xagent-wechat-connector --addr 127.0.0.1:19090 --api-key test-api
```

## Release Tag

当前微信连接器二进制发布使用：

```text
wechat-v0.0.1
```

WeChat Connector 使用按连接器区分的 tag，便于它和 xAgent、其他连接器独立发布。

## 发布附件

常见附件示例：

```text
xagent-wechat-connector-wechat-v0.0.1-linux-amd64.tar.gz
xagent-wechat-connector-wechat-v0.0.1-linux-arm64.tar.gz
xagent-wechat-connector-wechat-v0.0.1-darwin-amd64.tar.gz
xagent-wechat-connector-wechat-v0.0.1-darwin-arm64.tar.gz
SHA256SUMS
```

## 发布索引

连接器发布索引见 [`releases.json`](releases.json)。
