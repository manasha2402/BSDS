package main

import "sync"

// Entry holds a value and a logical version number.
// In the leaderless model, any node can receive a write,
// so version numbers are critical for detecting stale data.
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
// In the leaderless model the Write Coordinator generates
// the version and sends it to all other nodes — they just
// store what they receive, same as a follower in leader-follower.
func (s *Store) Set(key, value string, version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = Entry{Value: value, Version: version}
}

// Get returns the Entry and true if the key exists.
func (s *Store) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e, ok
}

// NextVersion returns the next logical version for a key.
// Any node acting as Write Coordinator calls this to stamp
// the write before propagating it to peers.
func (s *Store) NextVersion(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.data[key]; ok {
		return e.Version + 1
	}
	return 1
}
