package agent

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func controlPackageFixture(t *testing.T) (Run, map[string]string) {
	t.Helper()
	f := newScheduleFixture(t, "local")
	for {
		next := f.next()
		if next.Action == nil {
			t.Fatal("fixture did not reach package")
		}
		kind, _, _ := scheduleAction(*next.Action)
		if kind == "package" {
			break
		}
		request := f.issue(next)
		f.finish(request)
	}
	f.r.Control.Schedule = f.state
	files := map[string]string{}
	for _, change := range f.r.Control.Audit.Proposal.Changes {
		if change.After != nil {
			files[change.Path] = *change.After
		}
	}
	return f.r, files
}

func TestControlPackageExactApprovedBytesAndDeterminism(t *testing.T) {
	r, files := controlPackageFixture(t)
	first, err := controlPackageBytes(r, files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := controlPackageBytes(r, files)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatalf("non deterministic archive: %v", err)
	}
	if err = verifyControlPackageBytes(r, files, first); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil || len(reader.File) != len(files)+1 {
		t.Fatalf("complete ZIP: %v", err)
	}
	for _, entry := range reader.File {
		stream, _ := entry.Open()
		actual, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		name := entry.Name[len(r.Control.Audit.Plan.ProjectName)+1:]
		if name != manifestName && string(actual) != files[name] {
			t.Fatalf("source bytes drift: %s", name)
		}
	}
	if _, ok := files[manifestName]; ok {
		t.Fatal("package mutated approved source set")
	}
}

func TestControlPackageRejectsSourceMetadataAndArchiveTampering(t *testing.T) {
	r, files := controlPackageFixture(t)
	data, err := controlPackageBytes(r, files)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("source", func(t *testing.T) {
		changed := map[string]string{}
		for k, v := range files {
			changed[k] = v
		}
		changed["README.md"] += "\nunapproved"
		if _, e := controlPackageBytes(r, changed); e == nil {
			t.Fatal("unapproved bytes packaged")
		}
	})
	t.Run("source-version", func(t *testing.T) {
		changed := clone(r)
		changed.SourceVersion++
		if _, e := controlPackageBytes(changed, files); e == nil {
			t.Fatal("stale source packaged")
		}
	})
	t.Run("crc", func(t *testing.T) {
		changed := append([]byte{}, data...)
		reader, _ := zip.NewReader(bytes.NewReader(changed), int64(len(changed)))
		offset, _ := reader.File[0].DataOffset()
		changed[offset] ^= 1
		if e := verifyControlPackageBytes(r, files, changed); e == nil {
			t.Fatal("CRC/byte corruption accepted")
		}
	})
	t.Run("trailer", func(t *testing.T) {
		if e := verifyControlPackageBytes(r, files, append(append([]byte{}, data...), []byte("secret trailer")...)); e == nil {
			t.Fatal("unapproved trailing data accepted")
		}
	})
	for _, variant := range []string{"duplicate", "traversal", "symlink", "missing", "metadata"} {
		t.Run(variant, func(t *testing.T) {
			entries, _ := controlPackageEntries(r, files)
			var buf bytes.Buffer
			w := zip.NewWriter(&buf)
			for name, body := range entries {
				if variant == "missing" && name == r.Control.Audit.Plan.ProjectName+"/README.md" {
					continue
				}
				header := &zip.FileHeader{Name: name, Method: zip.Store}
				header.SetMode(0600)
				if variant == "traversal" {
					header.Name = "../" + name
				}
				if variant == "symlink" {
					header.SetMode(os.ModeSymlink | 0600)
				}
				if variant == "metadata" && name == r.Control.Audit.Plan.ProjectName+"/"+manifestName {
					body = []byte("{}")
				}
				entry, _ := w.CreateHeader(header)
				entry.Write(body)
				if variant == "duplicate" {
					duplicate := *header
					entry, _ = w.CreateHeader(&duplicate)
					entry.Write(body)
				}
			}
			w.Close()
			if e := verifyControlPackageBytes(r, files, buf.Bytes()); e == nil {
				t.Fatal("tampered archive accepted")
			}
		})
	}
}

func TestControlPackagePublicationNeverOverwritesOrFollowsLinks(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	_, filename := controlPackageIdentity("exact-original-operation")
	body := []byte("the immutable package bytes")
	if err = controlPublishPackage(root, filename, body); err != nil {
		t.Fatal(err)
	}
	if err = controlPublishPackage(root, filename, []byte("overwrite")); err == nil {
		t.Fatal("original operation file overwritten")
	}
	actual, err := controlReadPackageAt(root, filename, int64(len(body)))
	if err != nil || !bytes.Equal(body, actual) {
		t.Fatalf("immutable bytes: %v", err)
	}
	if _, err = controlReadPackageAt(root, filename, 2); err == nil {
		t.Fatal("unbounded package read")
	}
	link := ".control-packages/linked.zip"
	if err = os.Symlink(filepath.Join(dir, filename), filepath.Join(dir, link)); err != nil {
		t.Fatal(err)
	}
	if _, err = controlReadPackageAt(root, link, 1024); err == nil {
		t.Fatal("symlink package read")
	}
	if err = controlPublishPackage(root, "../escape.zip", body); err == nil {
		t.Fatal("escaped package root")
	}
}

func TestControlPackageActualFileSecretAndSymbolicLinkRejected(t *testing.T) {
	r, files := controlPackageFixture(t)
	dir := t.TempDir()
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := &Service{}
	s.Config.AIKey = "fixture-only-secret"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(s.Config.AIKey), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.controlToolFiles(r, dir); err == nil {
		t.Fatal("actual credential accepted into package input")
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(files["README.md"]), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "README.md"), filepath.Join(dir, "extra-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.controlToolFiles(r, dir); err == nil {
		t.Fatal("actual symlink accepted into package input")
	}
}
