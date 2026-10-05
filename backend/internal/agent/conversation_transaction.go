package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"agentstudio/internal/storage"
)

func syncDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}

func (s *Store) commitConversationLogs(r Run) error {
	if e := storage.CommitTranscript(s.transcriptPath(r.ID), r.Control.Audit.Messages); e != nil {
		return e
	}
	// Keep the explicitly legacy display log as a separate exact-byte stream.
	var expected bytes.Buffer
	for _, m := range r.Messages {
		b, e := json.Marshal(m)
		if e != nil {
			return e
		}
		expected.Write(b)
		expected.WriteByte('\n')
	}
	path := filepath.Join(s.dir, "runs", r.ID+".jsonl")
	if info, e := os.Lstat(path); e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("legacy transcript is not a regular file")
	}
	old, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		old = nil
	} else if e != nil {
		return e
	}
	if !bytes.HasPrefix(expected.Bytes(), old) {
		return fail("transcript_conflict", "legacy display log differs from original transaction")
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(expected.Bytes()[len(old):]); e != nil {
		return e
	}
	return f.Sync()
}

func (s *Store) persistConversation(r Run, body []byte) error {
	dir := filepath.Join(s.dir, "runs")
	pending := filepath.Join(dir, ".pending-"+r.ID+".json")
	f, e := os.OpenFile(pending, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return fail("conversation_pending", "storage transaction needs recovery before another write")
	}
	_, e = f.Write(body)
	if e == nil {
		e = f.Sync()
	}
	closed := f.Close()
	if e == nil {
		e = closed
	}
	if e != nil {
		return e
	}
	if e = syncDirectory(dir); e != nil {
		return e
	}
	if e = s.commitConversationLogs(r); e != nil {
		return e
	}
	if e = os.Rename(pending, filepath.Join(dir, r.ID+".json")); e != nil {
		return e
	}
	return syncDirectory(dir)
}
