package service

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestCache(t *testing.T) *DiskCache {
	t.Helper()
	return NewDiskCache(t.TempDir())
}

func TestDiskCache_MissReturnsNotFound(t *testing.T) {
	_, ok := newTestCache(t).Get("nonexistent")
	if ok {
		t.Error("expected miss for unknown key")
	}
}

func TestDiskCache_SetThenGet_ReturnsValue(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("k", "the response"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	val, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if val != "the response" {
		t.Errorf("expected %q, got %q", "the response", val)
	}
}

func TestDiskCache_SetOverwrites(t *testing.T) {
	c := newTestCache(t)
	c.Set("k", "first")
	c.Set("k", "second")
	val, _ := c.Get("k")
	if val != "second" {
		t.Errorf("expected %q after overwrite, got %q", "second", val)
	}
}

func TestDiskCache_TwoKeys_Independent(t *testing.T) {
	c := newTestCache(t)
	c.Set("A", "alpha")
	c.Set("B", "beta")
	a, _ := c.Get("A")
	b, _ := c.Get("B")
	if a != "alpha" || b != "beta" {
		t.Errorf("cache entries crossed: A=%q B=%q", a, b)
	}
}

func TestDiskCache_Clear_RemovesAllEntries(t *testing.T) {
	c := newTestCache(t)
	c.Set("k1", "v1")
	c.Set("k2", "v2")
	if err := c.Clear(); err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	if _, ok := c.Get("k1"); ok {
		t.Error("k1 should be gone after Clear")
	}
	if _, ok := c.Get("k2"); ok {
		t.Error("k2 should be gone after Clear")
	}
}

func TestDiskCache_Clear_EmptyCacheIsNoop(t *testing.T) {
	if err := newTestCache(t).Clear(); err != nil {
		t.Errorf("Clear on empty cache should not error: %v", err)
	}
}

func TestDiskCache_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	NewDiskCache(dir).Set("key", "hello")

	// Second instance at the same dir — simulates next process run.
	val, ok := NewDiskCache(dir).Get("key")
	if !ok {
		t.Fatal("expected hit from second instance")
	}
	if val != "hello" {
		t.Errorf("expected %q, got %q", "hello", val)
	}
}

func TestDiskCache_CreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	if err := NewDiskCache(dir).Set("k", "v"); err != nil {
		t.Fatalf("Set should create missing dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("cache dir should exist after Set: %v", err)
	}
}
