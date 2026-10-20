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

// Cache is a key-value store for LLM responses.
type Cache interface {
	Get(key string) (string, bool)
	// Set is only called for successful LLM responses, never on errors.
	Set(key, value string) error
	Clear() error
}

// DiskCache stores each entry as a file named by its key.
type DiskCache struct {
	dir string
}

// NewDiskCache returns a DiskCache rooted at dir; the directory is created lazily on first Set.
func NewDiskCache(dir string) *DiskCache {
	return &DiskCache{dir: dir}
}

// validCacheKey returns an error if key cannot be used as a plain filename component.
func validCacheKey(key string) error {
	if key == "" {
		return fmt.Errorf("cache key must not be empty")
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c == '/' || c == '\\' || c == 0 || c < 0x20 || c == 0x7f {
			return fmt.Errorf("cache key contains invalid character %q at index %d", rune(c), i)
		}
	}
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
		return nil
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

type CacheEntry struct {
	Key       string
	SizeBytes int64
	ModTime   time.Time
}

// CacheInspector is an optional Cache capability for listing stored entries.
type CacheInspector interface {
	// Stat returns every entry newest first; an empty cache returns a non-nil empty slice.
	Stat() ([]CacheEntry, error)
}

func (c *DiskCache) Stat() ([]CacheEntry, error) {
	entries, err := os.ReadDir(c.dir)
	if os.IsNotExist(err) {
		return []CacheEntry{}, nil
	}
	if err != nil {
		return nil, err
	}

	result := make([]CacheEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
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

	sort.Slice(result, func(i, j int) bool {
		return result[i].ModTime.After(result[j].ModTime)
	})

	return result, nil
}

func (NoopCache) Stat() ([]CacheEntry, error) { return []CacheEntry{}, nil }

// buildCacheKey hashes tmplHash and the length-prefixed fields, so shifted field boundaries cannot collide.
func buildCacheKey(tmplHash [32]byte, fields ...string) string {
	h := sha256.New()
	h.Write(tmplHash[:])
	for _, f := range fields {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(len(f)))
		h.Write(buf)
		io.WriteString(h, f)
	}
	return hex.EncodeToString(h.Sum(nil))
}

var (
	commitTmplHash = sha256.Sum256([]byte(commitTmpl))
	branchTmplHash = sha256.Sum256([]byte(branchTmpl))
	prTmplHash     = sha256.Sum256([]byte(prTmpl))
)

func commitCacheKey(model, binaryHash, diff, typeStr, modeStr string, explain bool) string {
	return buildCacheKey(commitTmplHash, model, binaryHash, diff, typeStr, modeStr, boolStr(explain))
}

func branchCacheKey(model, binaryHash, task, typeStr, modeStr string, explain bool) string {
	return buildCacheKey(branchTmplHash, model, binaryHash, task, typeStr, modeStr, boolStr(explain))
}

func prCacheKey(model, binaryHash string, commits []string, typeStr, modeStr string, explain bool) string {
	return buildCacheKey(prTmplHash, model, binaryHash, strings.Join(commits, "\n"), typeStr, modeStr, boolStr(explain))
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// BinaryHash returns the hex SHA-256 of the running executable, or "" if it cannot be read.
func BinaryHash() string {
	return binaryHashOnce()
}

var binaryHashOnce = sync.OnceValue(func() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
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
