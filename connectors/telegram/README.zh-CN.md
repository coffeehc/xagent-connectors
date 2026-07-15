# Telegram Connector

[English](README.md)

Telegram Connector 用于将 xAgent 接入 Telegram Bot API IM 场景。

连接器使用动态表单认证流程。用户提交自己的 `bot_token` 和目标 `chat_id`；connector 在本地保存这组绑定，以 bot 为单位维护一个 `getUpdates` 长轮询 worker，并按 `bot_id + chat_id` 将入站消息路由到对应 connector channel。

## 构建

```bash
go test ./connectors/telegram/...
cd connectors/telegram
go build -trimpath -o dist/xagent-telegram-connector .
```

## 本地运行

```bash
./connectors/telegram/dist/xagent-telegram-connector --addr 127.0.0.1:19091 --api-key test-api
```

绑定私聊前，Telegram 用户必须先 start 或主动给 bot 发消息，否则 Bot API 无法访问该 chat。

## 认证流程

- Connector Card ID：`im.telegram`
- Auth flow ID：`telegram_bot_binding`
- 必填表单字段：`bot_token`、`chat_id`
- 文本发送工具：`telegram_message_send`

bot token 不会写入 Connector Card、Skill 或 xAgent 工具参数，只保存在 connector 本地状态目录中。

## Release Tag

当前连接器发布使用仓库级 tag：

```text
v0.0.4
```

同一个 Release 同时包含 Telegram、微信和飞书附件。Telegram 附件使用
`xagent-telegram-connector` 前缀区分。

## 发布附件

常见附件示例：

```text
xagent-telegram-connector-v0.0.4-linux-amd64.tar.gz
xagent-telegram-connector-v0.0.4-linux-arm64.tar.gz
xagent-telegram-connector-v0.0.4-darwin-amd64.tar.gz
xagent-telegram-connector-v0.0.4-darwin-arm64.tar.gz
SHA256SUMS
```

## 发布索引

连接器发布索引见 [`releases.json`](releases.json)。
