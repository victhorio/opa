package core

type Tool struct {
	Name   string
	Desc   string
	Params map[string]ToolParam
}

type ToolParam struct {
	Type JSType `json:"type"`
	Desc string `json:"description"`

	// we can tell that the type is nullable
	Nullable *bool `json:"nullable,omitempty"`

	// if Type == JSTArray, Items indicate the type of the items in the array
	Items *ToolParam `json:"items,omitempty"`

	// if Type == JSTString it can optionally be an enumerator with specific values
	Enum []string `json:"enum,omitempty"`
}

type ToolMap = map[string]func(string) (string, error)

type JSType string

const (
	JSTString  JSType = "string"
	JSTNumber  JSType = "number"
	JSTBoolean JSType = "boolean"
	JSTArray   JSType = "array"
)
