package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"agentstudio/internal/contracts"
)

// controlPackageEntries constructs the exact approved byte set plus one
// explicitly derived metadata entry. It never writes to the approved tree.
func controlPackageEntries(r Run, files map[string]string) (map[string][]byte, error) {
	if r.Control == nil || r.FrozenDefinition == nil || r.Control.Audit.Plan == nil || r.Control.Audit.Manifest == nil || r.Control.Audit.Proposal == nil {
		return nil, fail("package_not_ready", "打包缺少完整批准来源")
	}
	m := r.Control.Audit.Manifest
	p := r.Control.Audit.Proposal
	if !hashMatches("ArtifactManifest", *m, m.Hash) || m.RunID != r.ID || m.SourceVersion != int64(r.SourceVersion) || m.DefinitionSnapshotHash != r.FrozenDefinition.SnapshotHash || m.ProposalSnapshotHash != p.SnapshotHash || m.ProposalID != p.ProposalID || m.ProposalVersion != p.ProposalVersion || m.PlanHash != r.Control.Audit.Plan.PlanHash {
		return nil, fail("artifact_source_mismatch", "包清单未绑定当前完整批准")
	}
	if err := controlMatchApproved(r, files); err != nil {
		return nil, err
	}
	if _, collision := files[manifestName]; collision {
		return nil, fail("package_metadata_collision", "批准文件与包内manifest元数据保留名冲突")
	}
	prefix := r.Control.Audit.Plan.ProjectName
	if prefix == "" || prefix == "." || prefix == ".." || strings.ContainsAny(prefix, "/\\") {
		return nil, fail("invalid_path", "包项目名称不是独立安全目录名")
	}
	budget := r.FrozenDefinition.Definition.BudgetPolicy
	if int64(len(files)) > budget.MaxFiles {
		return nil, fail("artifact_budget_exceeded", "包文件数量超过冻结预算")
	}
	entries := map[string][]byte{}
	var size int64
	for name, content := range files {
		if err := contracts.Validate("agent.schema.json#/$defs/RelativePath", name); err != nil {
			return nil, err
		}
		if !utf8.ValidString(content) || int64(len(content)) > budget.MaxArtifactBytes-size {
			return nil, fail("artifact_budget_exceeded", "包源文件编码或字节不满足冻结预算")
		}
		size += int64(len(content))
		entries[prefix+"/"+name] = []byte(content)
	}
	metadata, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	metadata, err = contracts.Canonicalize(metadata)
	if err != nil {
		return nil, err
	}
	entries[prefix+"/"+manifestName] = metadata
	return entries, nil
}

func controlPackageLimit(r Run) int64 {
	// Metadata is bounded by the public codec; every entry gets a bounded ZIP
	// header allowance. This is an output bound, not a larger source budget.
	b := r.FrozenDefinition.Definition.BudgetPolicy
	return min(int64(4_194_304), b.MaxArtifactBytes+16_777_216+(b.MaxFiles+1)*1024)
}

func controlPackageBytes(r Run, files map[string]string) ([]byte, error) {
	entries, err := controlPackageEntries(r, files)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		header.SetMode(0600)
		entry, err := w.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err = entry.Write(entries[name]); err != nil {
			return nil, err
		}
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	if int64(out.Len()) > controlPackageLimit(r) {
		return nil, fail("artifact_budget_exceeded", "包实际字节超过有界输出限制")
	}
	return out.Bytes(), nil
}

func verifyControlPackageBytes(r Run, files map[string]string, data []byte) error {
	entries, err := controlPackageEntries(r, files)
	if err != nil {
		return err
	}
	if int64(len(data)) > controlPackageLimit(r) {
		return fail("artifact_budget_exceeded", "包读取超过有界输出限制")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if len(reader.File) != len(entries) {
		return fail("package_integrity_failed", "包条目集合与完整批准集合不同")
	}
	previous := ""
	for _, f := range reader.File {
		expected, exists := entries[f.Name]
		if !exists || f.Name <= previous || path.Clean(f.Name) != f.Name || strings.HasPrefix(f.Name, "/") || strings.Contains(f.Name, "\\") || !f.Mode().IsRegular() || f.Mode().Perm() != 0600 || f.Method != zip.Store || f.UncompressedSize64 != uint64(len(expected)) {
			return fail("package_integrity_failed", "包包含非批准、重复、链接或无效条目")
		}
		previous = f.Name
		stream, err := f.Open()
		if err != nil {
			return err
		}
		actual, readErr := io.ReadAll(io.LimitReader(stream, int64(len(expected))+1))
		closeErr := stream.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !bytes.Equal(expected, actual) {
			return fail("package_integrity_failed", "包实际内容或manifest原文不符")
		}
	}
	canonical, err := controlPackageBytes(r, files)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, data) {
		return fail("package_integrity_failed", "包含额外字节或未冻结的归档元数据")
	}
	return nil
}

func controlPackageIdentity(operation string) (id, filename string) {
	id = "package_" + contracts.BytesHash([]byte(operation))[:40]
	return id, ".control-packages/" + id + ".zip"
}

func (s *Service) controlPackageSecretBoundary(data []byte) error {
	for _, secret := range []string{s.Config.GitLabToken, s.Config.AIKey} {
		if secret != "" && bytes.Contains(data, []byte(secret)) {
			return fail("secret_output", "归档文件、路径或元数据包含执行器凭据")
		}
	}
	return nil
}

func (s *Service) controlPackageRoot(r Run, request contracts.ExecutionRequest) (*os.Root, error) {
	paths, err := s.controlToolPaths(r, request, "package")
	if err != nil {
		return nil, err
	}
	if within(paths.approved, filepath.Join(paths.runRoot, ".control-packages")) || within(filepath.Join(paths.runRoot, ".control-packages"), paths.approved) {
		return nil, fail("invalid_path", "包输出目录与批准目录重叠")
	}
	return os.OpenRoot(paths.runRoot)
}

func controlReadPackageAt(root *os.Root, filename string, limit int64) ([]byte, error) {
	if err := controlWorkspaceSafeRelative(root, filename); err != nil {
		return nil, err
	}
	f, err := root.OpenFile(filename, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fail("package_integrity_failed", "包文件类型或实际字节超界")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fail("artifact_budget_exceeded", "包读取期间超出字节界限")
	}
	return data, err
}

// controlPublishPackage never overwrites an already published operation path.
// The atomic hard-link is confined to the same root and filesystem.
func controlPublishPackage(root *os.Root, filename string, data []byte) error {
	return controlPublishRaw(root, ".control-packages", filename, data)
}

func controlPublishRaw(root *os.Root, directory, filename string, data []byte) error {
	if err := controlWorkspaceSafeRelative(root, filename); err != nil {
		return err
	}
	if _, err := root.Lstat(filename); !os.IsNotExist(err) {
		return fail("package_exists", "原操作包路径已存在，必须只读核实")
	}
	if err := root.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err := controlWorkspaceSafeRelative(root, filename); err != nil {
		return err
	}
	temp := directory + "/.pending-" + newID("raw_")
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	actual, err := controlReadPackageAt(root, temp, int64(len(data)))
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, data) {
		return fail("package_integrity_failed", "原子发布前包字节发生变化")
	}
	if err = root.Link(temp, filename); err != nil {
		return err
	}
	dir, err := root.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ExecuteControlPackage packages an already issued request. Prior successful
// receipts are replayed read-only; an uncertain original write cannot be retried.
func (s *Service) ExecuteControlPackage(ctx context.Context, id string, request contracts.ExecutionRequest) (Run, contracts.PackageReceipt, error) {
	var empty contracts.PackageReceipt
	r, err := s.Store.Get(id)
	if err != nil {
		return r, empty, err
	}
	kind, raw, err := scheduleAction(request.Action)
	if err != nil || kind != "package" || request.RunID != id || r.Control == nil || !contracts.DeliveryContract(request.ContractVersion) || !contracts.DeliveryContract(r.Control.Audit.ContractVersion) {
		return r, empty, fail("package_not_ready", "打包仅接受具有完整1.3公开回执的准确package请求")
	}
	var params contracts.PackageParameters
	if err = json.Unmarshal(raw, &params); err != nil {
		return r, empty, err
	}
	for _, old := range r.Control.Audit.PackageReceipts {
		if old.ExecutionID != request.ExecutionID || old.Status != "succeeded" {
			continue
		}
		if err = controlPackageReceiptBinding(r, request, old); err != nil {
			return r, empty, err
		}
		if _, err = s.ReadControlPackage(id, *scheduleRef(old.PackageRef)); err != nil {
			return r, empty, err
		}
		return r, old, nil
	}
	if err = ctx.Err(); err != nil {
		return r, empty, err
	}
	r, reservation, err := s.beginControlExecution(id, request)
	if err != nil {
		return r, empty, err
	}
	started := time.Now().UTC()
	deadline, err := time.Parse(time.RFC3339Nano, *reservation.DeadlineAt)
	if err != nil {
		return s.finishControlPackage(r, request, reservation, started, nil, false, err)
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	root, err := s.controlPackageRoot(r, request)
	if err != nil {
		return s.finishControlPackage(r, request, reservation, started, nil, false, err)
	}
	defer root.Close()
	files, err := s.controlToolFiles(r, r.Workspace)
	if err == nil && (r.Control.Audit.Manifest == nil || params.ManifestRef != scheduleManifestRef(*r.Control.Audit.Manifest)) {
		err = fail("artifact_source_mismatch", "打包请求没有引用当前准确manifest")
	}
	var data []byte
	if err == nil {
		data, err = controlPackageBytes(r, files)
	}
	if err == nil {
		err = s.controlPackageSecretBoundary(data)
	}
	if err == nil {
		err = verifyControlPackageBytes(r, files, data)
	}
	if err == nil {
		err = bounded.Err()
	}
	if err != nil {
		return s.finishControlPackage(r, request, reservation, started, nil, false, err)
	}
	// The atomic begin transaction owns ActiveStep; check controls again before
	// the first file effect. A concurrent control change never grants a write.
	current, err := s.Store.Get(id)
	if err == nil {
		err = controlPackageLiveBinding(current, request)
	}
	if err != nil {
		return s.finishControlPackage(r, request, reservation, started, nil, false, err)
	}
	_, filename := controlPackageIdentity(request.OperationID)
	if err = controlPublishPackage(root, filename, data); err == nil {
		var actual []byte
		actual, err = controlReadPackageAt(root, filename, controlPackageLimit(r))
		if err == nil {
			err = verifyControlPackageBytes(r, files, actual)
		}
		if err == nil && !bytes.Equal(actual, data) {
			err = fail("package_integrity_failed", "发布后包字节改变")
		}
	}
	if err == nil {
		err = bounded.Err()
	}
	if err != nil {
		return s.finishControlPackage(r, request, reservation, started, nil, true, err)
	}
	return s.finishControlPackage(r, request, reservation, started, data, true, nil)
}

func controlPackageLiveBinding(r Run, request contracts.ExecutionRequest) error {
	if r.Control == nil || r.SourceVersion != int(request.SourceVersion) || r.ActiveStep != request.ExecutionID || r.CancelRequested || r.PauseRequested || r.Control.Audit.Approval == nil || r.Control.Audit.Approval.ApprovalID != request.Approval.ApprovalID || r.Control.Audit.Approval.RevokedAt != nil || r.Control.Audit.PermissionSnapshot == nil || r.Control.Audit.PermissionSnapshot.Revoked {
		return fail("execution_stopped", "打包边界的来源、批准或用户控制已改变")
	}
	return nil
}

func (s *Service) finishControlPackage(r Run, request contracts.ExecutionRequest, reservation contracts.BudgetReservation, started time.Time, data []byte, touched bool, cause error) (Run, contracts.PackageReceipt, error) {
	completed := time.Now().UTC()
	status, outcome, operationStatus := "succeeded", "succeeded", "verified"
	if cause != nil {
		status, outcome, operationStatus = "failed", "failed", "rejected"
		if touched {
			status, outcome, operationStatus = "unknown", "unknown", "unknown"
		}
	}
	var params contracts.PackageParameters
	_, raw, _ := scheduleAction(request.Action)
	_ = json.Unmarshal(raw, &params)
	receipt := contracts.PackageReceipt{
		ContractVersion: request.ContractVersion, ReceiptID: newID("package_receipt_"), RunID: request.RunID, SourceVersion: request.SourceVersion,
		DefinitionSnapshotHash: request.DefinitionSnapshotHash, ExecutionID: request.ExecutionID, OperationID: request.OperationID,
		AttemptID: reservation.AttemptID, RequestHash: request.RequestHash, ApprovalID: request.Approval.ApprovalID,
		ApprovalHash: r.Control.Audit.Approval.ApprovalHash, ProposalSnapshotHash: request.Approval.ProposalSnapshotHash,
		PlanHash: request.PlanHash, Workspace: request.Workspace, ManifestRef: contracts.PackageReceiptManifestRef(params.ManifestRef),
		StartedAt: started.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano), Status: status,
	}
	if cause == nil {
		hash := contracts.BytesHash(data)
		packageID, _ := controlPackageIdentity(request.OperationID)
		ref := contracts.PackageReceiptPackageRef{ObjectType: "package", ObjectID: packageID, ObjectVersion: 1, SourceVersion: request.SourceVersion, Hash: hash}
		receipt.PackageRef, receipt.PackageSHA256, receipt.PackageBytes, receipt.FileCount = &ref, &hash, int64(len(data)), int64(len(r.Control.Audit.Manifest.Files))
	}
	var err error
	receipt.ReceiptHash, err = contracts.HashObject("PackageReceipt", receipt)
	if err == nil {
		err = contracts.Validate("PackageReceipt", receipt)
	}
	if err != nil {
		return r, receipt, errors.Join(cause, err)
	}
	updated, saveErr := s.Store.Update(r.ID, func(run *Run) error {
		if run.Control == nil || cause == nil && (run.SourceVersion != r.SourceVersion || run.ActiveStep != request.ExecutionID) {
			return fail("execution_binding_mismatch", "包结束时原动作身份改变，不能登记成功")
		}
		if cause == nil {
			if err := controlPackageLiveBinding(*run, request); err != nil {
				return err
			}
			root, err := s.controlPackageRoot(*run, request)
			if err != nil {
				return err
			}
			defer root.Close()
			_, filename := controlPackageIdentity(request.OperationID)
			actual, err := controlReadPackageAt(root, filename, controlPackageLimit(*run))
			if err != nil {
				return err
			}
			files, err := s.controlToolFiles(*run, run.Workspace)
			if err != nil {
				return err
			}
			if err = verifyControlPackageBytes(*run, files, actual); err != nil || !bytes.Equal(actual, data) {
				return errors.Join(err, fail("package_integrity_failed", "登记前包或批准源字节已改变"))
			}
		}
		found := false
		for i := range run.Control.Audit.Operations {
			op := &run.Control.Audit.Operations[i]
			if op.OperationID != request.OperationID {
				continue
			}
			if op.RequestHash != request.RequestHash || op.Status != "sent" {
				return fail("operation_conflict", "包原操作状态已改变")
			}
			op.Status, op.UpdatedAt = operationStatus, receipt.CompletedAt
			op.EvidenceRefs = append(op.EvidenceRefs, controlPackageReceiptRef(receipt))
			if cause == nil {
				op.ExternalObjectID = receipt.PackageSHA256
				result := CanonicalActionResult{RunID: run.ID, SourceVersion: request.SourceVersion, DefinitionSnapshotHash: request.DefinitionSnapshotHash, PlanHash: request.PlanHash, ApprovalID: receipt.ApprovalID, ApprovalHash: receipt.ApprovalHash, ProposalSnapshotHash: receipt.ProposalSnapshotHash, ExecutionID: request.ExecutionID, OperationID: request.OperationID, AttemptID: reservation.AttemptID, RequestHash: request.RequestHash, TargetHash: op.TargetHash, ActionType: "package", ManifestRef: &params.ManifestRef, PackageSHA256: *receipt.PackageSHA256, PackageBytes: receipt.PackageBytes, CompletedAt: receipt.CompletedAt}
				var err error
				result.ResultHash, err = HashCanonicalActionResult(result)
				if err != nil {
					return err
				}
				run.Control.Schedule.Results = append(run.Control.Schedule.Results, result)
			}
			found = true
		}
		if !found {
			return fail("operation_conflict", "包原操作不存在")
		}
		if err := settleControlInRun(run, reservation, outcome, time.Now().UTC()); err != nil {
			return err
		}
		run.Control.Audit.PackageReceipts = append(run.Control.Audit.PackageReceipts, receipt)
		if run.ActiveStep == request.ExecutionID {
			run.ActiveStep = ""
		}
		if cause != nil {
			run.State, run.ResumeTo, run.Error = "BLOCKED", "DELIVER", "打包未能核实；保留原请求和回执"
		}
		controlWorkspaceBoundary(run)
		return nil
	})
	if saveErr != nil && cause == nil {
		// The successful file may already exist. Persist uncertainty on the same
		// original reservation instead of leaving sent/reserved or writing again.
		return s.finishControlPackage(r, request, reservation, started, nil, true, saveErr)
	}
	return updated, receipt, errors.Join(cause, saveErr)
}

func controlPackageReceiptRef(receipt contracts.PackageReceipt) contracts.ObjectReference {
	return contracts.ObjectReference{ObjectType: "evidence", ObjectID: receipt.ReceiptID, ObjectVersion: 1, SourceVersion: receipt.SourceVersion, Hash: receipt.ReceiptHash}
}

func controlPackageReceiptBinding(r Run, request contracts.ExecutionRequest, receipt contracts.PackageReceipt) error {
	if r.Control == nil || !hashMatches("ExecutionRequest", request, request.RequestHash) || !hashMatches("PackageReceipt", receipt, receipt.ReceiptHash) || contracts.Validate("PackageReceipt", receipt) != nil {
		return fail("package_receipt_mismatch", "包回执或原请求hash不符")
	}
	kind, raw, err := scheduleAction(request.Action)
	var params contracts.PackageParameters
	if err != nil || kind != "package" || json.Unmarshal(raw, &params) != nil {
		return fail("package_receipt_mismatch", "包回执没有原package请求")
	}
	scheduler := canonicalScheduler{r: r}
	if r.Control.Audit.Approval == nil || !scheduler.receiptBinding(request, receipt.RunID, receipt.SourceVersion, receipt.DefinitionSnapshotHash, receipt.ExecutionID, receipt.OperationID, receipt.RequestHash, receipt.ApprovalID, receipt.ApprovalHash, receipt.ProposalSnapshotHash, receipt.PlanHash, receipt.Workspace, receipt.StartedAt, receipt.CompletedAt) || !equalContractValue(receipt.ManifestRef, params.ManifestRef) || receipt.Status != "succeeded" || receipt.PackageRef == nil || receipt.PackageSHA256 == nil || receipt.PackageBytes <= 0 || !scheduler.settled(request, receipt.AttemptID, "action", "package", receipt.StartedAt, receipt.CompletedAt) {
		return fail("package_receipt_mismatch", "包回执来源、实际字节或结算不完整")
	}
	packageID, _ := controlPackageIdentity(request.OperationID)
	ref := contracts.ObjectReference{ObjectType: "package", ObjectID: packageID, ObjectVersion: 1, SourceVersion: request.SourceVersion, Hash: *receipt.PackageSHA256}
	if !equalContractValue(ref, receipt.PackageRef) {
		return fail("package_receipt_mismatch", "原始包引用与回执不一致")
	}
	foundRequest, foundOp := false, false
	for _, old := range r.Control.Audit.Executions {
		foundRequest = foundRequest || equalContractValue(old, request)
	}
	for _, op := range r.Control.Audit.Operations {
		if op.OperationID == request.OperationID && op.RequestHash == request.RequestHash && op.Status == "verified" && op.ExternalObjectID != nil && *op.ExternalObjectID == *receipt.PackageSHA256 && scheduleHasRef(op.EvidenceRefs, controlPackageReceiptRef(receipt)) {
			foundOp = true
		}
	}
	if !foundRequest || !foundOp {
		return fail("package_receipt_mismatch", "包未被准确原操作核实和引用")
	}
	return nil
}

// ReadControlPackage implements the exact public raw-object read. Historical
// sources remain readable from their immutable receipt and original request;
// this does not grant execution under the current approval.
func (s *Service) ReadControlPackage(id string, ref contracts.ObjectReference) ([]byte, error) {
	r, err := s.Store.Get(id)
	if err != nil {
		return nil, err
	}
	return s.readControlPackage(r, ref)
}

func (s *Service) readControlPackage(r Run, ref contracts.ObjectReference) ([]byte, error) {
	var err error
	id := r.ID
	if r.Control == nil || ref.ObjectType != "package" || ref.ObjectVersion != 1 {
		return nil, fail("object_not_found", "没有准确包对象")
	}
	for _, receipt := range r.Control.Audit.PackageReceipts {
		if receipt.Status != "succeeded" || receipt.PackageRef == nil || !equalContractValue(receipt.PackageRef, ref) || !hashMatches("PackageReceipt", receipt, receipt.ReceiptHash) || receipt.PackageSHA256 == nil || *receipt.PackageSHA256 != ref.Hash {
			continue
		}
		for _, request := range r.Control.Audit.Executions {
			if request.ExecutionID != receipt.ExecutionID || request.RequestHash != receipt.RequestHash || request.RunID != id || request.SourceVersion != ref.SourceVersion || !hashMatches("ExecutionRequest", request, request.RequestHash) {
				continue
			}
			authority := clone(r)
			approvals := append([]contracts.Approval{}, r.Control.Audit.ApprovalHistory...)
			if r.Control.Audit.Approval != nil {
				approvals = append(approvals, *r.Control.Audit.Approval)
			}
			for _, approval := range approvals {
				if approval.ApprovalID == receipt.ApprovalID && approval.ApprovalHash == receipt.ApprovalHash {
					copy := approval
					authority.Control.Audit.Approval = &copy
					break
				}
			}
			if err = controlPackageReceiptBinding(authority, request, receipt); err != nil {
				return nil, err
			}
			// Only the workspace generation changes here; the package path always
			// remains inside the original Run root and uses the operation digest.
			bound := r
			bound.Workspace = filepath.Join(s.Config.DataDir, "workspaces", filepath.FromSlash(request.Workspace.StorageKey))
			root, err := s.controlPackageRoot(bound, request)
			if err != nil {
				return nil, err
			}
			defer root.Close()
			packageID, filename := controlPackageIdentity(request.OperationID)
			if packageID != ref.ObjectID {
				return nil, fail("object_not_found", "包身份不属于原操作")
			}
			data, err := controlReadPackageAt(root, filename, controlPackageLimit(bound))
			if err != nil {
				return nil, err
			}
			if int64(len(data)) != receipt.PackageBytes || contracts.BytesHash(data) != ref.Hash {
				return nil, fail("package_integrity_failed", "不可变包实际字节已改变")
			}
			if err = s.controlPackageSecretBoundary(data); err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fail("object_not_found", "没有与五字段引用准确匹配的成功包")
}
