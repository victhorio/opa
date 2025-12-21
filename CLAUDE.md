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

- `./*.go` - Currenrtly is a simple CLI demo with interactive REPL.
- `agg/*.go` - Defines a Store interface so it can handle conversation history, Tool abstractions
  so that it can call tools at runtimes and the critical Agent struct definition, which contains
  methods able to internally handle store/tools for agentic behavior.
- `agg/com/*.go` - Defines common types used across modules. For example, Message abstractions for
  the different types of messages (Text Messages, Tool Calls, Tool Results, Reasoning Blocks) as
  well as Tool JSON Schema definition types, Usage types, etc.
- `agg/openai/*.go` - Implements support for the OpenAI provider, using the responses endpoint. Main
  file here is `stream.go`.
