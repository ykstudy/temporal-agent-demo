package contracts

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed runtime_schemas/*.json
var runtimeReleaseFS embed.FS
var runtimeOnce sync.Once
var runtimeSchemas map[string]*jsonschema.Schema
var runtimeSchemaError error

func initializeRuntimeSchemas() {
	runtimeSchemas = map[string]*jsonschema.Schema{}
	c := newCompiler()
	files, err := SchemaFilesFor("1.4.0")
	if err != nil {
		runtimeSchemaError = err
		return
	}
	raw, err := runtimeReleaseFS.ReadFile("runtime_schemas/runtime.schema.json")
	if err != nil || BytesHash(raw) != RuntimeSchemaHash {
		runtimeSchemaError = fmt.Errorf("runtime publication digest mismatch")
		return
	}
	files["runtime.schema.json"] = raw
	var names map[string]json.RawMessage
	for _, b := range files {
		value, e := ParseJSON(b)
		if e != nil {
			runtimeSchemaError = e
			return
		}
		object := value.(map[string]any)
		if e = c.AddResource(object["$id"].(string), value); e != nil {
			runtimeSchemaError = e
			return
		}
		if object["$id"] == RuntimeSchemaID {
			var d struct {
				Defs map[string]json.RawMessage `json:"$defs"`
			}
			_ = json.Unmarshal(b, &d)
			names = d.Defs
		}
	}
	for name := range names {
		schema, e := c.Compile(RuntimeSchemaID + "#/$defs/" + name)
		if e != nil {
			runtimeSchemaError = e
			return
		}
		runtimeSchemas[name] = schema
	}
}

func ValidateRuntime(name string, value any) error {
	runtimeOnce.Do(initializeRuntimeSchemas)
	if runtimeSchemaError != nil {
		return runtimeSchemaError
	}
	schema, ok := runtimeSchemas[name]
	if !ok {
		return fmt.Errorf("unknown runtime object")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	key := structuralKey(name, "runtime-1.0.0-"+RuntimeSchemaHash, raw)
	if structureCached(key) {
		return nil
	}
	parsed, err := ParseJSON(raw)
	if err != nil {
		return err
	}
	if err = schema.Validate(parsed); err != nil {
		return fmt.Errorf("runtime schema rejected %s", name)
	}
	rememberStructure(key)
	return nil
}
