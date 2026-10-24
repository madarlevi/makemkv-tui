package batchstate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Schema 1 plans written before staged-artifact checkpoints omit every new field.
func TestLegacySchemaOneWithoutRecoveryFields(t *testing.T) {
	original := fixture(t)
	b, e := json.Marshal(original)
	if e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"StagedFile", "EngineSucceeded", "StagingInvalid", "AttributeCodes", "StreamCodes"} {
		if bytes.Contains(b, []byte(`"`+field+`"`)) {
			t.Fatalf("legacy payload unexpectedly has %s", field)
		}
	}
	path := filepath.Join(t.TempDir(), "legacy plan 日本語.json")
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	s, e := store.Load()
	if e != nil {
		t.Fatal(e)
	}
	title := s.ISOs[0].Titles[0]
	if s.SchemaVersion != 1 || !title.Selected || title.ID != 7 || title.Output != "movie/title7.mkv" || s.InputRoot != original.InputRoot || s.OutputRoot != original.OutputRoot {
		t.Fatal("legacy selections/paths changed")
	}
	if e = store.Save(s); e != nil {
		t.Fatal(e)
	}
	if _, e = store.Load(); e != nil {
		t.Fatal(e)
	}
}
