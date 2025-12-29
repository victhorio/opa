package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/victhorio/opa/agg"
	"github.com/victhorio/opa/agg/core"
	"github.com/victhorio/opa/agg/openai"
	"github.com/victhorio/opa/obsidian"
	"github.com/victhorio/opa/prompts"
)

func loadToolSpec(name string) core.Tool {
	spec, err := prompts.LoadToolSpec(name)
	if err != nil {
		panic(err)
	}
	return spec
}

func createReadNoteTool(vault *obsidian.Vault) agg.Tool {
	spec := loadToolSpec("read_note")

	wrapper := func(
		ctx context.Context,
		args struct {
			NoteName string `json:"note_name"`
		},
	) (string, error) {
		note, err := vault.ReadNote(args.NoteName)
		if err != nil {
			return fmt.Sprintf("<error>Failed to read note %s: %s</error>", args.NoteName, err.Error()), nil
		}

		return note, nil
	}

	return agg.NewTool(wrapper, spec)
}

func createSmartReadNoteTool(vault *obsidian.Vault, client *http.Client) agg.Tool {
	if client == nil {
		client = http.DefaultClient
	}

	model := openai.NewModel(openai.GPT5Mini, "low")
	spec := loadToolSpec("smart_read_note")

	wrapper := func(
		ctx context.Context,
		args struct {
			NoteName string `json:"note_name"`
			Prompt   string `json:"prompt"`
		},
	) (string, error) {
		note, err := vault.ReadNote(args.NoteName)
		if err != nil {
			return fmt.Sprintf("<error>Failed to read note %s: %s</error>", args.NoteName, err.Error()), nil
		}

		sysPrompt := strings.NewReplacer("{note}", note).Replace(prompts.SmartReadNotePrompt)
		msgs := []*core.Msg{
			core.NewMsgContent("system", sysPrompt),
			core.NewMsgContent("user", args.Prompt),
		}

		stream, err := model.OpenStream(ctx, client, msgs, []core.Tool{}, core.StreamCfg{})
		if err != nil {
			return fmt.Sprintf("<error>Failed to send message to LLM: %s</error>", err.Error()), nil
		}

		ch := make(chan core.Event, 1)
		go stream.Consume(ctx, ch)

		var resp *core.Response
		for event := range ch {
			switch event.Type {
			case core.EvResp:
				resp = &event.Response
			case core.EvError:
				return fmt.Sprintf("<error>Failed to read message from LLM: %s</error>", event.Err.Error()), nil
			}
		}

		if resp == nil {
			return "", fmt.Errorf("left channel without receiving a response, something went wrong")
		}

		msg := resp.Messages[len(resp.Messages)-1]
		content, ok := msg.AsContent()
		if !ok {
			return "", fmt.Errorf("expected content message, got %d", msg.Type)
		}

		return content.Text, nil
	}

	return agg.NewTool(wrapper, spec)
}

func createListDirTool(vault *obsidian.Vault) agg.Tool {
	spec := loadToolSpec("list_dir")

	wrapper := func(
		ctx context.Context,
		args struct {
			SubPath string `json:"sub_path"`
		},
	) (string, error) {
		items, err := vault.ListDir(args.SubPath)
		if err != nil {
			return fmt.Sprintf("<error>Failed to list directory %s: %s</error>", args.SubPath, err.Error()), nil
		}

		return strings.Join(items, "\n"), nil
	}

	return agg.NewTool(wrapper, spec)
}

func createRipGrepTool(vault *obsidian.Vault) agg.Tool {
	spec := loadToolSpec("rip_grep")

	wrapper := func(
		ctx context.Context,
		args struct {
			Pattern       string `json:"pattern"`
			Folder        string `json:"folder"`
			CaseSensitive bool   `json:"case_sensitive"`
		},
	) (string, error) {
		matches, err := vault.RipGrep(args.Pattern, args.Folder, args.CaseSensitive)
		if err != nil {
			return fmt.Sprintf("<error>Failed to search vault for pattern %s: %s</error>", args.Pattern, err.Error()), nil
		}

		var sb strings.Builder

		for _, match := range matches {
			fmt.Fprintf(&sb, "NOTE %s\n", match.NoteName)
			for _, line := range match.MatchedLines {
				fmt.Fprintf(&sb, "LINE %s\n", line)
			}
			sb.WriteString("\n")
		}

		ret := sb.String()
		if ret == "" {
			return "<error>No matches found</error>", nil
		}

		return ret, nil
	}

	return agg.NewTool(wrapper, spec)
}

func createSemanticSearchTool(vault *obsidian.Vault) agg.Tool {
	spec := loadToolSpec("semantic_search")

	wrapper := func(
		ctx context.Context,
		args struct {
			QueryText string `json:"query_text"`
			K         int    `json:"k"`
		},
	) (string, error) {
		matches, err := vault.SemanticSearch(args.QueryText, args.K)
		if err != nil {
			return fmt.Sprintf("<error>Failed to perform semantic search for query '%s': %s</error>", args.QueryText, err.Error()), nil
		}

		var sb strings.Builder

		for i, match := range matches {
			sb.WriteString(fmt.Sprintf("%d. %s (score: %.4f)\n", i+1, match.Name, match.Score))
		}

		return sb.String(), nil
	}

	return agg.NewTool(wrapper, spec)
}
