# Feishu Connector

[简体中文](README.zh-CN.md)

Feishu Connector connects xAgent to the domestic Feishu IM platform. Users scan
a QR code to create a preconfigured `xAgent助手` application. The connector owns
the app credentials, event subscription, long connection, message references,
and image cache.

## Build And Test

```bash
go test ./connectors/feishu/...
cd connectors/feishu
go build -trimpath -o dist/xagent-feishu-connector .
```

## Current Scope

- Domestic Feishu only; Lark is not supported.
- Receive direct messages and group messages that mention the bot.
- Send or reply in the default direct conversation without a `reply_ref`.
- Reply to group and topic-group mentions through an opaque `reply_ref`.
- Upload and download images through connector-managed `media_ref` values.
- No selection of other contacts or groups, documents, calendars, cards, or streaming replies.

## Image Permission

To send images from Feishu to xAgent, open the corresponding `xAgent助手`
application in the [Feishu Open Platform](https://open.feishu.cn/app) and enable
the "Obtain and upload image or file resources" permission (`im:resource`). The
connector cannot download user-sent images without this permission.

## Release Tag

The current connector release uses the repository-level tag:

```text
v0.0.4
```

The same release includes Telegram, WeChat, and Feishu assets. Feishu assets use
the `xagent-feishu-connector` prefix.

## Assets

```text
xagent-feishu-connector-v0.0.4-linux-amd64.tar.gz
xagent-feishu-connector-v0.0.4-linux-arm64.tar.gz
xagent-feishu-connector-v0.0.4-darwin-amd64.tar.gz
xagent-feishu-connector-v0.0.4-darwin-arm64.tar.gz
SHA256SUMS
```

## Manifest

See [`releases.json`](releases.json) for the connector release index.
