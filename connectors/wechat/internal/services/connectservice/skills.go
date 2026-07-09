package connectservice

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"

	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

//go:embed assets/skills/im-connector-reply/SKILL.md
var connectorSkillIMReplyContent string

// ConnectorSkillContent 表示 connector 对外提供的 Skill 文件内容。
type ConnectorSkillContent struct {
	// SkillID 是 connector 主 Skill 的稳定标识，用于响应头和缓存诊断。
	SkillID string
	// ContentType 是 HTTP 返回的 content type。
	ContentType string
	// Content 是 SKILL.md 文本内容。
	Content string
	// SHA256 是 Content 的 SHA-256 十六进制摘要。
	SHA256 string
}

func connectorSkillSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// ReadConnectorSkill 读取 connector v1 标准主 Skill 内容。
func (impl *serviceImpl) ReadConnectorSkill(_ context.Context) (*ConnectorSkillContent, error) {
	content := connectorSkillIMReplyContent
	return &ConnectorSkillContent{
		SkillID:     protocol.ConnectorSkillIMReplyID,
		ContentType: "text/markdown; charset=utf-8",
		Content:     content,
		SHA256:      connectorSkillSHA256(content),
	}, nil
}
