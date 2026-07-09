package connectservice

import (
	_ "embed"
	"encoding/json"
	"fmt"

	connectorprotocol "github.com/coffeehc/xagent-connectors/connectors/protocol"
	"github.com/coffeehc/xagent-connectors/connectors/wechat/internal/services/protocol"
)

//go:embed assets/connector_card.json
var connectorCardJSON []byte

// BuildConnectorCard 构建未绑定前公开读取的 Connector Card。
func (impl *serviceImpl) BuildConnectorCard() *connectorprotocol.ConnectorCard {
	card, err := readConnectorCardAsset()
	if err != nil {
		panic(err)
	}
	impl.applyConnectorCardRuntimeFields(card)
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
	card.Connector.Version = protocol.DefaultVersion
	if card.Security != nil {
		card.Security.APIKeyRequired = impl.apiKey != ""
	}
}
