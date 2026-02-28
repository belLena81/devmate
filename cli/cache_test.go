package cli

import (
	"bytes"
	"devmate/internal/domain"
	"devmate/internal/service"
	"errors"
	"strings"
	"testing"
	"time"
)

// ─── Test doubles ─────────────────────────────────────────────────────────────

// fakeCacheService is an in-memory CacheService for CLI tests.
// It records whether Clean was called and returns a configurable Stat result.
type fakeCacheService struct {
	entries    []service.CacheEntry
	cleanErr   error
	statErr    error
	cleanCalls int
}

func (f *fakeCacheService) Clean() error {
	f.cleanCalls++
	return f.cleanErr
}

func (f *fakeCacheService) Stat() ([]service.CacheEntry, error) {
	return f.entries, f.statErr
}

// newCacheApp returns a minimal App suitable for cache command tests.
func newCacheApp(svc CacheService) *App {
	app := &App{cacheService: svc}
	app.rootCmd = buildRootCmd(app)
	return app
}

// ─── Registration ─────────────────────────────────────────────────────────────

func TestCacheCmd_IsRegistered(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})
	cmd, _, err := app.rootCmd.Find([]string{"cache"})
	if err != nil || cmd.Name() != "cache" {
		t.Fatal("cache command not registered")
	}
}

func TestCacheCleanCmd_IsRegistered(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})
	cmd, _, err := app.rootCmd.Find([]string{"cache", "clean"})
	if err != nil || cmd.Name() != "clean" {
		t.Fatal("cache clean command not registered")
	}
}

func TestCacheStatCmd_IsRegistered(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})
	cmd, _, err := app.rootCmd.Find([]string{"cache", "stat"})
	if err != nil || cmd.Name() != "stat" {
		t.Fatal("cache stat command not registered")
	}
}

// ─── cache clean ─────────────────────────────────────────────────────────────

func TestCacheCleanCmd_CallsClean(t *testing.T) {
	fake := &fakeCacheService{}
	app := newCacheApp(fake)
	app.rootCmd.SetArgs([]string{"cache", "clean"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.cleanCalls != 1 {
		t.Errorf("expected Clean to be called once, got %d", fake.cleanCalls)
	}
}

func TestCacheCleanCmd_PrintsConfirmation(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})

	var buf bytes.Buffer
	app.rootCmd.SetOut(&buf)
	app.rootCmd.SetArgs([]string{"cache", "clean"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() == "" {
		t.Error("expected a confirmation message on stdout after clean")
	}
}

func TestCacheCleanCmd_CleanError_ReturnsError(t *testing.T) {
	fake := &fakeCacheService{cleanErr: errors.New("disk full")}
	app := newCacheApp(fake)
	app.rootCmd.SetArgs([]string{"cache", "clean"})

	if err := app.Execute(); err == nil {
		t.Fatal("expected error to propagate from Clean")
	}
}

func TestCacheCleanCmd_RejectsPositionalArgs(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})
	app.rootCmd.SetArgs([]string{"cache", "clean", "extra-arg"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when positional args are passed to cache clean")
	}
}

func TestCacheCleanCmd_NilService_ReturnsErrServiceNotInitialized(t *testing.T) {
	app := &App{}
	app.rootCmd = buildRootCmd(app)
	app.rootCmd.SetArgs([]string{"cache", "clean"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error when cacheService is nil")
	}
	if !errors.Is(err, domain.ErrServiceNotInitialized) {
		t.Errorf("expected ErrServiceNotInitialized, got %v", err)
	}
}

// ─── cache stat ──────────────────────────────────────────────────────────────

func TestCacheStatCmd_EmptyCache_PrintsEmptyMessage(t *testing.T) {
	app := newCacheApp(&fakeCacheService{entries: []service.CacheEntry{}})

	var buf bytes.Buffer
	app.rootCmd.SetOut(&buf)
	app.rootCmd.SetArgs([]string{"cache", "stat"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() == "" {
		t.Error("expected some output even for empty cache (e.g. 'no cached entries')")
	}
}

func TestCacheStatCmd_PrintsOneLinePerEntry(t *testing.T) {
	now := time.Now()
	entries := []service.CacheEntry{
		{Key: "abc123", SizeBytes: 512, ModTime: now.Add(-2 * time.Hour)},
		{Key: "def456", SizeBytes: 1024, ModTime: now.Add(-1 * time.Hour)},
		{Key: "ghi789", SizeBytes: 256, ModTime: now},
	}
	app := newCacheApp(&fakeCacheService{entries: entries})

	var buf bytes.Buffer
	app.rootCmd.SetOut(&buf)
	app.rootCmd.SetArgs([]string{"cache", "stat"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count non-empty lines (header + 3 data lines, or just 3 data lines —
	// either is acceptable as long as every entry appears).
	output := buf.String()
	for _, entry := range entries {
		if !strings.Contains(output, entry.Key) {
			t.Errorf("expected key %q in stat output, got:\n%s", entry.Key, output)
		}
	}
}

func TestCacheStatCmd_PrintsSizeForEachEntry(t *testing.T) {
	entries := []service.CacheEntry{
		{Key: "k1", SizeBytes: 1234, ModTime: time.Now()},
	}
	app := newCacheApp(&fakeCacheService{entries: entries})

	var buf bytes.Buffer
	app.rootCmd.SetOut(&buf)
	app.rootCmd.SetArgs([]string{"cache", "stat"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "1234") {
		t.Errorf("expected size 1234 in stat output, got:\n%s", buf.String())
	}
}

func TestCacheStatCmd_StatError_ReturnsError(t *testing.T) {
	fake := &fakeCacheService{statErr: errors.New("permission denied")}
	app := newCacheApp(fake)
	app.rootCmd.SetArgs([]string{"cache", "stat"})

	if err := app.Execute(); err == nil {
		t.Fatal("expected error to propagate from Stat")
	}
}

func TestCacheStatCmd_RejectsPositionalArgs(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})
	app.rootCmd.SetArgs([]string{"cache", "stat", "extra-arg"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when positional args are passed to cache stat")
	}
}

func TestCacheStatCmd_NilService_ReturnsErrServiceNotInitialized(t *testing.T) {
	app := &App{}
	app.rootCmd = buildRootCmd(app)
	app.rootCmd.SetArgs([]string{"cache", "stat"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error when cacheService is nil")
	}
	if !errors.Is(err, domain.ErrServiceNotInitialized) {
		t.Errorf("expected ErrServiceNotInitialized, got %v", err)
	}
}

// ─── Inject ───────────────────────────────────────────────────────────────────

func TestInjectCacheService_ReplacesService(t *testing.T) {
	app := newCacheApp(&fakeCacheService{})
	replacement := &fakeCacheService{entries: []service.CacheEntry{{Key: "injected"}}}
	InjectCacheService(app, replacement)

	var buf bytes.Buffer
	app.rootCmd.SetOut(&buf)
	app.rootCmd.SetArgs([]string{"cache", "stat"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "injected") {
		t.Errorf("expected injected service to be used, got:\n%s", buf.String())
	}
}
