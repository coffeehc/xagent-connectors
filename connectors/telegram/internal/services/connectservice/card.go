package connectservice

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/telegram/internal/services/protocol"
)

//go:embed assets/connector_card.json
var connectorCardJSON []byte

//go:embed assets/skills/im-connector-reply/SKILL.md
var connectorSkillMarkdown string

// BuildConnectorCard 构建未绑定前公开读取的 Connector Card。
func (impl *serviceImpl) BuildConnectorCard() *connectorprotocol.ConnectorCard {
	card, err := readConnectorCardAsset()
	if err != nil {
		panic(err)
	}
	impl.applyConnectorCardRuntimeFields(card)
	if err := applyConnectorToolInputSchemas(card); err != nil {
		panic(err)
	}
	return card
}

func readConnectorCardAsset() (*connectorprotocol.ConnectorCard, error) {
	var card connectorprotocol.ConnectorCard
	if err := json.Unmarshal(connectorCardJSON, &card); err != nil {
		return nil, fmt.Errorf("decode embedded connector card: %w", err)
	}
	return &card, nil
}

func (impl *serviceImpl) applyConnectorCardRuntimeFields(card *connectorprotocol.ConnectorCard) {
	if card == nil {
		return
	}
	card.ConnectorCardID = protocol.ConnectorCardID
	card.Connector.Name = protocol.ConnectorName
	if card.Security != nil {
		card.Security.APIKeyRequired = impl.apiKey != ""
	}
}

// applyConnectorToolInputSchemas 将协议保留的 connector_channel_id 模型参数注入所有工具 schema。
func applyConnectorToolInputSchemas(card *connectorprotocol.ConnectorCard) error {
	if card == nil {
		return nil
	}
	for index := range card.Tools {
		inputSchema, err := connectorprotocol.WithConnectorChannelIDInputSchema(card.Tools[index].InputSchema)
		if err != nil {
			return fmt.Errorf("apply connector channel id input schema for tool %s: %w", card.Tools[index].ToolID, err)
		}
		card.Tools[index].InputSchema = inputSchema
	}
	return connectorprotocol.ValidateConnectorCardToolInputSchemas(card)
}

// ReadConnectorSkill 读取 connector v1 标准主 Skill 内容。
func (impl *serviceImpl) ReadConnectorSkill(context.Context) (*ConnectorSkillContent, error) {
	sum := sha256.Sum256([]byte(connectorSkillMarkdown))
	return &ConnectorSkillContent{
		SkillID:     protocol.ConnectorSkillIMReplyID,
		ContentType: "text/markdown; charset=utf-8",
		Content:     connectorSkillMarkdown,
		SHA256:      hex.EncodeToString(sum[:]),
	}, nil
}
