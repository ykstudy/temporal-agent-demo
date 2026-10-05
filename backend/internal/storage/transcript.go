package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"agentstudio/internal/contracts"
)

// TranscriptBytes is also the source-range codec. These exact bytes, including
// each LF, are appended at the initial commit, never reconstructed from display
// strings or a legacy transcript during migration.
func TranscriptBytes(messages []contracts.MessageEnvelope) ([]byte, error) {
	var out bytes.Buffer
	for i, m := range messages {
		if err := contracts.Validate("MessageEnvelope", m); err != nil {
			return nil, err
		}
		if i > 0 && (m.RunID != messages[0].RunID || m.Sequence != messages[i-1].Sequence+1) {
			return nil, fmt.Errorf("non-contiguous transcript")
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		b, err = contracts.Canonicalize(b)
		if err != nil || len(b) > 256<<10 {
			return nil, fmt.Errorf("invalid transcript envelope size or encoding")
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// CommitTranscript only finishes the exact bytes frozen in a durable pending
// snapshot. A torn final line is a prefix, not a reason to truncate evidence.
func openTranscriptRoot(path string) (*os.Root, string, error) {
	parent, name := filepath.Dir(path), filepath.Base(path)
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, "", fmt.Errorf("transcript parent must be a real directory")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return nil, "", err
	}
	if info, e := root.Lstat(name); e == nil && !info.Mode().IsRegular() {
		root.Close()
		return nil, "", fmt.Errorf("transcript must be a regular file")
	} else if e != nil && !os.IsNotExist(e) {
		root.Close()
		return nil, "", e
	}
	return root, name, nil
}

func transcriptRead(root *os.Root, name string, limit int64) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("transcript exceeds committed byte bound")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("transcript exceeds committed byte bound")
	}
	return b, err
}

func CommitTranscript(path string, messages []contracts.MessageEnvelope) error {
	want, err := TranscriptBytes(messages)
	if err != nil {
		return err
	}
	root, name, err := openTranscriptRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	old, err := transcriptRead(root, name, int64(len(want)))
	if os.IsNotExist(err) {
		old = nil
	} else if err != nil {
		return err
	}
	if !bytes.HasPrefix(want, old) {
		return fmt.Errorf("transcript conflicts with durable pending snapshot")
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(old)) {
		return fmt.Errorf("transcript changed before append")
	}
	if _, err = f.Write(want[len(old):]); err != nil {
		return err
	}
	return f.Sync()
}

func ReadTranscript(path string, messages []contracts.MessageEnvelope) ([]byte, error) {
	want, err := TranscriptBytes(messages)
	if err != nil {
		return nil, err
	}
	root, name, err := openTranscriptRoot(path)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	actual, err := transcriptRead(root, name, int64(len(want)))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(actual, want) {
		return nil, fmt.Errorf("committed transcript bytes missing, changed or unexplained tail")
	}
	return actual, nil
}
