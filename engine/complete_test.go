package engine

import "testing"

func TestComplete(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"SELECT 1", false},
		{"SELECT 1;", true},
		{"SELECT 1; SELECT 2", false},
		{"SELECT 1; SELECT 2;", true},
	}
	for _, c := range cases {
		got, err := Complete(c.sql)
		if err != nil {
			t.Errorf("Complete(%q) error = %v", c.sql, err)
			continue
		}
		if got != c.want {
			t.Errorf("Complete(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

func TestCompleteStringsAndComments(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		// A ';' inside a string literal is not a statement separator.
		{"SELECT ';';", true},
		{"SELECT ';'", false},
		// A single quote doubled to escape one, per SQL's own rule
		// (never write the two adjacent in a doc comment: gofmt -s
		// silently rewrites '' to a right single quotation mark inside
		// a declaration's doc comment, see .claude/rules/testing.md).
		{"SELECT 'it' || 's';", true},
		{"-- comment only\n", false},
		{"SELECT 1; -- trailing comment", true},
		{"/* unterminated", false},
		{"SELECT 1; /* unterminated", false},
		{"/* a ; inside a comment */ SELECT 1;", true},
		// A bracketed identifier's ']' does not need escaping inside,
		// so a ';' there is not a statement separator either.
		{"SELECT [a;b];", true},
		{"SELECT [a;b]", false},
		// A backtick-quoted identifier (MySQL-style, but SQLite accepts
		// it too).
		{"SELECT `a;b`;", true},
	}
	for _, c := range cases {
		got, err := Complete(c.sql)
		if err != nil {
			t.Errorf("Complete(%q) error = %v", c.sql, err)
			continue
		}
		if got != c.want {
			t.Errorf("Complete(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

// TestCompleteCreateTrigger is the case Complete exists for: a CREATE
// TRIGGER body's BEGIN...END must stay open across the body's own
// internal ';' statement separators, and a CASE...END expression inside
// that body must not be mistaken for the trigger's own closing END. A
// naive BEGIN/END depth counter would get this wrong (the CASE's END
// looks, token-for-token, just like the trigger's).
func TestCompleteCreateTrigger(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT 1", false},
		{"CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT 1;", false},
		{"CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT 1; END", false},
		{"CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT 1; END;", true},
		{"CREATE TEMP TRIGGER trg AFTER INSERT ON t BEGIN SELECT 1; END;", true},
		{"CREATE TEMPORARY TRIGGER trg AFTER INSERT ON t BEGIN SELECT 1; END;", true},
		{
			"CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT CASE WHEN 1 THEN 2 ELSE 3 END",
			false,
		},
		{
			"CREATE TRIGGER trg AFTER INSERT ON t BEGIN SELECT CASE WHEN 1 THEN 2 ELSE 3 END; END;",
			true,
		},
	}
	for _, c := range cases {
		got, err := Complete(c.sql)
		if err != nil {
			t.Errorf("Complete(%q) error = %v", c.sql, err)
			continue
		}
		if got != c.want {
			t.Errorf("Complete(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

func TestCompleteRejectsEmbeddedNUL(t *testing.T) {
	_, err := Complete("SELECT 1;\x00 DROP TABLE t;")
	if err == nil {
		t.Fatal("Complete with an embedded NUL byte: expected an error, got nil")
	}
}

// TestCompleteDoesNotValidateSQL guards against the misunderstanding
// Complete's doc comment warns about: it answers "does this look like it
// ends at a statement boundary", not "is this valid SQL".
func TestCompleteDoesNotValidateSQL(t *testing.T) {
	got, err := Complete("NOT SQL AT ALL;")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !got {
		t.Fatal("Complete(\"NOT SQL AT ALL;\") = false, want true (Complete does not validate SQL, only statement boundaries)")
	}
}

func TestCompleteWorksWithoutAnOpenDB(t *testing.T) {
	// No Open/OpenSelf call anywhere in this test: Complete must not
	// need one (spec §11, "Complete does not touch any database").
	got, err := Complete("SELECT 1;")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !got {
		t.Fatal("Complete(\"SELECT 1;\") = false, want true")
	}
}
