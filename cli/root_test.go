package cli

import "testing"

func TestParseCmdMode(t *testing.T) {
	if parseCmdMode(true) != Detailed {
		t.Error("expected Detailed")
	}
	if parseCmdMode(false) != Short {
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
		if parsedCmdType != cmdTypeIndex[tt] {
			t.Error("expected ", cmdTypeIndex[tt], "got", parsedCmdType)
		}
	}
}

func TestInvalidCmdType(t *testing.T) {
	_, err := parseCmdType("invalid")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}
