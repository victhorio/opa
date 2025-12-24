This file was last updated 2025-12-23.

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
- `agg/core/dump.go` - Error logging utility.

- `agg/openai/` - Implements Model and ResponseStream interfaces for the OpenAI responses API.
- `agg/openai/stream.go` - OpenAI provider implementation.
- `agg/openai/model.go` - OpenAI model configuration and pricing.

- `agg/anthropic/` - Implements Model and ResponseStream interfaces for the Anthropic messages API.
- `agg/anthropic/stream.go` - Anthropic provider implementation.
- `agg/anthropic/model.go` - Anthropic model configuration and pricing.

- `agg/tools/` - Built-in useful tools.
- `agg/tools/perplexity.go` - Perplexity AI web search tools.
