package testenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	contents := "SELECT 1;\n" + scriptBoundary + "\nSELECT 'two;still-two';\n"
	statements := splitSQLStatements(contents)
	if len(statements) != 2 {
		t.Fatalf("splitSQLStatements() count = %d, want 2", len(statements))
	}
	if !strings.Contains(statements[1], "two;still-two") {
		t.Fatalf("splitSQLStatements() split a quoted semicolon: %q", statements[1])
	}
}

func TestFixtureScriptsHaveExplicitStatements(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	for _, backend := range SQLBackends() {
		for _, name := range []string{"001_schema.sql", "002_seed.sql"} {
			path := filepath.Join(repositoryRoot, "testdata", string(backend), name)
			statements, err := readSQLStatements(path)
			if err != nil {
				t.Fatalf("readSQLStatements(%s) error = %v", path, err)
			}
			if len(statements) < 2 {
				t.Fatalf("readSQLStatements(%s) count = %d, want at least 2", path, len(statements))
			}
		}
	}
}

func TestReadSQLStatementsRejectsEmptyScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.sql")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("write empty script: %v", err)
	}
	if _, err := readSQLStatements(path); err == nil {
		t.Fatal("readSQLStatements(empty) error = nil")
	}
}
