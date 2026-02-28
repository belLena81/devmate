package domain

// Options bundles the user-facing flags shared by all commands.
type Options struct {
	Type    CmdType
	Mode    CmdMode
	Explain bool
	// NoCache bypasses the response cache for this request: the LLM is always
	// called and its fresh response overwrites any existing cached entry.
	NoCache bool
}

// ─── CmdType ──────────────────────────────────────────────────────────────────

// CmdType represents a conventional-commit type (feat, fix, chore, …).
type CmdType int

const (
	Undefined CmdType = iota
	Feat
	Fix
	Chore
	Docs
	Refactor
	cmdTypeSentinel // marks the end of valid CmdType constants
)

// String returns the kebab-case name for the type, or ErrInvalidCmdType for
// unknown values.
func (t CmdType) String() (string, error) {
	switch t {
	case Undefined:
		return "", nil
	case Feat:
		return "feat", nil
	case Fix:
		return "fix", nil
	case Chore:
		return "chore", nil
	case Docs:
		return "docs", nil
	case Refactor:
		return "refactor", nil
	default:
		return "", ErrInvalidCmdType
	}
}

// CmdTypeIndex maps type strings back to CmdType constants.
// Used by CLI flag parsing to convert user input to a typed value.
var CmdTypeIndex = func() map[string]CmdType {
	index := make(map[string]CmdType)
	for t := Undefined; t < cmdTypeSentinel; t++ {
		str, _ := t.String()
		index[str] = t
	}
	return index
}()

// ─── CmdMode ──────────────────────────────────────────────────────────────────

// CmdMode controls how detailed the generated output should be.
type CmdMode int

const (
	Short    CmdMode = iota // concise single-line output (default)
	Detailed                // expanded output with body / breakdown
)

// String returns the human-readable mode name used in prompts and cache keys.
func (m CmdMode) String() string {
	switch m {
	case Detailed:
		return "detailed"
	default:
		return "short"
	}
}
