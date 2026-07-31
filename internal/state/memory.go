package state

import "sync"

// MemoryStore keeps rename intentions for the lifetime of the process.
type MemoryStore struct {
	mutex   sync.Mutex
	intents []Intent
}

// NewMemoryStore creates an empty in-process store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

// Load returns a copy of the current intentions.
func (s *MemoryStore) Load() ([]Intent, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return append([]Intent(nil), s.intents...), nil
}

// Save replaces the current intentions with a copy of the snapshot.
func (s *MemoryStore) Save(intents []Intent) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.intents = append([]Intent(nil), intents...)
	return nil
}
