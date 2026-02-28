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

// Parsed templates — compiled exactly once at package init from the embedded
// strings above. Parsing on every mustRender call re-compiled seven templates
// per LLM invocation with zero benefit; moving it here makes the cost pay-once
// and surfaces any template syntax errors at startup rather than mid-request.
var (
	commitT      = template.Must(template.New("commit").Parse(commitTmpl))
	branchT      = template.Must(template.New("branch").Parse(branchTmpl))
	prT          = template.Must(template.New("pr").Parse(prTmpl))
	chunkT       = template.Must(template.New("chunk").Parse(chunkTmpl))
	synthesisT   = template.Must(template.New("synthesis").Parse(synthesisTmpl))
	reduceT      = template.Must(template.New("reduce").Parse(reduceTmpl))
	prSynthesisT = template.Must(template.New("pr_synthesis").Parse(prSynthesisTmpl))
)

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
	return mustRender(commitT, commitData{
		TypeOverride: typeStr,
		Detailed:     o.Mode == domain.Detailed,
		Explain:      o.Explain,
		Diff:         diff,
	})
}

func BuildBranchPrompt(o BranchOptions) string {
	typeStr, _ := o.Type.String()
	return mustRender(branchT, branchData{
		TypeOverride: typeStr,
		Detailed:     o.Mode == domain.Detailed,
		Explain:      o.Explain,
		Task:         o.Task,
	})
}

func BuildPrPrompt(commits []string, o PrOptions) string {
	typeStr, _ := o.Type.String()
	return mustRender(prT, prData{
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
	return mustRender(chunkT, chunkData{
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

	return mustRender(synthesisT, synthesisData{
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

	return mustRender(reduceT, synthesisData{
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

	return mustRender(prSynthesisT, prSynthesisData{
		TypeOverride:      typeStr,
		Detailed:          o.Mode == domain.Detailed,
		Explain:           o.Explain,
		SourceBranch:      o.SourceBranch,
		DestinationBranch: o.DestinationBranch,
		Summaries:         numbered,
	})
}

// mustRender executes a pre-parsed template and returns the result as a string.
// Panics on execution error — this indicates a data/template contract violation,
// which is a programmer error, not a runtime condition.
func mustRender(t *template.Template, data any) string {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic("prompt template " + t.Name() + " failed to render: " + err.Error())
	}
	return strings.TrimSpace(buf.String()) + "\n"
}
