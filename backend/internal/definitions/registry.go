// Package definitions publishes immutable, content-addressed capability releases.
// Its runtime input is bundled product assets and its own durable registry, never
// design documents, validation fixtures, or the latest Worker template constants.
package definitions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"agentstudio/internal/contracts"
)

// ContractVersion is the backward-compatible default for BuildDefault. Existing
// releases always select validation using their own immutable ContractVersion.
const ContractVersion = "1.1.0"

var safeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)
var safeVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TemplateBundle contains the exact, unrendered template bytes. File paths are
// stable source identities, not locations that the registry executes or reads.
type TemplateBundle struct {
	ID      string
	Version string
	Files   map[string][]byte
}

// Release is an internal publication transaction. All public objects in it are
// complete contracts; schema/template bytes are preserved as immutable blobs.
type Release struct {
	Definition           contracts.AgentDefinition      `json:"definition"`
	Skills               []contracts.SkillDefinition    `json:"skills"`
	SchemaDependencies   []contracts.SchemaDependency   `json:"schema_dependencies"`
	TemplateDependencies []contracts.TemplateDependency `json:"template_dependencies"`
	ToolBindings         []contracts.ToolIdentity       `json:"tool_bindings"`
	Schemas              map[string][]byte              `json:"-"`
	TemplateContents     map[string][]byte              `json:"-"`
}

type Registry struct {
	root string
	mu   sync.Mutex
}

func Open(root string) (*Registry, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// Resolve trusted host aliases such as macOS /var -> /private/var once.
	// The selected registry itself and all subsequent entries cannot be symlinks.
	if info, e := os.Lstat(absolute); e == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("registry root is a symbolic link")
	}
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{"blobs", "definitions", "skills", "templates"} {
		if err = os.MkdirAll(filepath.Join(absolute, dir), 0700); err != nil {
			return nil, err
		}
	}
	return &Registry{root: absolute}, nil
}

func rejectSymlinks(path string) error {
	path = filepath.Clean(path)
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("definition storage contains symbolic link")
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	return nil
}
func (r *Registry) read(relative string) ([]byte, error) {
	path := filepath.Join(r.root, relative)
	if err := rejectSymlinks(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
func canonical(value any) ([]byte, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return nil, e
	}
	return contracts.Canonicalize(b)
}
func (r *Registry) immutable(relative string, data []byte) error {
	target := filepath.Join(r.root, relative)
	if e := rejectSymlinks(target); e != nil {
		return e
	}
	if old, e := os.ReadFile(target); e == nil {
		if !bytes.Equal(old, data) {
			return fmt.Errorf("immutable publication conflict: %s", relative)
		}
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(target), ".publishing-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	if e = os.Link(tmp, target); e != nil {
		if errors.Is(e, os.ErrExist) {
			old, readErr := os.ReadFile(target)
			if readErr == nil && bytes.Equal(old, data) {
				return nil
			}
		}
		return fmt.Errorf("immutable publication conflict: %s: %w", relative, e)
	}
	dir, e := os.Open(filepath.Dir(target))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func pairPath(kind, id, version string) (string, error) {
	if !safeName.MatchString(id) || !safeVersion.MatchString(version) {
		return "", fmt.Errorf("invalid published identity")
	}
	return filepath.Join(kind, id, version+".json"), nil
}
func (r *Registry) blob(hash string) ([]byte, error) {
	if !hashPattern.MatchString(hash) {
		return nil, fmt.Errorf("invalid dependency hash")
	}
	b, e := r.read(filepath.Join("blobs", hash))
	if e != nil {
		return nil, fmt.Errorf("missing dependency %s: %w", hash, e)
	}
	if contracts.BytesHash(b) != hash {
		return nil, fmt.Errorf("dependency content hash mismatch: %s", hash)
	}
	return b, nil
}

// Publish refuses same-version content changes for definitions, skills, and
// templates. Repeating an identical publication is safe and preserves originals.
func (r *Registry) Publish(release Release) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := validateRelease(release, func(hash string) ([]byte, error) {
		if b, ok := release.TemplateContents[hash]; ok {
			if contracts.BytesHash(b) != hash {
				return nil, fmt.Errorf("template bytes mismatch")
			}
			return b, nil
		}
		for _, b := range release.Schemas {
			if contracts.BytesHash(b) == hash {
				return b, nil
			}
		}
		return r.blob(hash)
	}); err != nil {
		return err
	}
	type write struct {
		path string
		data []byte
	}
	var entries []write
	add := func(kind, id, version string, value any) error {
		path, e := pairPath(kind, id, version)
		if e != nil {
			return e
		}
		data, e := canonical(value)
		if e != nil {
			return e
		}
		entries = append(entries, write{path, data})
		return nil
	}
	for _, s := range release.Skills {
		if e := add("skills", s.SkillID, s.SkillVersion, s); e != nil {
			return e
		}
	}
	for _, t := range release.TemplateDependencies {
		if e := add("templates", t.TemplateID, t.TemplateVersion, t); e != nil {
			return e
		}
	}
	if e := add("definitions", release.Definition.DefinitionID, release.Definition.DefinitionVersion, release); e != nil {
		return e
	}
	// Check every version identity before making any new release visible.
	for _, entry := range entries {
		if old, e := r.read(entry.path); e == nil && !bytes.Equal(old, entry.data) {
			return fmt.Errorf("immutable publication conflict: %s", entry.path)
		} else if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	for _, b := range release.Schemas {
		if e := r.immutable(filepath.Join("blobs", contracts.BytesHash(b)), b); e != nil {
			return e
		}
	}
	for hash, b := range release.TemplateContents {
		if e := r.immutable(filepath.Join("blobs", hash), b); e != nil {
			return e
		}
	}
	for _, entry := range entries {
		if e := r.immutable(entry.path, entry.data); e != nil {
			return e
		}
	}
	return nil
}

func (r *Registry) load(id, version, hash string) (Release, error) {
	var release Release
	if e := r.checkRevoked("definition", id, version); e != nil {
		return release, e
	}
	p, e := pairPath("definitions", id, version)
	if e != nil {
		return release, e
	}
	b, e := r.read(p)
	if e != nil {
		return release, e
	}
	if _, e = contracts.Canonicalize(b); e != nil {
		return release, fmt.Errorf("invalid publication JSON: %w", e)
	}
	// The persisted private container has no permissive field adapter.
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&release); e != nil {
		return release, e
	}
	var tail any
	if dec.Decode(&tail) != io.EOF {
		return release, fmt.Errorf("invalid publication JSON")
	}
	if release.Definition.DefinitionID != id || release.Definition.DefinitionVersion != version || release.Definition.DefinitionHash != hash {
		return release, fmt.Errorf("definition selection mismatch")
	}
	for _, skill := range release.Skills {
		if e = r.checkRevoked("skill", skill.SkillID, skill.SkillVersion); e != nil {
			return release, e
		}
	}
	for _, dep := range release.TemplateDependencies {
		if e = r.checkRevoked("template", dep.TemplateID, dep.TemplateVersion); e != nil {
			return release, e
		}
	}
	for _, dep := range release.SchemaDependencies {
		if e = r.checkRevoked("schema", dep.SchemaURI, dep.ContractVersion); e != nil {
			return release, e
		}
	}
	if e = validateRelease(release, r.blob); e != nil {
		return release, e
	}
	// Release and per-version indexes must agree; a partial/deleted publication is
	// not a complete trusted dependency closure.
	for _, s := range release.Skills {
		p, _ := pairPath("skills", s.SkillID, s.SkillVersion)
		b, e := r.read(p)
		if e != nil {
			return release, e
		}
		want, e := canonical(s)
		if e != nil || !bytes.Equal(b, want) {
			return release, fmt.Errorf("skill publication mismatch")
		}
	}
	for _, t := range release.TemplateDependencies {
		p, _ := pairPath("templates", t.TemplateID, t.TemplateVersion)
		b, e := r.read(p)
		if e != nil {
			return release, e
		}
		want, e := canonical(t)
		if e != nil || !bytes.Equal(b, want) {
			return release, fmt.Errorf("template publication mismatch")
		}
	}
	return release, nil
}

// Freeze binds a complete release once. Provider credentials are deliberately
// absent; provider endpoint/model are copied into the snapshot's hash boundary.
func (r *Registry) Freeze(id, version, hash string, provider *contracts.ProviderBinding, boundAt time.Time) (contracts.DefinitionSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	release, e := r.load(id, version, hash)
	if e != nil {
		return contracts.DefinitionSnapshot{}, e
	}
	if boundAt.IsZero() {
		return contracts.DefinitionSnapshot{}, fmt.Errorf("bound_at must be supplied by trusted server")
	}
	snapshot := snapshotFrom(release, provider, boundAt)
	snapshot.SnapshotHash, e = contracts.HashObject("DefinitionSnapshot", snapshot)
	if e != nil {
		return snapshot, e
	}
	if e = contracts.ValidateVersion("DefinitionSnapshot", snapshot.ContractVersion, snapshot); e != nil {
		return snapshot, e
	}
	return snapshot, nil
}
func snapshotFrom(release Release, provider *contracts.ProviderBinding, at time.Time) contracts.DefinitionSnapshot {
	return contracts.DefinitionSnapshot{ContractVersion: release.Definition.ContractVersion, DefinitionID: release.Definition.DefinitionID, DefinitionVersion: release.Definition.DefinitionVersion, DefinitionHash: release.Definition.DefinitionHash, SnapshotVersion: 1, Definition: release.Definition, Skills: release.Skills, SchemaDependencies: release.SchemaDependencies, TemplateDependencies: release.TemplateDependencies, ToolBindings: release.ToolBindings, ProviderBinding: provider, BoundAt: at.UTC().Format(time.RFC3339Nano)}
}

func (r *Registry) VerifySnapshot(snapshot contracts.DefinitionSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.verifySnapshot(snapshot)
}
func (r *Registry) verifySnapshot(snapshot contracts.DefinitionSnapshot) error {
	if e := contracts.ValidateVersion("DefinitionSnapshot", snapshot.ContractVersion, snapshot); e != nil {
		return e
	}
	h, e := contracts.HashObject("DefinitionSnapshot", snapshot)
	if e != nil || h != snapshot.SnapshotHash {
		return fmt.Errorf("definition snapshot hash mismatch")
	}
	release, e := r.load(snapshot.DefinitionID, snapshot.DefinitionVersion, snapshot.DefinitionHash)
	if e != nil {
		return e
	}
	expected := snapshotFrom(release, snapshot.ProviderBinding, time.Time{})
	expected.BoundAt = snapshot.BoundAt
	expected.SnapshotVersion = snapshot.SnapshotVersion
	if expected.SnapshotVersion != 1 {
		return fmt.Errorf("unsupported frozen snapshot version")
	}
	want, e := contracts.HashObject("DefinitionSnapshot", expected)
	if e != nil || want != snapshot.SnapshotHash {
		return fmt.Errorf("snapshot does not match immutable release closure")
	}
	return nil
}

// TemplateFiles resolves only the exact source bytes bound to this snapshot.
func (r *Registry) TemplateFiles(snapshot contracts.DefinitionSnapshot, templateID string) (map[string][]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.verifySnapshot(snapshot); e != nil {
		return nil, e
	}
	for _, dep := range snapshot.TemplateDependencies {
		if dep.TemplateID == templateID {
			out := map[string][]byte{}
			for _, file := range dep.Files {
				b, e := r.blob(file.SHA256)
				if e != nil {
					return nil, e
				}
				out[file.Path] = append([]byte(nil), b...)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("template is not in bound definition: %s", templateID)
}

// SchemaBytes returns a copy after checking the snapshot and exact raw digest.
func (r *Registry) SchemaBytes(snapshot contracts.DefinitionSnapshot, uri string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.verifySnapshot(snapshot); e != nil {
		return nil, e
	}
	for _, dep := range snapshot.SchemaDependencies {
		if dep.SchemaURI == uri {
			return r.blob(dep.SHA256)
		}
	}
	return nil, fmt.Errorf("schema is not in bound definition")
}

func objectMap(value any) (map[string]any, error) {
	b, e := json.Marshal(value)
	if e != nil {
		return nil, e
	}
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	e = d.Decode(&m)
	return m, e
}
func stringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
func sortedUnique(values []string) bool {
	for i, v := range values {
		if v == "" || (i > 0 && values[i-1] >= v) {
			return false
		}
	}
	return true
}
func equalJSON(a, b any) bool {
	x, e := canonical(a)
	if e != nil {
		return false
	}
	y, e := canonical(b)
	return e == nil && bytes.Equal(x, y)
}
func sortTools(tools []contracts.ToolIdentity) {
	sort.Slice(tools, func(i, j int) bool {
		a, _ := canonical(tools[i])
		b, _ := canonical(tools[j])
		if tools[i].ToolID != tools[j].ToolID {
			return tools[i].ToolID < tools[j].ToolID
		}
		return string(a) < string(b)
	})
}
func schemaName(ref string) string { return strings.SplitN(ref, "#", 2)[0] }
