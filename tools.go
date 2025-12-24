package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/victhorio/opa/agg"
	"github.com/victhorio/opa/agg/core"
	"github.com/victhorio/opa/obsidian"
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
