// Package memstore is a small versioned in-memory key/value store shared by
// the provider fakes. It models what secret managers have in common: a named
// entry holding an append-only list of versions, with soft deletion.
package memstore

import (
	"errors"
	"sort"
	"sync"
)

var (
	ErrNotFound = errors.New("memstore: not found")
	ErrExists   = errors.New("memstore: already exists")
)

// Version is one immutable revision of an entry. Num starts at 1.
type Version[V any] struct {
	Num   int
	Value V
}

type entry[V any] struct {
	versions []Version[V]
	deleted  bool
}

// Store is safe for concurrent use.
type Store[V any] struct {
	mu    sync.RWMutex
	items map[string]*entry[V]
}

func New[V any]() *Store[V] {
	return &Store[V]{items: map[string]*entry[V]{}}
}

// Create adds a new entry with version 1. It fails with ErrExists if the key
// is live; a soft-deleted key is replaced.
func (s *Store[V]) Create(key string, v V) (Version[V], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.items[key]; ok && !e.deleted {
		return Version[V]{}, ErrExists
	}
	ver := Version[V]{Num: 1, Value: v}
	s.items[key] = &entry[V]{versions: []Version[V]{ver}}
	return ver, nil
}

// Put appends a new version, creating the entry if needed.
func (s *Store[V]) Put(key string, v V) Version[V] {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[key]
	if !ok || e.deleted {
		e = &entry[V]{}
		s.items[key] = e
	}
	ver := Version[V]{Num: len(e.versions) + 1, Value: v}
	e.versions = append(e.versions, ver)
	return ver
}

// Append adds a version to an existing live entry (ErrNotFound otherwise).
func (s *Store[V]) Append(key string, v V) (Version[V], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[key]
	if !ok || e.deleted {
		return Version[V]{}, ErrNotFound
	}
	ver := Version[V]{Num: len(e.versions) + 1, Value: v}
	e.versions = append(e.versions, ver)
	return ver, nil
}

// Latest returns the newest version of a live entry.
func (s *Store[V]) Latest(key string) (Version[V], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.items[key]
	if !ok || e.deleted {
		return Version[V]{}, ErrNotFound
	}
	return e.versions[len(e.versions)-1], nil
}

// At returns a specific version (1-based) of a live entry.
func (s *Store[V]) At(key string, num int) (Version[V], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.items[key]
	if !ok || e.deleted || num < 1 || num > len(e.versions) {
		return Version[V]{}, ErrNotFound
	}
	return e.versions[num-1], nil
}

// Versions returns all versions of a live entry, oldest first.
func (s *Store[V]) Versions(key string) ([]Version[V], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.items[key]
	if !ok || e.deleted {
		return nil, ErrNotFound
	}
	return append([]Version[V](nil), e.versions...), nil
}

// Delete soft-deletes an entry.
func (s *Store[V]) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[key]
	if !ok || e.deleted {
		return ErrNotFound
	}
	e.deleted = true
	return nil
}

// Keys lists live keys in sorted order.
func (s *Store[V]) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.items))
	for k, e := range s.items {
		if !e.deleted {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// Len counts live entries.
func (s *Store[V]) Len() int { return len(s.Keys()) }

// Update mutates one version (1-based) of a live entry in place.
func (s *Store[V]) Update(key string, num int, fn func(*V)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.items[key]
	if !ok || e.deleted || num < 1 || num > len(e.versions) {
		return ErrNotFound
	}
	fn(&e.versions[num-1].Value)
	return nil
}
