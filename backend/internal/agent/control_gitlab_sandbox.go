package agent

import (
	"context"
	"debug/macho"
	"encoding/base64"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agentstudio/internal/contracts"
)

func controlGitRemoteURL(target contracts.GitLabTarget) string {
	return strings.TrimRight(target.InstanceURL, "/") + "/" + target.NamespacePath + "/" + target.ProjectPath + ".git"
}

func controlGitArguments(policy contracts.GitCommandPolicy, x contracts.ExecutionRequest, target contracts.GitLabTarget) []string {
	replace := strings.NewReplacer("{ref}", target.Ref, "{operation_marker}", controlGitLabMarker(x), "{remote_url}", controlGitRemoteURL(target))
	args := make([]string, len(policy.ArgumentsTemplate))
	for i, value := range policy.ArgumentsTemplate {
		args[i] = replace.Replace(value)
	}
	return args
}

func controlGitAuthorization(token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("oauth2:"+token))
}

func controlGitEnvironment(paths controlToolPaths, binary, helper string, x contracts.ExecutionRequest, target contracts.GitLabTarget, command, token string) []string {
	env := []string{
		"PATH=" + filepath.Dir(binary), "GIT_EXEC_PATH=" + filepath.Dir(helper), "HOME=" + paths.home, "XDG_CONFIG_HOME=" + paths.home, "TMPDIR=" + paths.temporary,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/dev/null",
		"GIT_ALLOW_PROTOCOL=http:https", "GIT_ATTR_NOSYSTEM=1", "GIT_LFS_SKIP_SMUDGE=1", "GIT_OPTIONAL_LOCKS=0",
		"GIT_AUTHOR_NAME=Agent Studio", "GIT_COMMITTER_NAME=Agent Studio", "GIT_AUTHOR_EMAIL=agent@agentstudio.invalid", "GIT_COMMITTER_EMAIL=agent@agentstudio.invalid",
		"GIT_AUTHOR_DATE=" + x.CreatedAt, "GIT_COMMITTER_DATE=" + x.CreatedAt, "LANG=C", "LC_ALL=C", "TZ=UTC",
		"GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0=", "GIT_CONFIG_KEY_1=core.hooksPath", "GIT_CONFIG_VALUE_1=/dev/null",
	}
	if command == "git.push" {
		env = append(env, "GIT_CONFIG_COUNT=3", "GIT_CONFIG_KEY_2=http."+controlGitRemoteURL(target)+".extraHeader", "GIT_CONFIG_VALUE_2=Authorization: "+controlGitAuthorization(token))
	} else {
		env = append(env, "GIT_CONFIG_COUNT=2")
	}
	return env
}

// Read Mach-O dependency names from already verified binaries; never execute a
// shell, otool, loader or guessed helper to discover additional permissions.
func controlGitLibraries(binaries ...string) ([]string, error) {
	seen := map[string]bool{}
	var visit func(string) error
	visit = func(path string) error {
		if strings.HasPrefix(path, "/usr/lib/") || strings.HasPrefix(path, "/System/Library/") {
			return nil // OS shared-cache libraries have a fixed read-only boundary.
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fail("tool_library_unavailable", "固定Git动态库路径不可核实")
		}
		if seen[resolved] {
			return nil
		}
		seen[resolved] = true
		file, err := macho.Open(resolved)
		if err != nil {
			return fail("tool_library_unavailable", "固定Git动态库不是可核实的当前平台Mach-O")
		}
		imports, err := file.ImportedLibraries()
		_ = file.Close()
		if err != nil {
			return err
		}
		for _, library := range imports {
			if strings.HasPrefix(library, "@loader_path/") {
				library = filepath.Join(filepath.Dir(resolved), strings.TrimPrefix(library, "@loader_path/"))
			}
			if !filepath.IsAbs(library) {
				return fail("tool_library_unavailable", "固定Git发布含未解析动态库来源")
			}
			if err = visit(library); err != nil {
				return err
			}
		}
		return nil
	}
	for _, path := range binaries {
		if err := visit(path); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func controlGitNetwork(target contracts.GitLabTarget) (string, error) {
	u, err := url.Parse(target.InstanceURL)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fail("target_mismatch", "Git网络需要原批准的无凭据HTTP实例")
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	value, err := strconv.Atoi(port)
	if err != nil || value <= 0 || value > 65535 {
		return "", fail("target_mismatch", "Git目标端口无效")
	}
	// Seatbelt only accepts '*' or 'localhost', not arbitrary IP addresses.
	// The kernel constrains the port; the immutable argv, disabled redirects,
	// isolated config and URL-scoped header constrain the approved host/path.
	host := "*"
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" {
		host = "localhost"
	}
	return host + ":" + port, nil
}

func controlGitSandboxCommand(ctx context.Context, paths controlToolPaths, binary, helper string, x contracts.ExecutionRequest, target contracts.GitLabTarget, policy contracts.GitCommandPolicy, token string) (*exec.Cmd, error) {
	if runtime.GOOS != "darwin" {
		return nil, fail("tool_sandbox_unavailable", "本Git发布尚无当前平台的受控进程沙箱")
	}
	if _, err := os.Stat("/usr/bin/sandbox-exec"); err != nil {
		return nil, fail("tool_sandbox_unavailable", "固定Git执行需要macOS进程沙箱")
	}
	libraries, err := controlGitLibraries(binary, helper)
	if err != nil {
		return nil, err
	}
	quote := strconv.Quote
	profile := `(version 1)(deny default)(allow process-fork)(allow sysctl-read)(allow mach-lookup)(allow signal (target self))(allow file-read-metadata)`
	profile += `(allow process-exec (literal ` + quote(binary) + `) (literal ` + quote(helper) + `))`
	profile += `(allow file-read* (literal "/") (subpath "/System") (subpath "/usr/lib") (subpath "/Library/Apple/System/Library") (subpath "/dev") (subpath "/private/etc/ssl") (literal "/private/etc/localtime")`
	for _, path := range libraries {
		profile += ` (literal ` + quote(path) + `)`
	}
	for _, path := range []string{paths.candidate, paths.home, paths.temporary} {
		profile += ` (subpath ` + quote(path) + `)`
	}
	profile += `)(allow file-write* (literal "/dev/null")`
	for _, path := range []string{paths.candidate, paths.home, paths.temporary} {
		profile += ` (subpath ` + quote(path) + `)`
	}
	profile += `)`
	if policy.CommandID == "git.push" {
		remote, err := controlGitNetwork(target)
		if err != nil {
			return nil, err
		}
		profile += `(allow network-outbound (literal "/private/var/run/mDNSResponder"))`
		profile += `(allow network-outbound (remote tcp ` + quote(remote) + `))`
	}
	argv := append([]string{"-p", profile, binary}, controlGitArguments(policy, x, target)...)
	cmd := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", argv...)
	cmd.Dir = paths.candidate
	cmd.Env = controlGitEnvironment(paths, binary, helper, x, target, policy.CommandID, token)
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
