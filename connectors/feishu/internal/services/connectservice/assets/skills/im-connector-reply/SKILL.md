---
name: im-connector-reply
description: Send or reply to Feishu messages with text or images through connector-managed destinations and references.
---

# 飞书消息发送与回复

飞书消息分为两种模式，不要推断或要求用户提供飞书 `chat_id`、`message_id`、App ID 或 App Secret。

默认智能体单聊使用发送模式。纯文本调用 `feishu_message_send`，参数为 `text`；图片调用 `feishu_message_send_image`，参数为 `media_ref` 和可选 `text`。发送模式不使用 `reply_ref`，也适用于邮件监控等主动通知。

群聊或话题群的 @ 消息使用回复模式。入站消息会提供 `reply_ref`，必须原样传给 `feishu_message_reply` 或 `feishu_message_reply_image`，以回复到原消息上下文。

发送图片前，先通过 connector 的 `POST /media/uploads` 上传图片，携带系统 API key 和 `X-XAgent-Connector-Channel-ID`，取得 `media_ref`。可选文字会作为独立文本消息发送。

飞书传入的图片会出现在消息 `media.download_url` 中。该地址只读取 connector 已缓存的临时文件，不暴露飞书 `image_key`。
