package agg

import (
	"path/filepath"
	"testing"

	"github.com/victhorio/opa/agg/core"
)

func TestSQLiteStore_Memory(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory store: %v", err)
	}
	defer store.Close()

	t.Run("empty values for non-existent session", func(t *testing.T) {
		msgs := store.Messages("k1")
		usage := store.Usage("k1")

		if n := len(msgs); n != 0 {
			t.Fatalf("expected empty k1 messages at beginning, got %d", n)
		}
		if tt := usage.Total; tt != 0 {
			t.Fatalf("expected 0 total tokens at beginning, got %d", tt)
		}
	})

	t.Run("basic extend and retrieval", func(t *testing.T) {
		msgs := []*core.Msg{
			core.NewMsgContent("user", "Hello!"),
			core.NewMsgReasoning("123456", "thinking..."),
			core.NewMsgToolCall("1", "fn", "{}"),
			core.NewMsgToolResult("1", "ok"),
		}

		usage := core.Usage{
			Input:  1024,
			Output: 256,
			Total:  1024 + 256,
		}

		err := store.Extend("k1", msgs, usage)
		if err != nil {
			t.Fatalf("got err on Extend: %v", err)
		}

		// Verify messages were persisted
		retrievedMsgs := store.Messages("k1")
		if n := len(retrievedMsgs); n != 4 {
			t.Fatalf("expected 4 messages after initial entry, got %d", n)
		}

		// Verify usage was persisted
		retrievedUsage := store.Usage("k1")
		if tt := retrievedUsage.Total; tt != 1024+256 {
			t.Fatalf("expected 1280 total tokens after initial entry, got %d", tt)
		}
		if inp := retrievedUsage.Input; inp != 1024 {
			t.Fatalf("expected 1024 input tokens, got %d", inp)
		}
		if out := retrievedUsage.Output; out != 256 {
			t.Fatalf("expected 256 output tokens, got %d", out)
		}
	})

	t.Run("session isolation", func(t *testing.T) {
		// Verify k2 is still empty
		msgs := store.Messages("k2")
		usage := store.Usage("k2")

		if n := len(msgs); n != 0 {
			t.Fatalf("expected empty messages for non-existent key, got %d", n)
		}
		if tt := usage.Total; tt != 0 {
			t.Fatalf("expected 0 total tokens for non-existent key, got %d", tt)
		}
	})

	t.Run("extend accumulates correctly", func(t *testing.T) {
		msgs := []*core.Msg{
			core.NewMsgContent("assistant", "Ok!"),
			core.NewMsgContent("user", "Can you repeat my name to me?"),
			core.NewMsgContent("assistant", "Victhor"),
		}
		usage := core.Usage{
			Input:  1280,
			Cached: 1024,
			Output: 64,
		}

		err := store.Extend("k1", msgs, usage)
		if err != nil {
			t.Fatalf("got err on Extend: %v", err)
		}

		// Verify messages accumulated
		retrievedMsgs := store.Messages("k1")
		if n := len(retrievedMsgs); n != 7 {
			t.Fatalf("expected 7 messages after adding more, got %d", n)
		}

		// Verify usage accumulated
		retrievedUsage := store.Usage("k1")
		if it := retrievedUsage.Input; it != 1280+1024 {
			t.Fatalf("expected 2304 input tokens after adding more, got %d", it)
		}
		if ot := retrievedUsage.Output; ot != 256+64 {
			t.Fatalf("expected 320 output tokens after adding more, got %d", ot)
		}
		if ct := retrievedUsage.Cached; ct != 1024 {
			t.Fatalf("expected 1024 cached tokens after adding more, got %d", ct)
		}
	})

	t.Run("message ordering is preserved", func(t *testing.T) {
		msgs := store.Messages("k1")

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
	})

	t.Run("all message types serialize correctly", func(t *testing.T) {
		msgs := store.Messages("k1")

		// Check Content message
		content, ok := msgs[0].AsContent()
		if !ok {
			t.Fatal("expected first message to be Content")
		}
		if content.Role != "user" || content.Text != "Hello!" {
			t.Fatalf("content message not serialized correctly: got role=%s, text=%s", content.Role, content.Text)
		}

		// Check Reasoning message
		reasoning, ok := msgs[1].AsReasoning()
		if !ok {
			t.Fatal("expected second message to be Reasoning")
		}
		if reasoning.Encrypted != "123456" || reasoning.Text != "thinking..." {
			t.Fatalf("reasoning message not serialized correctly: got encrypted=%s, text=%s", reasoning.Encrypted, reasoning.Text)
		}

		// Check ToolCall message
		toolCall, ok := msgs[2].AsToolCall()
		if !ok {
			t.Fatal("expected third message to be ToolCall")
		}
		if toolCall.ID != "1" || toolCall.Name != "fn" || toolCall.Arguments != "{}" {
			t.Fatalf("toolCall message not serialized correctly: got id=%s, name=%s, args=%s", toolCall.ID, toolCall.Name, toolCall.Arguments)
		}

		// Check ToolResult message
		toolResult, ok := msgs[3].AsToolResult()
		if !ok {
			t.Fatal("expected fourth message to be ToolResult")
		}
		if toolResult.ID != "1" || toolResult.Result != "ok" {
			t.Fatalf("toolResult message not serialized correctly: got id=%s, result=%s", toolResult.ID, toolResult.Result)
		}
	})

	t.Run("total is recomputed on load", func(t *testing.T) {
		// Create a new session with specific usage that has Total set
		msgs := []*core.Msg{core.NewMsgContent("user", "test")}
		usage := core.Usage{
			Input:     100,
			Cached:    50,
			Output:    25,
			Reasoning: 10,
			Total:     100 + 50 + 25, // This should be recomputed, not used
		}

		err := store.Extend("k3", msgs, usage)
		if err != nil {
			t.Fatalf("failed to extend: %v", err)
		}

		// Retrieve and verify Total was recomputed
		retrievedUsage := store.Usage("k3")
		expectedTotal := int64(100 + 50 + 25)
		if retrievedUsage.Total != expectedTotal {
			t.Fatalf("expected Total to be recomputed to %d, got %d", expectedTotal, retrievedUsage.Total)
		}
	})

	t.Run("ephemeral cache is used", func(t *testing.T) {
		// First read populates the cache
		msgs1 := store.Messages("k1")

		// Second read should come from cache (no DB hit)
		// We can't directly test this without instrumenting the DB, but we can verify
		// the results are identical
		msgs2 := store.Messages("k1")

		if len(msgs1) != len(msgs2) {
			t.Fatalf("cache didn't return same number of messages: first=%d, second=%d", len(msgs1), len(msgs2))
		}

		// Also verify usage cache
		usage1 := store.Usage("k1")
		usage2 := store.Usage("k1")

		if usage1.Total != usage2.Total {
			t.Fatalf("cache didn't return same usage: first=%d, second=%d", usage1.Total, usage2.Total)
		}
	})
}

func TestSQLiteStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First session: create and populate
	t.Run("create and populate", func(t *testing.T) {
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		// Add session 1
		msgs1 := []*core.Msg{
			core.NewMsgContent("user", "Hello"),
			core.NewMsgContent("assistant", "Hi there!"),
		}
		usage1 := core.Usage{
			Input:  100,
			Output: 50,
			Total:  150,
		}
		if err := store.Extend("session1", msgs1, usage1); err != nil {
			t.Fatalf("failed to extend session1: %v", err)
		}

		// Add session 2
		msgs2 := []*core.Msg{
			core.NewMsgContent("user", "Test"),
			core.NewMsgReasoning("abc", "thinking"),
			core.NewMsgToolCall("t1", "tool", "{}"),
		}
		usage2 := core.Usage{
			Input:     200,
			Cached:    100,
			Output:    100,
			Reasoning: 25,
			Total:     400,
		}
		if err := store.Extend("session2", msgs2, usage2); err != nil {
			t.Fatalf("failed to extend session2: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	})

	// Second session: reopen and verify + add more data
	t.Run("reopen and verify first time", func(t *testing.T) {
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("failed to reopen store: %v", err)
		}

		// Verify session1
		msgs1 := store.Messages("session1")
		if len(msgs1) != 2 {
			t.Fatalf("expected 2 messages in session1, got %d", len(msgs1))
		}
		usage1 := store.Usage("session1")
		if usage1.Input != 100 || usage1.Output != 50 {
			t.Fatalf("session1 usage incorrect: input=%d, output=%d", usage1.Input, usage1.Output)
		}
		if usage1.Total != 150 {
			t.Fatalf("session1 Total not recomputed correctly: got %d, expected 150", usage1.Total)
		}

		// Verify session2
		msgs2 := store.Messages("session2")
		if len(msgs2) != 3 {
			t.Fatalf("expected 3 messages in session2, got %d", len(msgs2))
		}
		usage2 := store.Usage("session2")
		if usage2.Input != 200 || usage2.Cached != 100 || usage2.Output != 100 || usage2.Reasoning != 25 {
			t.Fatalf("session2 usage incorrect: input=%d, cached=%d, output=%d, reasoning=%d",
				usage2.Input, usage2.Cached, usage2.Output, usage2.Reasoning)
		}
		expectedTotal := int64(200 + 100 + 100)
		if usage2.Total != expectedTotal {
			t.Fatalf("session2 Total not recomputed correctly: got %d, expected %d", usage2.Total, expectedTotal)
		}

		// Add more data to session1
		newMsgs := []*core.Msg{core.NewMsgContent("user", "More data")}
		newUsage := core.Usage{Input: 50, Output: 25, Total: 75}
		if err := store.Extend("session1", newMsgs, newUsage); err != nil {
			t.Fatalf("failed to extend session1: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	})

	// Third session: reopen and verify accumulated data
	t.Run("reopen and verify second time", func(t *testing.T) {
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("failed to reopen store second time: %v", err)
		}
		defer store.Close()

		// Verify session1 now has 3 messages
		msgs1 := store.Messages("session1")
		if len(msgs1) != 3 {
			t.Fatalf("expected 3 messages in session1 after second reopen, got %d", len(msgs1))
		}

		// Verify session1 usage accumulated
		usage1 := store.Usage("session1")
		if usage1.Input != 150 || usage1.Output != 75 {
			t.Fatalf("session1 usage didn't accumulate: input=%d, output=%d", usage1.Input, usage1.Output)
		}
		expectedTotal := int64(150 + 75)
		if usage1.Total != expectedTotal {
			t.Fatalf("session1 Total incorrect after accumulation: got %d, expected %d", usage1.Total, expectedTotal)
		}

		// Verify session2 unchanged
		msgs2 := store.Messages("session2")
		if len(msgs2) != 3 {
			t.Fatalf("expected 3 messages in session2, got %d", len(msgs2))
		}

		// Verify message content is correct
		content, ok := msgs1[2].AsContent()
		if !ok || content.Text != "More data" {
			t.Fatal("third message in session1 not persisted correctly")
		}
	})
}
