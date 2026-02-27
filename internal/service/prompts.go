package service

import (
	"bytes"
	"devmate/internal/domain"
	_ "embed"
	"strings"
	"text/template"
)

//go:embed _resources/commit.tmpl
var commitTmpl string

//go:embed _resources/branch.tmpl
var branchTmpl string

//go:embed _resources/pr.tmpl
var prTmpl string

//go:embed _resources/chunk.tmpl
var chunkTmpl string

//go:embed _resources/synthesis.tmpl
var synthesisTmpl string

//go:embed _resources/reduce.tmpl
var reduceTmpl string

//go:embed _resources/pr_synthesis.tmpl
var prSynthesisTmpl string

// commitData holds the values injected into commit.tmpl.
type commitData struct {
	TypeOverride string // empty when type is auto-detected
	Detailed     bool
	Explain      bool
	Diff         string
}

// branchData holds the values injected into branch.tmpl.
type branchData struct {
	TypeOverride string
	Detailed     bool
	Explain      bool
	Task         string
}

// prData holds the values injected into pr.tmpl.
type prData struct {
	TypeOverride      string
	Detailed          bool
	Explain           bool
	SourceBranch      string
	DestinationBranch string
	Commits           []string
}

// chunkData holds the values injected into chunk.tmpl.
type chunkData struct {
	Chunk int
	Total int
	Diff  string
}

// numberedSummary is one entry in the synthesis summaries list.
// Carrying the number as a field avoids needing a custom template func.
type numberedSummary struct {
	N    int
	Text string
}

// synthesisData holds the values injected into synthesis.tmpl.
type synthesisData struct {
	TypeOverride string
	Detailed     bool
	Explain      bool
	Summaries    []numberedSummary
}

func BuildCommitPrompt(diff string, o CommitOptions) string {
	typeStr, _ := o.Type.String()
	return mustRender("commit", commitTmpl, commitData{
		TypeOverride: typeStr,
		Detailed:     o.Mode == domain.Detailed,
		Explain:      o.Explain,
		Diff:         diff,
	})
}

func BuildBranchPrompt(o BranchOptions) string {
	typeStr, _ := o.Type.String()
	return mustRender("branch", branchTmpl, branchData{
		TypeOverride: typeStr,
		Detailed:     o.Mode == domain.Detailed,
		Explain:      o.Explain,
		Task:         o.Task,
	})
}

func BuildPrPrompt(commits []string, o PrOptions) string {
	typeStr, _ := o.Type.String()
	return mustRender("pr", prTmpl, prData{
		TypeOverride:      typeStr,
		Detailed:          o.Mode == domain.Detailed,
		Explain:           o.Explain,
		SourceBranch:      o.SourceBranch,
		DestinationBranch: o.DestinationBranch,
		Commits:           commits,
	})
}

// BuildChunkPrompt builds the map-step prompt for summarising one chunk of a
// large diff. chunk and total tell the model it is seeing a partial view.
func BuildChunkPrompt(diff string, chunk, total int) string {
	return mustRender("chunk", chunkTmpl, chunkData{
		Chunk: chunk,
		Total: total,
		Diff:  diff,
	})
}

// BuildSynthesisPrompt builds the reduce-step prompt that turns per-chunk
// bullet summaries into a single conventional commit message.
func BuildSynthesisPrompt(summaries []string, cmdType domain.CmdType, mode domain.CmdMode, explain bool) string {
	typeStr, _ := cmdType.String()

	numbered := make([]numberedSummary, len(summaries))
	for i, s := range summaries {
		numbered[i] = numberedSummary{N: i + 1, Text: s}
	}

	return mustRender("synthesis", synthesisTmpl, synthesisData{
		TypeOverride: typeStr,
		Detailed:     mode == domain.Detailed,
		Explain:      explain,
		Summaries:    numbered,
	})
}

// BuildReducePrompt builds a prompt that condenses a group of intermediate
// summaries into a single shorter summary. Used when the collected chunk
// summaries are too large to fit into a single synthesis prompt.
func BuildReducePrompt(summaries []string) string {
	numbered := make([]numberedSummary, len(summaries))
	for i, s := range summaries {
		numbered[i] = numberedSummary{N: i + 1, Text: s}
	}

	return mustRender("reduce", reduceTmpl, synthesisData{
		Summaries: numbered,
	})
}

// prSynthesisData holds the values injected into pr_synthesis.tmpl.
type prSynthesisData struct {
	TypeOverride      string
	Detailed          bool
	Explain           bool
	SourceBranch      string
	DestinationBranch string
	Summaries         []numberedSummary
}

// BuildPrSynthesisPrompt builds the reduce-step prompt that turns per-chunk
// commit summaries into a single PR title and description.
func BuildPrSynthesisPrompt(summaries []string, o PrOptions) string {
	typeStr, _ := o.Type.String()

	numbered := make([]numberedSummary, len(summaries))
	for i, s := range summaries {
		numbered[i] = numberedSummary{N: i + 1, Text: s}
	}

	return mustRender("pr_synthesis", prSynthesisTmpl, prSynthesisData{
		TypeOverride:      typeStr,
		Detailed:          o.Mode == domain.Detailed,
		Explain:           o.Explain,
		SourceBranch:      o.SourceBranch,
		DestinationBranch: o.DestinationBranch,
		Summaries:         numbered,
	})
}

// mustRender executes a template and returns the result as a string.
// Panics on parse or execution error — both indicate a programmer error
// (malformed template embedded at compile time) not a runtime condition.
func mustRender(name, tmpl string, data any) string {
	t := template.Must(template.New(name).Parse(tmpl))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic("prompt template " + name + " failed to render: " + err.Error())
	}
	return strings.TrimSpace(buf.String()) + "\n"
}
