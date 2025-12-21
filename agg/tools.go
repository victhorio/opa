package agg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/victhorio/opa/agg/com"
)

type Tool struct {
	Handler ToolHandler
	Spec    com.Tool
}

func NewTool[T any](f ToolCallable[T], spec com.Tool) Tool {
	return Tool{
		Handler: createHandler(f),
		Spec:    spec,
	}
}

type ToolCallable[T any] func(context.Context, T) (string, error)
type ToolHandler func(context.Context, json.RawMessage) (string, error)

type ToolRegistry struct {
	m map[string]ToolHandler
}

func NewToolRegistry() ToolRegistry {
	return ToolRegistry{m: make(map[string]ToolHandler)}
}

func (r *ToolRegistry) Register(name string, h ToolHandler) {
	if _, ok := r.m[name]; ok {
		panic(fmt.Errorf("ToolRegistry.Register: tool %s already registered", name))
	}

	r.m[name] = h
}

func (r *ToolRegistry) Call(ctx context.Context, name string, args []byte) (string, error) {
	h, ok := r.m[name]
	if !ok {
		return "", fmt.Errorf("ToolRegistry.Call: tool %s not found", name)
	}

	out, err := h(ctx, json.RawMessage(args))
	if err != nil {
		return "", fmt.Errorf("ToolRegistry.Call: error calling handler: %w", err)
	}

	return out, nil
}

func createHandler[T any](f ToolCallable[T]) ToolHandler {
	return func(ctx context.Context, raw json.RawMessage) (string, error) {
		var args T

		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields() // let's catch problems early
		if err := dec.Decode(&args); err != nil {
			return "", fmt.Errorf("handler: invalid args: %w", err)
		}

		if dec.More() {
			// make sure there's no trailing junk
			return "", fmt.Errorf("handler: invalid args: extra JSON values: %s", raw)
		}

		return f(ctx, args)
	}
}
