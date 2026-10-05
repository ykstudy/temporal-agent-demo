package contracts

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dlclark/regexp2"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.json schemas/1.1.0/*.json schemas/1.2.0/*.json schemas/1.3.0/*.json schemas/1.4.0/*.json
var releaseFS embed.FS

type offlineLoader struct{}

func (offlineLoader) Load(string) (any, error) {
	return nil, fmt.Errorf("schema reference is outside the embedded release")
}

type ecmaRegexp struct{ re *regexp2.Regexp }

func (r ecmaRegexp) MatchString(s string) bool {
	ok, err := r.re.MatchString(s)
	return err == nil && ok
}
func (r ecmaRegexp) String() string { return r.re.String() }

func newCompiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	c.UseLoader(offlineLoader{})
	c.UseRegexpEngine(func(pattern string) (jsonschema.Regexp, error) {
		r, err := regexp2.Compile(pattern, regexp2.ECMAScript)
		if err != nil {
			return nil, err
		}
		r.MatchTimeout = 100 * time.Millisecond
		return ecmaRegexp{r}, nil
	})
	return c
}

// SchemaFiles returns owned copies of the exact published source bytes. Runtime
// code never reads design/ or downloads a $ref.
func SchemaFiles() map[string][]byte {
	out, _ := SchemaFilesFor(ContractVersion)
	return out
}

func SchemaDigests() map[string]string {
	out, _ := SchemaDigestsFor(ContractVersion)
	return out
}

// SupportedContractVersions lists exact independently published releases.
func SupportedContractVersions() []string {
	versions := make([]string, 0, len(generatedVersionSourceHashes))
	for version := range generatedVersionSourceHashes {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return versions
}

func SchemaFilesFor(version string) (map[string][]byte, error) {
	hashes, ok := generatedVersionSourceHashes[version]
	if !ok {
		return nil, fmt.Errorf("unsupported contract version")
	}
	out := make(map[string][]byte, len(hashes))
	for filename, digest := range hashes {
		data, err := releaseFS.ReadFile("schemas/" + version + "/" + filename)
		if err != nil || BytesHash(data) != digest {
			return nil, fmt.Errorf("embedded schema source digest mismatch")
		}
		out[filename] = append([]byte(nil), data...)
	}
	return out, nil
}

func SchemaDigestsFor(version string) (map[string]string, error) {
	hashes, ok := generatedVersionSourceHashes[version]
	if !ok {
		return nil, fmt.Errorf("unsupported contract version")
	}
	out := make(map[string]string, len(hashes))
	for filename, digest := range hashes {
		out[filename] = digest
	}
	return out, nil
}

// SchemaRef resolves a generated type name or an explicit release-local $defs
// reference. A caller cannot use a URL to cause network or filesystem access.
func SchemaRef(name string) (string, error) {
	return SchemaRefFor(name, LatestContractVersion)
}

func SchemaRefFor(name, version string) (string, error) {
	refs, ok := generatedVersionSchemaRefs[version]
	if !ok {
		return "", fmt.Errorf("unsupported contract version")
	}
	if ref, ok := refs[name]; ok {
		return ref, nil
	}
	prefix := "https://agentstudio.invalid/contracts/" + version + "/"
	for _, ref := range refs {
		if ref == name || ref == prefix+name {
			return ref, nil
		}
	}
	return "", fmt.Errorf("unknown published contract type")
}

var compiledOnce sync.Once
var compiledSchemas map[string]*jsonschema.Schema
var compiledError error

func initializeSchemas() {
	compiledSchemas = make(map[string]*jsonschema.Schema)
	for _, version := range SupportedContractVersions() {
		// A separate compiler per release makes cross-release refs impossible.
		c := newCompiler()
		files, err := SchemaFilesFor(version)
		if err != nil {
			compiledError = err
			return
		}
		for _, data := range files {
			doc, err := ParseJSON(data)
			if err != nil {
				compiledError = err
				return
			}
			object := doc.(map[string]any)
			if err := c.AddResource(object["$id"].(string), doc); err != nil {
				compiledError = err
				return
			}
		}
		names := make([]string, 0, len(generatedVersionSchemaRefs[version]))
		for name := range generatedVersionSchemaRefs[version] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			ref := generatedVersionSchemaRefs[version][name]
			if _, ok := compiledSchemas[ref]; ok {
				continue
			}
			schema, err := c.Compile(ref)
			if err != nil {
				compiledError = fmt.Errorf("compile published %s %s: %w", version, name, err)
				return
			}
			compiledSchemas[ref] = schema
		}
	}
}

// ValidateJSON performs all published Draft 2020-12 constraints, including format
// assertions, allOf/if/then/oneOf and unevaluatedProperties. It does not prove hash
// correctness, reference existence, authorization, or a runtime completion gate.
func ValidateJSON(name string, data []byte) error {
	key := structuralKey(name, "declared", data)
	if structureCached(key) {
		return nil
	}
	value, err := ParseJSON(data)
	if err != nil {
		return err
	}
	version := LatestContractVersion
	if object, ok := value.(map[string]any); ok {
		if declared, ok := object["contract_version"].(string); ok {
			version = declared
		}
	}
	if err := validateParsed(name, version, value); err != nil {
		return err
	}
	rememberStructure(key)
	return nil
}

func ValidateJSONVersion(name, version string, data []byte) error {
	key := structuralKey(name, version, data)
	if structureCached(key) {
		return nil
	}
	value, err := ParseJSON(data)
	if err != nil {
		return err
	}
	if err := validateParsed(name, version, value); err != nil {
		return err
	}
	rememberStructure(key)
	return nil
}

func validateParsed(name, version string, value any) error {
	ref, err := SchemaRefFor(name, version)
	if err != nil {
		return err
	}
	compiledOnce.Do(initializeSchemas)
	if compiledError != nil {
		return compiledError
	}
	if err := compiledSchemas[ref].Validate(value); err != nil {
		// Validation errors can embed user content. Keep public errors structural.
		return fmt.Errorf("value violates published %s Schema", name)
	}
	return nil
}

func Validate(name string, value any) error {
	data, err := jsonBytes(value)
	if err != nil {
		return err
	}
	return ValidateJSON(name, data)
}

func ValidateVersion(name, version string, value any) error {
	data, err := jsonBytes(value)
	if err != nil {
		return err
	}
	return ValidateJSONVersion(name, version, data)
}

// A generated union can occur inside either release. Its parent boundary must
// select and validate the full release before decoding, so this method merely
// permits exact supported union shapes without assuming the active release.
func validateUnion(name string, data []byte) error {
	key := structuralKey(name, "supported-union", data)
	if structureCached(key) {
		return nil
	}
	value, err := ParseJSON(data)
	if err != nil {
		return err
	}
	if object, ok := value.(map[string]any); ok {
		if version, ok := object["contract_version"].(string); ok {
			if err := validateParsed(name, version, value); err != nil {
				return err
			}
			rememberStructure(key)
			return nil
		}
	}
	for _, version := range SupportedContractVersions() {
		if err := validateParsed(name, version, value); err == nil {
			rememberStructure(key)
			return nil
		}
	}
	return fmt.Errorf("value violates published %s union Schema", name)
}

// ValidateDocument is for published Skill parameter schemas. It uses the same
// full validator and embedded references; no remote resource loading is possible.
func ValidateDocument(schema any, value any) error {
	return ValidateDocumentVersion(ContractVersion, schema, value)
}

func ValidateDocumentVersion(version string, schema any, value any) error {
	schemaBytes, err := jsonBytes(schema)
	if err != nil {
		return err
	}
	valueBytes, err := jsonBytes(value)
	if err != nil {
		return err
	}
	key := structuralKey("parameter:"+BytesHash(schemaBytes), version, valueBytes)
	if structureCached(key) {
		return nil
	}
	files, err := SchemaFilesFor(version)
	if err != nil {
		return err
	}
	schemaValue, err := ParseJSON(schemaBytes)
	if err != nil {
		return err
	}
	instance, err := ParseJSON(valueBytes)
	if err != nil {
		return err
	}
	c := newCompiler()
	for _, data := range files {
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("invalid embedded schema")
		}
		if err := c.AddResource(doc["$id"].(string), doc); err != nil {
			return fmt.Errorf("cannot register embedded schema")
		}
	}
	id := "https://agentstudio.invalid/contracts/" + version + "/parameter-validation.json"
	if object, ok := schemaValue.(map[string]any); ok {
		if declared, ok := object["$id"].(string); ok && !strings.HasPrefix(declared, "https://agentstudio.invalid/contracts/"+version+"/") {
			return fmt.Errorf("parameter Schema identity is outside the selected release")
		}
	}
	if err := c.AddResource(id, schemaValue); err != nil {
		return fmt.Errorf("invalid parameter Schema")
	}
	compiled, err := c.Compile(id)
	if err != nil {
		return fmt.Errorf("invalid or unresolved parameter Schema")
	}
	if err := compiled.Validate(instance); err != nil {
		return fmt.Errorf("parameters violate published Skill Schema")
	}
	rememberStructure(key)
	return nil
}
