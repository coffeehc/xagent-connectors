---
name: im-connector-reply
description: Use when handling IM connector inbound messages from WeChat and replies must be sent back through connector tools.
---

# IM Connector Reply

## Purpose

Handle WeChat IM connector events by responding through the connector-provided tools instead of only writing a session reply.

This skill owns the WeChat attachment sending workflow. Load it before sending images, videos, generated artifacts, workspace files, or other attachments through the WeChat connector.

## Event Contract

Inbound IM events use message.push payloads with provider = wechat, profile = xagent.im.v1, and event_kind = im.message.received.

Important payload fields:

- payload.text is the received text message when present.
- payload.reply.tool_id identifies the connector reply tool.
- payload.media[].media_ref identifies inbound media already tracked by the connector.
- payload.media[].download_url can be used to fetch media only when an available runtime tool can read that URL.

## Reply Workflow

When answering an inbound WeChat IM message:

1. Decide whether the WeChat reply is text-only or includes media/file artifacts.
2. For text-only replies, call wechat_message_send with text set to the response text.
3. For replies that include an image, video, file, generated artifact, workspace file, or other attachment, do not call wechat_message_send; follow the Media Messages workflow and call wechat_message_send_media.
4. If both text and media should be sent, send text with wechat_message_send and media with wechat_message_send_media.
5. Do not stop after writing the response in the xAgent session; the WeChat user will not receive it unless the connector tool is called.

## Proactive Messages

Use wechat_message_send for text-only messages. Use wechat_message_send_media for images, videos, and files after obtaining media_ref. The connector resolves the WeChat recipient from the current channel binding.

This connector does not provide contact search. Do not invent recipient references or call unavailable contact tools.

## Media Messages

To send an image, video, or file:

1. Ensure there is a real local file or artifact to send. Do not claim a media message was sent if there is no file.
2. Upload the file to Connector HTTP endpoint POST /media/uploads for the current connector channel, using multipart field file and X-XAgent-Connector-Channel-ID set to the current channel id, to obtain media_ref.
3. Call wechat_message_send_media with media_ref and optional text when a separate text message should be sent.
4. Do not put local paths, file bytes, base64 payloads, CDN tokens, target-system credentials, recipient refs, or download_url into tool arguments.
5. Never call wechat_message_send to represent an attachment, even if the text contains a local path, URL, markdown image, or file name.
6. If the runtime cannot upload the file to POST /media/uploads, explain that the media cannot be sent instead of calling wechat_message_send with a fake link.

Inbound media should stay as connector media references. Download media only when the task actually requires inspecting the content.

## Tool Selection Rules

- Use wechat_message_send only for plain text.
- Use wechat_message_send_media for image, video, and file delivery.
- If the user's requested reply contains any attachment, wechat_message_send is the wrong tool.
- media_ref is the only required argument for wechat_message_send_media; media type and WeChat recipient are resolved by the connector.
- For inbound media forwarding, reuse payload.media[].media_ref when the same file should be sent back or forwarded in the same channel.
