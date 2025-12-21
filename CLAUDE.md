This file was last updated 2025-12-20.

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

- `./*.go` - Currently a simple CLI demo with interactive REPL.
- `agg/*.go` - Defines a Store interface for conversation history, Tool abstractions for runtime
  tool calls, and the Agent struct which orchestrates agentic behavior. The Agent is provider-
  agnostic and depends on the `core.Model` interface rather than any specific provider.
- `agg/core/*.go` - Defines shared types and interfaces used across modules:
  - `Model` and `ResponseStream` interfaces for provider abstraction
  - Message types (text, tool calls, tool results, reasoning)
  - Event types for streaming responses
  - Tool JSON Schema definitions, Usage types, etc.
- `agg/openai/*.go` - Implements the OpenAI provider via the responses endpoint. `NewModel()`
  returns a `core.Model` implementation. Main file is `stream.go`.
