# Telegram Connector

[简体中文](README.zh-CN.md)

Telegram Connector connects xAgent with Telegram Bot API IM scenarios.

The connector uses a dynamic form auth flow. A user submits their own
`bot_token` and target `chat_id`; the connector stores that pair locally,
keeps one `getUpdates` long-poll worker per bot, and routes inbound messages by
`bot_id + chat_id` to the bound connector channel.

## Build

```bash
go test ./connectors/telegram/...
cd connectors/telegram
go build -trimpath -o dist/xagent-telegram-connector .
```

## Run Locally

```bash
./connectors/telegram/dist/xagent-telegram-connector --addr 127.0.0.1:19091 --api-key test-api
```

Before binding a private chat, the Telegram user must start or message the bot
so the Bot API can access that chat.

## Auth Flow

- Connector Card ID: `im.telegram`
- Auth flow ID: `telegram_bot_binding`
- Required form fields: `bot_token`, `chat_id`
- Text send tool: `telegram_message_send`

The bot token is never placed in the Connector Card, Skill, or xAgent tool
arguments. It stays in the connector's local state directory.

## Release Tag

The current connector release uses the repository-level tag:

```text
v0.0.4
```

The same release includes Telegram, WeChat, and Feishu assets. Telegram assets use the
`xagent-telegram-connector` prefix.

## Assets

Typical release assets:

```text
xagent-telegram-connector-v0.0.4-linux-amd64.tar.gz
xagent-telegram-connector-v0.0.4-linux-arm64.tar.gz
xagent-telegram-connector-v0.0.4-darwin-amd64.tar.gz
xagent-telegram-connector-v0.0.4-darwin-arm64.tar.gz
SHA256SUMS
```

## Manifest

See [`releases.json`](releases.json) for the connector release index.
