package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// These tests execute the generated project rather than asserting template text.
func TestGeneratedLoggingRuntime(t *testing.T) {
	for _, output := range []string{"console", "file"} {
		t.Run(output, func(t *testing.T) {
			plan := defaultPlan()
			plan.ProjectName = "logging-service"
			plan.Skills = []string{"add_logging"}
			plan.LogOutput = output
			plan.LogOnWriteError = "fail_closed"
			files, err := RenderFiles(plan)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for name, content := range files {
				target := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", "test", "-race", "./...")
			cmd.Dir = dir
			if result, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("generated %s runtime tests: %v\n%s", output, err, result)
			}
		})
	}
}
func TestLogParametersRejectOutOfScopeValues(t *testing.T) {
	plan := defaultPlan()
	plan.ProjectName = "logging-service"
	plan.Skills = []string{"add_logging"}
	for _, change := range []func(*Plan){
		func(p *Plan) { p.LogPath = "../escape.log" },
		func(p *Plan) { p.LogPath = "/tmp/app.log" },
		func(p *Plan) { p.LogPath = "logs/../app.log" },
		func(p *Plan) { p.LogPath = "logs/app\".log" },
		func(p *Plan) { p.LogMaxSize = 1 },
		func(p *Plan) { p.LogMaxBackups = 101 },
		func(p *Plan) { p.LogMaxAge = "9000h" },
		func(p *Plan) { p.LogOutput = "remote" },
		func(p *Plan) { p.LogOnWriteError = "ignore" },
	} {
		candidate := plan
		change(&candidate)
		if _, err := RenderFiles(candidate); err == nil {
			t.Fatal("unsafe logging parameters accepted")
		}
	}
}

func TestLoggingCannotOverwriteApprovedSources(t *testing.T) {
	plan := defaultPlan()
	plan.ProjectName = "logging-service"
	plan.Skills = []string{"add_logging"}
	plan.LogOutput = "file"
	for _, path := range []string{"README.md", "cmd/server/main.go", "cmd/server/main.go/log", "AGENT-MANIFEST.json", "internal"} {
		plan.LogPath = path
		if _, err := RenderFiles(plan); err == nil {
			t.Fatalf("source overlap accepted: %s", path)
		}
	}
}
