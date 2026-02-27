# Devmate

Devmate is a local-first CLI assistant that helps developers automate common Git workflows by drafting content — never executing it.

It analyzes repository state and produces human-reviewed text suggestions for:
- Conventional commit messages
- Clean, team-standard branch names
- Structured Pull Request descriptions

Devmate never executes Git mutations and never applies AI output automatically.

It is a drafting tool — not an autonomous agent.

## Design Principles

### 1. Read-Only Git Access

All Git interactions are informational only:
- git diff --cached
- commit history

No mutating commands are ever executed.

### 2. Text In → Text Out

The LLM layer:
- Accepts structured prompts
- Returns raw text
- Has no system-level authority
- Cannot trigger execution

All AI output is treated as untrusted suggestions.

### 3. Local-First by Default

- Works fully offline
- Supports Ollama for local model execution
- No telemetry
- No external APIs required

### 4. Clean Architecture Enforcement

Devmate strictly separates concerns to prevent accidental privilege escalation.

```
cmd/devmate/          CLI entrypoint
internal/
  cli/                Cobra commands & UX
  domain/             Core interfaces
  service/            Orchestration & AI pipeline
  infra/
    git/              Git runner (read-only)
    llm/              LLM adapters (Ollama)
    progress/         Spinner / CLI feedback
```

Dependency Direction
```
CLI → Service → Domain Interfaces ← Infrastructure
```
Infrastructure never leaks into business logic.

## Supported Commands
### 1. devmate commit
   Analyzes staged changes and drafts a Conventional Commit message.
   `devmate commit`
   
Uses:
- git diff --cached
- Chunking + reduction pipeline (for large diffs)
- Commit prompt template

Example output:
```
feat(auth): add token refresh mechanism

- Introduces refresh token rotation
- Updates middleware validation logic
- Adds unit tests for expiration edge cases
```
### 2. devmate branch
Drafts a clean branch name from a description.
   `devmate branch "Add retry logic to payment webhook handler"`
Example:
```
add/retry-logic-to-payment-webhook-handler
```
Uses a branch-specific prompt template.

### 3. devmate pr
Drafts a structured Pull Request description.
   `devmate pr --base main --head feat/payment-webhook-retry`
Output includes:
- Summary
- Technical decisions
- Risk & impact notes

## AI Processing Pipeline

Devmate implements a multi-stage LLM pipeline to safely handle large diffs.

### 1. Chunking

Large diffs are split into manageable chunks.

### 2. Per-Chunk Analysis

Each chunk is analyzed independently using
- chunk.tmpl

### 3. Reduction

Chunk summaries are merged via:
- reduce.tmpl

### 4. Synthesis

Final structured output is produced using:
- synthesis.tmpl
- Command-specific templates (commit.tmpl, pr.tmpl, branch.tmpl)

This staged approach:
- Prevents context overflow
- Improves signal quality 
- Maintains deterministic structure

## Caching Layer

Devmate includes a service-level caching mechanism to:
- Avoid re-analyzing identical diffs 
- Improve performance
- Reduce redundant LLM calls
Cache keys are derived from structured input state, not arbitrary text.

Caching is internal and does not persist sensitive repository content outside process scope.

## Templates

All prompts are template-driven and stored in:
```internal/service/_resources/```
This allows:
- Deterministic prompt structure
- Easy customization
- Clear separation of prompt logic from business logic

Templates include:
- branch.tmpl
- commit.tmpl
- pr.tmpl
- chunk.tmpl
- reduce.tmpl
- synthesis.tmpl

## Progress Feedback

A lightweight spinner system provides CLI feedback during LLM processing.

This:
- Improves UX during long operations
- Does not alter program logic
- Lives in infra/progress

## LLM Support

Currently supported:
- Ollama (local models)

Adapter-based design allows new providers without modifying:
- CLI layer
- Git layer
- Service orchestration
LLM interface lives in internal/domain/llm.go.

## Configuration

Configuration file:
```config/config.toml```
Allows structured runtime configuration without hardcoding model or environment decisions.

## Docker Support

The repository includes:
- Dockerfile
- docker-compose-ollama.yml
These allow:
- Running Devmate in containerized environments
- Spinning up Ollama locally for fully isolated development

## Installation
### From Source
```
git clone https://github.com/belLena81/devmate
cd devmate
go build ./cmd/devmate
```
### Install via Go
```go install github.com/belLena81/devmate/cmd/devmate@latest```

## Requirements
 - Go 1.25+
 - Git
 - Ollama

## Development

```
git clone https://github.com/belLena81/devmate
cd devmate
go build ./cmd/devmate
```

## Testing

The project includes:
- CLI tests
- Service tests
- Cache tests
- Prompt tests
- Chunking & reduction tests
- Git runner tests
- LLM adapter tests

Testing strategy enforces:
- No hidden side effects
- Interface-based mocking
- Deterministic service behavior

## Non-Goals

Devmate intentionally does not:
- Execute Git mutations
- Apply commits automatically
- Push to remotes
- Manage credentials
- Run AI-generated commands
- Modify files
- Operate as an autonomous agent

## Security Model

- No shell execution of AI output
- No remote calls unless explicitly configured
- No filesystem writes outside controlled scope
- No background agents
- No implicit authority escalation
- AI suggestions are untrusted text.

You review. You decide.

## Design Philosophy
- Small explicit interfaces
- No reflection
- No magic behavior
- Deterministic boundaries
- Strict separation of concerns
- The goal is controlled augmentation — not automation.
