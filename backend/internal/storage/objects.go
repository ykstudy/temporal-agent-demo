// Package storage owns persistence without granting execution authority.
package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"agentstudio/internal/contracts"
)

var ErrMissingObject = errors.New("immutable object not found")
var ErrObjectConflict = errors.New("immutable object identity already has different content")

// Objects stores the published object body itself. There is no private wrapper
// that a public reader could mistake for a canonical domain object. The owning
// service holds the data-root single-writer lock; this mutex serializes threads.
type Objects struct {
	root string
	fs   *os.Root
	mu   sync.Mutex
}

func OpenObjects(root string) (*Objects, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(abs); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("object root must be a real directory")
	}
	fs, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	return &Objects{root: abs, fs: fs}, nil
}

func (s *Objects) Close() error { return s.fs.Close() }
func (s *Objects) relative(name string) string {
	relative, _ := filepath.Rel(s.root, name)
	return relative
}

func (s *Objects) readBounded(name string, limit int64) ([]byte, error) {
	file, err := s.fs.Open(s.relative(name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("immutable object file exceeds its type or size bound")
	}
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("immutable object file exceeds its size bound")
	}
	return body, nil
}

type objectIdentity struct{ kind, id, version, hash string }

// Logical hashes intentionally exclude some lifecycle fields. The private
// index also seals the entire archived body so those excluded fields cannot be
// changed later while keeping the same public object reference.
type objectIndex struct {
	Reference  contracts.ObjectReference `json:"reference"`
	BodySHA256 string                    `json:"body_sha256"`
}

func decodeIndex(body []byte) (objectIndex, error) {
	var index objectIndex
	if _, err := contracts.ParseJSON(body); err != nil {
		return index, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return index, err
	}
	if err := contracts.Validate("ObjectReference", index.Reference); err != nil {
		return index, err
	}
	if err := contracts.Validate("Hash", index.BodySHA256); err != nil {
		return index, err
	}
	return index, nil
}

// The field mapping is the published ObjectRef table, not a shape validator.
// Complete Schema validation runs before any identity field is inspected.
var identities = map[string]objectIdentity{
	"MessageEnvelope":        {"message", "message_id", "", ""},
	"DecisionProposal":       {"decision", "decision_id", "", "decision_hash"},
	"ActionValidation":       {"validation", "validation_id", "", "validation_hash"},
	"ExecutionRequest":       {"execution", "execution_id", "", "request_hash"},
	"ProjectPlan":            {"plan", "", "source_version", "plan_hash"},
	"Proposal":               {"proposal", "proposal_id", "proposal_version", "snapshot_hash"},
	"Approval":               {"approval", "approval_id", "", "approval_hash"},
	"ArtifactManifest":       {"manifest", "manifest_id", "manifest_version", "hash"},
	"VerificationReport":     {"verification_report", "report_id", "report_version", "report_hash"},
	"SummaryRecord":          {"summary", "summary_id", "summary_version", "summary_hash"},
	"SummaryAttempt":         {"summary_attempt", "attempt_id", "", "attempt_hash"},
	"RunSnapshot":            {"run", "run_id", "state_sequence", "snapshot_hash"},
	"ContextRecord":          {"context", "context_id", "", "context_hash"},
	"BudgetLedger":           {"budget", "run_id", "ledger_version", "ledger_hash"},
	"BudgetReservation":      {"budget_reservation", "reservation_id", "", "reservation_hash"},
	"BudgetSettlement":       {"budget_settlement", "settlement_id", "", "settlement_hash"},
	"ToolExecutionReceipt":   {"evidence", "receipt_id", "", "receipt_hash"},
	"WorkspaceReceipt":       {"workspace", "receipt_id", "", "receipt_hash"},
	"RemoteExecutionReceipt": {"evidence", "receipt_id", "", "receipt_hash"},
	"PackageReceipt":         {"evidence", "receipt_id", "", "receipt_hash"},
	"VerificationCandidate":  {"verification_candidate", "candidate_id", "", "candidate_hash"},
}

// Reference validates the full published object and its content hash, then
// creates the exact reference for its owner Run. Plans have no Run/source fields
// and receive those from the authoritative owner that is registering them.
func Reference(runID string, sourceVersion int64, schema string, value any) (contracts.ObjectReference, error) {
	if err := contracts.Validate("Id", runID); err != nil {
		return contracts.ObjectReference{}, err
	}
	if sourceVersion < 1 {
		return contracts.ObjectReference{}, fmt.Errorf("invalid source version")
	}
	identity, ok := identities[schema]
	if !ok {
		return contracts.ObjectReference{}, fmt.Errorf("unsupported immutable object Schema")
	}
	if err := contracts.Validate(schema, value); err != nil {
		return contracts.ObjectReference{}, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return contracts.ObjectReference{}, err
	}
	data, err = contracts.Canonicalize(data)
	if err != nil {
		return contracts.ObjectReference{}, err
	}
	parsed, err := contracts.ParseJSON(data)
	if err != nil {
		return contracts.ObjectReference{}, err
	}
	object := parsed.(map[string]any)
	if owner, exists := object["run_id"]; exists && owner != runID {
		return contracts.ObjectReference{}, fmt.Errorf("object belongs to another Run")
	}
	integer := func(name string) (int64, error) {
		n, ok := object[name].(json.Number)
		if !ok {
			return 0, fmt.Errorf("missing canonical integer %s", name)
		}
		return strconv.ParseInt(n.String(), 10, 64)
	}
	if _, exists := object["source_version"]; exists {
		source, e := integer("source_version")
		if e != nil || source != sourceVersion {
			return contracts.ObjectReference{}, fmt.Errorf("object source mismatch")
		}
	}
	hash, err := contracts.HashObject(schema, value)
	if err != nil {
		return contracts.ObjectReference{}, err
	}
	if identity.hash != "" && object[identity.hash] != hash {
		return contracts.ObjectReference{}, fmt.Errorf("object hash mismatch")
	}
	id, _ := object[identity.id].(string)
	version := int64(1)
	if schema == "ProjectPlan" {
		id, version = "plan_"+hash, sourceVersion
	} else if identity.version != "" {
		version, err = integer(identity.version)
		if err != nil {
			return contracts.ObjectReference{}, err
		}
	}
	ref := contracts.ObjectReference{ObjectType: identity.kind, ObjectID: id, ObjectVersion: version, SourceVersion: sourceVersion, Hash: hash}
	contractVersion, hasVersion := object["contract_version"].(string)
	if hasVersion {
		err = contracts.ValidateVersion("ObjectReference", contractVersion, ref)
	} else {
		err = contracts.Validate("ObjectReference", ref)
	}
	if err != nil {
		return ref, err
	}
	return ref, nil
}

func (s *Objects) objectPath(runID string, ref contracts.ObjectReference, makeDirs bool) (string, error) {
	if err := contracts.Validate("Id", runID); err != nil {
		return "", err
	}
	if err := contracts.Validate("ObjectReference", ref); err != nil {
		return "", err
	}
	parts := []string{runID, ref.ObjectType, ref.ObjectID, strconv.FormatInt(ref.ObjectVersion, 10), strconv.FormatInt(ref.SourceVersion, 10)}
	current := s.root
	for _, part := range parts {
		if part == "" || filepath.Base(part) != part || part == "." || part == ".." {
			return "", fmt.Errorf("object index path invalid")
		}
		current = filepath.Join(current, part)
		if makeDirs {
			if err := s.fs.Mkdir(s.relative(current), 0700); err != nil && !os.IsExist(err) {
				return "", err
			}
		}
		info, err := s.fs.Lstat(s.relative(current))
		if os.IsNotExist(err) {
			return "", ErrMissingObject
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("object index contains a non-directory or symlink")
		}
	}
	return filepath.Join(current, ref.Hash+".json"), nil
}

func (s *Objects) Put(runID string, sourceVersion int64, schema string, value any) (contracts.ObjectReference, error) {
	ref, err := Reference(runID, sourceVersion, schema, value)
	if err != nil {
		return ref, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ref, err
	}
	data, err = contracts.Canonicalize(data)
	if err != nil {
		return ref, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.objectPath(runID, ref, true)
	if err != nil {
		return ref, err
	}
	// The five-field reference selects a body, but its stable object identity
	// cannot be registered again under a different source/hash. Keep that
	// binding separately so changing an excluded lifecycle field cannot replace
	// an archived object, and a new hash cannot hide an ID/version collision.
	indexPath := filepath.Join(filepath.Dir(filepath.Dir(file)), "identity.json")
	if info, e := s.fs.Lstat(s.relative(indexPath)); e == nil {
		if !info.Mode().IsRegular() {
			return ref, fmt.Errorf("invalid immutable identity index")
		}
		body, e := s.readBounded(indexPath, 4096)
		if e != nil {
			return ref, e
		}
		registered, e := decodeIndex(body)
		if e != nil {
			return ref, e
		}
		if registered.Reference != ref || registered.BodySHA256 != contracts.BytesHash(data) {
			return ref, ErrObjectConflict
		}
	} else if !os.IsNotExist(e) {
		return ref, e
	} else {
		body, e := json.Marshal(objectIndex{Reference: ref, BodySHA256: contracts.BytesHash(data)})
		if e != nil {
			return ref, e
		}
		if e = s.durableWrite(indexPath, body); e != nil {
			return ref, e
		}
	}
	if info, e := s.fs.Lstat(s.relative(file)); e == nil {
		if !info.Mode().IsRegular() {
			return ref, fmt.Errorf("object body is not a regular file")
		}
		existing, e := s.readBounded(file, contracts.MaxJSONBytes)
		if e != nil {
			return ref, e
		}
		if !bytes.Equal(existing, data) {
			return ref, ErrObjectConflict
		}
		return ref, nil
	} else if !os.IsNotExist(e) {
		return ref, e
	}
	// The complete body is fsynced before the atomic publication. Interrupted
	// temporary files cannot be discovered by a hash-indexed public read.
	err = s.durableWrite(file, data)
	return ref, err
}

func (s *Objects) durableWrite(file string, data []byte) error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(s.relative(file)), ".pending-"+hex.EncodeToString(nonce[:]))
	f, err := s.fs.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer s.fs.Remove(temp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = s.fs.Rename(temp, s.relative(file)); err != nil {
		return err
	}
	dir, err := s.fs.Open(filepath.Dir(s.relative(file)))
	if err != nil {
		return err
	}
	err = dir.Sync()
	_ = dir.Close()
	return err
}

func (s *Objects) Get(runID string, ref contracts.ObjectReference) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := s.objectPath(runID, ref, false)
	if err != nil {
		return nil, err
	}
	indexPath := filepath.Join(filepath.Dir(filepath.Dir(file)), "identity.json")
	indexInfo, err := s.fs.Lstat(s.relative(indexPath))
	if err != nil || !indexInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("immutable identity index missing or invalid")
	}
	indexBody, err := s.readBounded(indexPath, 4096)
	if err != nil {
		return nil, err
	}
	registered, err := decodeIndex(indexBody)
	if err != nil || registered.Reference != ref {
		return nil, fmt.Errorf("immutable identity index mismatch")
	}
	info, err := s.fs.Lstat(s.relative(file))
	if os.IsNotExist(err) {
		return nil, ErrMissingObject
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("object body is not a regular file")
	}
	data, err := s.readBounded(file, contracts.MaxJSONBytes)
	if err != nil {
		return nil, err
	}
	if contracts.BytesHash(data) != registered.BodySHA256 {
		return nil, fmt.Errorf("immutable object body digest mismatch")
	}
	for schema, identity := range identities {
		if identity.kind != ref.ObjectType {
			continue
		}
		actual, e := Reference(runID, ref.SourceVersion, schema, json.RawMessage(data))
		if e == nil && actual == ref {
			return json.RawMessage(data), nil
		}
	}
	return nil, fmt.Errorf("stored object Schema, identity or hash mismatch")
}
