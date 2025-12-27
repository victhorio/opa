package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/victhorio/opa/agg"
	"github.com/victhorio/opa/agg/core"
	"github.com/victhorio/opa/agg/openai"
	"github.com/victhorio/opa/agg/tools"
	"github.com/victhorio/opa/obsidian"
	"github.com/victhorio/opa/prompts"
)

const sessionID = "tui-session"

func main() {
	vault, err := obsidian.LoadVault("~/Documents/Cortex", obsidian.Cfg{ComputeEmbeddings: true})
	if err != nil {
		log.Fatalf("error loading vault: %v", err)
	}

	agent := newAgent(vault)
	if err := runTUI(agent, sessionID); err != nil {
		log.Fatalf("error running TUI: %v", err)
	}

	u := agent.Store.Usage(sessionID)
	printUsage(u)
}

func newAgent(vault *obsidian.Vault) agg.Agent {
	model := openai.NewModel(openai.GPT5Mini, "minimal")
	store, err := agg.NewSQLiteStore(":memory:")
	if err != nil {
		log.Fatalf("error creating SQLite store: %v", err)
	}

	webSearchTool, err := tools.CreateAgenticWebSearchTool(http.DefaultClient)
	if err != nil {
		log.Fatalf("error creating web search tool: %v", err)
	}

	sysPrompt, err := loadSysPrompt(vault)
	if err != nil {
		log.Fatalf("error loading system prompt: %v", err)
	}

	return agg.NewAgent(
		sysPrompt,
		model,
		store,
		[]agg.Tool{
			createReadNoteTool(vault),
			createSmartReadNoteTool(vault, nil),
			createListDirTool(vault),
			createRipGrepTool(vault),
			createSemanticSearchTool(vault),
			webSearchTool,
		},
	)
}

func loadSysPrompt(vault *obsidian.Vault) (string, error) {
	recentDailies, err := vault.ReadRecentDailies(3)
	if err != nil {
		return "", fmt.Errorf("error reading recent daily notes: %w", err)
	}

	agentsMD, err := vault.ReadNote("AGENTS")
	if err != nil {
		return "", fmt.Errorf("error reading agents note: %w", err)
	}

	r := strings.NewReplacer(
		"{name}", "Victhor",
		"{now}", time.Now().Format("2006-01-02 15:04:05"),
		"{agents_md}", agentsMD,
		"{recent_dailies}", strings.Join(recentDailies, "\n\n"),
	)

	return r.Replace(prompts.OpaSysPrompt), nil
}

func printUsage(u core.Usage) {
	fmt.Printf("\n\033[33;1mUsage:\033[0m\n")
	fmt.Printf("  \033[33mInput:\033[0m %d\n", u.Input)
	fmt.Printf("    \033[33mCached:\033[0m %d\n", u.Cached)
	fmt.Printf("  \033[33mOutput:\033[0m %d\n", u.Output)
	fmt.Printf("  \033[33;1mCost:\033[0m $%.3f\n", float64(u.Cost)/1_000_000_000)
}
