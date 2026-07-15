package protocol

import (
	"encoding/json"
	"testing"
)

func TestProtocolConstants(t *testing.T) {
	if ConnectorCardSchema != "xagent.connector/v1" {
		t.Fatalf("unexpected connector card schema: %s", ConnectorCardSchema)
	}
	if ConnectionDescriptorSchema != "xagent.connection/v1" {
		t.Fatalf("unexpected connection descriptor schema: %s", ConnectionDescriptorSchema)
	}
	if PacketSchema != "xagent.connector.packet/v1" {
		t.Fatalf("unexpected packet schema: %s", PacketSchema)
	}
	if DataPlaneSubprotocol != "xagent.connector.packet.v1" {
		t.Fatalf("unexpected data plane subprotocol: %s", DataPlaneSubprotocol)
	}
}

func TestConnectorCardUserChannelModeJSON(t *testing.T) {
	var card ConnectorCard
	if err := json.Unmarshal([]byte(`{
		"schema":"xagent.connector/v1",
		"connector_card_id":"im.wechat",
		"connector":{"name":"WeChat Connector","version":"0.0.2"},
		"supports":{"user_channel_mode":"single","target_types":["im"],"profiles":["xagent.im.v1"]}
	}`), &card); err != nil {
		t.Fatalf("decode ConnectorCard failed: %v", err)
	}
	if card.Supports.UserChannelMode != ConnectorUserChannelModeSingle {
		t.Fatalf("unexpected user_channel_mode: %s", card.Supports.UserChannelMode)
	}
}

func TestConnectorAuthStartRequestJSON(t *testing.T) {
	var request ConnectorAuthStartRequest
	if err := json.Unmarshal([]byte(`{"flow_id":"telegram_bot_binding","input":{"bot_token":"secret","chat_id":"123"}}`), &request); err != nil {
		t.Fatalf("decode ConnectorAuthStartRequest failed: %v", err)
	}
	if request.FlowID != "telegram_bot_binding" {
		t.Fatalf("unexpected flow_id: %s", request.FlowID)
	}
	if request.Input["bot_token"] != "secret" {
		t.Fatalf("unexpected bot_token: %s", request.Input["bot_token"])
	}
	if request.Input["chat_id"] != "123" {
		t.Fatalf("unexpected chat_id: %s", request.Input["chat_id"])
	}
}

func TestConnectorCardAuthFlowFormFields(t *testing.T) {
	var card ConnectorCard
	if err := json.Unmarshal([]byte(`{
		"schema":"xagent.connector/v1",
		"connector_card_id":"im.telegram",
		"connector":{"name":"Telegram Connector","version":"0.0.1"},
		"supports":{"target_types":["im"],"profiles":["xagent.im.v1"]},
		"auth_flows":[{
			"id":"telegram_bot_binding",
			"target_type":"im",
			"type":"form",
			"title":"Telegram Bot 绑定",
			"fields":[
				{"name":"bot_token","label":"Bot Token","input_type":"password","required":true,"secret":true},
				{"name":"chat_id","label":"Chat ID","input_type":"text","required":true}
			]
		}]
	}`), &card); err != nil {
		t.Fatalf("decode ConnectorCard failed: %v", err)
	}
	if len(card.AuthFlows) != 1 {
		t.Fatalf("unexpected auth flow count: %d", len(card.AuthFlows))
	}
	flow := card.AuthFlows[0]
	if flow.Type != ConnectorAuthFlowTypeForm {
		t.Fatalf("unexpected auth flow type: %s", flow.Type)
	}
	if len(flow.Fields) != 2 {
		t.Fatalf("unexpected field count: %d", len(flow.Fields))
	}
	if flow.Fields[0].InputType != ConnectorAuthInputTypePassword {
		t.Fatalf("unexpected first field input_type: %s", flow.Fields[0].InputType)
	}
	if flow.Fields[1].InputType != ConnectorAuthInputTypeText {
		t.Fatalf("unexpected second field input_type: %s", flow.Fields[1].InputType)
	}
}

func TestConnectorToolDescriptorRelatedSkillIDs(t *testing.T) {
	var card ConnectorCard
	if err := json.Unmarshal([]byte(`{
		"schema":"xagent.connector/v1",
		"connector_card_id":"im.wechat",
		"connector":{"name":"WeChat Connector","version":"0.0.1"},
		"supports":{"target_types":["im"],"profiles":["xagent.im.v1"]},
		"tools":[{
			"tool_id":"wechat_message_send",
			"related_skill_ids":["im-connector-reply"],
			"input_schema":{"type":"object"}
		}]
	}`), &card); err != nil {
		t.Fatalf("decode ConnectorCard failed: %v", err)
	}
	if len(card.Tools) != 1 {
		t.Fatalf("unexpected tool count: %d", len(card.Tools))
	}
	if len(card.Tools[0].RelatedSkillIDs) != 1 || card.Tools[0].RelatedSkillIDs[0] != "im-connector-reply" {
		t.Fatalf("unexpected related skill ids: %+v", card.Tools[0].RelatedSkillIDs)
	}
}

func TestWithConnectorChannelIDInputSchemaInjectsStandardField(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required":             []string{"text"},
		"additionalProperties": false,
	}
	output, err := WithConnectorChannelIDInputSchema(schema)
	if err != nil {
		t.Fatalf("WithConnectorChannelIDInputSchema failed: %v", err)
	}
	if output["additionalProperties"] != false {
		t.Fatalf("additionalProperties should be preserved: %+v", output)
	}
	properties, ok := output["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties should be object: %+v", output["properties"])
	}
	if _, exists := properties["text"]; !exists {
		t.Fatalf("existing property should be preserved: %+v", properties)
	}
	channelProperty, ok := properties[ConnectorToolParamConnectorChannelID].(map[string]any)
	if !ok || channelProperty["type"] != "string" {
		t.Fatalf("connector_channel_id property mismatch: %+v", properties[ConnectorToolParamConnectorChannelID])
	}
	required, ok := output["required"].([]string)
	if !ok {
		t.Fatalf("required should be []string: %+v", output["required"])
	}
	if len(required) != 2 || required[0] != "text" || required[1] != ConnectorToolParamConnectorChannelID {
		t.Fatalf("required should preserve existing fields and append channel: %+v", required)
	}
	originalProperties := schema["properties"].(map[string]any)
	if _, exists := originalProperties[ConnectorToolParamConnectorChannelID]; exists {
		t.Fatalf("input schema should not be mutated: %+v", schema)
	}
}

func TestWithConnectorChannelIDInputSchemaHandlesEmptyRequired(t *testing.T) {
	output, err := WithConnectorChannelIDInputSchema(map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	})
	if err != nil {
		t.Fatalf("WithConnectorChannelIDInputSchema failed: %v", err)
	}
	required, ok := output["required"].([]string)
	if !ok || len(required) != 1 || required[0] != ConnectorToolParamConnectorChannelID {
		t.Fatalf("empty required should only contain channel id: %+v", output["required"])
	}
}

func TestWithConnectorChannelIDInputSchemaRejectsConflictField(t *testing.T) {
	_, err := WithConnectorChannelIDInputSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			ConnectorToolParamConnectorChannelID: map[string]any{"type": "string"},
		},
	})
	if err == nil {
		t.Fatalf("expected conflict field error")
	}
}

func TestWithConnectorChannelIDInputSchemaRejectsNonObjectSchema(t *testing.T) {
	_, err := WithConnectorChannelIDInputSchema(map[string]any{
		"type": "string",
	})
	if err == nil {
		t.Fatalf("expected non-object schema error")
	}
}

func TestValidateConnectorCardToolInputSchemasAcceptsAllTools(t *testing.T) {
	textSchema, err := WithConnectorChannelIDInputSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	})
	if err != nil {
		t.Fatalf("build text schema failed: %v", err)
	}
	mediaSchema, err := WithConnectorChannelIDInputSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"media_ref": map[string]any{"type": "string"},
		},
		"required": []string{"media_ref"},
	})
	if err != nil {
		t.Fatalf("build media schema failed: %v", err)
	}
	card := &ConnectorCard{
		Tools: []ConnectorToolDescriptor{
			{ToolID: "wechat_message_send", InputSchema: textSchema},
			{ToolID: "wechat_message_send_media", InputSchema: mediaSchema},
		},
	}
	if err := ValidateConnectorCardToolInputSchemas(card); err != nil {
		t.Fatalf("ValidateConnectorCardToolInputSchemas failed: %v", err)
	}
}

func TestValidateConnectorCardToolInputSchemasRejectsNonStringField(t *testing.T) {
	err := ValidateConnectorCardToolInputSchemas(&ConnectorCard{Tools: []ConnectorToolDescriptor{{
		ToolID: "bad_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				ConnectorToolParamConnectorChannelID: map[string]any{"type": "number"},
			},
			"required": []string{ConnectorToolParamConnectorChannelID},
		},
	}}})
	if err == nil {
		t.Fatalf("expected non-string field error")
	}
}

func TestValidateConnectorCardToolInputSchemasRejectsMissingField(t *testing.T) {
	err := ValidateConnectorCardToolInputSchemas(&ConnectorCard{Tools: []ConnectorToolDescriptor{{
		ToolID: "bad_tool",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
	}}})
	if err == nil {
		t.Fatalf("expected missing field error")
	}
}

func TestValidateConnectorCardToolInputSchemasRejectsNonObjectSchema(t *testing.T) {
	err := ValidateConnectorCardToolInputSchemas(&ConnectorCard{Tools: []ConnectorToolDescriptor{{
		ToolID: "bad_tool",
		InputSchema: map[string]any{
			"type": "array",
		},
	}}})
	if err == nil {
		t.Fatalf("expected non-object schema error")
	}
}

func TestValidateConnectorCardToolInputSchemasRejectsMissingRequired(t *testing.T) {
	err := ValidateConnectorCardToolInputSchemas(&ConnectorCard{Tools: []ConnectorToolDescriptor{{
		ToolID: "bad_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				ConnectorToolParamConnectorChannelID: map[string]any{"type": "string"},
			},
			"required": []string{},
		},
	}}})
	if err == nil {
		t.Fatalf("expected missing required error")
	}
}

func TestConnectorChannelIDInputSchemaJSONStructure(t *testing.T) {
	schema, err := WithConnectorChannelIDInputSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	})
	if err != nil {
		t.Fatalf("WithConnectorChannelIDInputSchema failed: %v", err)
	}
	payload, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal schema failed: %v", err)
	}
	properties, ok := decoded["properties"].(map[string]any)
	if !ok {
		t.Fatalf("decoded properties should be object: %+v", decoded["properties"])
	}
	channelProperty, ok := properties[ConnectorToolParamConnectorChannelID].(map[string]any)
	if !ok || channelProperty["type"] != "string" {
		t.Fatalf("decoded connector_channel_id property mismatch: %+v", properties[ConnectorToolParamConnectorChannelID])
	}
	required, ok := decoded["required"].([]any)
	if !ok || len(required) != 2 || required[0] != "text" || required[1] != ConnectorToolParamConnectorChannelID {
		t.Fatalf("decoded required mismatch: %+v", decoded["required"])
	}
}
