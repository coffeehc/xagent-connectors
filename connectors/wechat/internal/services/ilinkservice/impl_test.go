package ilinkservice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

func TestSendTextMessageUsesBotTokenAndParsesMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ilink/bot/sendmessage" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer bot-token-a" {
			t.Fatalf("bot token header 异常: %s", request.Header.Get("Authorization"))
		}
		body := map[string]any{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		msg, ok := body["msg"].(map[string]any)
		if !ok {
			t.Fatalf("发送请求体缺少 msg: %+v", body)
		}
		if msg["context_token"] != "ctx_contact_1" {
			t.Fatalf("发送请求体 context_token 异常: %+v", body)
		}
		items, ok := msg["item_list"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("发送请求体 item_list 异常: %+v", body)
		}
		item, ok := items[0].(map[string]any)
		if !ok {
			t.Fatalf("发送请求体 item 异常: %+v", body)
		}
		textItem, ok := item["text_item"].(map[string]any)
		if !ok || textItem["text"] != "你好" {
			t.Fatalf("发送请求体异常: %+v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"ret": 0})
	}))
	defer server.Close()

	service := newService(server.URL, protocol.WeChatBotType, server.Client())
	result, err := service.SendTextMessage(context.Background(), SendTextMessageInput{
		BaseURL:      server.URL,
		BotToken:     "bot-token-a",
		ContextToken: "ctx_contact_1",
		Text:         "你好",
	})
	if err != nil {
		t.Fatalf("SendTextMessage returned error: %v", err)
	}
	if result.MessageID == "" {
		t.Fatalf("消息发送结果解析异常: %+v", result)
	}
}

func TestGetUpdatesUsesOfficialEndpointAndBaseInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ilink/bot/getupdates" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer bot-token-a" {
			t.Fatalf("bot token header 异常: %s", request.Header.Get("Authorization"))
		}
		body := map[string]any{}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		if body["get_updates_buf"] != "cursor-1" {
			t.Fatalf("get_updates_buf 异常: %+v", body)
		}
		baseInfo, ok := body["base_info"].(map[string]any)
		if !ok {
			t.Fatalf("缺少 base_info: %+v", body)
		}
		if baseInfo["bot_agent"] != "xAgent/0.1" {
			t.Fatalf("bot_agent 应使用 OpenClaw UA 风格 token，got=%+v", baseInfo)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ret":             0,
			"get_updates_buf": "cursor-2",
			"msgs": []map[string]any{{
				"message_id":   1001,
				"from_user_id": "wx_user_1",
				"item_list": []map[string]any{{
					"type":      MessageItemTypeText,
					"text_item": map[string]any{"text": "hi"},
				}},
			}},
		})
	}))
	defer server.Close()

	service := newService(server.URL, protocol.WeChatBotType, server.Client())
	result, err := service.GetUpdates(context.Background(), GetUpdatesInput{
		BaseURL:       server.URL,
		BotToken:      "bot-token-a",
		GetUpdatesBuf: "cursor-1",
	})
	if err != nil {
		t.Fatalf("GetUpdates returned error: %v", err)
	}
	if result.GetUpdatesBuf != "cursor-2" || len(result.Messages) != 1 || result.Messages[0].FromUserID != "wx_user_1" {
		t.Fatalf("getupdates 响应解析异常: %+v", result)
	}
}

func TestGetUploadURLAndNotifyEndpoints(t *testing.T) {
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		if request.Header.Get("Authorization") != "Bearer bot-token-a" {
			t.Fatalf("bot token header 异常: %s", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/ilink/bot/getuploadurl":
			_ = json.NewEncoder(writer).Encode(map[string]any{"upload_full_url": "https://cdn.example/upload"})
		case "/ilink/bot/msg/notifystart", "/ilink/bot/msg/notifystop":
			_ = json.NewEncoder(writer).Encode(map[string]any{"ret": 0})
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	service := newService(server.URL, protocol.WeChatBotType, server.Client())
	upload, err := service.GetUploadURL(context.Background(), GetUploadURLInput{
		BaseURL:   server.URL,
		BotToken:  "bot-token-a",
		FileKey:   "file-key",
		MediaType: UploadMediaTypeImage,
	})
	if err != nil {
		t.Fatalf("GetUploadURL returned error: %v", err)
	}
	if upload.UploadFullURL != "https://cdn.example/upload" {
		t.Fatalf("upload url 解析异常: %+v", upload)
	}
	if _, err = service.NotifyStart(context.Background(), NotifyInput{BaseURL: server.URL, BotToken: "bot-token-a"}); err != nil {
		t.Fatalf("NotifyStart returned error: %v", err)
	}
	if _, err = service.NotifyStop(context.Background(), NotifyInput{BaseURL: server.URL, BotToken: "bot-token-a"}); err != nil {
		t.Fatalf("NotifyStop returned error: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("调用路径数量异常: %+v", paths)
	}
}
