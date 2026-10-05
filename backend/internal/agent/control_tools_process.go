package agent

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"agentstudio/internal/contracts"
)

func controlGoIdentity(r Run) (contracts.ToolIdentity, string, error) {
	var tool contracts.ToolIdentity
	if r.FrozenDefinition == nil {
		return tool, "", fail("definition_unavailable", "缺少冻结工具来源")
	}
	for _, bound := range r.FrozenDefinition.ToolBindings {
		if bound.ToolID == "go" {
			tool = bound
		}
	}
	if tool.ToolID == "" || tool.Platform == nil || *tool.Platform != runtime.GOOS+"_"+runtime.GOARCH {
		return tool, "", fail("tool_identity_mismatch", "冻结Go工具平台与执行器不符")
	}
	binary, err := filepath.EvalSymlinks(filepath.Join(runtime.GOROOT(), "bin", "go"))
	if err != nil {
		return tool, "", fail("tool_unavailable", "固定Go二进制不可用")
	}
	info, err := buildinfo.ReadFile(binary)
	if err != nil || info.GoVersion != tool.Version || info.Path != "cmd/go" {
		return tool, "", fail("tool_identity_mismatch", "真实Go二进制版本与冻结版本不符")
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["GOOS"] != runtime.GOOS || settings["GOARCH"] != runtime.GOARCH {
		return tool, "", fail("tool_identity_mismatch", "真实Go二进制目标平台不符")
	}
	if tool.BinarySHA256 != nil {
		data, e := os.ReadFile(binary)
		if e != nil || contracts.BytesHash(data) != *tool.BinarySHA256 {
			return tool, "", fail("tool_identity_mismatch", "真实Go二进制hash与冻结身份不符")
		}
	}
	return tool, binary, nil
}

type controlModuleEnvironment struct {
	proxy, sumdb        string
	systemNetworkTables bool
}

func controlFrozenModuleEnvironment(r Run) (controlModuleEnvironment, error) {
	if r.FrozenDefinition == nil || r.FrozenDefinition.ContractVersion != r.FrozenDefinition.Definition.ContractVersion {
		return controlModuleEnvironment{}, fail("definition_unavailable", "Go依赖环境缺少同版冻结定义")
	}
	switch r.FrozenDefinition.ContractVersion {
	case "1.2.0":
		// An older frozen object did not contain GoModulePolicy. New fields or
		// worker environment must never backfill its historical source facts.
		return controlModuleEnvironment{proxy: "https://proxy.golang.org", sumdb: "sum.golang.org"}, nil
	case "1.3.0", "1.4.0":
		policy := r.FrozenDefinition.Definition.GoModulePolicy
		if err := contracts.ValidateVersion("GoModulePolicy", "1.3.0", policy); err != nil {
			return controlModuleEnvironment{}, fail("invalid_tool_environment", "Go依赖环境必须完整符合本Run冻结策略")
		}
		return controlModuleEnvironment{proxy: policy.ProxyURL, sumdb: policy.Sumdb, systemNetworkTables: true}, nil
	default:
		return controlModuleEnvironment{}, fail("invalid_tool_environment", "不支持该版本的受控Go依赖环境")
	}
}

func controlToolEnvironment(paths controlToolPaths, binary, command string, module controlModuleEnvironment) []string {
	proxy, sumdb := "off", "off"
	if command == "go.mod_tidy" {
		proxy, sumdb = module.proxy, module.sumdb
	}
	return []string{"PATH=" + filepath.Dir(binary), "HOME=" + paths.home, "TMPDIR=" + paths.temporary, "GOTMPDIR=" + paths.temporary, "GOCACHE=" + filepath.Join(paths.cache, "build"), "GOMODCACHE=" + filepath.Join(paths.cache, "mod"), "GOPATH=" + filepath.Join(paths.cache, "gopath"), "GOPROXY=" + proxy, "GOSUMDB=" + sumdb, "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOWORK=off", "GOENV=off", "GOVCS=*:off", "GOTELEMETRY=off", "LANG=C", "TZ=UTC"}
}

// controlSandboxCommand never falls back to an unconfined process. Go may only
// execute its fixed compiler/linker and compiled test binaries in Run temp.
func controlSandboxCommand(ctx context.Context, paths controlToolPaths, binary string, argv []string, command string, moduleSources ...controlModuleEnvironment) (*exec.Cmd, error) {
	// Omitted input is solely the historical 1.2 environment used by direct
	// sandbox probes. Actual execution always supplies the frozen Run policy.
	module := controlModuleEnvironment{proxy: "https://proxy.golang.org", sumdb: "sum.golang.org"}
	if len(moduleSources) > 1 {
		return nil, fail("invalid_tool_environment", "Go依赖环境来源必须唯一")
	}
	if len(moduleSources) == 1 {
		module = moduleSources[0]
	}
	if (module.proxy != "https://proxy.golang.org" && module.proxy != "https://goproxy.cn") || module.sumdb != "sum.golang.org" {
		return nil, fail("invalid_tool_environment", "Go依赖环境必须是准确公开代理与签名校验源")
	}
	root := filepath.Dir(filepath.Dir(binary))
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
			return nil, fail("tool_sandbox_unavailable", "缺少必需的macOS进程沙箱")
		}
		quote := strconv.Quote
		profile := `(version 1)(deny default)(allow process-fork)(allow sysctl-read)(allow mach-lookup)(allow signal (target self))`
		profile += `(allow process-exec (subpath ` + quote(root) + `) (subpath ` + quote(paths.temporary) + `))`
		profile += `(allow file-read-metadata)(allow file-read* (literal "/") (subpath "/System") (subpath "/usr/lib") (subpath "/Library/Apple/System/Library") (subpath "/dev") (subpath "/private/etc/ssl") (literal "/private/etc/resolv.conf") (literal "/private/var/run/resolv.conf") (literal "/private/etc/hosts") (literal "/private/etc/localtime") (subpath ` + quote(root) + `)`
		if module.systemNetworkTables {
			profile += ` (literal "/private/etc/protocols") (literal "/private/etc/services")`
		}
		for _, path := range []string{paths.candidate, paths.cache, paths.home, paths.temporary} {
			profile += ` (subpath ` + quote(path) + `)`
		}
		profile += `)(allow file-write* (literal "/dev/null")`
		for _, path := range []string{paths.candidate, paths.cache, paths.home, paths.temporary} {
			profile += ` (subpath ` + quote(path) + `)`
		}
		profile += `)`
		if command == "go.mod_tidy" {
			profile += `(allow network-outbound (remote tcp "*:443"))(allow network-outbound (remote udp "*:53"))(allow network-outbound (literal "/private/var/run/mDNSResponder"))`
		} else {
			profile += `(allow network-inbound (local ip "localhost:*"))(allow network-outbound (remote ip "localhost:*"))(allow network-bind (local ip "localhost:*"))`
		}
		args := append([]string{"-p", profile, binary}, argv...)
		cmd = exec.CommandContext(ctx, "/usr/bin/sandbox-exec", args...)
	case "linux":
		wrapper := ""
		for _, path := range []string{"/usr/bin/bwrap", "/bin/bwrap"} {
			if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
				wrapper = path
				break
			}
		}
		if wrapper == "" {
			return nil, fail("tool_sandbox_unavailable", "缺少必需的Linux bubblewrap沙箱")
		}
		args := []string{"--die-with-parent", "--unshare-all", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp"}
		if command == "go.mod_tidy" {
			args = append(args, "--share-net")
		}
		readOnly := []string{root, "/usr/lib", "/usr/lib64", "/lib", "/lib64", "/etc/ld.so.cache", "/etc/ssl", "/etc/resolv.conf", "/etc/hosts"}
		if module.systemNetworkTables {
			readOnly = append(readOnly, "/etc/protocols", "/etc/services")
		}
		for _, path := range readOnly {
			if _, err := os.Stat(path); err == nil {
				args = append(args, "--ro-bind", path, path)
			}
		}
		for _, path := range []string{paths.candidate, paths.cache, paths.home, paths.temporary} {
			args = append(args, "--bind", path, path)
		}
		args = append(args, "--chdir", paths.candidate, "--", binary)
		cmd = exec.CommandContext(ctx, wrapper, append(args, argv...)...)
	default:
		return nil, fail("tool_sandbox_unavailable", "当前平台没有受控工具沙箱")
	}
	cmd.Dir = paths.candidate
	cmd.Env = controlToolEnvironment(paths, binary, command, module)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd, nil
}

func (s *Service) controlRunTool(ctx context.Context, r Run, request contracts.ExecutionRequest, command contracts.PostCommand, tool contracts.ToolIdentity, binary string, paths controlToolPaths, reservation contracts.BudgetReservation) (contracts.ToolExecutionReceipt, error) {
	var argv []string
	if err := json.Unmarshal(command.Arguments, &argv); err != nil {
		return contracts.ToolExecutionReceipt{}, err
	}
	module, err := controlFrozenModuleEnvironment(r)
	if err != nil {
		return contracts.ToolExecutionReceipt{}, err
	}
	cmd, err := controlSandboxCommand(ctx, paths, binary, argv, command.CommandID, module)
	if err != nil {
		return contracts.ToolExecutionReceipt{}, err
	}
	output := newControlToolOutput([]string{s.Config.GitLabToken, s.Config.AIKey})
	cmd.Stdout, cmd.Stderr = output, output
	start := time.Now().UTC()
	runErr := cmd.Run()
	end := time.Now().UTC()
	text, truncated := output.finish()
	status := "succeeded"
	var code *int64
	if cmd.ProcessState != nil {
		value := int64(cmd.ProcessState.ExitCode())
		code = &value
	}
	if ctx.Err() != nil {
		status = "interrupted"
	} else if code == nil {
		status = "unknown"
	} else if *code != 0 {
		status = "failed"
	}
	ref := controlManifestRef(*r.Control.Audit.Manifest)
	receipt := contracts.ToolExecutionReceipt{ContractVersion: request.ContractVersion, ReceiptID: "receipt_" + strings.TrimPrefix(reservation.AttemptID, "tool_"), RunID: request.RunID, SourceVersion: request.SourceVersion, DefinitionSnapshotHash: request.DefinitionSnapshotHash, ExecutionID: request.ExecutionID, OperationID: request.OperationID, AttemptID: reservation.AttemptID, RequestHash: request.RequestHash, ApprovalID: request.Approval.ApprovalID, ApprovalHash: r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: request.Approval.ProposalSnapshotHash, PlanHash: request.PlanHash, Workspace: request.Workspace, Tool: tool, CommandID: command.CommandID, Arguments: append(contracts.ToolExecutionReceiptArguments(nil), command.Arguments...), CwdScope: "run_workspace", EnvironmentPolicy: "isolated", InputManifestRef: &contracts.ToolExecutionReceiptInputManifestRef{ObjectType: ref.ObjectType, ObjectID: ref.ObjectID, ObjectVersion: ref.ObjectVersion, SourceVersion: ref.SourceVersion, Hash: ref.Hash}, StartedAt: start.Format(time.RFC3339Nano), CompletedAt: end.Format(time.RFC3339Nano), ExitCode: code, Status: status, Output: text, OutputSHA256: contracts.BytesHash([]byte(text)), OutputTruncated: truncated}
	if runErr != nil {
		return receipt, fail("tool_"+status, "受控Go命令未成功："+s.redact(runErr.Error()))
	}
	return receipt, nil
}

// Redact before buffering. A small pending suffix handles secrets split across
// arbitrary stdout/stderr writes. Raw output is never written to disk/logs.
type controlToolOutput struct {
	mu             sync.Mutex
	secrets        [][]byte
	pending, saved []byte
	maxSecret      int
	truncated      bool
}

func newControlToolOutput(secrets []string) *controlToolOutput {
	b := &controlToolOutput{maxSecret: 1}
	for _, secret := range secrets {
		if secret != "" {
			b.secrets = append(b.secrets, []byte(secret))
			if len(secret) > b.maxSecret {
				b.maxSecret = len(secret)
			}
		}
	}
	return b
}
func (b *controlToolOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		size := min(len(p), 4096)
		b.pending = append(b.pending, p[:size]...)
		p = p[size:]
		b.drain(false)
	}
	return n, nil
}
func (b *controlToolOutput) drain(final bool) {
	for len(b.pending) > 0 && (final || len(b.pending) >= b.maxSecret) {
		n := 1
		chunk := b.pending[:1]
		for _, secret := range b.secrets {
			if bytes.HasPrefix(b.pending, secret) {
				n = len(secret)
				chunk = []byte("[redacted]")
				break
			}
		}
		if len(b.saved)+len(chunk) <= 65540 {
			b.saved = append(b.saved, chunk...)
		} else {
			b.truncated = true
		}
		b.pending = b.pending[n:]
	}
}
func (b *controlToolOutput) finish() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.drain(true)
	text := strings.ToValidUTF8(string(b.saved), "�")
	if len(text) > 65536 {
		text = text[:65536]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
		b.truncated = true
	}
	return text, b.truncated
}

var _ io.Writer = (*controlToolOutput)(nil)
