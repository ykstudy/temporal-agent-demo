package agent

import (
	"context"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

// The test server is a local bare Git repository served by the actual Git HTTP
// backend. Only the client runs inside the product's process sandbox.
func controlGitHTTPFixture(t *testing.T) (string, string, *atomic.Int64) {
	handler, bare, writes := controlGitHTTPServer(t)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL, bare, writes
}

func controlGitHTTPServer(t *testing.T) (http.Handler, string, *atomic.Int64) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "schedule-group", "create-service.git")
	for _, args := range [][]string{{"init", "--bare", bare}, {"-C", bare, "config", "http.receivepack", "true"}, {"-C", bare, "config", "receive.advertisePushOptions", "true"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("local Git server setup: %v %s", err, out)
		}
	}
	backend, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Fatal(err)
	}
	handler := &cgi.Handler{Path: filepath.Join(strings.TrimSpace(string(backend)), "git-http-backend"), Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"}}
	writes := &atomic.Int64{}
	server := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != controlGitAuthorization("controlled-gitlab-test-secret") {
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			writes.Add(1)
		}
		handler.ServeHTTP(w, r)
	})
	return server, bare, writes
}

func TestControlGitSandboxActualSixCommands(t *testing.T) {
	s := testService(t)
	r := newControlCompletionRun(t, s, "rules")
	_, binary, err := controlGitIdentity(r)
	if err != nil {
		t.Fatal(err)
	}
	_, helper, err := controlGitHTTPIdentity(r)
	if err != nil {
		t.Fatal(err)
	}
	remote, bare, writes := controlGitHTTPFixture(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := controlToolPaths{runRoot: root, candidate: filepath.Join(root, "candidate"), home: filepath.Join(root, "home"), temporary: filepath.Join(root, "tmp"), cache: filepath.Join(root, "cache")}
	if err = paths.prepare(); err != nil {
		t.Fatal(err)
	}
	if err = controlCopyFiles(paths.candidate, map[string]string{"main.go": "package main\nfunc main() {}\n"}); err != nil {
		t.Fatal(err)
	}
	x := contracts.ExecutionRequest{CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), OperationID: "sandbox_test", RequestHash: strings.Repeat("a", 64)}
	target := contracts.GitLabTarget{InstanceURL: remote, NamespacePath: "schedule-group", ProjectPath: "create-service", Ref: "main"}
	var head string
	for _, policy := range r.FrozenDefinition.Definition.RepositoryPolicy.GitExecution.Commands {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		cmd, err := controlGitSandboxCommand(ctx, paths, binary, helper, x, target, policy, "controlled-gitlab-test-secret")
		if err != nil {
			cancel()
			t.Fatal(policy.CommandID, err)
		}
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s: %v %s", policy.CommandID, err, out)
		}
		if policy.CommandID == "git.head" {
			head = strings.TrimSpace(string(out))
		}
	}
	actual, err := exec.Command("git", "-C", bare, "rev-parse", "refs/heads/main").Output()
	if err != nil || strings.TrimSpace(string(actual)) != head || writes.Load() != 1 {
		t.Fatal("actual remote SHA or write count", err, string(actual), head, writes.Load())
	}
	config, err := os.ReadFile(filepath.Join(paths.candidate, ".git", "config"))
	if err != nil || strings.Contains(string(config), "controlled-gitlab-test-secret") || strings.Contains(string(config), controlGitAuthorization("controlled-gitlab-test-secret")) {
		t.Fatal("credential reached repository config", err)
	}
	outside := filepath.Join(t.TempDir(), "outside-config")
	if err = os.WriteFile(outside, []byte("[private]\nvalue = fixture-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"config", "--file", outside, "--get", "private.value"}, {"config", "--file", outside, "private.value", "changed"}} {
		policy := contracts.GitCommandPolicy{CommandID: "git.head", ArgumentsTemplate: args}
		cmd, e := controlGitSandboxCommand(context.Background(), paths, binary, helper, x, target, policy, "controlled-gitlab-test-secret")
		if e != nil {
			t.Fatal(e)
		}
		out, e := cmd.CombinedOutput()
		if e == nil || strings.Contains(string(out), "fixture-secret") {
			t.Fatal("Git sandbox allowed unrelated file read/write", e, string(out))
		}
	}
	unchanged, err := os.ReadFile(outside)
	if err != nil || !strings.Contains(string(unchanged), "fixture-secret") {
		t.Fatal("outside file changed", err)
	}
	otherCalls := &atomic.Int64{}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { otherCalls.Add(1); http.Error(w, "unexpected", 500) }))
	defer other.Close()
	policy := contracts.GitCommandPolicy{CommandID: "git.push", ArgumentsTemplate: []string{"ls-remote", other.URL + "/repo.git"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd, err := controlGitSandboxCommand(ctx, paths, binary, helper, x, target, policy, "controlled-gitlab-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := cmd.CombinedOutput(); err == nil || otherCalls.Load() != 0 {
		t.Fatal("Git sandbox allowed unrelated network port", err, string(out), otherCalls.Load())
	}
}
