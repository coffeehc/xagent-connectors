package protocol

import "fmt"

const connectorToolInputTypeObject = "object"
const connectorToolInputTypeString = "string"

// ConnectorToolParamConnectorChannelID 是 Connector 工具保留的模型可见 channel 路由参数名。
//
// 语义边界：
// 1. 该参数表示本次 Connector 工具调用的目标用户级 channel；
// 2. 调用方必须原样传递该 opaque ID，不能用 Connector name、provider、ConnectorCardID 或账号名称替代；
// 3. xAgent adapter 负责从模型参数中提取该值并映射到 tool.invoke envelope 的 connector_channel_id；
// 4. Connector 业务工具不应把它当作普通业务参数消费，也不允许解释为目标系统账号 ID。
const ConnectorToolParamConnectorChannelID = "connector_channel_id"

// WithConnectorChannelIDInputSchema 为 Connector 工具 object input_schema 注入标准 channel 路由参数。
//
// 输入约束：
// 1. schema 必须是 JSON Schema object，且 type 必须等于 object；
// 2. 如果 properties 或 required 已声明 connector_channel_id，返回错误，避免覆盖调用方手写冲突；
// 3. properties 必须是 object；required 为空时会创建，非空时必须是字符串数组。
//
// 返回值语义：
// 1. 返回新的 schema map，不修改输入 map；
// 2. 保留原有 properties、required 和 additionalProperties 语义；
// 3. 自动把 connector_channel_id 加入 properties 和 required。
func WithConnectorChannelIDInputSchema(schema map[string]any) (map[string]any, error) {
	output, properties, err := cloneObjectInputSchema(schema)
	if err != nil {
		return nil, err
	}
	if _, exists := properties[ConnectorToolParamConnectorChannelID]; exists {
		return nil, fmt.Errorf("input_schema already declares reserved %s property", ConnectorToolParamConnectorChannelID)
	}
	required, err := requiredStrings(schema["required"])
	if err != nil {
		return nil, err
	}
	if containsString(required, ConnectorToolParamConnectorChannelID) {
		return nil, fmt.Errorf("input_schema already declares reserved %s required field", ConnectorToolParamConnectorChannelID)
	}
	properties[ConnectorToolParamConnectorChannelID] = connectorChannelIDInputProperty()
	output["properties"] = properties
	output["required"] = append(required, ConnectorToolParamConnectorChannelID)
	return output, nil
}

// ValidateConnectorCardToolInputSchemas 校验 Connector Card 中所有模型可调用工具的 channel 路由参数协议。
//
// 错误语义：返回首个不满足协议的工具错误；无工具时返回 nil。
func ValidateConnectorCardToolInputSchemas(card *ConnectorCard) error {
	if card == nil {
		return fmt.Errorf("connector card required")
	}
	for _, tool := range card.Tools {
		if err := validateConnectorToolInputSchema(tool); err != nil {
			if tool.ToolID == "" {
				return fmt.Errorf("tool input_schema invalid: %w", err)
			}
			return fmt.Errorf("tool %s input_schema invalid: %w", tool.ToolID, err)
		}
	}
	return nil
}

func validateConnectorToolInputSchema(tool ConnectorToolDescriptor) error {
	_, properties, err := cloneObjectInputSchema(tool.InputSchema)
	if err != nil {
		return err
	}
	property, exists := properties[ConnectorToolParamConnectorChannelID]
	if !exists {
		return fmt.Errorf("%s property required", ConnectorToolParamConnectorChannelID)
	}
	propertyObject, ok := property.(map[string]any)
	if !ok {
		return fmt.Errorf("%s property must be object", ConnectorToolParamConnectorChannelID)
	}
	propertyType, ok := propertyObject["type"].(string)
	if !ok || propertyType != connectorToolInputTypeString {
		return fmt.Errorf("%s property type must be string", ConnectorToolParamConnectorChannelID)
	}
	required, err := requiredStrings(tool.InputSchema["required"])
	if err != nil {
		return err
	}
	if !containsString(required, ConnectorToolParamConnectorChannelID) {
		return fmt.Errorf("%s must be required", ConnectorToolParamConnectorChannelID)
	}
	return nil
}

func cloneObjectInputSchema(schema map[string]any) (map[string]any, map[string]any, error) {
	if schema == nil {
		return nil, nil, fmt.Errorf("input_schema required")
	}
	schemaType, ok := schema["type"].(string)
	if !ok || schemaType != connectorToolInputTypeObject {
		return nil, nil, fmt.Errorf("input_schema type must be object")
	}
	output := make(map[string]any, len(schema)+1)
	for key, value := range schema {
		output[key] = value
	}
	properties := map[string]any{}
	if propertiesValue, exists := schema["properties"]; exists {
		typedProperties, ok := propertiesValue.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("input_schema properties must be object")
		}
		properties = make(map[string]any, len(typedProperties)+1)
		for key, value := range typedProperties {
			properties[key] = value
		}
	}
	return output, properties, nil
}

func connectorChannelIDInputProperty() map[string]any {
	return map[string]any{
		"type":        connectorToolInputTypeString,
		"description": "目标用户级 Connector channel ID。必须使用 connector_channels_list 或入站 source_connector_channel_id 得到的 opaque ID 原样传递；不能用 Connector name、provider、ConnectorCardID 或账号名称替代。",
	}
}

func requiredStrings(value any) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	switch typedValue := value.(type) {
	case []string:
		output := make([]string, 0, len(typedValue))
		output = append(output, typedValue...)
		return output, nil
	case []any:
		output := make([]string, 0, len(typedValue))
		for _, item := range typedValue {
			itemString, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("input_schema required must contain only strings")
			}
			output = append(output, itemString)
		}
		return output, nil
	default:
		return nil, fmt.Errorf("input_schema required must be string array")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
