package agent

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type BackupStateInfo struct {
	Files          int
	Bytes          int64
	ManifestSHA256 string
}

// CopyOfflineState requires the same exclusive writer lock as the product.
// Copies are complete directories, never individual snapshots from a live Run.
func CopyOfflineState(source, destination string) (info BackupStateInfo, err error) {
	source, err = filepath.Abs(source)
	if err != nil {
		return info, err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return info, err
	}
	if source == destination || strings.HasPrefix(destination, source+string(os.PathSeparator)) || strings.HasPrefix(source, destination+string(os.PathSeparator)) {
		return info, fmt.Errorf("backup paths must be disjoint")
	}
	stat, e := os.Lstat(source)
	if e != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return info, fmt.Errorf("source must be a real data directory")
	}
	if _, e = os.Stat(filepath.Join(source, "runs")); e != nil {
		return info, fmt.Errorf("source lacks Run storage")
	}
	if _, e = os.Lstat(destination); !os.IsNotExist(e) {
		return info, fmt.Errorf("destination must not exist")
	}
	lock, e := os.OpenFile(filepath.Join(source, ".writer.lock"), os.O_RDWR, 0600)
	if e != nil {
		return info, e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return info, fmt.Errorf("stop the owning service before backup: writer lock held")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	temporary := destination + ".partial-" + newID("")
	if e = os.Mkdir(temporary, 0700); e != nil {
		return info, e
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(temporary)
		}
	}()
	var manifest strings.Builder
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		if rel == ".writer.lock" || rel == "SHA256SUMS" {
			return nil
		}
		if strings.ContainsAny(rel, "\r\n\t") {
			return fmt.Errorf("unsupported backup filename")
		}
		stat, e := entry.Info()
		if e != nil {
			return e
		}
		target := filepath.Join(temporary, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("backup rejects links and non-regular entries")
		}
		if stat.Size() > 2<<30 || info.Bytes+stat.Size() > 16<<30 {
			return fmt.Errorf("backup resource limit exceeded")
		}
		in, e := os.Open(path)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, stat.Mode().Perm()&0700)
		if e != nil {
			return e
		}
		h := sha256.New()
		n, e := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, stat.Size()+1))
		if e == nil && n != stat.Size() {
			e = fmt.Errorf("source changed during offline copy")
		}
		if e == nil {
			e = out.Sync()
		}
		closed := out.Close()
		if e == nil {
			e = closed
		}
		if e != nil {
			return e
		}
		info.Files++
		info.Bytes += n
		fmt.Fprintf(&manifest, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return info, err
	}
	data := []byte(manifest.String())
	f, e := os.OpenFile(filepath.Join(temporary, "SHA256SUMS"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return info, e
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	_ = f.Close()
	if err != nil {
		return info, err
	}
	// Sync every newly created directory before publishing the destination name.
	var dirs []string
	_ = filepath.WalkDir(temporary, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		if err = syncDirectory(dirs[i]); err != nil {
			return info, err
		}
	}
	if err = os.Rename(temporary, destination); err != nil {
		return info, err
	}
	err = syncDirectory(filepath.Dir(destination))
	sum := sha256.Sum256(data)
	info.ManifestSHA256 = hex.EncodeToString(sum[:])
	return info, err
}

// VerifyOfflineCopy verifies the complete byte set before product recovery. It
// does not grant permission to use a copied workspace against external targets.
func VerifyOfflineCopy(directory string) (BackupStateInfo, error) {
	var result BackupStateInfo
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer root.Close()
	f, err := root.Open("SHA256SUMS")
	if err != nil {
		return result, err
	}
	defer f.Close()
	h := sha256.New()
	scan := bufio.NewScanner(io.TeeReader(io.LimitReader(f, 16<<20), h))
	scan.Buffer(make([]byte, 4096), 8192)
	seen := map[string]bool{}
	for scan.Scan() {
		line := scan.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return result, fmt.Errorf("invalid checksum manifest")
		}
		name := line[66:]
		if filepath.IsAbs(name) || filepath.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || seen[name] {
			return result, fmt.Errorf("invalid backup entry")
		}
		seen[name] = true
		entry, e := root.Lstat(name)
		if e != nil || !entry.Mode().IsRegular() {
			return result, fmt.Errorf("backup entry missing or non-regular")
		}
		body, e := root.Open(name)
		if e != nil {
			return result, e
		}
		hash := sha256.New()
		n, e := io.Copy(hash, io.LimitReader(body, 2<<30+1))
		_ = body.Close()
		if e != nil || n > 2<<30 || hex.EncodeToString(hash.Sum(nil)) != line[:64] {
			return result, fmt.Errorf("backup content digest mismatch")
		}
		result.Files++
		result.Bytes += n
	}
	if err = scan.Err(); err != nil {
		return result, err
	}
	err = filepath.WalkDir(directory, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(directory, path)
		if e != nil {
			return e
		}
		if rel == "SHA256SUMS" || rel == ".writer.lock" {
			return nil
		}
		if !seen[filepath.ToSlash(rel)] {
			return fmt.Errorf("unexpected backup entry")
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	result.ManifestSHA256 = hex.EncodeToString(h.Sum(nil))
	return result, nil
}
