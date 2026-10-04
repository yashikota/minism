package memstore

import (
	"errors"
	"slices"
	"testing"
)

func TestVersioningAndSoftDelete(t *testing.T) {
	s := New[string]()

	if _, err := s.Create("a", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("a", "x"); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if v, err := s.Append("a", "v2"); err != nil || v.Num != 2 {
		t.Fatalf("append: %v %v", v, err)
	}
	if v, _ := s.Latest("a"); v.Value != "v2" {
		t.Fatalf("latest = %v", v)
	}
	if v, _ := s.At("a", 1); v.Value != "v1" {
		t.Fatalf("at 1 = %v", v)
	}
	if _, err := s.At("a", 3); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Latest("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted entry visible: %v", err)
	}
	if _, err := s.Append("a", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("append to deleted: %v", err)
	}
	// A deleted key can be created afresh, restarting at version 1.
	if v, err := s.Create("a", "again"); err != nil || v.Num != 1 {
		t.Fatalf("recreate: %v %v", v, err)
	}
}

func TestKeysSortedAndLive(t *testing.T) {
	s := New[int]()
	s.Put("b", 1)
	s.Put("a", 1)
	s.Put("c", 1)
	_ = s.Delete("b")
	if got := s.Keys(); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("keys = %v", got)
	}
	if s.Len() != 2 {
		t.Fatalf("len = %d", s.Len())
	}
}
