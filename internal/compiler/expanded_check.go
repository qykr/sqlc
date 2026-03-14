package compiler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sqlc-dev/sqlc/internal/sql/ast"
	"github.com/sqlc-dev/sqlc/internal/sql/validate"
)

// CheckExpandedQueryResultTypes parses and analyzes each expanded query,
// returning an error if any variant is invalid SQL or infers a different
// result-column shape than the first variant.
func (c *Compiler) CheckExpandedQueryResultTypes(queries []string) error {
	if len(queries) == 0 {
		return errors.New("no expanded queries provided")
	}

	var expected []*Column
	for i, query := range queries {
		anlys, err := c.analyzeExpandedQuery(query)
		if err != nil {
			return fmt.Errorf("expanded query %d: %w", i+1, err)
		}
		if i == 0 {
			expected = anlys.Columns
			continue
		}
		if err := c.checkResultColumnsCompatible(expected, anlys.Columns); err != nil {
			return fmt.Errorf("expanded query %d result type mismatch: %w", i+1, err)
		}
	}

	return nil
}

func (c *Compiler) analyzeExpandedQuery(query string) (*analysis, error) {
	stmts, err := c.parser.Parse(strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		return nil, errors.New("no statements in expanded query")
	}
	if len(stmts) != 1 {
		return nil, fmt.Errorf("expected 1 statement in expanded query, found %d", len(stmts))
	}

	raw := stmts[0].Raw
	if raw == nil {
		return nil, errors.New("expanded query statement is nil")
	}
	if err := validate.SqlcFunctions(raw); err != nil {
		return nil, err
	}

	return c.analyzeRawQuery(context.Background(), raw, query)
}

func (c *Compiler) checkResultColumnsCompatible(expected, actual []*Column) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("column count mismatch: %d != %d", len(expected), len(actual))
	}
	for i := range expected {
		if err := c.checkResultColumnCompatible(i, expected[i], actual[i]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) checkResultColumnCompatible(index int, expected, actual *Column) error {
	column := index + 1
	if expected == nil || actual == nil {
		if expected == actual {
			return nil
		}
		return fmt.Errorf("column %d mismatch: nil column metadata", column)
	}
	if expected.Name != actual.Name {
		return fmt.Errorf("column %d name mismatch: %q != %q", column, expected.Name, actual.Name)
	}
	if expected.OriginalName != actual.OriginalName {
		return fmt.Errorf("column %d original name mismatch: %q != %q", column, expected.OriginalName, actual.OriginalName)
	}
	if !sameResultColumnType(expected, actual) {
		return fmt.Errorf("column %d type mismatch: %q != %q", column, resultColumnTypeString(expected), resultColumnTypeString(actual))
	}
	if expected.NotNull != actual.NotNull {
		return fmt.Errorf("column %d nullability mismatch: %t != %t", column, expected.NotNull, actual.NotNull)
	}
	if expected.Unsigned != actual.Unsigned {
		return fmt.Errorf("column %d unsigned mismatch: %t != %t", column, expected.Unsigned, actual.Unsigned)
	}
	if expected.IsArray != actual.IsArray {
		return fmt.Errorf("column %d array mismatch: %t != %t", column, expected.IsArray, actual.IsArray)
	}
	if expected.ArrayDims != actual.ArrayDims {
		return fmt.Errorf("column %d array dimensions mismatch: %d != %d", column, expected.ArrayDims, actual.ArrayDims)
	}
	if !sameOptionalInt(expected.Length, actual.Length) {
		return fmt.Errorf("column %d length mismatch: %s != %s", column, optionalIntString(expected.Length), optionalIntString(actual.Length))
	}
	if !c.sameResultTableName(expected.Table, actual.Table) {
		return fmt.Errorf("column %d table mismatch: %s != %s", column, resultTableNameString(expected.Table), resultTableNameString(actual.Table))
	}
	if !c.sameResultTableName(expected.EmbedTable, actual.EmbedTable) {
		return fmt.Errorf("column %d embed table mismatch: %s != %s", column, resultTableNameString(expected.EmbedTable), resultTableNameString(actual.EmbedTable))
	}

	return nil
}

func sameResultColumnType(expected, actual *Column) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	if expected.Type != nil || actual.Type != nil {
		return sameResultTypeName(expected.Type, actual.Type)
	}
	return expected.DataType == actual.DataType
}

func sameResultTypeName(expected, actual *ast.TypeName) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}

	expectedSchema := expected.Schema
	actualSchema := actual.Schema
	if expectedSchema == "pg_catalog" {
		expectedSchema = ""
	}
	if actualSchema == "pg_catalog" {
		actualSchema = ""
	}

	return expected.Catalog == actual.Catalog && expectedSchema == actualSchema && expected.Name == actual.Name
}

func (c *Compiler) sameResultTableName(expected, actual *ast.TableName) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}

	defaultSchema := ""
	if c.catalog != nil {
		defaultSchema = c.catalog.DefaultSchema
	}

	expectedSchema := expected.Schema
	actualSchema := actual.Schema
	if expectedSchema == "" {
		expectedSchema = defaultSchema
	}
	if actualSchema == "" {
		actualSchema = defaultSchema
	}

	return expected.Catalog == actual.Catalog && expectedSchema == actualSchema && expected.Name == actual.Name
}

func resultColumnTypeString(col *Column) string {
	if col == nil {
		return "<nil>"
	}
	if col.Type != nil {
		return dataType(col.Type)
	}
	if col.DataType != "" {
		return col.DataType
	}
	return "<unknown>"
}

func resultTableNameString(table *ast.TableName) string {
	if table == nil {
		return "<nil>"
	}
	parts := make([]string, 0, 3)
	if table.Catalog != "" {
		parts = append(parts, table.Catalog)
	}
	if table.Schema != "" {
		parts = append(parts, table.Schema)
	}
	if table.Name != "" {
		parts = append(parts, table.Name)
	}
	if len(parts) == 0 {
		return "<empty>"
	}
	return strings.Join(parts, ".")
}

func sameOptionalInt(expected, actual *int) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return *expected == *actual
}

func optionalIntString(value *int) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *value)
}
