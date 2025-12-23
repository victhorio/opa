package agg

import (
	"testing"

	"github.com/victhorio/opa/agg/core"
)

func TestEphemeralStore(t *testing.T) {
	s := NewEphemeralStore()

	// make sure we get valid empty values for non-existent keys
	msgs := s.Messages("k1")
	usage := s.Usage("k1")
	if n := len(msgs); n != 0 {
		t.Fatalf("expected empty k1 messages at beginning, got %d", n)
	}
	if tt := usage.Total; tt != 0 {
		t.Fatalf("expected 0 total tokens at beginning, got %d", tt)
	}

	// add things under key "k1"
	msgs = []core.Msg{
		core.NewMsgContent("user", "Hello!"),
		core.NewMsgReasoning("123456", ""),
		core.NewMsgToolCall("1", "fn", "{}"),
		core.NewMsgToolResult("1", "ok"),
	}

	usage = core.Usage{
		Input:  1024,
		Output: 256,
		Total:  1024 + 256,
	}

	err := s.Extend("k1", msgs, usage)
	if err != nil {
		t.Fatalf("got err on Extend: %v", err)
	}

	// now let's make sure things are preserved

	msgs = s.Messages("k1")
	usage = s.Usage("k1")

	if n := len(msgs); n != 4 {
		t.Fatalf("expected 4 messages after initial entry, got %d", n)
	}

	if tt := usage.Total; tt != 1024+256 {
		t.Fatalf("expected 1280 total tokens after initial entry, got %d", tt)
	}

	// let's make sure that if we read stuff from another key it's still empty

	msgs = s.Messages("k2")
	usage = s.Usage("k2")

	if n := len(msgs); n != 0 {
		t.Fatalf("expected empty messages for non-existent key, got %d", n)
	}

	if tt := usage.Total; tt != 0 {
		t.Fatalf("expected 0 total tokens for non-existent key, got %d", tt)
	}

	// let's add more messages and make sure extend works as intended

	msgs = []core.Msg{
		core.NewMsgContent("assistant", "Ok!"),
		core.NewMsgContent("user", "Can you repeat my name to me?"),
		core.NewMsgContent("assistant", "Victhor"),
	}
	usage = core.Usage{
		Input:  1280,
		Cached: 1024,
		Output: 64,
		Total:  1280 + 64,
	}

	err = s.Extend("k1", msgs, usage)
	if err != nil {
		t.Fatalf("got err on Extend: %v", err)
	}

	// now let's make sure they got added correctly

	msgs = s.Messages("k1")
	usage = s.Usage("k1")

	if n := len(msgs); n != 7 {
		t.Fatalf("expected 7 messages after adding more, got %d", n)
	}

	if it := usage.Input; it != 1280+1024 {
		t.Fatalf("expected 2304 input tokens after adding more, got %d", it)
	}
	if ot := usage.Output; ot != 256+64 {
		t.Fatalf("expected 320 output tokens after adding more, got %d", ot)
	}
	if ct := usage.Cached; ct != 1024 {
		t.Fatalf("expected 1024 cached tokens after adding more, got %d", ct)
	}

	// make sure messages are in the correct order
	expectedMsgTypes := []core.MsgType{
		core.MsgTypeContent,
		core.MsgTypeReasoning,
		core.MsgTypeToolCall,
		core.MsgTypeToolResult,
		core.MsgTypeContent,
		core.MsgTypeContent,
		core.MsgTypeContent,
	}

	for i, msg := range msgs {
		if msg.Type != expectedMsgTypes[i] {
			t.Fatalf("expected message type %d at index %d, got %d", expectedMsgTypes[i], i, msg.Type)
		}
	}
}
