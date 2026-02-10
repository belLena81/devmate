# Devmate

Devmate is a local-first CLI assistant that helps developers automate common Git workflows by drafting content — never executing it.

It analyzes repository state and produces human-reviewed text output for:
 - Conventional commit messages
 - Clean, team-standard branch names
 - Detailed Pull Request descriptions

Devmate is designed for private and sensitive repositories where security, privacy, and developer intent are non-negotiable.

## Design Principles

### Read-Only by Default

Devmate never runs git commit, git push, git checkout, or any mutating Git command.
All Git interactions are strictly informational.

### Text-Only Output

All output is plain text written to stdout.
Nothing is piped to a shell, executed, or used to trigger logic.

### Offline & Private

- Fully functional without an internet connection
- Supports Ollama for 100% local LLM execution
- No telemetry, no external APIs by default

### Untrusted AI Output

AI responses are treated as raw suggestions.
Human review is always required before use.

## Supported Commands
1. devmate commit
Analyzes staged changes (git diff --cached) and drafts a conventional commit message.
   `devmate commit`

####    Output example:
feat(auth): add token refresh mechanism

- Introduces refresh token rotation
- Updates middleware validation logic
- Adds unit tests for expiration edge cases

2. devmate branch
Analyzes the current branch name and drafts a team-standard branch name.
   `devmate branch "Add retry logic to payment webhook handler"`
####    Output example:
add/retry-logic-to-payment-webhook-handler

3. devmate pr
Analyzes the current branch and drafts a detailed Pull Request description.
   `devmate pr --base main --head feat/payment-webhook-retry`
   Output includes:
- Summary of changes
- Key technical decisions
- Impact and risk notes

## Non-Goals

Devmate intentionally does not:
- Execute Git mutations
- Auto-apply changes
- Manage credentials
- Interact with remote Git providers
- Run AI-generated commands

This is a drafting tool, not an agent with authority.

## Architecture Overview

Devmate follows a Clean Architecture approach to enforce safety and maintainability.

`cmd/            CLI entrypoint`

`internal/`

`cli/          Cobra commands and UX`

`app/          Use-case orchestration`

`domain/       Core interfaces and prompts`

`infra/        Git and LLM implementations`

### Key Boundaries

- Git access is isolated and read-only
- AI logic is pure text-in / text-out
- CLI owns all user interaction

### LLM Support

Devmate currently supports:
- Ollama (local models)

LLM integration is adapter-based and can be extended without touching CLI or Git logic.

## Installation
go install github.com/belLena81/devmate/cmd/devmate@latest

Requires:

- Go 1.25+
- Git
- Ollama (optional, for AI features)

## Development

`git clone https://github.com/belLena81/devmate`

`cd devmate`

`go build ./cmd/devmate`


## Code Style

- Small, explicit interfaces
- No hidden side effects
- No reflection or magic behavior
- Dependency direction strictly enforced

## Security Considerations

- No network calls unless explicitly configured
- No shell execution of AI output
- No file system writes outside temporary process scope
- No background processes

If Devmate suggests something dangerous, it’s because you asked it to think, not because it took action.