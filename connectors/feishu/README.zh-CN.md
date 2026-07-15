# 飞书 Connector

飞书 Connector 用于将 xAgent 接入国内飞书 IM。用户只需要扫码确认创建预设名称的
`xAgent助手` 应用；App ID、App Secret、事件订阅、长连接、消息路由和图片缓存全部由
connector 管理。

## 构建与测试

```bash
go test ./connectors/feishu/...
cd connectors/feishu
go build -trimpath -o dist/xagent-feishu-connector .
```

## 当前范围

- 只支持国内飞书，暂不支持 Lark。
- 接收单聊消息和群聊中 @ 机器人的消息。
- 默认单聊使用 `feishu_message_send` 主动发送或回复，不需要 `reply_ref`。
- 群聊和话题群 @ 消息通过不透明的 `reply_ref` 回复。
- 图片通过 connector 管理的 `media_ref` 上传或下载。
- 暂不支持主动选择其他联系人或群聊、文档、日历、卡片和流式回复。

## 图片权限

如果需要将飞书中的图片传给 xAgent，需要前往[飞书开放平台](https://open.feishu.cn/app)，进入对应的 `xAgent助手` 应用并开通“获取与上传图片或文件资源”权限（`im:resource`）。未开通该权限时，connector 无法下载用户发送的图片。

## Release Tag

当前连接器发布使用仓库级 tag：

```text
v0.0.4
```

同一个 Release 同时包含 Telegram、微信和飞书附件。飞书附件使用
`xagent-feishu-connector` 前缀区分。

## 发布附件

```text
xagent-feishu-connector-v0.0.4-linux-amd64.tar.gz
xagent-feishu-connector-v0.0.4-linux-arm64.tar.gz
xagent-feishu-connector-v0.0.4-darwin-amd64.tar.gz
xagent-feishu-connector-v0.0.4-darwin-arm64.tar.gz
SHA256SUMS
```

## 发布索引

连接器发布索引见 [`releases.json`](releases.json)。
