package agent

import (
	"agentstudio/internal/contracts"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeOfflineCopyRecoveryAndMissingDependencies(t *testing.T) {
	s := testService(t)
	r := runtimeFixture(t, s, "backup_runtime")
	root := t.TempDir()
	copyDir := filepath.Join(root, "copy")
	if _, e := CopyOfflineState(s.Config.DataDir, copyDir); e == nil {
		t.Fatal("copied a live writer")
	}
	s.Close()
	info, e := CopyOfflineState(s.Config.DataDir, copyDir)
	if e != nil || info.Files == 0 {
		t.Fatal(e)
	}
	verified, e := VerifyOfflineCopy(copyDir)
	if e != nil || verified != info {
		t.Fatal("copy hash mismatch", e)
	}
	restored, e := New(Config{Mode: "local", DataDir: copyDir})
	if e != nil {
		t.Fatal(e)
	}
	v, e := restored.RuntimeAudit(r.ID)
	if e != nil || v.HeadHash == nil {
		t.Fatal(e)
	}
	restored.Close()
	sourceData, _ := os.ReadFile(filepath.Join(s.Config.DataDir, "runs", r.ID+".json"))
	copyData, _ := os.ReadFile(filepath.Join(copyDir, "runs", r.ID+".json"))
	if contracts.BytesHash(sourceData) != contracts.BytesHash(copyData) {
		t.Fatal("clean restore changed historical bytes")
	}
	dep := r.FrozenDefinition.TemplateDependencies[0].Files[0]
	missing := filepath.Join(copyDir, "definitions", "blobs", dep.SHA256)
	if e = os.Remove(missing); e != nil {
		t.Fatal(e)
	}
	if _, e = VerifyOfflineCopy(copyDir); e == nil {
		t.Fatal("missing dependency passed byte verification")
	}
	if restarted, e := New(Config{Mode: "local", DataDir: copyDir}); e == nil {
		restarted.Close()
		t.Fatal("startup replenished missing frozen dependency")
	}
	if _, e = os.Stat(missing); !os.IsNotExist(e) {
		t.Fatal("missing frozen source was silently republished")
	}
}
