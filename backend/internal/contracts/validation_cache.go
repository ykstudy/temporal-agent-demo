package contracts

import "sync"

// Published schemas are immutable within a binary. Cache only successful full
// structural validation of exact input bytes, never semantic or authorization
// results. The version and type remain part of the key; changing one byte or
// selecting another release still runs the complete validator. Bound metadata
// to avoid retaining attacker-controlled documents or unbounded identities.
var structuralCache = struct {
	sync.Mutex
	keys map[string]struct{}
}{keys: make(map[string]struct{})}

func structuralKey(name, version string, data []byte) string {
	return name + "\x00" + version + "\x00" + BytesHash(data)
}

func structureCached(key string) bool {
	structuralCache.Lock()
	defer structuralCache.Unlock()
	_, ok := structuralCache.keys[key]
	return ok
}

func rememberStructure(key string) {
	structuralCache.Lock()
	defer structuralCache.Unlock()
	if len(structuralCache.keys) >= 2048 {
		clear(structuralCache.keys)
	}
	structuralCache.keys[key] = struct{}{}
}
