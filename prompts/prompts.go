package prompts

import (
	_ "embed"
)

//go:embed opa.txt
var OpaSysPrompt string

//go:embed smart_read_note.txt
var SmartReadNotePrompt string
