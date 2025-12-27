# opa

A personal AI assistant that lives in the terminal and knows my Obsidian vault.

This is a personal project, early in development, and not really intended for general use.

I'm building it for myself, but the code is public in case anyone finds it interesting.

## What it is

`opa` (Obsidian Powered AI) is a TUI chat interface backed by an LLM that can read, search, and
reason about notes in my Obsidian vault, as well as do web searches through the Perplexity API.

It also includes `agg`, a small Go framework for building AI agents that I'm developing alongside
it, so I don't need to use SDKs directly (supports OpenAI and Anthropic for now, including OpenAI
embeddings API) or marry to any framework.

An initial version of this project in Python can be found [here](https://github.com/victhorio/oba).

## What it does

- Chat interface in the terminal (Bubble Tea)
- Read and search vault notes (including ripgrep and semantic search with naive RAG)
- Web search via Perplexity
- Some automatic context (recent daily notes) provided in system message
- Tracks token usage and costs

## Structure

- `main.go`, `tui.go`, `tools.go` - The actual assistant
- `agg/` - Agent framework (model abstraction, tool handling, conversation storage)
- `obsidian/` - Vault loading, indexing, and search
- `prompts/` - System prompts and tool specs

## Requirements

- Go 1.21+
- OpenAI API key (set `OPENAI_API_KEY`)
- Anthropic API key (set `ANTHROPIC_API_KEY`)
- Perplexity API key for web search (set `PERPLEXITY_API_KEY`)
- ripgrep (`rg`) for vault search
- An Obsidian vault (currently hardcoded to my own path)
