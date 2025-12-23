# Message Payload Caching Optimization

## Problem Statement

Currently, both OpenAI and Anthropic stream implementations transform `core.Message` types into provider-specific payload formats on every API call. In agentic conversations with multiple tool-calling rounds, the same messages (especially earlier conversation history) are re-transformed repeatedly, causing unnecessary CPU overhead and allocations.

## Current Behavior

Example conversation flow:

1. Turn 1: User message → transform 1 message → API call
2. Turn 2: User message + response → transform 2 messages (user message re-transformed) → API call
3. Turn 3: User + response + tool call + tool result → transform 4 messages (user + response re-transformed) → API call
4. Turn 4: All previous + new response → transform 5+ messages (all previous re-transformed) → API call

The transformation overhead grows quadratically with conversation length.

## Performance Impact

- **Repeated work**: Each message transformed O(n) times where n is rounds
- **Allocations**: Creating new payload objects repeatedly
- **JSON operations**: Re-marshaling tool call arguments, re-formatting content
- **String operations**: Building provider-specific message structures

For long conversations (50+ messages) with multiple tool rounds, this can add measurable latency.

## High-Level Solution

Cache the transformed payload for each message.

Once a message is transformed for a provider:

- Store the result in memory
- On subsequent API calls, reuse the cached payload
- No re-transformation needed

## Key Design Considerations

### Cache Location

Where should the cache live?

- On the `Message` interface/types themselves?
- In a separate cache structure in the model implementations?
- Trade-offs: memory overhead vs. lookup complexity

### Thread Safety

- Are messages shared across goroutines?
- Do we need synchronization (mutex) for cache access?
- Or can we rely on single-threaded access patterns?

### Invalidation

- Messages are typically immutable once created
- No invalidation needed in normal flow

### Memory Trade-offs

- Cached payloads add memory overhead
- For ephemeral stores: minimal impact (cleared after session)
- Is memory cost worth the performance gain?

## Option: add message IDs and each provider can keep its own cache

Maintain a `map[string]providerMsgType` in each model implementation, where the key is a message ID.

**Pros:**

- Keeps core types clean
- Cache isolated to model layer
- Easy to enable/disable

## Testing Strategy

Before optimization:

1. Benchmark message transformation overhead in current implementation
2. Profile multi-turn conversations to confirm transformation is measurable

After optimization:

1. Benchmark again to measure improvement
2. Verify correctness: cached payloads produce identical API calls
3. Test with both providers (OpenAI and Anthropic)
4. Test multi-turn conversations
5. Memory profiling to ensure reasonable overhead

## Files to Modify

- `agg/core/message.go` - Potentially add cache support
- `agg/openai/stream.go` - Implement caching in payload transformation
- `agg/anthropic/stream.go` - Implement caching in payload transformation
- `agg/*_test.go` - Add benchmarks and tests

## Success Criteria

- Measurable reduction in CPU time for multi-turn conversations (benchmark)
- No change in API behavior (same payloads sent)
- Minimal memory overhead (profile)
- Code remains clean and maintainable

## Non-Goals

- Don't over-engineer: this is an optimization, not a framework
- Don't cache API responses (different problem)
- Don't add complexity unless benchmarks show meaningful gains
