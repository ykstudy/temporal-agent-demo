package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"agentstudio/internal/contracts"
)

// These probes never execute an unverified binary or treat the frozen list as
// an observation. Version metadata belongs to the exact published binary hash.
func controlGitIdentity(r Run) (contracts.ToolIdentity, string, error) {
	binary, err := exec.LookPath("git")
	if err != nil {
		return contracts.ToolIdentity{}, "", fail("tool_unavailable", "固定Git工具不可用")
	}
	return controlGitBinaryIdentity(r, "git", binary)
}

func controlGitHTTPIdentity(r Run) (contracts.ToolIdentity, string, error) {
	_, binary, err := controlGitIdentity(r)
	if err != nil {
		return contracts.ToolIdentity{}, "", err
	}
	// The supported release installs Git and its HTTP/HTTPS helper under the
	// same prefix. Never invoke --exec-path on an untrusted host executable.
	root := filepath.Dir(filepath.Dir(binary))
	paths := []string{filepath.Join(root, "libexec", "git-core", "git-remote-http"), filepath.Join(root, "lib", "git-core", "git-remote-http")}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return controlGitBinaryIdentity(r, "git_remote_http", path)
		}
	}
	return contracts.ToolIdentity{}, "", fail("tool_unavailable", "冻结Git HTTP helper不可用")
}

func controlGitBinaryIdentity(r Run, id, path string) (contracts.ToolIdentity, string, error) {
	var tool contracts.ToolIdentity
	if r.FrozenDefinition == nil || !contracts.DeliveryContract(r.FrozenDefinition.ContractVersion) {
		return tool, "", fail("tool_identity_mismatch", "受控Git需要1.3完整冻结身份")
	}
	for _, candidate := range r.FrozenDefinition.ToolBindings {
		if candidate.ToolID == id {
			if tool.ToolID != "" {
				return tool, "", fail("tool_identity_mismatch", "Git工具身份不唯一")
			}
			tool = candidate
		}
	}
	if tool.ToolID == "" || tool.Platform == nil || *tool.Platform != runtime.GOOS+"_"+runtime.GOARCH || tool.BinarySHA256 == nil {
		return tool, "", fail("tool_identity_mismatch", "Git版本、平台及原二进制摘要未完整冻结")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return tool, "", fail("tool_unavailable", "Git二进制路径不可核实")
	}
	stat, err := os.Stat(resolved)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm()&0111 == 0 {
		return tool, "", fail("tool_unavailable", "Git身份不是可执行普通文件")
	}
	bytes, err := os.ReadFile(resolved)
	if err != nil || contracts.BytesHash(bytes) != *tool.BinarySHA256 {
		return tool, "", fail("tool_identity_mismatch", "实际Git二进制与冻结摘要不符")
	}
	return tool, resolved, nil
}
