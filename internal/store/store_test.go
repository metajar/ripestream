package store

import (
	"reflect"
	"testing"
)

func TestSplitStatementsIgnoresCommentSemicolons(t *testing.T) {
	input := `-- bootstrap; this must not become a query
CREATE DATABASE IF NOT EXISTS ripestream;

-- Existing rows receive defaults; new rows populate typed values.
ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS sent UInt32;
-- trailing comment only; still not a query`
	want := []string{
		"CREATE DATABASE IF NOT EXISTS ripestream",
		"ALTER TABLE ripestream.atlas_results ADD COLUMN IF NOT EXISTS sent UInt32",
	}
	if got := splitStatements(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("splitStatements() = %#v, want %#v", got, want)
	}
}

func TestSplitStatementsPreservesQuotedSemicolonsAndDashes(t *testing.T) {
	input := "SELECT 'value;--still quoted'; SELECT `semi;column` FROM t;"
	want := []string{"SELECT 'value;--still quoted'", "SELECT `semi;column` FROM t"}
	if got := splitStatements(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("splitStatements() = %#v, want %#v", got, want)
	}
}
