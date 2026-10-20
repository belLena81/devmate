# devmate

A local-first CLI that drafts Git workflow text — commit messages, branch names, and PR descriptions — using a locally running LLM. It reads repository state and writes suggestions to stdout. It never executes Git mutations or applies AI output automatically.

## How it works

devmate reads your repository (staged diff or commit history), builds a structured prompt, sends it to a local Ollama instance, and prints the result. For large diffs or long commit histories it runs a map-reduce pipeline: chunks are summarised in parallel, summaries are hierarchically reduced, then a final synthesis prompt produces the output.

```
git diff / git log  →  chunker  →  LLM (parallel)  →  reducer  →  synthesiser  →  stdout
```

Nothing is written back to the repository.

## Commands

### `devmate commit`

Analyses staged changes (`git diff --cached`) and drafts a Conventional Commit message.

```sh
devmate commit
devmate commit --detailed
devmate commit --type fix
devmate commit --explain
devmate commit --no-cache
```

| Flag | Description |
|---|---|
| `-t, --type` | Override commit type: `feat`, `fix`, `chore`, `docs`, `refactor` |
| `--short` | Single-line subject only (default) |
| `--detailed` | Subject + body with bullet points |
| `--explain` | Append reasoning to the output |
| `--no-cache` | Bypass the cache and always call the LLM; overwrites the cached entry |

`--short` and `--detailed` are mutually exclusive.

Example output:

```
feat(auth): add token refresh mechanism

- Introduces refresh token rotation
- Updates middleware validation logic
- Adds unit tests for expiration edge cases
```

### `devmate branch <description>`

Drafts a branch name from a plain-text task description.

```sh
devmate branch "add retry logic to payment webhook handler"
devmate branch "fix login crash on empty password" --type fix
devmate branch "add retry logic to payment webhook handler" --no-cache
```

Output follows the `<type>/<slug>` convention:

```
feat/add-retry-logic-to-payment-webhook-handler
```

| Flag | Description |
|---|---|
| `-t, --type` | Override branch type |
| `--short` | Concise slug (default) |
| `--detailed` | Longer, more descriptive slug |
| `--explain` | Append reasoning to the output |
| `--no-cache` | Bypass the cache and always call the LLM; overwrites the cached entry |

### `devmate pr <source> <target>`

Drafts a PR title and description from `git log` between two branches.

```sh
devmate pr feat/payment-webhook-retry main
devmate pr feat/payment-webhook-retry main --detailed
devmate pr feat/payment-webhook-retry main --no-cache
```

| Flag | Description |
|---|---|
| `-t, --type` | Override PR type |
| `--short` | Title + bullet summary (default) |
| `--detailed` | Full description with technical decisions and risk notes |
| `--explain` | Append reasoning to the output |
| `--no-cache` | Bypass the cache and always call the LLM; overwrites the cached entry |

Output includes a title, summary, technical decisions, and risk/impact notes.

### `devmate cache`

Manages the local LLM response cache.

#### `devmate cache stat`

Lists every cached entry with its key, size in bytes, and last-written timestamp. Output is sorted newest-first.

```sh
devmate cache stat
```

```
KEY                                                               SIZE (B)  MODIFIED
a3f1c9e2b7d084561f2a3c4e5d6b7890abcdef1234567890abcdef12345678       512  2025-08-01 14:22:01
d94e2b1f9a3c0587e6d241a8f7b5c390fedcba9876543210fedcba9876543210      1024  2025-07-30 09:11:44
```

If the cache is empty, prints `No cached entries.`

#### `devmate cache clean`

Deletes all cached entries. The next request for any command will contact the LLM regardless of prior inputs.

```sh
devmate cache clean
```

```
Cache cleared.
```

## Installation

**From source:**

```sh
git clone https://github.com/belLena81/devmate
cd devmate
go build ./cmd/devmate
```

**Via `go install`:**

```sh
go install github.com/belLena81/devmate/cmd/devmate@latest
```

**Requirements:** Go 1.26+, Git 2.24+, [Ollama](https://ollama.com)

Build with a version string baked in:

```sh
go build -ldflags "-X devmate/cli.Version=v1.0.0" ./cmd/devmate
```

## Global flags

| Flag | Description |
|---|---|
| `--config <path>` | Config file to load (see Configuration for the default location) |
| `-v, --verbose` | Debug logging, including config resolution |
| `-q, --quiet` | Log errors only |
| `--version` | Print the version and exit |

`--help`, `--version` and `completion` work anywhere; only `commit`, `branch` and `pr` need a git repository, and only they contact the LLM.

Ctrl-C cancels in-flight LLM requests and exits with status 130. Errors are printed once to stderr as `Error: ...` and exit with status 1.

**Shell completion:**

```sh
source <(devmate completion bash)   # or zsh, fish, powershell
```

Completion also offers the valid values for `--type`.

## Configuration

Settings are resolved in this priority order: **env vars → config file → built-in defaults**.

The config file is looked up in this order:

1. `--config <path>` flag
2. `DEVMATE_CONFIG` env var
3. The per-user config directory: `~/.config/devmate/config.json` on Linux, `~/Library/Application Support/devmate/config.json` on macOS, `%AppData%\devmate\config.json` on Windows

A missing file is not an error. `config.example.json` in the repository root is a starting point.

**`config.json` (all fields optional — defaults shown):**

```json
{
  "ollama": {
    "base_url": "http://localhost:11434",
    "generate_path": "/api/generate",
    "model": "llama3.2:3b",
    "request_timeout_sec": 180,
    "max_response_bytes": 10485760
  },
  "service": {
    "chunk_threshold": 3000,
    "max_concurrency": 2,
    "max_retries": 0,
    "retry_base_delay_sec": 1
  },
  "cache": {
    "dir": ""
  },
  "log": {
    "level": "info"
  }
}
```

An empty `cache.dir` means the per-user cache directory (see Caching).

**Environment variables:**

| Variable | Overrides |
|---|---|
| `DEVMATE_CONFIG` | Config file path |
| `DEVMATE_OLLAMA_BASE_URL` | `ollama.base_url` |
| `DEVMATE_OLLAMA_GENERATE_PATH` | `ollama.generate_path` |
| `DEVMATE_OLLAMA_MODEL` | `ollama.model` |
| `DEVMATE_REQUEST_TIMEOUT_SEC` | `ollama.request_timeout_sec` |
| `DEVMATE_MAX_RESPONSE_BYTES` | `ollama.max_response_bytes` |
| `DEVMATE_CHUNK_THRESHOLD` | `service.chunk_threshold` |
| `DEVMATE_MAX_CONCURRENCY` | `service.max_concurrency` |
| `DEVMATE_MAX_RETRIES` | `service.max_retries` |
| `DEVMATE_RETRY_BASE_DELAY_SEC` | `service.retry_base_delay_sec` |
| `DEVMATE_CACHE_DIR` | `cache.dir` |
| `DEVMATE_LOG_LEVEL` | `log.level` (`debug`, `info`, `warn`, `error`) |

Integer variables are validated at startup: a non-integer value such as `DEVMATE_MAX_RETRIES=two` is reported as an error instead of being ignored. Empty values are ignored.

**Seeing config load progress:** run with `--verbose` (or `DEVMATE_LOG_LEVEL=debug`) to see which values came from the file, which from env vars, and the resolved result:

```
level=DEBUG component=config msg="config file loaded" path=/home/me/.config/devmate/config.json
level=DEBUG component=config msg="file override" key=ollama.model value=mistral:7b
level=DEBUG component=config msg="env override" key=DEVMATE_MAX_RETRIES value=3
level=DEBUG component=config msg="config resolved" ollama.base_url=http://localhost:11434 ...
```

## LLM pipeline

For diffs or commit histories that exceed `service.chunk_threshold` (default 3 000 bytes), devmate runs a multi-stage pipeline instead of sending everything in one prompt:

1. **Chunk** — the input is split on file boundaries and bin-packed so large files get their own chunks and small files are grouped to minimise LLM round-trips.
2. **Map** — each chunk is summarised independently via parallel LLM calls (up to `max_concurrency` at a time).
3. **Reduce** — summaries are iteratively condensed in groups until the combined size fits within the threshold.
4. **Synthesise** — a final prompt produces the structured output (commit message, branch name, or PR description).

This keeps every prompt within the model's context window regardless of repository size.

## Caching

Responses are cached in the per-user cache directory (`~/.cache/devmate` on Linux, `~/Library/Caches/devmate` on macOS, `%LocalAppData%\devmate` on Windows), overridable via `cache.dir` or `DEVMATE_CACHE_DIR`. Entries are written atomically, so concurrent runs never read a partial entry. Cache keys are derived from: model name, binary hash, git input (diff or commit list), and all option flags. Changing any of these produces a new key. Changing the binary invalidates all prior entries automatically.

Pass `--no-cache` to `commit`, `branch`, or `pr` to bypass the cache for a single run. The LLM is always called and its fresh response overwrites the existing cached entry, so subsequent ordinary calls immediately serve the updated value. Use this when you want a different suggestion for the same input without clearing the entire cache.

Use `devmate cache stat` to inspect the cache and `devmate cache clean` to wipe it.

## Retry behaviour

Every LLM call goes through a single retry site in the service layer. A failed call is retried up to `service.max_retries` times (default 0, i.e. one attempt) with exponential back-off starting at `service.retry_base_delay_sec` (default 1 s) and doubling each time.

Only transient failures are retried: connection errors, timeouts, and HTTP 408, 429 and 5xx responses. Client-side failures such as an unknown model (404) or a malformed request are reported immediately. Ctrl-C aborts the current attempt and any pending back-off.

## Project structure

```
cmd/devmate/          entry point — signal handling, config, dependency wiring
cli/                  Cobra commands, global flags, shared generate flags
  cache.go            cache clean and cache stat subcommands
internal/
  config/             configuration (defaults + JSON file + env vars)
  domain/             core interfaces and errors (LLM, GitClient, Progress)
  service/            orchestration — chunking, map-reduce, caching, retry
    cache.go          Cache interface, DiskCache, CacheInspector, NoopCache
  infra/
    git/              read-only Git runner (diff, log)
    llm/              Ollama HTTP client (single attempt per call)
    progress/         terminal spinner and plain-text reporter for non-TTY output
```

Dependency direction: `CLI → Service → Domain ← Infrastructure`. Infrastructure never imports service or CLI packages.

## Dependencies

| Module | Used for |
|---|---|
| `github.com/spf13/cobra` | Command tree, flags, help, shell completion |
| `github.com/caarlos0/env/v11` | Typed `DEVMATE_*` env var overlay via struct tags |
| `github.com/cenkalti/backoff/v4` | Exponential back-off for LLM retries |
| `golang.org/x/sync/errgroup` | Bounded worker pool for parallel chunk summarisation |
| `golang.org/x/term` | Terminal detection to choose spinner vs plain progress output |

## Testing

```sh
go test ./...
```

Test coverage spans: config loading and env-var validation, CLI option parsing, nil-service guards, service orchestration, map-reduce pipeline, chunker, cache key derivation, disk cache (including `Stat` and `Clear`), prompt templates, sanitisation, Git runner, Ollama client retry logic, and the progress spinner.

Tests use interface-based fakes — no real Git repository or Ollama instance is required.

## Design principles

- **Read-only** — devmate never runs `git commit`, `git push`, or any mutating Git command.
- **Local-first** — works fully offline via Ollama; no external APIs, no telemetry.
- **Text in, text out** — the LLM has no system authority; its output is untrusted text printed to stdout.
- **Explicit over magic** — no reflection, no hidden side effects, no autonomous behaviour.
- **You review. You decide.** — devmate drafts; you commit.

## Further enhancements

Dependencies and tooling that would fit the current design if the project grows:

| Candidate | What it would add |
|---|---|
| `github.com/knadh/koanf` or `github.com/spf13/viper` | Config layering with more sources (YAML/TOML, flags) and per-key provenance |
| `github.com/ollama/ollama/api` | Official Ollama client with streaming and `OLLAMA_HOST` support; large dependency tree |
| `github.com/charmbracelet/glamour` | Render the markdown PR description in the terminal while keeping plain text when piped |
| `github.com/charmbracelet/lipgloss` or `github.com/fatih/color` | Coloured output that honours `NO_COLOR` |
| `github.com/pkoukk/tiktoken-go` | Chunk thresholds measured in tokens instead of bytes |
| `github.com/google/renameio/v2` | Cross-platform atomic file writes if the cache grows beyond simple entries |
| `github.com/stretchr/testify` | Assertion helpers; the standard-library tests are intentionally kept dependency-free |
| `goreleaser` | Release builds with the version injected via `-ldflags` |
| `golangci-lint`, `govulncheck` | Linting and vulnerability scanning in CI |
