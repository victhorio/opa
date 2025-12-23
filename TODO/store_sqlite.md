# SQLite Store Implementation

## Overview

Implement a persistent storage backend for the `Store` interface using SQLite. This provides durable conversation history and usage tracking across sessions.

## Interface to Implement

The implementation should satisfy the `Store` interface defined in `agg/store.go`:

```go
type Store interface {
    Messages(sessionID string) []core.Message
    Usage(sessionID string) core.Usage
    Extend(sessionID string, msgs []core.Message, usage core.Usage) error
}
```

## Database Schema

### Messages Table

```sql
CREATE TABLE IF NOT EXISTS messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_session_id_id
    ON messages(session_id, id);
```

- `id`: Auto-incrementing primary key for ordering
- `session_id`: Identifier for the conversation session
- `payload`: JSON-serialized message (see serialization format below)
- `created_at`: Timestamp for when the message was stored

### Usage Table

```sql
CREATE TABLE IF NOT EXISTS usage (
    session_id TEXT PRIMARY KEY,
    input_tokens INTEGER NOT NULL DEFAULT 0,
    input_tokens_cached INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    total_cost REAL NOT NULL DEFAULT 0.0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

- `session_id`: Primary key, unique per conversation
- Token counts: Track input (regular and cached) and output tokens
- Costs: Track total API costs and tool execution costs separately
- Uses `ON CONFLICT` to accumulate values when extending

## Message Serialization Format

Each message type should be serialized to JSON with a `type` discriminator:

### Content Message

```json
{
  "type": "content",
  "role": "...",
  "text": "..."
}
```

### Reasoning Message

```json
{
  "type": "reasoning",
  "encrypted_content": "...",
  "content": "..."
}
```

### Tool Call Message

```json
{
  "type": "tool_call",
  "call_id": "...",
  "name": "...",
  "args": "..."
}
```

### Tool Result Message

```json
{
  "type": "tool_result",
  "call_id": "...",
  "result": "..."
}
```

## Implementation Details

### Initialization

1. Accept a database path (string or file path)
2. Support `:memory:` for in-memory databases
3. Create parent directories if needed for file-based databases
4. Enable WAL (Write-Ahead Logging) mode: `PRAGMA journal_mode=WAL;`
5. Initialize schema on first connection

### Ephemeral Clone Feature (Optional but Recommended)

Maintain an in-memory `EphemeralStore` copy alongside SQLite for performance:

- On first `Messages()` call for a session, load from SQLite into ephemeral copy
- Subsequent calls read from ephemeral copy (fast)
- All `Extend()` calls write to both ephemeral and SQLite
- Check ephemeral copy first in `get_usage()` - if usage has non-zero tokens/costs, use it

Benefits:

- Reduces SQLite reads during active conversations
- Only pays deserialization cost once per session
- Still maintains persistence

### Method Implementations

#### `Messages(sessionID string) []core.Message`

1. If using ephemeral clone, check if session is loaded; if yes, return from memory
2. Query SQLite: `SELECT payload FROM messages WHERE session_id = ? ORDER BY id ASC`
3. Deserialize each payload into appropriate message type
4. If using ephemeral clone, also load usage and populate ephemeral copy
5. Return messages slice

#### `Usage(sessionID string) core.Usage`

1. If using ephemeral clone, check if loaded (non-zero input_tokens or tool_costs)
2. Query SQLite: `SELECT input_tokens, input_tokens_cached, output_tokens, total_cost, tool_costs FROM usage WHERE session_id = ?`
3. If no row found, return zero-valued Usage
4. Return populated Usage struct

#### `Extend(sessionID string, msgs []core.Message, usage core.Usage) error`

1. If using ephemeral clone, update it first
2. Begin transaction
3. Serialize all messages to JSON
4. Insert messages: `INSERT INTO messages (session_id, payload) VALUES (?, ?)`
5. Upsert usage with accumulation:
   ```sql
   INSERT INTO usage (session_id, input_tokens, input_tokens_cached, output_tokens, total_cost, tool_costs)
   VALUES (?, ?, ?, ?, ?, ?)
   ON CONFLICT(session_id) DO UPDATE SET
       input_tokens = usage.input_tokens + excluded.input_tokens,
       input_tokens_cached = usage.input_tokens_cached + excluded.input_tokens_cached,
       output_tokens = usage.output_tokens + excluded.output_tokens,
       total_cost = usage.total_cost + excluded.total_cost,
   ```
6. Commit transaction

Note: The `excluded` prefix refers to the values from the INSERT clause in SQLite's UPSERT syntax.

## Testing Considerations

- Test serialization/deserialization round-trip for all message types
- Test session isolation (different session_ids don't interfere)
- Test usage accumulation across multiple Extend() calls
- Test ephemeral clone stays in sync with SQLite
- Test database persistence (close and reopen)
- Test `:memory:` database mode

## Files to Create

- `agg/store_sqlite.go` - Main implementation
- `agg/store_sqlite_test.go` - Unit tests
