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
)

func main() {
	clickButtonSpec := core.Tool{
		Name: "clickButton",
		Desc: "Call this tool when the user asks you to click a button",
		Params: map[string]core.ToolParam{
			"index": {
				Type: core.JSTNumber,
				Desc: "The index of the button to click (0-based)",
			},
		},
	}
	clickButtonFunc := func(ctx context.Context, args struct {
		Index int `json:"index"`
	}) (string, error) {
		return fmt.Sprintf("Clicked button %d", args.Index), nil
	}
	clickButtonTool := agg.NewTool(clickButtonFunc, clickButtonSpec)

	store := agg.NewEphemeralStore()

	model := openai.NewModel(openai.GPT51, "low")
	// model := anthropic.NewModel(anthropic.Sonnet, 2048, 1024, true)

	agent := agg.NewAgent(
		"You are a helpful assistant.",
		model,
		&store,
		[]agg.Tool{clickButtonTool},
	)

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

	u := store.Usage("123")
	printUsage(u)
}

func printUsage(u core.Usage) {
	fmt.Printf("\n\033[33;1mUsage:\033[0m\n")
	fmt.Printf("  \033[33mInput:\033[0m %d\n", u.Input)
	fmt.Printf("    \033[33mCached:\033[0m %d\n", u.Cached)
	fmt.Printf("  \033[33mOutput:\033[0m %d\n", u.Output)
	fmt.Printf("  \033[33;1mCost:\033[0m $%.3f\n", float64(u.Cost)/1_000_000_000)
}
