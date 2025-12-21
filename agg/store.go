package agg

import "github.com/victhorio/opa/agg/core"

type Store interface {
	Messages(string) []core.Message
	Usage(string) core.Usage
	Extend(string, []core.Message, core.Usage) error
}
