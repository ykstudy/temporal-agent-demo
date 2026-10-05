package agent

import (
	"bytes"
	"context"
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"agentstudio/internal/contracts"
)

var controlCoverageLine = regexp.MustCompile(`^([^\s:]+(?:/[^\s:]*)?):([0-9]+)\.([0-9]+),([0-9]+)\.([0-9]+) ([0-9]+) ([0-9]+)$`)

// Parse all coverage records, not only a magic prefix. At least one actual
// instrumented source block must exist; malformed trailing data cannot pass.
func controlCIArtifactFormat(path string, data []byte) (string, error) {
	bad := func() (string, error) {
		return "", fail("ci_artifact_invalid", "CI工件内容或可执行结构无效")
	}
	if len(data) == 0 || len(data) > 16<<20 {
		return bad()
	}
	if path == "coverage.out" {
		if !utf8.Valid(data) || !bytes.HasSuffix(data, []byte("\n")) {
			return bad()
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if len(lines) < 2 || !chainContains([]string{"mode: set", "mode: count", "mode: atomic"}, lines[0]) {
			return bad()
		}
		seen := map[string]bool{}
		for _, line := range lines[1:] {
			parts := controlCoverageLine.FindStringSubmatch(line)
			if parts == nil || seen[line] {
				return bad()
			}
			seen[line] = true
			n := make([]uint64, 6)
			for i := range n {
				v, e := strconv.ParseUint(parts[i+2], 10, 64)
				if e != nil {
					return bad()
				}
				n[i] = v
			}
			if n[0] == 0 || n[1] == 0 || n[2] == 0 || n[3] == 0 || n[4] == 0 || n[0] > n[2] || n[0] == n[2] && n[1] >= n[3] || lines[0] == "mode: set" && n[5] > 1 {
				return bad()
			}
		}
		return "go_coverage", nil
	}
	if path != "bin/server" {
		return bad()
	}
	if bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) {
		file, err := elf.NewFile(bytes.NewReader(data))
		if err != nil {
			return bad()
		}
		defer file.Close()
		if (file.Type != elf.ET_EXEC && file.Type != elf.ET_DYN) || file.Entry == 0 || !chainContains([]string{"EM_X86_64", "EM_AARCH64"}, file.Machine.String()) {
			return bad()
		}
		entry := false
		for _, p := range file.Progs {
			if p.Off > uint64(len(data)) || p.Filesz > uint64(len(data))-p.Off || p.Filesz > p.Memsz {
				return bad()
			}
			if p.Type == elf.PT_LOAD && p.Flags&elf.PF_X != 0 && file.Entry >= p.Vaddr && file.Entry-p.Vaddr < p.Filesz {
				entry = true
			}
		}
		if !entry {
			return bad()
		}
		return "elf_executable", nil
	}
	file, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return bad()
	}
	defer file.Close()
	if file.Type != macho.TypeExec || file.Cpu != macho.CpuAmd64 && file.Cpu != macho.CpuArm64 {
		return bad()
	}
	code := false
	for _, load := range file.Loads {
		if seg, ok := load.(*macho.Segment); ok {
			if seg.Offset > uint64(len(data)) || seg.Filesz > uint64(len(data))-seg.Offset {
				return bad()
			}
		}
	}
	for _, section := range file.Sections {
		if section.Name == "__text" && section.Seg == "__TEXT" && section.Size > 0 && uint64(section.Offset) <= uint64(len(data)) && section.Size <= uint64(len(data))-uint64(section.Offset) {
			code = true
		}
	}
	if !code {
		return bad()
	}
	return "mach_o_executable", nil
}

func (s *Service) controlCIArtifactRoot(r Run, x contracts.ExecutionRequest) (*os.Root, error) {
	bound := r
	bound.Workspace = filepath.Join(s.Config.DataDir, "workspaces", filepath.FromSlash(x.Workspace.StorageKey))
	paths, err := s.controlToolPaths(bound, x, "ci_artifact")
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(paths.runRoot, ".control-ci-artifacts")
	if within(paths.approved, dir) || within(dir, paths.approved) {
		return nil, fail("invalid_path", "CI工件目录与批准目录重叠")
	}
	return os.OpenRoot(paths.runRoot)
}

func (s *Service) controlCIArtifactHTTP(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.Config.GitLabURL, "/")+"/api/v4"+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("PRIVATE-TOKEN", s.Config.GitLabToken)
	client := http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, 0, errors.Join(ctx.Err(), &InfrastructureError{Reason: "CI artifact request interrupted"})
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, fail("ci_artifact_unavailable", fmt.Sprintf("CI工件读取HTTP %d", response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil {
		return nil, response.StatusCode, errors.Join(ctx.Err(), &InfrastructureError{Reason: "CI artifact response interrupted"})
	}
	if len(data) == 0 || len(data) > 16<<20 {
		return nil, response.StatusCode, fail("ci_artifact_invalid", "CI工件为空或超界")
	}
	return data, response.StatusCode, nil
}

func (s *Service) controlCIDownload(ctx context.Context, id string, x contracts.ExecutionRequest, project, pipeline, job int64, path, readKind string) error {
	readPath := remoteProjectReadPath(project) + "/jobs/" + strconv.FormatInt(job, 10) + "/artifacts/" + url.PathEscape(path)
	var data []byte
	_, err := s.controlRemoteRead(ctx, id, x, readPath, readKind, nil, func(bounded context.Context) (int, error) {
		var status int
		var err error
		data, status, err = s.controlCIArtifactHTTP(bounded, readPath)
		return status, err
	}, func() (controlRemoteFact, error) {
		format, err := controlCIArtifactFormat(path, data)
		if err != nil {
			return controlRemoteFact{}, err
		}
		if err = s.controlPackageSecretBoundary(data); err != nil {
			return controlRemoteFact{}, err
		}
		r, err := s.Store.Get(id)
		if err != nil {
			return controlRemoteFact{}, err
		}
		root, err := s.controlCIArtifactRoot(r, x)
		if err != nil {
			return controlRemoteFact{}, err
		}
		defer root.Close()
		artifactID, filename := controlCIArtifactIdentity(x, pipeline, job, path)
		// Re-reading an immutable original artifact never overwrites its bytes.
		if _, e := root.Lstat(filename); e == nil {
			previous, e := controlReadPackageAt(root, filename, 16<<20)
			if e != nil || !bytes.Equal(previous, data) {
				return controlRemoteFact{}, fail("ci_artifact_changed", "原CI工件字节已变化")
			}
		} else if !os.IsNotExist(e) {
			return controlRemoteFact{}, e
		} else if err = controlPublishRaw(root, ".control-ci-artifacts", filename, data); err != nil {
			return controlRemoteFact{}, err
		}
		hash := contracts.BytesHash(data)
		fact := contracts.RemoteArtifactEvidence{Path: path, ArtifactRef: contracts.RemoteArtifactEvidenceArtifactRef{ObjectType: "ci_artifact", ObjectID: artifactID, ObjectVersion: 1, SourceVersion: x.SourceVersion, Hash: hash}, ByteCount: int64(len(data)), SHA256: hash, Format: format, Validated: true}
		return controlRemoteFact{"artifact", fact}, nil
	})
	return err
}

func (s *Service) ReadControlCIArtifact(id string, ref contracts.ObjectReference) ([]byte, error) {
	r, err := s.Store.Get(id)
	if err != nil {
		return nil, err
	}
	return s.readControlCIArtifact(r, ref)
}

func (s *Service) readControlCIArtifact(r Run, ref contracts.ObjectReference) ([]byte, error) {
	if r.Control == nil || ref.ObjectType != "ci_artifact" || contracts.Validate("ObjectReference", ref) != nil {
		return nil, fail("object_not_found", "没有准确CI工件引用")
	}
	for _, q := range r.Control.Audit.RemoteReceipts {
		if q.Pipeline == nil || ValidateControlRemoteReceipt(r, q) != nil {
			continue
		}
		for _, job := range q.Jobs {
			for _, artifact := range job.Artifacts {
				if !chainEqual(artifact.ArtifactRef, ref) {
					continue
				}
				for _, x := range r.Control.Audit.Executions {
					if x.ExecutionID != q.ExecutionID || x.RequestHash != q.RequestHash {
						continue
					}
					id, filename := controlCIArtifactIdentity(x, q.Pipeline.PipelineID, job.JobID, artifact.Path)
					if id != ref.ObjectID {
						return nil, fail("ci_artifact_mismatch", "工件ID不绑定原operation/job/path")
					}
					root, err := s.controlCIArtifactRoot(r, x)
					if err != nil {
						return nil, err
					}
					defer root.Close()
					data, err := controlReadPackageAt(root, filename, 16<<20)
					if err != nil {
						return nil, err
					}
					if int64(len(data)) != artifact.ByteCount || contracts.BytesHash(data) != ref.Hash || artifact.SHA256 != ref.Hash {
						return nil, fail("ci_artifact_changed", "不可变CI工件实际字节已变化")
					}
					format, err := controlCIArtifactFormat(artifact.Path, data)
					if err != nil || format != artifact.Format || !artifact.Validated {
						return nil, fail("ci_artifact_invalid", "CI工件完整格式核对失败")
					}
					if err = s.controlPackageSecretBoundary(data); err != nil {
						return nil, err
					}
					return data, nil
				}
			}
		}
	}
	return nil, fail("object_not_found", "没有原回执绑定的CI工件")
}
