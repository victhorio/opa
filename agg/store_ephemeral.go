package agg

import "github.com/victhorio/opa/agg/core"

type EphemeralStore struct {
	m map[string][]*core.Msg
	u map[string]core.Usage
}

func NewEphemeralStore() EphemeralStore {
	return EphemeralStore{
		m: make(map[string][]*core.Msg),
		u: make(map[string]core.Usage),
	}
}

func (s EphemeralStore) Messages(key string) []*core.Msg {
	m, ok := s.m[key]
	if !ok {
		return []*core.Msg{}
	}
	return m
}

func (s EphemeralStore) Usage(key string) core.Usage {
	u, ok := s.u[key]
	if !ok {
		return core.Usage{}
	}
	return u
}

func (s *EphemeralStore) Extend(
	key string,
	msgs []*core.Msg,
	usage core.Usage,
) error {
	m := s.Messages(key)
	m = append(m, msgs...)
	s.m[key] = m

	u := s.Usage(key)
	u.Inc(usage)
	s.u[key] = u

	return nil
}
