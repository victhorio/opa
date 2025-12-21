package com

type Tool struct {
	Name   string
	Desc   string
	Params map[string]ToolParam
}

type ToolParam struct {
	Type JSType
	Desc string

	// we can tell that the type is nullable
	Nullable *bool

	// if Type == JSTArray, Items indicate the type of the items in the array
	Items *ToolParam

	// if Type == JSTString it can optionally be an enumerator with specific values
	Enum []string
}

type ToolMap = map[string]func(string) (string, error)

type JSType string

const (
	JSTString  JSType = "string"
	JSTNumber  JSType = "number"
	JSTBoolean JSType = "boolean"
	JSTArray   JSType = "array"
)
