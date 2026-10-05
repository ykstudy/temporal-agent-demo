package contracts

import (
	"sync"
	"testing"
)

func TestStructuralCacheCannotReuseDifferentBytesTypeOrVersion(t *testing.T) {
	good := []byte(`"run_a"`)
	if err := ValidateJSONVersion("Id", "1.3.0", good); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if err := ValidateJSONVersion("Id", "1.3.0", good); err != nil {
					t.Error(err)
				}
				if err := ValidateJSONVersion("Hash", "1.3.0", good); err == nil {
					t.Error("type reused")
				}
				if err := ValidateJSONVersion("Id", "unpublished", good); err == nil {
					t.Error("release reused")
				}
				if err := ValidateJSONVersion("Id", "1.3.0", []byte(`true`)); err == nil {
					t.Error("bytes reused")
				}
			}
		}()
	}
	wg.Wait()
	schema := map[string]any{"type": "string", "enum": []string{"allowed"}}
	if err := ValidateDocumentVersion("1.3.0", schema, "allowed"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDocumentVersion("1.3.0", schema, "forged"); err == nil {
		t.Fatal("parameter value cache collision")
	}
	schema["enum"] = []string{"different"}
	if err := ValidateDocumentVersion("1.3.0", schema, "allowed"); err == nil {
		t.Fatal("parameter schema cache collision")
	}
}
