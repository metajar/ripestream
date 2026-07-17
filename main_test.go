package main

import (
	"strings"
	"testing"
)

func TestEmbeddedSchemaDoesNotMaterializeTTLOnStartup(t *testing.T) {
	const setting = "SETTINGS materialize_ttl_after_modify = 0"
	if got, want := strings.Count(schemaSQL, setting), 2; got != want {
		t.Fatalf("schema contains %d non-materializing TTL migrations, want %d", got, want)
	}
}
