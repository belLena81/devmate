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
```

| Flag | Description |
|---|---|
| `-t, --type` | Override commit type: `feat`, `fix`, `chore`, `docs`, `refactor` |
| `--short` | Single-line subject only (default) |
| `--detailed` | Subject + body with bullet points |
| `--explain` | Append reasoning to the output |

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

### `devmate pr <source> <target>`

Drafts a PR title and description from `git log` between two branches.

```sh
devmate pr feat/payment-webhook-retry main
devmate pr feat/payment-webhook-retry main --detailed
```

| Flag | Description |
|---|---|
| `-t, --type` | Override PR type |
| `--short` | Title + bullet summary (default) |
| `--detailed` | Full description with technical decisions and risk notes |
| `--explain` | Append reasoning to the output |

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

**Requirements:** Go 1.25+, Git, [Ollama](https://ollama.com)

## Configuration

Settings are resolved in this priority order: **env vars → config file → built-in defaults**.

The config file lives at `./config/config.json` relative to the working directory (commit it to the repo to share settings with the team). Override the path with `DEVMATE_CONFIG`.

**`config/config.json` (all fields optional — defaults shown):**

```json
{
  "ollama": {
    "base_url": "http://localhost:11434",
    "model": "llama3.2:3b",
    "request_timeout_sec": 180,
    "http_max_retries": 0,
    "retry_base_delay_sec": 0
  },
  "service": {
    "chunk_threshold": 3000,
    "max_concurrency": 2,
    "max_retries": 0,
    "retry_base_delay_sec": 2
  },
  "cache": {
    "dir": ""
  },
  "log": {
    "level": "info"
  }
}
```

**Environment variables:**

| Variable | Overrides |
|---|---|
| `DEVMATE_CONFIG` | Config file path |
| `DEVMATE_OLLAMA_BASE_URL` | `ollama.base_url` |
| `DEVMATE_OLLAMA_MODEL` | `ollama.model` |
| `DEVMATE_OLLAMA_REQUEST_TIMEOUT_SEC` | `ollama.request_timeout_sec` |
| `DEVMATE_OLLAMA_HTTP_MAX_RETRIES` | `ollama.http_max_retries` |
| `DEVMATE_OLLAMA_RETRY_BASE_DELAY_SEC` | `ollama.retry_base_delay_sec` |
| `DEVMATE_SERVICE_CHUNK_THRESHOLD` | `service.chunk_threshold` |
| `DEVMATE_SERVICE_MAX_CONCURRENCY` | `service.max_concurrency` |
| `DEVMATE_SERVICE_MAX_RETRIES` | `service.max_retries` |
| `DEVMATE_SERVICE_RETRY_BASE_DELAY_SEC` | `service.retry_base_delay_sec` |
| `DEVMATE_CACHE_DIR` | `cache.dir` |
| `DEVMATE_LOG_LEVEL` | `log.level` |

All integer env vars are validated at startup: a non-integer value (e.g. `DEVMATE_SERVICE_MAX_RETRIES=two`) causes `Load()` to return a descriptive error rather than crash.

**Seeing config load progress:** Set `DEVMATE_LOG_LEVEL=debug` to see which values were loaded from the file, which came from env vars, and which fell back to defaults:

```
level=DEBUG component=config msg="config file loaded" path=config/config.json
level=DEBUG component=config msg="file override" key=ollama.model value=mistral:7b
level=DEBUG component=config msg="config resolved" ollama.base_url=http://localhost:11434 ...
```

If no config file is present, all built-in defaults are used with no error.

## LLM pipeline

For diffs or commit histories that exceed `service.chunk_threshold` (default 3 000 bytes), devmate runs a multi-stage pipeline instead of sending everything in one prompt:

1. **Chunk** — the input is split on file boundaries and bin-packed so large files get their own chunks and small files are grouped to minimise LLM round-trips.
2. **Map** — each chunk is summarised independently via parallel LLM calls (up to `max_concurrency` at a time).
3. **Reduce** — summaries are iteratively condensed in groups until the combined size fits within the threshold.
4. **Synthesise** — a final prompt produces the structured output (commit message, branch name, or PR description).

This keeps every prompt within the model's context window regardless of repository size.

## Caching

Responses are cached to `~/.cache/devmate` (overridable via `cache.dir` or `DEVMATE_CACHE_DIR`). Cache keys are derived from: model name, binary hash, git input (diff or commit list), and all option flags. Changing any of these produces a new key. Changing the binary invalidates all prior entries automatically.

Use `devmate cache stat` to inspect the cache and `devmate cache clean` to wipe it.

## Retry behaviour

devmate has two independent retry layers:

- **HTTP retries** (`ollama.http_max_retries`, default 0) — the Ollama client retries transient HTTP failures with exponential back-off starting at `ollama.retry_base_delay_sec`.
- **Service retries** (`service.max_retries`, default 0) — an additional safety net at the service layer, on top of HTTP retries. Back-off starts at `service.retry_base_delay_sec` (default 2 s) and doubles on each attempt.

## Project structure

```
cmd/devmate/          entry point — loads config, wires dependencies, runs CLI
cli/                  Cobra commands and option parsing
  cache.go            cache clean and cache stat subcommands
internal/
  config/             unified configuration (file + env + defaults)
  domain/             core interfaces (LLM, GitClient, Progress)
  service/            orchestration — chunking, map-reduce, caching, retry
    cache.go          Cache interface, DiskCache, CacheInspector, NoopCache
  infra/
    git/              read-only Git runner (diff, log)
    llm/              Ollama HTTP client with retry
    progress/         terminal spinner
```

Dependency direction: `CLI → Service → Domain ← Infrastructure`. Infrastructure never imports service or CLI packages.

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