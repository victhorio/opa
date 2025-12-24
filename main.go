package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/victhorio/opa/agg"
	"github.com/victhorio/opa/agg/core"
	"github.com/victhorio/opa/agg/openai"
	"github.com/victhorio/opa/agg/tools"
	"github.com/victhorio/opa/obsidian"
)

func main() {
	vault, err := obsidian.LoadVault("~/Documents/Cortex")
	if err != nil {
		log.Fatalf("error loading vault: %v", err)
	}

	agent := newAgent(vault)
	repl(&agent)

	u := agent.Store.Usage("123")
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

	dailyNotes, err := vault.ReadRecentDailies(3)
	if err != nil {
		log.Fatalf("error reading recent daily notes: %v", err)
	}

	return agg.NewAgent(
		fmt.Sprintf(`
		You are a helpful assistant for a user.
		
		These are the most recent daily notes from the user's Obsidian vault:
		
		<daily_notes>
		%s
		</daily_notes>
		`, strings.Join(dailyNotes, "\n\n")),
		model,
		store,
		[]agg.Tool{webSearchTool},
	)
}

func repl(agent *agg.Agent) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\033[34mYou:\033[0m ")
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("failed to read input: %v", err)
		}
		input = strings.TrimSpace(input)

		if input == ":q" {
			break
		}

		resp, err := agent.Run(context.Background(), &http.Client{}, "123", input, true)
		if err != nil {
			log.Fatalf("error running agent: %v", err)
		}

		fmt.Printf("\033[32mAssistant:\033[0m\n%s\n", resp)
	}

}

func printUsage(u core.Usage) {
	fmt.Printf("\n\033[33;1mUsage:\033[0m\n")
	fmt.Printf("  \033[33mInput:\033[0m %d\n", u.Input)
	fmt.Printf("    \033[33mCached:\033[0m %d\n", u.Cached)
	fmt.Printf("  \033[33mOutput:\033[0m %d\n", u.Output)
	fmt.Printf("  \033[33;1mCost:\033[0m $%.3f\n", float64(u.Cost)/1_000_000_000)
}
