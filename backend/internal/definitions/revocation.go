package definitions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Revocation is a trusted registry administration record. It is not part of a
// DefinitionSnapshot and cannot be supplied by user/model Run input. A snapshot
// preserves its original bytes while every later resolution checks revocation.
type Revocation struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Version   string `json:"version"`
	Reason    string `json:"reason"`
	RevokedAt string `json:"revoked_at"`
}

func revocationPath(kind, id, version string) (string, error) {
	if kind != "definition" && kind != "skill" && kind != "template" && kind != "schema" {
		return "", fmt.Errorf("unsupported revocation kind")
	}
	p, e := pairPath(kind, id, version)
	if e != nil {
		return "", e
	}
	return filepath.Join("revocations", p), nil
}

// Revoke permanently prevents resolving a previously published exact identity.
// Repeating the same reason preserves the original service-issued timestamp.
func (r *Registry) Revoke(kind, id, version, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(reason) == "" || len(reason) > 2048 {
		return fmt.Errorf("revocation requires bounded non-secret reason")
	}
	p, e := revocationPath(kind, id, version)
	if e != nil {
		return e
	}
	if b, e := r.read(p); e == nil {
		var old Revocation
		if json.Unmarshal(b, &old) != nil {
			return fmt.Errorf("corrupt revocation record")
		}
		if old.Reason == reason {
			return nil
		}
		return fmt.Errorf("immutable revocation conflict")
	} else if !os.IsNotExist(e) {
		return e
	}
	record := Revocation{Kind: kind, ID: id, Version: version, Reason: reason, RevokedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	b, e := canonical(record)
	if e != nil {
		return e
	}
	return r.immutable(p, b)
}
func (r *Registry) checkRevoked(kind, id, version string) error {
	p, e := revocationPath(kind, id, version)
	if e != nil {
		return e
	}
	if _, e = r.read(p); e == nil {
		return fmt.Errorf("published %s dependency is revoked: %s@%s", kind, id, version)
	} else if !os.IsNotExist(e) {
		return e
	}
	return nil
}
