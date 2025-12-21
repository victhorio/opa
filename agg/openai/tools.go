package openai

import "github.com/victhorio/opa/agg/core"

type tool struct {
	Type        string     `json:"type"` // always "function"
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  toolParams `json:"parameters"`
	Strict      bool       `json:"strict"`
}

type toolParams struct {
	Type                 core.JSType          `json:"type"` // always "object"
	Properties           map[string]paramProp `json:"properties,omitempty"`
	Required             []string             `json:"required,omitempty"`
	AdditionalProperties *bool                `json:"additionalProperties,omitempty"`
}

type paramProp struct {
	Type        core.JSType `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`

	// structural
	Items                *paramProp `json:"items,omitempty"`
	AdditionalProperties *bool      `json:"additionalProperties,omitempty"`

	// validation / constraints
	Enum     []string `json:"enum,omitempty"`
	Nullable *bool    `json:"nullable,omitempty"`
}

func fromCoreTools(tools []core.Tool) []tool {
	adapted := make([]tool, 0, len(tools))
	for _, tool := range tools {
		adapted = append(adapted, fromCoreTool(tool))
	}
	return adapted
}

func fromCoreTool(x core.Tool) tool {
	r := tool{
		Type:        "function",
		Name:        x.Name,
		Description: x.Desc,
		Parameters: toolParams{
			Type:                 "object",
			Properties:           make(map[string]paramProp),
			Required:             make([]string, 0),
			AdditionalProperties: boolPtr(false),
		},
		Strict: true,
	}

	for paramName, param := range x.Params {
		r.Parameters.Required = append(r.Parameters.Required, paramName)

		var items *paramProp
		if param.Items != nil {
			items = &paramProp{
				Type: param.Items.Type,
				Enum: param.Items.Enum,
			}
		}

		r.Parameters.Properties[paramName] = paramProp{
			Type:        param.Type,
			Description: param.Desc,
			Nullable:    param.Nullable,
			Items:       items,
			Enum:        param.Enum,
		}
	}

	return r
}
