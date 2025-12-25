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

func createReadNoteTool(vault *obsidian.Vault) agg.Tool {
	spec := core.Tool{
		Name: "ReadNote",
		Desc: `Use this function to read a note from the vault. If the note exists, it will be
returned wrapped in XML tags <note> and </note>. If the underlying function fails, it will instead
return an error message wrapped in XML tags <error> and </error>.`,
		Params: map[string]core.ToolParam{
			"note_name": {
				Type: core.JSTString,
				Desc: `The name of the note to read, written in the same way as notes are referenced
in the vault. For example, to read the ./AGENTS.md file, use note_name='AGENTS'; to read the note in
'./0 Daily/2025-10-11.md', use note_name='2025-10-11'.`,
			},
		},
	}

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

	spec := core.Tool{
		Name: "SmartReadNote",
		Desc: `Use this function to have a separate LLM read a note from the vault and return
relevant content to you based on the 'prompt' you provide. The idea is that you won't need to
overload your context with a lot of potentially irrelevant content if you only need something
specific about a given note.`,
		Params: map[string]core.ToolParam{
			"note_name": {
				Type: core.JSTString,
				Desc: `The name of the note to read, written in the same way as notes are referenced
in the vault. For example, to read the ./AGENTS.md file, use note_name='AGENTS'; to read the note in
'./0 Daily/2025-10-11.md', use note_name='2025-10-11'.`,
			},
			"prompt": {
				Type: core.JSTString,
				Desc: `The prompt passed to the LLM that will read the note. This prompt indicates
what it should return to you. For example, 'Does this note mention X?' or 'Does the note contain
benchmark results for Y? If so, output them.'`,
			},
		},
	}

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
	spec := core.Tool{
		Name: "ListDir",
		Desc: `Use this function to list the contents of a directory in the vault. You will be
returned a string with the contents of the directory, each separated by a newline. Directories will
have a '/' suffix, whereas regular files will not.`,
		Params: map[string]core.ToolParam{
			"sub_path": {
				Type: core.JSTString,
				Desc: `The path of the directory to list, considering that the vault path will
already be prepended. For example, if you want to list the contents of the root, use '.', and if
you want to list the contents of a 'folder' at the root, use 'folder'.`,
			},
		},
	}

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
	spec := core.Tool{
		Name: "RipGrep",
		Desc: `Use this function to search the vault for a specific regex pattern using ripgrep.
You will be returned a string with the results of the search, including the names of the files that
had a match, along with a snippet of the match from the note. Only valid vault notes will be included
in the search.

Use 'folder' to limit the search to a specific folder. Set it to '.' to search the entire vault.`,
		Params: map[string]core.ToolParam{
			"pattern": {
				Type: core.JSTString,
				Desc: `The regex pattern used to search the contents of the vault notes.`,
			},
			"folder": {
				Type: core.JSTString,
				Desc: `The folder to search in. Set it to '.' to search the entire vault.`,
			},
			"case_sensitive": {
				Type: core.JSTBoolean,
				Desc: `Whether to search in a case-sensitive manner.`,
			},
		},
	}

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
			sb.WriteString(fmt.Sprintf("NOTE %s\n", match.NoteName))
			for _, line := range match.MatchedLines {
				sb.WriteString(fmt.Sprintf("LINE %s\n", line))
			}
			sb.WriteString("\n")
		}

		return sb.String(), nil
	}

	return agg.NewTool(wrapper, spec)
}

func createSemanticSearchTool(vault *obsidian.Vault) agg.Tool {
	spec := core.Tool{
		Name: "SemanticSearch",
		Desc: `Use this function to search for note names in the vault using semantic search. The function
will return the top K note names whose content is the most similar to the "query text".`,
		Params: map[string]core.ToolParam{
			"query_text": {
				Type: core.JSTString,
				Desc: `The text to search for in the vault. Usually a very brief natural sounding sentence that describes the type of content you're looking for.`,
			},
			"k": {
				Type: core.JSTNumber,
				Desc: `The number of note names to return.`,
			},
		},
	}

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
