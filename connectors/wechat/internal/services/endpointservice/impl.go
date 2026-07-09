package endpointservice

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/coffeehc/base/log"
	"github.com/coffeehc/httpx"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/channelservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/configservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/connectservice"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/mediaservice"
	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
)

type serviceImpl struct {
	httpServer httpx.Service
	connect    connectservice.Service
	channel    channelservice.Service
	media      mediaservice.Service
}

func newService(connect connectservice.Service, channel channelservice.Service, media mediaservice.Service) *serviceImpl {
	connect.BindMessagePusher(channelMessagePusher{channel: channel})
	return &serviceImpl{
		connect: connect,
		channel: channel,
		media:   media,
	}
}

type channelMessagePusher struct {
	channel channelservice.Service
}

func (pusher channelMessagePusher) PushMessage(ctx context.Context, connectorChannelID string, payload map[string]any) error {
	return pusher.channel.PushMessage(ctx, channelservice.MessagePushInput{
		ConnectorChannelID: connectorChannelID,
		Payload:            payload,
	})
}

func (pusher channelMessagePusher) PushConnectionDescriptor(ctx context.Context, connectorChannelID string, descriptor *connectorprotocol.ConnectionDescriptor) error {
	return pusher.channel.PushConnectionDescriptor(ctx, channelservice.DescriptorPushInput{
		ConnectorChannelID: connectorChannelID,
		Descriptor:         descriptor,
	})
}

// Start 启动微信 Connector Server HTTP endpoint 服务。
func (impl *serviceImpl) Start(context.Context) error {
	addr := configservice.EffectiveAddr()
	httpCfg := httpx.GetDefaultConfig(addr, "wechat-connector-server")
	httpCfg.ReadTimeoutMs = int64((5 * time.Second) / time.Millisecond)
	httpServer := httpx.NewService(httpCfg)
	if httpServer == nil {
		return fmt.Errorf("创建 wechat connector http server 失败")
	}
	impl.httpServer = httpServer
	impl.registerRoutes(httpServer.GetEngine())
	httpServer.Start(nil)
	log.Debug("wechat connector server 启动完成",
		zap.String("addr", addr),
		zap.String("result", "succeeded"),
	)
	return nil
}

// Stop 停止 connector HTTP endpoint 服务。
func (impl *serviceImpl) Stop(context.Context) error {
	if impl.httpServer == nil {
		return nil
	}
	if err := impl.httpServer.Shutdown(); err != nil {
		return fmt.Errorf("connector server shutdown failed: %w", err)
	}
	impl.httpServer = nil
	return nil
}

func (impl *serviceImpl) registerRoutes(app *fiber.App) {
	app.Get(connectorprotocol.ConnectorCardPath, impl.handleConnectorCard())
	app.Get(connectorprotocol.ConnectorSkillPath, impl.handleConnectorSkill())
	app.Get("/health", impl.handleHealth())
	app.Get("/connections/current", impl.handleCurrentConnection())
	app.Post(connectorprotocol.ConnectorMediaUploadPath, impl.handleMediaUpload())
	app.Get(connectorprotocol.ConnectorMediaRefPathPrefix+"/:media_ref", impl.handleMediaDownload())
	app.Get(connectorprotocol.ConnectorDataPlanePath, impl.handleDataPlane())
	log.Debug("wechat connector HTTP 路由已注册",
		zap.String("connector_card_path", connectorprotocol.ConnectorCardPath),
		zap.String("connector_skill_path", connectorprotocol.ConnectorSkillPath),
		zap.String("health_path", "/health"),
		zap.String("current_connection_path", "/connections/current"),
		zap.String("media_upload_path", connectorprotocol.ConnectorMediaUploadPath),
		zap.String("media_download_path", connectorprotocol.ConnectorMediaRefPathPrefix+"/:media_ref"),
		zap.String("websocket_path", connectorprotocol.ConnectorDataPlanePath),
		zap.String("result", "registered"),
	)
}

func (impl *serviceImpl) handleConnectorCard() fiber.Handler {
	return func(c fiber.Ctx) error {
		log.Debug("读取 wechat connector card",
			zap.String("path", c.Path()),
			zap.String("result", "succeeded"),
		)
		return writeJSON(c, fiber.StatusOK, impl.connect.BuildConnectorCard())
	}
}

func (impl *serviceImpl) handleConnectorSkill() fiber.Handler {
	return func(c fiber.Ctx) error {
		content, err := impl.connect.ReadConnectorSkill(c.Context())
		if err != nil {
			log.Debug("读取 wechat connector skill 失败",
				zap.String("path", c.Path()),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			return writeJSON(c, fiber.StatusInternalServerError, map[string]any{"error": "skill_read_failed", "message": err.Error()})
		}
		if content == nil {
			log.Debug("读取 wechat connector skill 失败：skill 不存在",
				zap.String("path", c.Path()),
				zap.String("result", "not_found"),
			)
			return writeJSON(c, fiber.StatusNotFound, map[string]any{"error": "skill_not_found"})
		}
		c.Set("Content-Type", content.ContentType)
		c.Set("X-XAgent-Skill-ID", content.SkillID)
		c.Set("X-XAgent-Skill-SHA256", content.SHA256)
		log.Debug("读取 wechat connector skill",
			zap.String("path", c.Path()),
			zap.String("skill_id", content.SkillID),
			zap.String("sha256", content.SHA256),
			zap.String("result", "succeeded"),
		)
		return c.SendString(content.Content)
	}
}

func (impl *serviceImpl) handleHealth() fiber.Handler {
	return func(c fiber.Ctx) error {
		if ok, err := impl.authorizedConnectorRequest(c); !ok || err != nil {
			return err
		}
		card := impl.connect.BuildConnectorCard()
		connectorCardVersion := ""
		if card != nil {
			connectorCardVersion = card.Connector.Version
		}
		return writeJSON(c, fiber.StatusOK, map[string]any{
			"status":                 "ok",
			"connector_card_id":      impl.connect.ConnectorID(),
			"connector_card_version": connectorCardVersion,
		})
	}
}

func (impl *serviceImpl) handleCurrentConnection() fiber.Handler {
	return func(c fiber.Ctx) error {
		if ok, err := impl.authorizedConnectorRequest(c); !ok || err != nil {
			return err
		}
		connectorChannelID := strings.TrimSpace(c.Get("X-XAgent-Connector-Channel-ID"))
		connection := impl.connect.ConnectionByChannel(connectorChannelID)
		if connection == nil {
			log.Debug("读取 wechat connector 当前连接失败",
				zap.String("path", c.Path()),
				zap.String("connector_channel_id", connectorChannelID),
				zap.String("result", "connection_not_found"),
			)
			return writeJSON(c, fiber.StatusNotFound, map[string]any{"error": "connection_not_found"})
		}
		log.Debug("读取 wechat connector 当前连接完成",
			zap.String("path", c.Path()),
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("account_hint", connection.AccountHint),
			zap.String("result", "succeeded"),
		)
		return writeJSON(c, fiber.StatusOK, impl.connect.BuildConnectionDescriptor(connection))
	}
}

func (impl *serviceImpl) handleMediaUpload() fiber.Handler {
	return func(c fiber.Ctx) error {
		if ok, err := impl.authorizedConnectorRequest(c); !ok || err != nil {
			return err
		}
		connectorChannelID := strings.TrimSpace(c.Get("X-XAgent-Connector-Channel-ID"))
		recipientRef := strings.TrimSpace(c.FormValue("recipient_ref"))
		if recipientRef == "" {
			recipientRef = strings.TrimSpace(c.FormValue("reply_token"))
		}
		fileHeader, err := c.FormFile("file")
		if err != nil {
			log.Debug("上传微信媒体失败：缺少文件",
				zap.String("path", c.Path()),
				zap.String("connector_channel_id", connectorChannelID),
				zap.String("result", "bad_request"),
				zap.Error(err),
			)
			return writeJSON(c, fiber.StatusBadRequest, map[string]any{"error": "file_required"})
		}
		file, err := fileHeader.Open()
		if err != nil {
			return writeJSON(c, fiber.StatusBadRequest, map[string]any{"error": "file_open_failed", "message": err.Error()})
		}
		defer file.Close()
		result, err := impl.connect.UploadMedia(c.Context(), connectservice.UploadMediaInput{
			ConnectorChannelID: connectorChannelID,
			RecipientRef:       recipientRef,
			Filename:           fileHeader.Filename,
			ContentType:        fileHeader.Header.Get("Content-Type"),
			Source:             file,
			Size:               fileHeader.Size,
		})
		if err != nil {
			status := mediaUploadErrorStatus(err)
			log.Debug("上传微信媒体失败",
				zap.String("path", c.Path()),
				zap.String("connector_channel_id", connectorChannelID),
				zap.String("recipient_ref", recipientRef),
				zap.String("filename", fileHeader.Filename),
				zap.Int("status", status),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			return writeJSON(c, status, map[string]any{"error": "media_upload_failed", "message": err.Error()})
		}
		log.Debug("上传微信媒体完成",
			zap.String("path", c.Path()),
			zap.String("connector_channel_id", connectorChannelID),
			zap.String("recipient_ref", recipientRef),
			zap.String("media_ref", result.MediaRef),
			zap.String("result", "uploaded"),
		)
		return writeJSON(c, fiber.StatusOK, result)
	}
}

func (impl *serviceImpl) handleMediaDownload() fiber.Handler {
	return func(c fiber.Ctx) error {
		mediaRef := strings.TrimSpace(c.Params("media_ref"))
		result, err := impl.media.OpenMediaStream(c.Context(), mediaservice.OpenMediaStreamInput{
			MediaRef: mediaRef,
		})
		if err != nil {
			status := mediaDownloadErrorStatus(err)
			log.Debug("下载微信媒体失败",
				zap.String("path", c.Path()),
				zap.String("media_ref", mediaRef),
				zap.Int("status", status),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			return writeJSON(c, status, map[string]any{"error": "media_download_failed", "message": err.Error()})
		}
		defer result.Reader.Close()
		payload, err := io.ReadAll(result.Reader)
		if err != nil {
			status := mediaDownloadErrorStatus(err)
			log.Debug("读取微信媒体流失败",
				zap.String("path", c.Path()),
				zap.String("media_ref", mediaRef),
				zap.Int("status", status),
				zap.String("result", "failed"),
				zap.Error(err),
			)
			return writeJSON(c, status, map[string]any{"error": "media_download_failed", "message": err.Error()})
		}
		log.Debug("下载微信媒体完成",
			zap.String("path", c.Path()),
			zap.String("media_ref", mediaRef),
			zap.String("content_type", result.ContentType),
			zap.Int("byte_size", len(payload)),
			zap.String("result", "succeeded"),
		)
		c.Set("Content-Type", result.ContentType)
		if result.Filename != "" {
			c.Set("Content-Disposition", "inline; filename=\""+strings.ReplaceAll(result.Filename, "\"", "")+"\"")
		}
		return c.Send(payload)
	}
}

func (impl *serviceImpl) handleDataPlane() fiber.Handler {
	return func(c fiber.Ctx) error {
		if ok, err := impl.authorizedConnectorRequest(c); !ok || err != nil {
			return err
		}
		log.Debug("升级 wechat connector data plane",
			zap.String("path", c.Path()),
			zap.String("result", "started"),
		)
		return impl.channel.HandleDataPlane(c)
	}
}

func mediaUploadErrorStatus(err error) int {
	if err == nil {
		return fiber.StatusOK
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "required"), strings.Contains(message, "empty"), strings.Contains(message, "not bound"):
		return fiber.StatusBadRequest
	case strings.Contains(message, "not found"):
		return fiber.StatusNotFound
	default:
		return fiber.StatusInternalServerError
	}
}

func mediaDownloadErrorStatus(err error) int {
	if err == nil {
		return fiber.StatusOK
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "not found"), strings.Contains(message, "expired"):
		return fiber.StatusNotFound
	case strings.Contains(message, "mismatch"):
		return fiber.StatusForbidden
	default:
		return fiber.StatusInternalServerError
	}
}

func (impl *serviceImpl) authorizedConnectorRequest(c fiber.Ctx) (bool, error) {
	if impl.connect.APIKey() == "" {
		return true, nil
	}
	expected := "Bearer " + impl.connect.APIKey()
	if string(c.Request().Header.Peek("Authorization")) == expected {
		return true, nil
	}
	log.Debug("拒绝 wechat connector HTTP 请求",
		zap.String("method", c.Method()),
		zap.String("path", c.Path()),
		zap.String("result", "unauthorized"),
	)
	return false, writeJSON(c, fiber.StatusUnauthorized, map[string]any{"error": "unauthorized"})
}

func writeJSON(c fiber.Ctx, status int, payload any) error {
	c.Set("Content-Type", "application/json; charset=utf-8")
	return c.Status(status).JSON(payload)
}
