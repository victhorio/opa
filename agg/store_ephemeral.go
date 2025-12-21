package agg

import "github.com/victhorio/opa/agg/com"

type EphemeralStore struct {
	m map[string][]com.Message
	u map[string]com.Usage
}

func NewEphemeralStore() EphemeralStore {
	return EphemeralStore{
		m: make(map[string][]com.Message),
		u: make(map[string]com.Usage),
	}
}

func (s EphemeralStore) Messages(key string) []com.Message {
	m, ok := s.m[key]
	if !ok {
		return []com.Message{}
	}
	return m
}

func (s EphemeralStore) Usage(key string) com.Usage {
	u, ok := s.u[key]
	if !ok {
		return com.Usage{}
	}
	return u
}

func (s *EphemeralStore) Extend(
	key string,
	msgs []com.Message,
	usage com.Usage,
) error {
	m := s.Messages(key)
	m = append(m, msgs...)
	s.m[key] = m

	u := s.Usage(key)
	u.Inc(usage)
	s.u[key] = u

	return nil
}
