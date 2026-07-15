package connectservice

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/coffeehc/xagent-connectors/connectors/feishu/internal/services/protocol"
	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
)

//go:embed assets/connector_card.json
var connectorCardJSON []byte

//go:embed assets/skills/im-connector-reply/SKILL.md
var connectorSkillMarkdown string

// BuildConnectorCard 构建未绑定前公开读取的 Connector Card。
func (impl *serviceImpl) BuildConnectorCard() *connectorprotocol.ConnectorCard {
	card := &connectorprotocol.ConnectorCard{}
	if err := json.Unmarshal(connectorCardJSON, card); err != nil {
		panic(err)
	}
	card.ConnectorCardID = protocol.ConnectorCardID
	card.Connector.Name = protocol.ConnectorName
	if card.Security != nil {
		card.Security.APIKeyRequired = impl.apiKey != ""
	}
	for index := range card.Tools {
		schema, err := connectorprotocol.WithConnectorChannelIDInputSchema(card.Tools[index].InputSchema)
		if err != nil {
			panic(err)
		}
		card.Tools[index].InputSchema = schema
	}
	if err := connectorprotocol.ValidateConnectorCardToolInputSchemas(card); err != nil {
		panic(fmt.Errorf("validate Feishu connector card: %w", err))
	}
	return card
}

// ReadConnectorSkill 读取 connector v1 标准主 Skill 内容。
func (impl *serviceImpl) ReadConnectorSkill(context.Context) (*ConnectorSkillContent, error) {
	sum := sha256.Sum256([]byte(connectorSkillMarkdown))
	return &ConnectorSkillContent{SkillID: protocol.ConnectorSkillIMReplyID, ContentType: "text/markdown; charset=utf-8", Content: connectorSkillMarkdown, SHA256: hex.EncodeToString(sum[:])}, nil
}
