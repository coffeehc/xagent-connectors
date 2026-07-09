---
name: im-connector-reply
description: Use when handling IM connector inbound messages from WeChat and replies must be sent back through connector tools.
---

# IM Connector Reply

## Purpose

Handle WeChat IM connector events by responding through the connector-provided tools instead of only writing a session reply.

## Event Contract

Inbound IM events use message.push payloads with provider = wechat, profile = xagent.im.v1, and event_kind = im.message.received.

Important payload fields:

- payload.text is the received text message when present.
- payload.reply.tool_id identifies the connector reply tool.
- payload.media[].media_ref identifies inbound media already tracked by the connector.
- payload.media[].download_url can be used to fetch media only when an available runtime tool can read that URL.

## Reply Workflow

When answering an inbound WeChat IM message:

1. Produce the response text for the user.
2. Call wechat_message_send with text set to the response text.
3. Do not stop after writing the response in the xAgent session; the WeChat user will not receive it unless the connector tool is called.

## Proactive Messages

Use wechat_message_send with text only. The connector resolves the WeChat recipient from the current channel binding.

This connector does not provide contact search. Do not invent recipient references or call unavailable contact tools.

## Media Messages

To send an image, video, or file:

1. Upload the file to the connector media upload endpoint for the current channel to obtain media_ref.
2. Call wechat_message_send_media with that media_ref.
3. Do not put file bytes, base64 payloads, CDN tokens, or target-system credentials into tool arguments.

Inbound media should stay as connector media references. Download media only when the task actually requires inspecting the content.
