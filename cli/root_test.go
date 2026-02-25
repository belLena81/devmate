package cli

import (
	"devmate/internal/domain"
	"testing"
)

func TestParseCmdMode(t *testing.T) {
	if parseCmdMode(true) != domain.Detailed {
		t.Error("expected Detailed")
	}
	if parseCmdMode(false) != domain.Short {
		t.Error("expected Short")
	}
}

func TestParseValidCmdType(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		parsedCmdType, err := parseCmdType(tt)
		if err != nil {
			t.Error(err)
		}
		if parsedCmdType != domain.CmdTypeIndex[tt] {
			t.Error("expected ", domain.CmdTypeIndex[tt], "got", parsedCmdType)
		}
	}
}

func TestInvalidCmdType(t *testing.T) {
	_, err := parseCmdType("invalid")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}
