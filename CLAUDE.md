This file was last updated 2025-12-25.

This file provides guidance to AI agents about universal aspects of this repository.

## Repository Overview

This repository is for `opa`, an "Obsidian Powered AI" assistant.

There are two main elements to this repository, the actual `opa` assistant, and the "internal" AI
agent framework which is `agg`.

The project is in very early stages, so most of the code is still unwritten.

## Goals

The goals are:

- For the entire codebase to have high-quality, idiomatic Go code with strong engineering practices.
- For `agg` to be a relatively complete and self-sufficient framework for AI Agent development.
- For `opa` to be a very capable AI agent presented through a TUI, that leverages content from the
  user's Obisidian vault.

## Architecture

- `main.go` - CLI demo with interactive REPL showing how to use the framework.

- `agg/` - The "AI framework" library.
- `agg/agent.go` - Most important file in `agg/`. Agent struct and Run() method orchestrating the agentic loop.
- `agg/store.go` - Store interface for conversation history.
- `agg/store_sqlite.go` - SQLite Store implementation.
- `agg/store_ephemeral.go` - In-memory Store implementation.
- `agg/tools.go` - Implements handlers that wrap callables and associate them with a tool specification, so agents can call it.

- `agg/core/` - Defines common types and interfaces.
- `agg/core/model.go` - Defines a Model and ResponseStream interfaces, allowing for abstracting providers.
- `agg/core/message.go` - Defines the different types of messages (e.g. text, reasoning, tool calls).
- `agg/core/response.go` - Defines a common "Response" and "Usage" types for generation results.
- `agg/core/event.go` - Event types for streaming (deltas, tool calls, errors).
- `agg/core/tools.go` - Common tool specification and JSON Schema types.
- `agg/core/embeddings.go` - Embedder interface and EmbeddingsResult type for generating embeddings.
- `agg/core/dump.go` - Error logging utility.

- `agg/openai/` - Implements Model and ResponseStream interfaces for the OpenAI responses API.
- `agg/openai/stream.go` - OpenAI provider implementation.
- `agg/openai/model.go` - OpenAI model configuration and pricing.

- `agg/anthropic/` - Implements Model and ResponseStream interfaces for the Anthropic messages API.
- `agg/anthropic/stream.go` - Anthropic provider implementation.
- `agg/anthropic/model.go` - Anthropic model configuration and pricing.

- `agg/embeddings/` - Embeddings provider implementations.
- `agg/embeddings/openai.go` - OpenAI embeddings API client implementation.

- `agg/tools/` - Built-in useful tools.
- `agg/tools/perplexity.go` - Perplexity AI web search tools.

- `obsidian/` - Obsidian vault integration and management.
- `obsidian/vault.go` - Vault loading, indexing, note reading, directory listing, and ripgrep search.
- `obsidian/embeddings.go` - Semantic search functionality using embeddings for vault notes.

- `prompts/` - System prompts and prompt management.
- `prompts/prompts.go` - Embeds system prompt files and tool specifications. Provides LoadToolSpec() function to load tool specs from YAML.
- `prompts/opa.txt` - Main system prompt for the opa assistant.
- `prompts/smart_read_note.txt` - System prompt for the SmartReadNote tool's LLM.
- `prompts/tools/` - YAML specifications for vault operation tools (read_note, smart_read_note, list_dir, rip_grep, semantic_search).

- `tools.go` - Tool wrappers for vault operations (ReadNote, SmartReadNote, ListDir, RipGrep, SemanticSearch). Uses loadToolSpec() helper to load YAML specs from prompts/tools/.
