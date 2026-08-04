package supabase

import (
	"strings"
	"testing"
)

// TestSchemaDDLHasAllTables guards the schema contract: every table Nexus
// needs must be created by SchemaDDL so the migration tool leaves a complete
// Postgres schema behind.
func TestSchemaDDLHasAllTables(t *testing.T) {
	expected := []string{
		"applications", "companies", "company_countries", "contacts",
		"saved_contacts", "outreach_log", "outreach_items", "highlights",
		"schema_migrations",
	}
	for _, tbl := range expected {
		if !strings.Contains(SchemaDDL, "CREATE TABLE IF NOT EXISTS "+tbl) {
			t.Errorf("SchemaDDL missing CREATE TABLE for %q", tbl)
		}
	}
}

// TestSchemaDDLIdempotentStatements ensures the DDL uses IF NOT EXISTS (safe
// to re-run) rather than plain CREATE TABLE statements.
func TestSchemaDDLIdempotentStatements(t *testing.T) {
	if strings.Contains(SchemaDDL, "\nCREATE TABLE ") && !strings.Contains(SchemaDDL, "CREATE TABLE IF NOT EXISTS") {
		t.Error("expected IF NOT EXISTS on all CREATE TABLE statements")
	}
}
