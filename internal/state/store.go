package state

import (
	"sync"
	"time"
)

// Status represents the high-level state of a spec.
type Status string

const (
	StatusHealthy Status = "healthy"
	StatusFailing Status = "failing"
)

// SpecState tracks cycle counters and status for one spec.
type SpecState struct {
	Status               Status
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastCycleStartedAt   time.Time
	LastCycleAt          time.Time
}

// Store defines state persistence behavior.
type Store interface {
	Get(specName string) (SpecState, bool)
	Set(specName string, specState SpecState)
}

// InMemoryStore keeps states in memory.
type InMemoryStore struct {
	mu     sync.RWMutex
	states map[string]SpecState
}

// NewInMemoryStore creates an in-memory state store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		states: make(map[string]SpecState),
	}
}

// Get returns state for spec name, if any.
func (s *InMemoryStore) Get(specName string) (SpecState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.states[specName]
	return state, ok
}

// Set stores state for spec name.
func (s *InMemoryStore) Set(specName string, specState SpecState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.states[specName] = specState
}
