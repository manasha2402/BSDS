package main

import "sync"

// Entry holds a value and a logical version number.
// The version is incremented on every write so that
// quorum reads can pick the most recent value.
type Entry struct {
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

// Store is a thread-safe in-memory key-value store.
type Store struct {
	mu   sync.RWMutex
	data map[string]Entry
}

func NewStore() *Store {
	return &Store{data: make(map[string]Entry)}
}

// Set writes a key-value pair with an explicit version.
// The caller (leader or follower) controls the version number
// so that all nodes stay in sync.
func (s *Store) Set(key, value string, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = Entry{Value: value, Version: version}
}

// Get returns the Entry and true if the key exists, otherwise false.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e, ok
}

// NextVersion returns current version + 1 for a key,
// or 1 if the key doesn't exist yet.
// Only the Leader calls this to generate a new version number.
func (s *Store) NextVersion(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.data[key]; ok {
		return e.Version + 1
	}
	return 1
}
