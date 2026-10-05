// Package assets contains publication source declarations. They are never served
// as frozen definitions: the registry resolves exact dependency bytes, tool
// identities and real content hashes before publishing complete public objects.
package assets

import "embed"

// Sources is part of the product binary, independent of design/ and validation/.
//
//go:embed definitions/*.json skills/*/skill.json templates/golang/*/*/*.json releases
var Sources embed.FS
