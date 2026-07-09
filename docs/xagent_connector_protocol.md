# xAgent Connector Common Protocol

本文档定义 xAgent Connector 通用协议。第三方开发者实现 Connector Server 时，应以本文档作为 wire contract。

架构责任、事实归属和生命周期见 [xAgent Connector Architecture](xagent_connection_architecture.md)。

当前协议版本：`1.0`。

## 1. 协议族

| 名称 | 值 |
| --- | --- |
| Connector Card schema | `xagent.connector/v1` |
| Connection Descriptor schema | `xagent.connection/v1` |
| Packet schema | `xagent.connector.packet/v1` |
| Data plane subprotocol | `xagent.connector.packet.v1` |
| Protocol version | `1.0` |

术语：

- `connector.version`：Connector Card 中的 Connector 自身版本。工具、认证流程、Skill 或能力声明变化时升级。
- Protocol version：xAgent Connector 协议版本。Envelope、packet 类型、必填字段或核心状态语义变化时升级。
- Base URL：管理员在 xAgent 中配置的 Connector Server 根地址。它归 xAgent catalog 管，不写入 Connector Card。
- System API key：xAgent backend 与 Connector Server 之间的系统级认证密钥。它不进入前端、Agent、Skill、Card、Descriptor、tool 参数或 message payload。

## 2. 最小实现清单

一个可接入 xAgent 的 Connector Server 至少需要实现：

1. `GET /connector-card.json`：公开返回 Connector Card。
2. `GET /skill.md`：公开返回 Connector Skill；没有 Skill 时返回 `204` 或 `404`。
3. `GET /health`：系统级健康检查。
4. `GET /ws`：WebSocket data plane。
5. Data plane packet：
   - `connector.hello` / `connector.hello.ack`
   - `channel.open` / `channel.open.ack`
   - `auth.start` / `auth.start.ack`，如果需要用户认证
   - `auth.status` / `auth.status.ack`，如果认证需要轮询
   - `auth.cancel` / `auth.cancel.ack`，如果认证可取消
   - `auth.logout` / `auth.logout.ack`，如果支持登出
   - `connection.descriptor.get` / `connection.descriptor.get.ack`
   - `tool.invoke` / `tool.invoke.ack`
   - `message.push`，如果 Connector 有入站消息
   - `ping` / `pong`
6. 如果涉及文件、图片、视频等媒体：
   - `POST /media/uploads`
   - `GET /media/refs/{media_ref}`

## 3. Base URL 和认证

xAgent 以管理员配置的 Connector Base URL 为根路径访问固定 endpoint。

Base URL 规则：

- 只允许 `http` 或 `https`。
- 必须包含 host。
- 不允许包含 userinfo、query 或 fragment。
- 可以包含 path 前缀；xAgent 会在该前缀下拼接固定 endpoint。

系统认证规则：

- `/connector-card.json` 和 `/skill.md` 必须可公开读取，不应要求系统 API key。
- `/health` 可以要求系统 API key；xAgent 配置了 API key 时会发送 `Authorization: Bearer <api_key>`。
- `/ws` 可以要求系统 API key；xAgent 配置了 API key 时会发送 `Authorization: Bearer <api_key>`。
- `/media/uploads` 应要求系统 API key；它只允许 xAgent backend 调用。
- `/media/refs/{media_ref}` 如果作为 `message.push.media[].download_url` 返回，当前 xAgent 下载链不会附加额外 header；Connector 应使用不可猜测、短 TTL 的 URL，或把授权材料放在一次性 URL 中。

安全规则：

- 系统 API key 只用于 xAgent backend 到 Connector Server。
- 目标系统 token、cookie、refresh token、bot token、context token 不得进入任何 xAgent 可见对象。
- `connector_channel_id`、`request_id`、`media_ref` 都不是鉴权凭证。
- Connector 必须在服务端校验 `connector_id`、`connector_channel_id`、目标系统登录态和工具权限。

## 4. Control Plane HTTP

### 4.1 `GET /connector-card.json`

读取 Connector Card。

认证：无。

成功响应必须是 JSON object。

最小响应：

```json
{
  "schema": "xagent.connector/v1",
  "connector_card_id": "im.wechat",
  "connector": {
    "name": "WeChat Connector",
    "version": "0.0.1.beta",
    "vendor": "Example",
    "description": "Bridge WeChat messages into xAgent."
  },
  "supports": {
    "target_types": ["im"],
    "targets": [
      {
        "target_type": "im",
        "provider": "wechat",
        "label": "微信"
      }
    ],
    "profiles": ["xagent.im.v1"]
  },
  "tools": [
    {
      "tool_id": "wechat_message_send",
      "profile": "xagent.im.v1",
      "title": "发送微信消息",
      "description": "向当前 channel 绑定的微信联系人发送文本消息。",
      "input_schema": {
        "type": "object",
        "required": ["text"],
        "properties": {
          "text": {
            "type": "string",
            "description": "要发送的文本。"
          }
        }
      },
      "output_schema": {
        "type": "object"
      }
    }
  ],
  "auth_flows": [
    {
      "id": "wechat_qr_login",
      "target_type": "im",
      "type": "qr_login",
      "title": "微信扫码登录"
    }
  ],
  "ui": {
    "login_flow": {
      "flow_id": "wechat_qr_login",
      "steps": [
        {
          "type": "qr_code",
          "request_type": "auth.start",
          "response_type": "auth.start.ack"
        },
        {
          "type": "polling",
          "request_type": "auth.status",
          "response_type": "auth.status.ack"
        }
      ]
    }
  },
  "security": {
    "trust_level": "third_party",
    "api_key_required": true,
    "data_classes": ["message", "image"]
  }
}
```

硬校验：

- `schema` 必须是 `xagent.connector/v1`。
- `connector_card_id` 必填，并应由 Connector 开发者固定，不随部署变化。
- `connector.name` 必填，并应由 Connector 开发者固定。
- `connector.version` 必填。
- `supports.target_types` 必须非空；当前支持 `im`、`email`、`calendar`、`ticket`。
- `supports.profiles` 必须非空。
- `tools[].tool_id` 必须非空且不能重复。
- `tools[].tool_id` 会直接作为模型函数名暴露，长度不能超过 256，只能包含 ASCII 字母、数字、下划线、连字符和点号。

建议：

- `supports.targets[]` 应声明 `target_type`、`provider`、`label`，否则 xAgent 只能回退到 Connector 名称作为来源。
- `auth_flows[].target_type` 应与 `supports.target_types` 对齐。
- `security.trust_level` 可取 `unknown`、`builtin`、`verified`、`third_party`、`local`。
- `security.api_key_required` 表示系统链路是否需要 API key。
- `security.data_classes` 声明可能触达的数据类别。

禁止：

- 不要在 Card 中放 `server_base_url`。
- 不要在 Card 中放系统 API key、目标系统 token、一次性二维码、OAuth state 或真实敏感身份。
- 不要把未来可能支持、但当前调用会失败的工具放进 `tools[]`。

### 4.2 `GET /skill.md`

读取 Connector 主 Skill。

认证：无。

响应：

- `200`：返回 Markdown 文本。
- `204` 或 `404`：表示 Connector 不提供主 Skill。

Skill 只表达 Agent 如何处理事件和使用工具，不得包含密钥、系统 API key、目标系统 token 或一次性认证材料。

### 4.3 `GET /health`

探测 Connector 系统级健康状态。

认证：可要求 `Authorization: Bearer <api_key>`。

成功响应：

```json
{
  "status": "ok",
  "connector_card_id": "im.wechat",
  "connector_card_version": "0.0.1.beta"
}
```

规则：

- `2xx` 表示 Connector endpoint 可用。
- `401` 或 `403` 表示系统 API key 未通过鉴权。
- 非 `2xx` 表示 degraded。
- 如果返回 `connector_card_id`，必须与接入时的 `connector_card_id` 一致。
- 如果返回 `connector_card_version` 且版本变化，xAgent 会重新拉取 Card 和 Skill。

## 5. Transfer Plane HTTP

Transfer Plane 只允许 xAgent backend 调用。前端和 LLM 不直接访问。

### 5.1 `POST /media/uploads`

上传待发送文件，返回 Connector 内部 `media_ref`。

认证：应要求 `Authorization: Bearer <api_key>`。

请求：

```http
POST /media/uploads
Authorization: Bearer <api_key>
X-XAgent-Connector-Channel-ID: <connector_channel_id>
Content-Type: multipart/form-data
```

表单字段：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `file` | 是 | 待上传文件正文 |
| `recipient_ref` | 否 | 目标系统收件人引用 |
| `reply_token` | 否 | 回复目标引用；缺少 `recipient_ref` 时可作为 fallback |

成功响应：

```json
{
  "media_ref": "media_abc123",
  "media_type": "image",
  "filename": "image.jpg",
  "byte_size": 155000,
  "expires_at": 1790000000000
}
```

规则：

- `media_ref` 是 Connector 内部不透明 key。
- `media_ref` 必须绑定 `connector_channel_id`。
- `media_ref` 过期策略由 Connector 管理。
- 上传只表示文件已进入 Connector/目标系统媒体链路，不等于消息已发送。
- 发送消息仍需调用 Connector Card 中声明的媒体发送工具，例如 `wechat_message_send_media`。

### 5.2 `GET /media/refs/{media_ref}`

下载 Connector 暂存媒体流。

认证：由 Connector 决定；但如果该 URL 出现在 `message.push.payload.media[].download_url`，当前 xAgent 资源解析链只携带 URL，不携带额外 header。

响应：

- `2xx`：返回文件字节流。
- `404`：`media_ref` 不存在或已过期。
- `403`：`media_ref` 与当前 channel 或授权不匹配。

规则：

- 该 endpoint 不返回 JSON，而是返回原始字节流。
- 必须设置合理的 `Content-Type`。
- 建议设置 `Content-Disposition` 文件名。
- 不得透出目标系统 CDN token、bot token、context token 或 API key。
- 对于可能过期或一次性的目标系统 CDN，Connector 应在收到入站媒体时立即下载并缓存到 Connector 本地。

## 6. Data Plane WebSocket

Endpoint：

```http
GET /ws
Sec-WebSocket-Protocol: xagent.connector.packet.v1
Authorization: Bearer <api_key>
```

规则：

- 只传 WebSocket TextMessage。
- 每条消息是一个 JSON packet。
- 首包必须是 `connector.hello`。
- `connector.hello.ack` 之前不能发送用户级 packet。
- WebSocket 不传文件正文、base64 或目标系统 CDN 字节流。
- 断线后 xAgent 会自动重连，并重新打开持久化的用户 channel。

## 7. Packet Envelope

所有 data plane packet 使用同一个 envelope。

```json
{
  "schema": "xagent.connector.packet/v1",
  "packet_id": "pkt_...",
  "request_id": "req_...",
  "reply_to": "pkt_...",
  "connector_channel_id": "cch_...",
  "type": "tool.invoke",
  "time": 1790000000000,
  "payload": {},
  "error": {
    "code": "tool_invoke_failed",
    "message": "message text required",
    "retryable": false,
    "details": {}
  }
}
```

字段语义：

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `schema` | 是 | 固定 `xagent.connector.packet/v1` |
| `packet_id` | 是 | 当前 packet 唯一 ID |
| `request_id` | 请求和 ack 必填 | xAgent 生成请求 ID；Connector 回包必须原样带回 |
| `reply_to` | ack 建议填 | 当前 packet 回复的 `packet_id` |
| `connector_channel_id` | 用户级 packet 必填 | Connector 分配的用户级 channel ID |
| `type` | 是 | packet 类型 |
| `time` | 建议填 | Unix 毫秒时间戳 |
| `payload` | 按类型决定 | packet 业务负载 |
| `error` | 错误时填 | 稳定错误码和可读说明 |

路由规则：

- xAgent 通过 `request_id` 路由同步请求回包。
- Connector 主动推送按 `connector_channel_id` 路由到用户 channel。
- `request_id` 不是鉴权凭证。
- `connector_channel_id` 不是鉴权凭证。
- Ack packet 应带回请求的 `request_id`，建议用 `reply_to` 指向请求 `packet_id`。

错误语义：

- 协议、身份、路由、hello 顺序错误使用 `type = "error"`。
- 业务操作失败使用对应 ack packet 的 `error` 字段，例如 `tool.invoke.ack.error`。
- `error.code` 应为稳定机器可读字符串。
- `error.message` 面向日志和开发者，不应包含密钥或目标系统 token。
- `error.details` 可选；不要放大对象、token、cookie 或完整上游响应。

## 8. Packet 类型

### 8.1 `connector.hello`

方向：xAgent -> Connector。

payload：

```json
{
  "connector_card_id": "im.wechat",
  "connector_id": "conn_im_wechat_xxx"
}
```

规则：

- `connector_card_id` 必填，必须匹配当前 Connector Card。
- `connector_id` 首次连接可为空。
- Connector 如已分配过 `connector_id`，后续 hello 必须校验一致。
- Connector 不应因为 xAgent 首次未传 `connector_id` 而拒绝连接。

### 8.2 `connector.hello.ack`

方向：Connector -> xAgent。

payload：

```json
{
  "connector_card_id": "im.wechat",
  "connector_id": "conn_im_wechat_xxx"
}
```

规则：

- `connector_card_id` 必须与请求一致。
- `connector_id` 必填。
- xAgent 如已有不同 `connector_id`，会视为身份异常，不能覆盖本地事实。

### 8.3 `channel.open`

方向：xAgent -> Connector。

Envelope 的 `connector_channel_id`：

- 首次打开时为空。
- 恢复已有 channel 时填已持久化的 `connector_channel_id`。

payload 当前可为空。

规则：

- Connector 可复用已知 channel。
- Connector 如果无法识别旧 channel，可以重新分配新的 `connector_channel_id`。
- `channel.open` 只打开运行时路由，不代表目标系统已经完成认证。

### 8.4 `channel.open.ack`

方向：Connector -> xAgent。

必须返回：

- envelope `connector_channel_id`。
- `payload.connector_channel_id`。
- `payload.connection_descriptor`。

示例：

```json
{
  "schema": "xagent.connector.packet/v1",
  "packet_id": "pkt_2",
  "request_id": "req_1",
  "reply_to": "pkt_1",
  "connector_channel_id": "cch_123",
  "type": "channel.open.ack",
  "time": 1790000000000,
  "payload": {
    "connector_channel_id": "cch_123",
    "connection_descriptor": {
      "schema": "xagent.connection/v1",
      "connection": {
        "connector_card_id": "im.wechat",
        "connector_id": "conn_im_wechat_xxx",
        "connector_channel_id": "cch_123",
        "target_type": "im",
        "profile": "xagent.im.v1",
        "status": "created"
      },
      "target": {
        "provider": "wechat",
        "label": "微信",
        "display_name": "未绑定微信"
      }
    }
  }
}
```

### 8.5 `channel.close`

方向：xAgent -> Connector。

语义：关闭运行时 channel 路由。

规则：

- 不删除 Connector 内目标系统登录态。
- 不要求 xAgent 删除本地持久绑定。
- Connector 应停止向该 WebSocket route 推送该 channel 的 `message.push`。

### 8.6 `channel.close.ack`

方向：Connector -> xAgent。

payload：

```json
{
  "status": "ok"
}
```

### 8.7 `auth.start`

方向：xAgent -> Connector。

payload：

```json
{
  "flow_id": "wechat_qr_login"
}
```

规则：

- 必须在已打开 channel 上调用。
- `flow_id` 来自 Connector Card `auth_flows[].id`。
- 如果该 channel 已有可复用登录态，Connector 可以直接返回 `authenticated` 和 `connection_descriptor`。

### 8.8 `auth.start.ack`

方向：Connector -> xAgent。

payload 字段：

| 字段 | 说明 |
| --- | --- |
| `connector_channel_id` | 当前认证所属 channel |
| `flow_id` | auth flow id |
| `auth_session_id` | Connector 认证会话 ID |
| `status` | `pending`、`scanned`、`authenticated`、`expired`、`qr_refresh_required`、`failed` |
| `qr_code_text` | 二维码原始内容 |
| `qr_code_image` | 二维码图片 URL 或 data URL |
| `expires_at` | Unix 毫秒时间戳 |
| `poll_interval_millis` | 前端建议轮询间隔 |
| `message` | 可读状态说明 |
| `connection_descriptor` | 已认证时可直接返回 |

示例：

```json
{
  "connector_channel_id": "cch_123",
  "flow_id": "wechat_qr_login",
  "auth_session_id": "auth_123",
  "status": "pending",
  "qr_code_text": "https://example/qr",
  "expires_at": 1790000000000,
  "poll_interval_millis": 2000,
  "message": "请扫码登录"
}
```

### 8.9 `auth.status`

方向：xAgent -> Connector。

payload：

```json
{
  "flow_id": "wechat_qr_login",
  "auth_session_id": "auth_123",
  "refresh": false
}
```

规则：

- `refresh = true` 表示请求 Connector 刷新认证材料，例如二维码。
- 未找到认证会话时，Connector 应返回 `auth.status.ack.error` 或 `type = "error"`，错误码建议 `auth_session_not_found`。

### 8.10 `auth.status.ack`

方向：Connector -> xAgent。

字段与 `auth.start.ack` 基本一致，`status` 取值：

- `pending`
- `scanned`
- `authenticated`
- `unauthenticated`
- `expired`
- `qr_refresh_required`
- `failed`

认证成功时应返回 `connection_descriptor`。xAgent 用它回正用户连接投影和工具可用性。

### 8.11 `auth.cancel`

方向：xAgent -> Connector。

语义：取消未完成的认证会话。

payload：

```json
{
  "auth_session_id": "auth_123"
}
```

规则：

- 必须在已打开 channel 上调用。
- 只取消认证流程，不删除已存在的目标系统登录态。
- 如果认证已经完成，Connector 可以返回 `ignored` 并附带当前 `connection_descriptor`。

### 8.12 `auth.cancel.ack`

方向：Connector -> xAgent。

payload：

```json
{
  "connector_channel_id": "cch_123",
  "auth_session_id": "auth_123",
  "status": "canceled",
  "auth_status": "unauthenticated",
  "message": "认证已取消"
}
```

`status` 取值：

- `canceled`
- `ignored`
- `not_found`

### 8.13 `auth.logout`

方向：xAgent -> Connector。

语义：退出当前 channel 绑定的目标系统真实登录态。

规则：

- 必须在已打开 channel 上调用。
- Connector 应清理目标系统登录态或授权材料。
- xAgent 在成功后删除本地 channel 绑定和运行时路由。
- 它不是 `channel.close`。

### 8.14 `auth.logout.ack`

方向：Connector -> xAgent。

payload：

```json
{
  "status": "ok",
  "connection_descriptor": {}
}
```

`connection_descriptor` 应反映登出后的状态，例如 `created`、`expired` 或 `revoked`。

### 8.15 `connection.descriptor.get`

方向：xAgent -> Connector。

语义：请求当前 channel 的 Connection Descriptor。

payload 当前可为空。

### 8.16 `connection.descriptor.get.ack`

方向：Connector -> xAgent。

payload：

```json
{
  "connection_descriptor": {}
}
```

### 8.17 `connection.descriptor.push`

方向：Connector -> xAgent。

语义：Connector 主动推送当前 channel 的 descriptor 变化。

payload：

```json
{
  "connection_descriptor": {}
}
```

规则：

- 用于认证成功、权限变化、目标系统离线、token 过期等状态回正。
- xAgent 会校验 descriptor 身份，不匹配时忽略。
- Connector 不需要等待 xAgent 轮询后才推送重要状态变化。

### 8.18 `tool.invoke`

方向：xAgent -> Connector。

payload：

```json
{
  "tool_id": "wechat_message_send",
  "arguments": {
    "text": "你好"
  },
  "context": {
    "session_id": "session_123",
    "tool_call_id": "call_123"
  }
}
```

规则：

- `tool_id` 必须来自 Connector Card。
- xAgent 只会在当前 descriptor 中 tool 可用时投递。
- Connector 必须再次按目标系统权限校验。
- `arguments` 只能包含 tool schema 声明的业务参数。
- `context` 是 xAgent 运行时上下文，Connector 只能作为关联信息使用，不能当鉴权材料。
- `connector_channel_id` 不进入 `arguments`，由 xAgent 放在 envelope。

### 8.19 `tool.invoke.ack`

方向：Connector -> xAgent。

成功 payload：

```json
{
  "tool_id": "wechat_message_send",
  "result": {
    "status": "sent",
    "message_id": "msg_123"
  }
}
```

失败 packet 使用 envelope 顶层 `error`：

```json
{
  "type": "tool.invoke.ack",
  "error": {
    "code": "tool_invoke_failed",
    "message": "message text required"
  }
}
```

规则：

- 不得返回目标系统 token、bot token、context token、API key 或目标系统 CDN 签名原文。
- 有副作用工具必须具备幂等或重复调用识别能力。
- 文件发送类工具应消费 `media_ref`，不消费文件正文、base64 或目标系统 URL。

### 8.20 `tool.progress.push`

方向：Connector -> xAgent。

语义：长耗时工具的进度事件。

payload 建议：

```json
{
  "tool_id": "long_running_tool",
  "status": "running",
  "message": "处理中",
  "progress": 0.5
}
```

当前 xAgent 主要等待 `tool.invoke.ack` 作为终态；progress 只能作为运行时事件，不替代 ack。

### 8.21 `message.push`

方向：Connector -> xAgent。

语义：Connector 主动推送目标系统入站消息。

payload 示例：

```json
{
  "provider": "wechat",
  "profile": "xagent.im.v1",
  "event_kind": "im.message.received",
  "message_id": "7479013024887233416",
  "sender_id": "wx_user_1",
  "display_name": "张三",
  "text": "来自微信的用户消息：\n发送方：张三\n消息类型：文本\n用户文本：睡觉了",
  "raw_text": "睡觉了",
  "reply": {
    "required": true,
    "tool_id": "wechat_message_send"
  }
}
```

规则：

- envelope `connector_channel_id` 必填。
- `message_id` 建议稳定，用于 refs 和去重。
- `text`、`content` 或 `message` 是用户可见正文。
- `activation_message` 是内部执行目标，不等同于用户可见正文。
- 可以用 `target_session_ref`、`session_ref` 或 `target_session_id` 指定目标会话；不得同时出现多个目标 session ref。
- xAgent 默认把未指定目标的消息投递到用户主会话。
- Connector 应在本地持久化 pending 队列和消费游标；channel 未打开时不要丢消息。

媒体消息 payload 示例：

```json
{
  "provider": "wechat",
  "profile": "xagent.im.v1",
  "event_kind": "im.message.received",
  "message_id": "7479013024887233417",
  "text": "来自微信的用户消息：\n发送方：张三\n消息类型：图片\n用户文本：无",
  "media": [
    {
      "type": "image",
      "media_ref": "media_abc123",
      "filename": "wechat-image.jpg",
      "mime_type": "image/jpeg",
      "byte_size": 155000,
      "expires_at": 1790000000000,
      "download_url": "/media/refs/media_abc123"
    }
  ]
}
```

媒体规则：

- `media` 和 `media_refs` 都可被当前 xAgent 识别。
- 每个媒体项必须有 `media_ref`。
- 当前 xAgent 要把媒体变成模型可读文件，还必须有 `download_url` 或 `url`。
- `download_url` 可以是绝对 URL，也可以是相对 URI；相对 URI 由 xAgent 用 Connector catalog 的 `server_base_url` 补全。
- `download_url` 只能给 xAgent backend 或资源解析器消费，不能要求 LLM 手工访问。
- `filename`、`mime_type`、`byte_size`、`expires_at` 应尽量提供。

### 8.22 `ping` / `pong`

方向：双向。

规则：

- 收到 `ping` 后应回复 `pong`。
- `pong` 应带回同一个 `request_id`。

### 8.23 `error`

方向：双向。

用于协议、身份、路由和顺序错误。

示例：

```json
{
  "schema": "xagent.connector.packet/v1",
  "packet_id": "pkt_error",
  "request_id": "req_123",
  "connector_channel_id": "cch_123",
  "type": "error",
  "time": 1790000000000,
  "error": {
    "code": "channel_not_open",
    "message": "channel.open must complete before auth.start"
  }
}
```

常用错误码建议：

| 错误码 | 场景 |
| --- | --- |
| `invalid_packet` | JSON 或 envelope 非法 |
| `hello_required` | hello 完成前收到用户级 packet |
| `connector_card_id_mismatch` | hello 中 card ID 不匹配 |
| `connector_id_mismatch` | hello 中 connector ID 不匹配 |
| `channel_not_open` | 用户级 packet 没有已打开 channel |
| `connection_not_found` | channel 不存在或未绑定 |
| `connection_not_authenticated` | channel 尚未完成目标系统认证 |
| `auth_session_not_found` | 认证会话不存在 |
| `tool_invoke_failed` | 工具执行失败 |
| `unsupported_packet` | packet type 不支持 |

## 9. Connection Descriptor

Connection Descriptor 是用户级运行态投影。

最小结构：

```json
{
  "schema": "xagent.connection/v1",
  "connection": {
    "connector_card_id": "im.wechat",
    "connector_id": "conn_im_wechat_xxx",
    "connector_channel_id": "cch_conn_im_wechat_xxx",
    "target_type": "im",
    "profile": "xagent.im.v1",
    "status": "connected"
  },
  "target": {
    "provider": "wechat",
    "label": "微信",
    "display_name": "微信 0069***.bot",
    "account_hint": "0069***.bot"
  },
  "tools": [
    {
      "tool_id": "wechat_message_send",
      "status": "available",
      "target_permission_state": "granted"
    }
  ]
}
```

硬校验：

- `schema` 必须是 `xagent.connection/v1`。
- `connection.connector_card_id` 必须等于当前 Connector Card ID。
- `connection.connector_id` 必填。
- `connection.connector_channel_id` 必须等于当前 channel。
- `connection.target_type` 当前支持 `im`、`email`、`calendar`、`ticket`。
- `target.provider` 必填。
- `connection.status` 必须是支持状态。
- `tools[].tool_id` 必填。

`connection.status` 取值：

| 值 | 语义 |
| --- | --- |
| `created` | channel 已创建但尚未认证 |
| `authenticating` | 正在认证或绑定 |
| `connected` | 已绑定且当前在线可用 |
| `degraded` | 已绑定但部分能力降级 |
| `offline` | 绑定仍存在但目标或 Connector 当前离线 |
| `expired` | 认证材料已经过期 |
| `revoked` | 用户或目标系统撤销授权 |
| `error` | 无法自动分类的错误态 |

`tools[].status` 取值：

- `available`
- `unavailable`
- `denied_by_target`
- `requires_reauth`
- `not_supported`

`tools[].target_permission_state` 取值：

- `unknown`
- `granted`
- `denied`
- `requires_reauth`

规则：

- Card 中没有的工具不能出现在 Descriptor 中。
- Descriptor 中不可用的工具不能投影给 Agent。
- `target` 只能包含展示级账号信息和脱敏提示。
- `offline` 和 `error` 不等于登出；xAgent 会把它们视为已认证但当前不可激活。

## 10. Connector Card 工具声明

工具声明示例：

```json
{
  "tool_id": "wechat_message_send",
  "profile": "xagent.im.v1",
  "title": "发送微信 IM 消息",
  "description": "向当前 channel 绑定的微信用户发送文本消息；接收人由 connector 登录态决定。",
  "input_schema": {
    "type": "object",
    "required": ["text"],
    "properties": {
      "text": {
        "type": "string",
        "description": "要发送给微信用户的文本内容。"
      }
    }
  },
  "output_schema": {
    "type": "object",
    "properties": {
      "status": {
        "type": "string"
      },
      "message_id": {
        "type": "string"
      }
    }
  }
}
```

规则：

- 工具必须真实可调用。
- 不允许把未来可能支持、但当前调用会 404 的能力放进 Card。
- 不允许伪造联系人搜索、文件搜索等目标系统不存在的工具。
- `tool_id` 会作为模型函数名暴露，必须只包含 ASCII 字母、数字、下划线、连字符和点号，长度不能超过 256。
- `connector_channel_id` 不进入 `input_schema`，由 xAgent runtime 注入。
- 系统 API key、目标系统 token、transfer token 不进入 `input_schema`。
- 发送文件类工具应只消费 `media_ref`，不消费文件字节、base64 或目标系统 URL。

## 11. 入站消息到 xAgent 的转换

xAgent 接收 `message.push` 后会做以下转换：

1. 按 envelope `connector_channel_id` 找到 xAgent 用户。
2. 按 `target_session_ref` / `session_ref` / `target_session_id` 定位目标会话；未指定时使用用户主会话。
3. 将 `payload.text` / `payload.content` / `payload.message` 转为用户可见消息。
4. 将 Connector Card 或 Connection Descriptor 的来源标签补到可见消息前。
5. 将 `media` / `media_refs` 转为 `download_url` ResourceRef。
6. 提交为 `SessionEvent`，由 Agent 主链消费。

Connector 侧应提供足够信息让 Agent 理解来源和回复方式，但不需要理解 xAgent 内部 SessionEvent 结构。

## 12. 版本兼容

兼容规则：

- 新增可选字段通常只需要升级 `connector.version`。
- 新增工具、auth flow、profile 或 Skill 内容，也升级 `connector.version`。
- 删除工具、修改 `tool_id`、修改既有字段语义、把可选参数改必填，属于破坏性变更。
- 修改 packet envelope、ID 校验规则、必选 packet 或核心状态语义，必须升级 Protocol version。
- xAgent 发现 `connector.version` 变化后，会重新拉取 Card 和 Skill，并刷新工具投影。

## 13. 当前实现边界

当前 xAgent 实现已经覆盖：

- `GET /connector-card.json`
- `GET /skill.md`
- `GET /health`
- `GET /ws`
- `POST /media/uploads` 和 `GET /media/refs/{media_ref}` 的 Connector 侧协议
- `connector.hello`
- `channel.open`
- `channel.close`
- `auth.start`
- `auth.cancel`
- `auth.status`
- `auth.logout`
- `connection.descriptor.get`
- `connection.descriptor.push`
- `tool.invoke`
- `message.push`
- `ping` / `pong`
- 入站媒体 `download_url -> ResourceRef -> session attachment`

当前仍需注意：

- 用户 HTTP API 暂未暴露单独的 `channel.close` 入口；这不影响 Connector 实现 `channel.close` packet。
- `connector_media` ResourceRef 结构存在，但当前自动解析链只支持 `download_url`。
- 媒体下载 URL 如果是相对 URI，xAgent 会使用 catalog 中的 `server_base_url` 拼成绝对 URL。

## 14. 第三方实现建议

实现 Connector Server 时建议按以下顺序开发：

1. 固定 `connector_card_id`、Connector name、target type、provider、profile。
2. 实现公开 `/connector-card.json` 和 `/skill.md`。
3. 实现 `/health`，并在配置了 API key 时校验 `Authorization: Bearer <api_key>`。
4. 实现 `/ws` 和 `connector.hello`，持久化或稳定生成 `connector_id`。
5. 实现 `channel.open`，分配并持久化 `connector_channel_id`。
6. 实现 `connection.descriptor.get`，即使未认证也返回 `created` descriptor。
7. 实现目标系统认证流程：`auth.start`、`auth.status`、`auth.cancel`、`auth.logout`。
8. 实现 `tool.invoke`，并确保每个工具都在 Card 和 Descriptor 中一致声明。
9. 实现入站消息 pending 队列和 `message.push`。
10. 如果支持媒体，先本地缓存入站媒体，再返回 `media_ref` 和可下载 `download_url`。
11. 做断线恢复测试：xAgent 重连后重新 `channel.open`，Connector 应能 flush pending 消息。
12. 做安全检查：日志、Card、Skill、Descriptor、tool result 和 message payload 中不能出现密钥或目标系统 token。
