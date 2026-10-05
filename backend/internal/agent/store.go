package agent

import (
	"agentstudio/internal/contracts"
	"agentstudio/internal/storage"
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$`)
var projectName = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
var ErrNotFound = errors.New("run not found")

type APIError struct {
	Code    string
	Message string
}

func (e *APIError) Error() string     { return e.Message }
func fail(code, message string) error { return &APIError{Code: code, Message: message} }
func newID(prefix string) string {
	var b [10]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

type diskRun struct {
	Run
	Control           *ControlState                 `json:"canonical_control,omitempty"`
	FrozenDefinition  *contracts.DefinitionSnapshot `json:"frozen_definition,omitempty"`
	Processed         map[string]string             `json:"processed"`
	Workspace         string                        `json:"workspace"`
	RepoProjectID     int                           `json:"repo_project_id"`
	PipelineID        int                           `json:"pipeline_id"`
	CommitSHA         string                        `json:"commit_sha"`
	CreateFingerprint string                        `json:"create_fingerprint"`
	CommandIntents    map[string]string             `json:"command_intents"`
	UpdateAttempts    map[string]int                `json:"update_attempts"`
}

func disk(r Run) diskRun {
	return diskRun{Run: r, Control: r.Control, FrozenDefinition: r.FrozenDefinition, Processed: r.Processed, Workspace: r.Workspace, RepoProjectID: r.RepoProjectID, PipelineID: r.PipelineID, CommitSHA: r.CommitSHA, CreateFingerprint: r.CreateFingerprint, CommandIntents: r.CommandIntents, UpdateAttempts: r.UpdateAttempts}
}
func (d diskRun) value() Run {
	r := d.Run
	r.Control = d.Control
	r.FrozenDefinition = d.FrozenDefinition
	r.Processed = d.Processed
	r.Workspace = d.Workspace
	r.RepoProjectID = d.RepoProjectID
	r.PipelineID = d.PipelineID
	r.CommitSHA = d.CommitSHA
	r.CreateFingerprint = d.CreateFingerprint
	r.CommandIntents = d.CommandIntents
	r.UpdateAttempts = d.UpdateAttempts
	if r.CommandIntents == nil {
		r.CommandIntents = map[string]string{}
	}
	if r.UpdateAttempts == nil {
		r.UpdateAttempts = map[string]int{}
	}
	if r.Processed == nil {
		r.Processed = map[string]string{}
	}
	if r.Operations == nil {
		r.Operations = map[string]OperationRecord{}
	}
	if r.SourceVersion == 0 {
		r.SourceVersion = r.Version
	}
	if r.StateSequence == 0 {
		r.StateSequence = 1
	}
	return r
}
func clone(r Run) Run {
	b, _ := json.Marshal(disk(r))
	var d diskRun
	_ = json.Unmarshal(b, &d)
	return d.value()
}

type Store struct {
	mu       sync.Mutex
	dir      string
	runs     map[string]Run
	watchers map[string]map[chan Run]bool
	lockFile *os.File
	objects  *storage.Objects
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, runs: map[string]Run{}, watchers: map[string]map[chan Run]bool{}}
	lock, err := os.OpenFile(filepath.Join(dir, ".writer.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fail("storage_locked", "数据目录已有写者；首版仅支持单写者部署")
	}
	s.lockFile = lock
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	s.objects, err = storage.OpenObjects(filepath.Join(dir, "objects"))
	if err != nil {
		return nil, err
	}
	if err = s.recoverConversationTransactions(); err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "runs", "*.json"))
	if err != nil {
		return nil, err
	}
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		var d diskRun
		if e = json.Unmarshal(b, &d); e != nil {
			return nil, fmt.Errorf("read snapshot %s: %w", filepath.Base(p), e)
		}
		if d.Control != nil {
			if _, e = contracts.ParseJSON(b); e != nil {
				return nil, fmt.Errorf("invalid canonical snapshot encoding: %w", e)
			}
			decoder := json.NewDecoder(bytes.NewReader(b))
			decoder.DisallowUnknownFields()
			if e = decoder.Decode(&d); e != nil {
				return nil, fmt.Errorf("unknown canonical snapshot fields: %w", e)
			}
		}
		r := d.value()
		if !safeID.MatchString(r.ID) || filepath.Base(p) != r.ID+".json" {
			return nil, fail("invalid_snapshot", "snapshot identity mismatch")
		}
		if _, e = transcriptIDs(filepath.Join(dir, "runs", r.ID+".jsonl")); e != nil {
			return nil, e
		}
		if e = s.verifyCanonical(r); e != nil {
			return nil, e
		}
		if conversationState(r) != nil {
			if e = validateConversation(r); e != nil {
				return nil, e
			}
			if _, e = storage.ReadTranscript(s.transcriptPath(r.ID), r.Control.Audit.Messages); e != nil {
				return nil, e
			}
		}
		s.runs[r.ID] = r
	}
	ok = true
	return s, nil
}
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.objects != nil {
		_ = s.objects.Close()
		s.objects = nil
	}
	if s.lockFile != nil {
		_ = syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		_ = s.lockFile.Close()
		s.lockFile = nil
	}
}
func (s *Store) Get(id string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	return clone(r), nil
}
func (s *Store) List() []Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Run, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, clone(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}
func (s *Store) Create(r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.runs[r.ID]; ok {
		if old.CreateFingerprint != r.CreateFingerprint {
			return Run{}, fail("idempotency_conflict", "同一Run ID已绑定不同创建内容")
		}
		return clone(old), nil
	}
	if !safeID.MatchString(r.ID) {
		return Run{}, fail("invalid_id", "invalid run ID")
	}
	if err := s.prepareCanonical(&r); err != nil {
		return Run{}, err
	}
	if err := s.persist(r, 0); err != nil {
		return Run{}, err
	}
	s.runs[r.ID] = clone(r)
	return clone(r), nil
}
func (s *Store) Update(id string, fn func(*Run) error) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.runs[id]
	if !ok {
		return Run{}, ErrNotFound
	}
	r := clone(old)
	if err := fn(&r); err != nil {
		return clone(old), err
	}
	r.UpdatedAt = now()
	r.StateSequence = old.StateSequence + 1
	r.Version = r.SourceVersion // legacy field tracks business source, never ordinary progress.
	if err := s.prepareCanonical(&r); err != nil {
		return clone(old), err
	}
	if err := s.persist(r, len(old.Messages)); err != nil {
		return clone(old), err
	}
	s.runs[id] = clone(r)
	for ch := range s.watchers[id] {
		select {
		case ch <- clone(r):
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- clone(r):
			default:
			}
		}
	}
	return clone(r), nil
}
func (s *Store) persist(r Run, prior int) error {
	b, err := json.MarshalIndent(disk(r), "", "  ")
	if err != nil {
		return err
	}
	if r.Control != nil {
		if _, err = contracts.ParseJSON(b); err != nil {
			return fmt.Errorf("canonical snapshot resource bound: %w", err)
		}
	}
	if conversationState(r) != nil {
		return s.persistConversation(r, b)
	}
	target := filepath.Join(s.dir, "runs", r.ID+".json")
	f, err := os.CreateTemp(filepath.Dir(target), ".snapshot-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
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
	// Write the append-only transcript before the authoritative snapshot. Stable
	// message identities let consumers deduplicate a retry after a partial commit.
	if len(r.Messages) > prior {
		logPath := filepath.Join(s.dir, "runs", r.ID+".jsonl")
		ids, err := transcriptIDs(logPath)
		if err != nil {
			return err
		}
		log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		for _, m := range r.Messages[prior:] {
			line, e := json.Marshal(m)
			if previous, exists := ids[m.ID]; exists {
				if previous != string(line) {
					log.Close()
					return fail("transcript_conflict", "消息身份与追加原文冲突")
				}
				continue
			}
			if e == nil {
				_, e = log.Write(append(line, '\n'))
			}
			if e != nil {
				log.Close()
				return e
			}
			ids[m.ID] = string(line)
		}
		err = log.Sync()
		e := log.Close()
		if err != nil {
			return err
		}
		if e != nil {
			return e
		}
	}
	if err = os.Rename(tmp, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func transcriptIDs(path string) (map[string]string, error) {
	out := map[string]string{}
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		var m Message
		line := scan.Bytes()
		if json.Unmarshal(line, &m) != nil || m.ID == "" {
			return nil, fail("transcript_corrupt", "JSONL损坏或尾记录不完整；停止写入并要求恢复")
		}
		if old, ok := out[m.ID]; ok && old != string(line) {
			return nil, fail("transcript_conflict", "JSONL消息身份冲突")
		}
		out[m.ID] = string(line)
	}
	if e = scan.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
func (s *Store) Subscribe(id string) (<-chan Run, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return nil, nil, ErrNotFound
	}
	ch := make(chan Run, 1)
	if s.watchers[id] == nil {
		s.watchers[id] = map[chan Run]bool{}
	}
	s.watchers[id][ch] = true
	ch <- clone(r)
	return ch, func() { s.mu.Lock(); delete(s.watchers[id], ch); s.mu.Unlock() }, nil
}

func addMessage(r *Run, role, content, kind, proposal string) {
	r.Messages = append(r.Messages, Message{ID: newID("msg_"), Role: role, Content: content, Kind: kind, ProposalID: proposal, CreatedAt: now()})
}
func history(r *Run, action, detail string) {
	r.History = append(r.History, History{ID: newID("evt_"), Action: action, Version: r.Version, CreatedAt: now(), Detail: detail})
}
func terminal(state string) bool {
	return state == "DONE" || state == "FAILED" || state == "CANCELLED"
}
func setStep(r *Run, id, state, detail string) {
	for i := range r.Steps {
		if r.Steps[i].ID == id {
			r.Steps[i].State = state
			r.Steps[i].Detail = detail
		}
	}
}
