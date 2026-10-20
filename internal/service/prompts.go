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

var (
	commitT      = template.Must(template.New("commit").Parse(commitTmpl))
	branchT      = template.Must(template.New("branch").Parse(branchTmpl))
	prT          = template.Must(template.New("pr").Parse(prTmpl))
	chunkT       = template.Must(template.New("chunk").Parse(chunkTmpl))
	synthesisT   = template.Must(template.New("synthesis").Parse(synthesisTmpl))
	reduceT      = template.Must(template.New("reduce").Parse(reduceTmpl))
	prSynthesisT = template.Must(template.New("pr_synthesis").Parse(prSynthesisTmpl))
)

type commitData struct {
	TypeOverride string
	Detailed     bool
	Explain      bool
	Diff         string
}

type branchData struct {
	TypeOverride string
	Detailed     bool
	Explain      bool
	Task         string
}

type prData struct {
	TypeOverride      string
	Detailed          bool
	Explain           bool
	SourceBranch      string
	DestinationBranch string
	Commits           []string
}

type chunkData struct {
	Chunk int
	Total int
	Diff  string
}

type numberedSummary struct {
	N    int
	Text string
}

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

// BuildChunkPrompt builds the map-step prompt for one chunk of a large diff.
func BuildChunkPrompt(diff string, chunk, total int) string {
	return mustRender(chunkT, chunkData{
		Chunk: chunk,
		Total: total,
		Diff:  diff,
	})
}

// BuildSynthesisPrompt builds the prompt that turns chunk summaries into a single commit message.
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

// BuildReducePrompt builds the prompt that condenses a group of summaries into one shorter summary.
func BuildReducePrompt(summaries []string) string {
	numbered := make([]numberedSummary, len(summaries))
	for i, s := range summaries {
		numbered[i] = numberedSummary{N: i + 1, Text: s}
	}

	return mustRender(reduceT, synthesisData{
		Summaries: numbered,
	})
}

type prSynthesisData struct {
	TypeOverride      string
	Detailed          bool
	Explain           bool
	SourceBranch      string
	DestinationBranch string
	Summaries         []numberedSummary
}

// BuildPrSynthesisPrompt builds the prompt that turns chunk summaries into a single PR description.
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

func mustRender(t *template.Template, data any) string {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic("prompt template " + t.Name() + " failed to render: " + err.Error())
	}
	return strings.TrimSpace(buf.String()) + "\n"
}
