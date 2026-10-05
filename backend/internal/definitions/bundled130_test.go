package definitions

import (
	"encoding/json"
	"testing"
	"time"

	"agentstudio/internal/contracts"
)

func TestBundled320FreezesFull13ClosureWithoutChangingOlderPublications(t *testing.T) {
	registry, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"3.0.1", "3.1.1", "3.1.2", "3.2.0"} {
		release, err := LoadBundledRelease(version)
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		before, _ := json.Marshal(release.Definition)
		err = registry.Publish(release)
		if err != nil {
			t.Fatalf("publish %s: %v", version, err)
		}
		definition := release.Definition
		snapshot, err := registry.Freeze(definition.DefinitionID, version, definition.DefinitionHash, nil, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(snapshot.Definition)
		if string(before) != string(after) {
			t.Fatal("publication changed during binding", version)
		}
		if version == "3.2.0" {
			if snapshot.ContractVersion != "1.3.0" || snapshot.Definition.GoModulePolicy.ProxyURL != "https://goproxy.cn" || len(snapshot.Definition.RepositoryPolicy.GitExecution.Commands) != 6 || len(snapshot.ToolBindings) != 3 {
				t.Fatal("incomplete controlled delivery dependencies")
			}
			for _, skill := range snapshot.Skills {
				if skill.ContractVersion != "1.3.0" || skill.SkillVersion != "3.2.0" || contracts.ValidateVersion("SkillDefinition", "1.3.0", skill) != nil {
					t.Fatal("mixed Skill contract closure")
				}
			}
		}
	}
}
