---
name: im-connector-reply
description: Use when handling IM connector inbound messages from Telegram and replies must be sent back through connector tools.
---

# IM Connector Reply

## Purpose

Handle Telegram IM connector events by responding through the connector-provided tools instead of only writing a session reply.

This skill owns the Telegram attachment sending workflow. Load it before sending images, videos, generated artifacts, workspace files, or other attachments through the Telegram connector.

## Event Contract

Inbound IM events use message.push payloads with provider = telegram, profile = xagent.im.v1, and event_kind = im.message.received.

Important payload fields:

- payload.text is the visible prompt assembled by the connector.
- payload.raw_text is the original Telegram text message.
- payload.reply.tool_id identifies the connector reply tool.
- payload.media[].media_ref identifies inbound media already tracked by the connector.
- payload.media[].download_url can be used to fetch media only when an available runtime tool can read that URL.
- payload.chat_id is the bound Telegram chat id and should not be exposed unless needed for debugging.

## Reply Workflow

When answering an inbound Telegram IM message:

1. Decide whether the Telegram reply is text-only or includes media/file artifacts.
2. For text-only replies, call telegram_message_send with text set to the response text.
3. For replies that include an image, video, file, generated artifact, workspace file, or other attachment, do not call telegram_message_send; follow the Media And Files workflow and call telegram_message_send_media.
4. If both text and media should be sent, send the media with text as caption when suitable; otherwise send media first, then send follow-up text with telegram_message_send.
5. Do not stop after writing the response in the xAgent session; the Telegram user will not receive it unless the connector tool is called.

## Proactive Messages

Use telegram_message_send for text-only messages. Use telegram_message_send_media for images, videos, and files after obtaining media_ref. The connector resolves the Telegram chat from the current channel binding.

This connector does not provide contact search. Do not ask for or expose bot tokens, and do not invent chat ids.

## Media And Files

To send an image, video, or file:

1. Ensure there is a real local file or artifact to send. Do not claim a media message was sent if there is no file.
2. Upload the file to Connector HTTP endpoint POST /media/uploads for the current connector channel, using multipart field file and X-XAgent-Connector-Channel-ID set to the current channel id, to obtain media_ref.
3. Call telegram_message_send_media with media_ref and optional text caption.
4. Do not put local paths, file bytes, base64 payloads, Telegram file_id, bot tokens, chat_id, or download_url into tool arguments.
5. Never call telegram_message_send to represent an attachment, even if the text contains a local path, URL, markdown image, or file name.
6. If the runtime cannot upload the file to POST /media/uploads, explain that the media cannot be sent instead of calling telegram_message_send with a fake link.

Inbound media should stay as connector media references. Download media only when the task actually requires inspecting the content.

## Tool Selection Rules

- Use telegram_message_send only for plain text.
- Use telegram_message_send_media for image, video, and file delivery.
- If the user's requested reply contains any attachment, telegram_message_send is the wrong tool.
- media_ref is the only required argument for telegram_message_send_media; media type and Telegram destination are resolved by the connector.
- For inbound media forwarding, reuse payload.media[].media_ref when the same file should be sent back or forwarded in the same channel.
