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
