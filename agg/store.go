package agg

import "github.com/victhorio/opa/agg/core"

type Store interface {
	Messages(string) []*core.Msg
	Usage(string) core.Usage
	Extend(string, []*core.Msg, core.Usage) error
}
