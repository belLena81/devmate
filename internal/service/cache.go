package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Cache is a simple key-value store for LLM responses.
// Keys are opaque strings (typically hex-encoded hashes).
// Values are the raw LLM response text.
type Cache interface {
	// Get returns the cached value and true, or ("", false) on a miss.
	Get(key string) (string, bool)
	// Set stores a value. Only called on successful LLM responses — never on errors.
	Set(key, value string) error
	// Clear removes all cached entries.
	Clear() error
}

// DiskCache stores each cache entry as a file in a directory.
// The filename is the cache key, which must be a valid filename component
// (no path separators or null bytes). In practice all keys are lowercase
// hex-encoded SHA-256 digests produced by buildCacheKey.
//
// Location follows the XDG cache convention: ~/.cache/devmate/
type DiskCache struct {
	dir string
}

// NewDiskCache returns a DiskCache rooted at dir.
// The directory is created lazily on the first Set call.
func NewDiskCache(dir string) *DiskCache {
	return &DiskCache{dir: dir}
}

// validCacheKey returns an error if key cannot be used as a plain filename
// component. The check is intentionally strict: only printable ASCII that
// cannot confuse filepath.Join or the OS is allowed. All keys produced by
// buildCacheKey (hex SHA-256) satisfy this constraint trivially.
func validCacheKey(key string) error {
	if key == "" {
		return fmt.Errorf("cache key must not be empty")
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		// Reject path separators, null bytes, and non-printable ASCII.
		if c == '/' || c == '\\' || c == 0 || c < 0x20 || c == 0x7f {
			return fmt.Errorf("cache key contains invalid character %q at index %d", rune(c), i)
		}
	}
	// Guard against dot-relative paths (".", "..") that filepath.Join would
	// resolve to the cache directory itself or its parent.
	if key == "." || key == ".." {
		return fmt.Errorf("cache key %q is a reserved path component", key)
	}
	return nil
}

func (c *DiskCache) Get(key string) (string, bool) {
	if err := validCacheKey(key); err != nil {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(c.dir, key))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func (c *DiskCache) Set(key, value string) error {
	if err := validCacheKey(key); err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, key), []byte(value), 0o644)
}

func (c *DiskCache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return nil // nothing to clear
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(c.dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// CacheEntry holds metadata for a single cached response.
// The Key is the opaque hex-encoded SHA-256 digest that identifies the entry.
// SizeBytes is the byte length of the cached value.
// ModTime is when the entry was last written (i.e. when the LLM responded).
type CacheEntry struct {
	Key       string
	SizeBytes int64
	ModTime   time.Time
}

// CacheInspector is an optional capability for Cache implementations that
// support listing all stored entries. It is separate from Cache so that the
// minimal Cache interface stays small and NoopCache stays trivial.
type CacheInspector interface {
	// Stat returns metadata for every entry currently in the cache,
	// sorted by modification time, newest first. An empty cache returns
	// ([]CacheEntry{}, nil) — never (nil, nil).
	Stat() ([]CacheEntry, error)
}

// Stat implements CacheInspector for DiskCache.
// It reads directory entries without loading file contents, so it is cheap
// even for large cached values.
func (c *DiskCache) Stat() ([]CacheEntry, error) {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return []CacheEntry{}, nil // empty cache is not an error
	}
	if err != nil {
		return nil, err
	}

	result := make([]CacheEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue // the cache dir should only contain flat files, but be safe
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("cache stat %q: %w", e.Name(), err)
		}
		result = append(result, CacheEntry{
			Key:       e.Name(),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
		})
	}

	// Sort newest first so the most recently used entries appear at the top.
	sort.Slice(result, func(i, j int) bool {
		return result[i].ModTime.After(result[j].ModTime)
	})

	return result, nil
}

// Stat on NoopCache always returns an empty list; there is nothing to inspect.
func (NoopCache) Stat() ([]CacheEntry, error) { return []CacheEntry{}, nil }

// buildCacheKey computes a cache key from a template fingerprint plus a
// variable number of input fields. It uses length-prefixed encoding so that
// adjacent fields with different boundaries cannot produce the same hash —
// e.g. ("feat", "fix") ≠ ("fe", "atfix").
//
// Key composition: SHA256(tmplHash || len(f0) || f0 || len(f1) || f1 || ...)
//
// This means:
//   - Editing a template invalidates all entries for that command.
//   - Switching models invalidates all entries.
//   - Any change to git output or options invalidates the specific entry.
func buildCacheKey(tmplHash [32]byte, fields ...string) string {
	h := sha256.New()
	h.Write(tmplHash[:])
	for _, f := range fields {
		// Write 8-byte little-endian length prefix, then the field content.
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(len(f)))
		h.Write(buf)
		io.WriteString(h, f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Template fingerprints — computed once at startup from the embedded template
// strings. Changing any template file changes its hash, which changes every
// cache key for that command, automatically invalidating stale entries.
var (
	commitTmplHash = sha256.Sum256([]byte(commitTmpl))
	branchTmplHash = sha256.Sum256([]byte(branchTmpl))
	prTmplHash     = sha256.Sum256([]byte(prTmpl))
)

// commitCacheKey builds the cache key for a DraftMessage call.
// Inputs: model, raw diff, type override, mode, explain flag.
func commitCacheKey(model, binaryHash, diff, typeStr, modeStr string, explain bool) string {
	return buildCacheKey(commitTmplHash, model, binaryHash, diff, typeStr, modeStr, boolStr(explain))
}

// branchCacheKey builds the cache key for a DraftBranchName call.
// Inputs: model, task description, type override, mode, explain flag.
func branchCacheKey(model, binaryHash, task, typeStr, modeStr string, explain bool) string {
	return buildCacheKey(branchTmplHash, model, binaryHash, task, typeStr, modeStr, boolStr(explain))
}

// prCacheKey builds the cache key for a DraftPrDescription call.
// Inputs: model, binaryHash, commit messages (joined), type override, mode, explain flag.
func prCacheKey(model, binaryHash string, commits []string, typeStr, modeStr string, explain bool) string {
	return buildCacheKey(prTmplHash, model, binaryHash, strings.Join(commits, "\n"), typeStr, modeStr, boolStr(explain))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// BinaryHash returns a hex SHA256 of the running executable. It is computed
// once and cached. If the executable cannot be read (e.g. in tests using a
// fake path) it returns an empty string, which is safe — it simply means the
// binary version is not included in the cache key for that run.
func BinaryHash() string {
	return binaryHashOnce()
}

var binaryHashOnce = sync.OnceValue(func() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	// Resolve symlinks so `go install`-style updates are detected correctly.
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
})
