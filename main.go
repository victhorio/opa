package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	if err := setupLogging(); err != nil {
		log.Fatalf("error setting up logging: %v", err)
	}

	vault, err := obsidian.LoadVault("~/Documents/Cortex", obsidian.Cfg{ComputeEmbeddings: false})
	if err != nil {
		log.Fatalf("error loading vault: %v", err)
	}

	// Start embeddings refresh in background so TUI opens immediately.
	embeddingsDone := vault.RefreshEmbeddingsAsync()

	agent := newAgent(vault)
	if err := runTUI(agent, sessionID, embeddingsDone); err != nil {
		log.Fatalf("error running TUI: %v", err)
	}

	u := agent.Store.Usage(sessionID)
	printUsage(u)
}

func newAgent(vault *obsidian.Vault) agg.Agent {
	model := openai.NewModel(openai.GPT51, "low")
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
	recentDailies, err := vault.ReadRecentDailies(2)
	if err != nil {
		return "", fmt.Errorf("error reading recent daily notes: %w", err)
	}

	recentWeeklies, err := vault.ReadRecentWeeklies(1)
	if err != nil && err != os.ErrNotExist {
		return "", fmt.Errorf("error reading recent weekly notes: %w", err)
	}

	var recentWeekly string
	if len(recentWeeklies) > 0 {
		recentWeekly = recentWeeklies[0]
	} else {
		recentWeekly = "[No weekly notes found for this vault]"
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
		"{recent_weekly}", recentWeekly,
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

// setupLogging redirects log output to ~/.opa/opa.log so it doesn't interfere with the TUI.
func setupLogging() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	opaDir := filepath.Join(home, ".opa")
	if err := os.MkdirAll(opaDir, 0755); err != nil {
		return fmt.Errorf("failed to create ~/.opa directory: %w", err)
	}

	logPath := filepath.Join(opaDir, "opa.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("--- opa started ---")

	return nil
}
