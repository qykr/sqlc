package compiler

import (
	"strings"
	"testing"

	"github.com/sqlc-dev/sqlc/internal/config"
	"github.com/sqlc-dev/sqlc/internal/engine/postgresql"
)

func TestCheckExpandedQueryResultTypes(t *testing.T) {
	t.Parallel()

	comp := newPostgreSQLCompilerForExpandedCheck(t, `
		CREATE TABLE authors (
			id BIGINT NOT NULL,
			name TEXT NOT NULL
		);
	`)

	t.Run("matching variants", func(t *testing.T) {
		t.Parallel()

		err := comp.CheckExpandedQueryResultTypes([]string{
			`SELECT id, name FROM authors;`,
			`SELECT authors.id, authors.name FROM public.authors;`,
		})
		if err != nil {
			t.Fatalf("CheckExpandedQueryResultTypes returned error: %v", err)
		}
	})

	t.Run("mismatched result type", func(t *testing.T) {
		t.Parallel()

		err := comp.CheckExpandedQueryResultTypes([]string{
			`SELECT id FROM authors;`,
			`SELECT id::text AS id FROM authors;`,
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "type mismatch") {
			t.Fatalf("expected type mismatch error, got %v", err)
		}
	})

	t.Run("syntax invalid", func(t *testing.T) {
		t.Parallel()

		err := comp.CheckExpandedQueryResultTypes([]string{
			`SELECT id FROM authors;`,
			`SELECT FROM authors;`,
		})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "expanded query 2") {
			t.Fatalf("expected expanded query index in error, got %v", err)
		}
	})
}

func newPostgreSQLCompilerForExpandedCheck(t *testing.T, schema string) *Compiler {
	t.Helper()

	comp := &Compiler{
		conf:     config.SQL{Engine: config.EnginePostgreSQL},
		parser:   postgresql.NewParser(),
		catalog:  postgresql.NewCatalog(),
		selector: newDefaultSelector(),
	}

	stmts, err := comp.parser.Parse(strings.NewReader(schema))
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	for _, stmt := range stmts {
		if err := comp.catalog.Update(stmt, comp); err != nil {
			t.Fatalf("update catalog: %v", err)
		}
	}

	return comp
}
