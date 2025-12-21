package core

type Event struct {
	Type     EventType
	Delta    string
	Response Response
	Call     ToolCall
	Err      error
}

type EventType int

const (
	EvUnk EventType = iota
	EvDeltaReason
	EvDelta
	EvResp
	EvToolCall
	EvError
)

func NewEvDelta(delta string) Event {
	return Event{
		Type:  EvDelta,
		Delta: delta,
	}
}

func NewEvDeltaReason(delta string) Event {
	return Event{
		Type:  EvDeltaReason,
		Delta: delta,
	}
}

func NewEvResp(response Response) Event {
	return Event{
		Type:     EvResp,
		Response: response,
	}
}

func NewEvToolCall(toolCall ToolCall) Event {
	return Event{
		Type: EvToolCall,
		Call: toolCall,
	}
}

func NewEvError(err error) Event {
	return Event{
		Type: EvError,
		Err:  err,
	}
}
