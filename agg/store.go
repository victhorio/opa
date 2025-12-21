package agg

import "github.com/victhorio/opa/agg/com"

type Store interface {
	Messages(string) []com.Message
	Usage(string) com.Usage
	Extend(string, []com.Message, com.Usage) error
}
