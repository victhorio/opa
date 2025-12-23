package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/victhorio/opa/agg/core"
)

func TestSimpleMessage(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := &http.Client{}
	ch := make(chan core.Event, 1)

	msgs := []core.Msg{
		core.NewMsgContent("user", "What is the capital of France?"),
	}

	model := NewModel(Haiku, 2048, 1024, false)
	stream, err := model.OpenStream(ctx, client, msgs, []core.Tool{}, core.StreamCfg{})
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	go stream.Consume(ctx, ch)

	var r core.Response
	for event := range ch {
		switch event.Type {
		case core.EvDeltaReason:
			// ignore
		case core.EvDelta:
			// ignore
		case core.EvToolCall:
			t.Fatalf("Unexpected tool call: %v", event.Call)
		case core.EvResp:
			r = event.Response
		}
	}

	if r.Messages == nil {
		t.Fatalf("No messages in response")
	}

	msg := r.Messages[len(r.Messages)-1]
	content, ok := msg.AsContent()
	if !ok {
		t.Fatalf("Last message is not a content message, got %d", msg.Type)
	}

	if !strings.Contains(content.Text, "Paris") {
		t.Fatalf("Last message does not contain 'Paris', got %s", content.Text)
	}
}

func TestMultiTurnMessages(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := &http.Client{}
	ch := make(chan core.Event, 1)

	msgs := make([]core.Msg, 0, 4)
	msgs = append(
		msgs,
		core.NewMsgContent("user", "Hi! My name is Victhor, what is your name?"),
	)

	model := NewModel(Haiku, 2048, 1024, false)
	firstStream, err := model.OpenStream(ctx, client, msgs, []core.Tool{}, core.StreamCfg{})
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	go firstStream.Consume(ctx, ch)

	var r core.Response
	for event := range ch {
		switch event.Type {
		case core.EvDeltaReason:
			// ignore
		case core.EvDelta:
			// ignore
		case core.EvToolCall:
			t.Fatalf("Unexpected tool call: %v", event.Call)
		case core.EvResp:
			r = event.Response
		}
	}

	if r.Messages == nil {
		t.Fatalf("No messages in response")
	}

	msgs = append(msgs, r.Messages...)
	msgs = append(msgs, core.NewMsgContent("user", "Can you repeat my name to me?"))

	ch = make(chan core.Event, 1)
	secondStream, err := model.OpenStream(ctx, client, msgs, []core.Tool{}, core.StreamCfg{})
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	go secondStream.Consume(ctx, ch)

	var r2 core.Response
	for event := range ch {
		switch event.Type {
		case core.EvDeltaReason:
			// ignore
		case core.EvDelta:
			// ignore
		case core.EvToolCall:
			t.Fatalf("Unexpected tool call: %v", event.Call)
		case core.EvResp:
			r2 = event.Response
		}
	}

	if r2.Messages == nil {
		t.Fatalf("No messages in response")
	}

	msg := r2.Messages[len(r2.Messages)-1]
	content, ok := msg.AsContent()
	if !ok {
		t.Fatalf("Last message is not a content message, got %d", msg.Type)
	}

	if !strings.Contains(content.Text, "Victhor") {
		t.Fatalf("Last message does not contain 'Victhor', got %s", content.Text)
	}
}

func TestToolCall(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := &http.Client{}
	ch := make(chan core.Event, 1)

	msgs := []core.Msg{
		core.NewMsgContent("user", "What is the weather in Tokyo? In Celsius"),
	}

	model := NewModel(Haiku, 2048, 1024, false)
	stream, err := model.OpenStream(ctx, client, msgs, []core.Tool{getWeatherTool}, core.StreamCfg{})
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	go stream.Consume(ctx, ch)

	var tc_count int
	var tc core.ToolCall
	var r core.Response
	for event := range ch {
		switch event.Type {
		case core.EvDeltaReason:
			// ignore
		case core.EvDelta:
			// ignore
		case core.EvToolCall:
			tc_count++
			if tc_count > 1 {
				t.Fatalf("Expected only one tool call, got %d", tc_count)
			}

			tc = event.Call
		case core.EvResp:
			r = event.Response
		}
	}

	if tc_count != 1 {
		t.Fatalf("Expected one tool call, got %d", tc_count)
	}

	if tc.Name != "getWeather" {
		t.Fatalf("Expected tool call name to be 'getWeather', got %s", tc.Name)
	}

	type args struct {
		Location string `json:"location"`
		Units    string `json:"units"`
	}

	var a args
	err = json.Unmarshal([]byte(tc.Arguments), &a)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !strings.Contains(a.Location, "Tokyo") {
		t.Fatalf("Expected location to contain 'Tokyo', got %s", a.Location)
	}
	if a.Units != "Celsius" {
		t.Fatalf("Expected units to be 'Celsius', got %s", a.Units)
	}

	if r.Messages == nil {
		t.Fatalf("No messages in response")
	}

	msg := r.Messages[len(r.Messages)-1]
	msg_tc, ok := msg.AsToolCall()
	if !ok {
		t.Fatalf("Last message is not a tool call message, got %d", msg.Type)
	}
	if msg_tc.Name != "getWeather" {
		t.Fatalf("Expected tool call name to be 'getWeather', got %s", msg_tc.Name)
	}
	if msg_tc.Arguments != tc.Arguments {
		t.Fatalf("Expected tool call arguments to be %s, got %s", tc.Arguments, msg_tc.Arguments)
	}

	// now let's add a tool result and check if it answers correctly
	toolResult := core.NewMsgToolResult(tc.ID, `{"temperature": 25, "description": "Sunny"}`)
	msgs = append(msgs, r.Messages...)
	msgs = append(msgs, toolResult)

	ch = make(chan core.Event, 1)
	stream, err = model.OpenStream(ctx, client, msgs, []core.Tool{getWeatherTool}, core.StreamCfg{})
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	go stream.Consume(ctx, ch)

	var r2 core.Response
	for event := range ch {
		switch event.Type {
		case core.EvDeltaReason:
			// ignore
		case core.EvDelta:
			// ignore
		case core.EvToolCall:
			t.Fatalf("Unexpected tool call: %v", event.Call)
		case core.EvResp:
			r2 = event.Response
		}
	}

	if r2.Messages == nil {
		t.Fatalf("No messages in response")
	}

	msg = r2.Messages[len(r2.Messages)-1]
	content, ok := msg.AsContent()
	if !ok {
		t.Fatalf("Last message is not a content message, got %d", msg.Type)
	}

	if !strings.Contains(content.Text, "25") || !strings.Contains(strings.ToLower(content.Text), "sunny") {
		t.Fatalf("Expected weather in Tokyo to contain '25' and 'Sunny', got %s", content.Text)
	}
}

var getWeatherTool = core.Tool{
	Name: "getWeather",
	Desc: "Get the weather for a given location",
	Params: map[string]core.ToolParam{
		"location": {
			Type: core.JSTString,
			Desc: "The location to get the weather for",
		},
		"units": {
			Type: core.JSTString,
			Desc: "The units to use for the weather",
			Enum: []string{"Celsius", "Fahrenheit"},
		},
	},
}
