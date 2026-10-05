package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestControlToolsNetworkTablesVersionBoundary(t *testing.T) {
	s, old, request, _, attempt := controlToolsFixture(t, toolSourceFiles())
	legacy, err := controlFrozenModuleEnvironment(old)
	if err != nil || legacy.systemNetworkTables {
		t.Fatalf("old frozen sandbox was expanded: %v", err)
	}
	current := newControlCompletionRun(t, s, "rules")
	modern, err := controlFrozenModuleEnvironment(current)
	if err != nil || !modern.systemNetworkTables {
		t.Fatalf("current system runtime boundary unavailable: %v", err)
	}
	paths, err := s.controlToolPaths(old, request, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err = paths.prepare(); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(s.Config.DataDir, "outside-probe")
	if err = os.WriteFile(outside, []byte("run boundary canary"), 0600); err != nil {
		t.Fatal(err)
	}
	files := toolSourceFiles()
	if err = controlCopyFiles(paths.candidate, files); err != nil {
		t.Fatal(err)
	}
	_, binary, err := controlGoIdentity(old)
	if err != nil {
		t.Fatal(err)
	}
	// These are direct sandbox probes, not fabricated product tool receipts.
	// Opening for write uses neither truncation nor a Write call, so a failing
	// confinement assertion cannot modify either host system file.
	for _, probe := range []struct {
		name        string
		environment controlModuleEnvironment
		allowed     bool
	}{{"1.2", legacy, false}, {"1.3", modern, true}} {
		t.Run(probe.name, func(t *testing.T) {
			body := fmt.Sprintf(`package controlled
import("os";"os/exec";"testing")
func TestSystemBoundary(t *testing.T){
 for _,name:=range []string{"/etc/protocols","/etc/services"}{
  _,err:=os.ReadFile(name);if (err==nil)!=%t {t.Fatalf("unexpected read permission %%s: %%v",name,err)}
  if f,err:=os.OpenFile(name,os.O_WRONLY,0);err==nil {f.Close();t.Fatalf("system write access %%s",name)}
 }
 if _,err:=os.ReadDir("/etc");err==nil {t.Fatal("system directory listing allowed")}
 if _,err:=os.ReadFile(%q);err==nil {t.Fatal("unrelated source read allowed")}
 if err:=exec.Command("/bin/sh","-c","exit 0").Run();err==nil {t.Fatal("shell allowed")}
}
`, probe.allowed, outside)
			if err := os.WriteFile(filepath.Join(paths.candidate, "system_test.go"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd, err := controlSandboxCommand(ctx, paths, binary, []string{"test", "./..."}, "go.test", probe.environment)
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sandbox system boundary: %v %s", err, output)
			}
		})
	}
}
