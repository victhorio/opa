package openai

import "github.com/victhorio/opa/agg/com"

type Event struct {
	Type     EventType
	Delta    string
	Response com.Response
	Call     com.ToolCall
	Err      error
}

type EventType int

const (
	ETUninitialized EventType = iota
	ETDeltaReasoning
	ETDelta
	ETResponse
	ETToolCall
	ETError
)

func newEventDelta(delta string) Event {
	return Event{
		Type:  ETDelta,
		Delta: delta,
	}
}

func newEventDeltaReasoning(delta string) Event {
	return Event{
		Type:  ETDeltaReasoning,
		Delta: delta,
	}
}

func newEventResp(response com.Response) Event {
	return Event{
		Type:     ETResponse,
		Response: response,
	}
}

func newEventToolCall(toolCall com.ToolCall) Event {
	return Event{
		Type: ETToolCall,
		Call: toolCall,
	}
}

func newEventError(err error) Event {
	return Event{
		Type: ETError,
		Err:  err,
	}
}
