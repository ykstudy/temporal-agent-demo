package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentstudio/internal/contracts"
	"agentstudio/internal/definitions"
	"agentstudio/internal/governance"
)

func pureSQLitePrograms312(t *testing.T, old definitions.Release) map[string]templateProgram {
	t.Helper()
	programs := map[string]templateProgram{}
	for _, dep := range old.TemplateDependencies {
		var p templateProgram
		if len(dep.Files) != 1 {
			t.Fatal("expected exact published render program")
		}
		if err := json.Unmarshal(old.TemplateContents[dep.Files[0].SHA256], &p); err != nil {
			t.Fatal(err)
		}
		for i := range p.Rules {
			r := &p.Rules[i]
			if dep.TemplateID == "scaffold" && r.Path == "Dockerfile" {
				r.Source = strings.ReplaceAll(r.Source, `{{if and (skill "add_database") (eq .Database "sqlite")}}1{{else}}0{{end}}`, "0")
				r.Source = strings.ReplaceAll(r.Source, `{{if and (skill "add_database") (eq .Database "sqlite")}}base{{else}}static{{end}}`, "static")
			}
			if dep.TemplateID != "database" || r.When["database"] != "sqlite" {
				continue
			}
			switch r.Path {
			case "go.mod":
				if r.When["orm"] == "gorm" {
					r.Source = "\nrequire (\n\tgorm.io/driver/sqlite v1.6.0\n\tgorm.io/gorm v1.31.0\n\tmodernc.org/sqlite v1.40.1\n)\n"
				} else {
					r.Source = "\nrequire (\n\tgithub.com/jmoiron/sqlx v1.4.0\n\tmodernc.org/sqlite v1.40.1\n)\n"
				}
			case "go.sum":
				r.Source = ""
			case "internal/database/database.go":
				if r.When["orm"] == "gorm" {
					r.Source = strings.Replace(r.Source, `"gorm.io/driver/sqlite"`, "\"gorm.io/driver/sqlite\"\n    _ \"modernc.org/sqlite\"", 1)
					r.Source = strings.Replace(r.Source, "sqlite.Open(dsn)", `sqlite.New(sqlite.Config{DriverName: "sqlite", DSN: dsn})`, 1)
				} else {
					r.Source = strings.ReplaceAll(r.Source, "github.com/mattn/go-sqlite3", "modernc.org/sqlite")
					r.Source = strings.ReplaceAll(r.Source, `sqlx.Connect("sqlite3", dsn)`, `sqlx.Connect("sqlite", dsn)`)
				}
			case "internal/database/isolation_test.go":
				if r.When["orm"] == "gorm" {
					r.Source = pureSQLiteGORMTest312
				} else {
					r.Source = pureSQLiteSQLXTest312
				}
			}
		}
		programs[dep.TemplateID] = p
	}
	return programs
}

const pureSQLiteGORMTest312 = `package database
import("errors";"path/filepath";"testing";"gorm.io/gorm")
type sampleRecord struct { ID uint ` + "`gorm:\"primaryKey\"`" + `; Name string }
func TestIsolatedSQLiteMigration(t *testing.T){
    dsn:=filepath.Join(t.TempDir(),"isolated.db")
    db,err:=Open(dsn);if err!=nil{t.Fatal(err)}
    connection,err:=db.DB();if err!=nil{t.Fatal(err)};defer connection.Close()
    if err=Migrate(db);err!=nil{t.Fatal(err)};if err=Migrate(db);err!=nil{t.Fatal(err)}
    if !db.Migrator().HasTable("schema_versions"){t.Fatal("migration did not create table")}
    if err=db.AutoMigrate(&sampleRecord{});err!=nil{t.Fatal(err)}
    row:=sampleRecord{Name:"first"};if err=db.Create(&row).Error;err!=nil{t.Fatal(err)}
    if row.ID==0{t.Fatal("insert did not return ID")}
    var got sampleRecord;if err=db.First(&got,row.ID).Error;err!=nil||got.Name!="first"{t.Fatal("read",err,got)}
    if err=db.Model(&row).Update("name","second").Error;err!=nil{t.Fatal(err)}
    rollback:=errors.New("rollback")
    if err=db.Transaction(func(tx *gorm.DB)error{if e:=tx.Create(&sampleRecord{Name:"rolled-back"}).Error;e!=nil{return e};return rollback});!errors.Is(err,rollback){t.Fatal(err)}
    var count int64;if err=db.Model(&sampleRecord{}).Count(&count).Error;err!=nil||count!=1{t.Fatal("transaction rollback",err,count)}
    if err=db.First(&got,row.ID).Error;err!=nil||got.Name!="second"{t.Fatal("update",err,got)}
    if err=db.Delete(&row).Error;err!=nil{t.Fatal(err)}
    if err=db.Model(&sampleRecord{}).Count(&count).Error;err!=nil||count!=0{t.Fatal("delete",err,count)}
    if err=Check(dsn);err!=nil{t.Fatal(err)}
}
`
const pureSQLiteSQLXTest312 = `package database
import("path/filepath";"testing")
func TestIsolatedSQLiteMigration(t *testing.T){
    dsn:=filepath.Join(t.TempDir(),"isolated.db")
    db,err:=Open(dsn);if err!=nil{t.Fatal(err)};defer db.Close()
    if err=Migrate(db);err!=nil{t.Fatal(err)};if err=Migrate(db);err!=nil{t.Fatal(err)}
    var count int;if err=db.Get(&count,"SELECT count(*) FROM schema_versions");err!=nil{t.Fatal(err)}
    if _,err=db.Exec("CREATE TABLE sample_records (id INTEGER PRIMARY KEY, name TEXT NOT NULL)");err!=nil{t.Fatal(err)}
    result,err:=db.Exec("INSERT INTO sample_records(name) VALUES (?)","first");if err!=nil{t.Fatal(err)}
    id,err:=result.LastInsertId();if err!=nil||id==0{t.Fatal("insert",err,id)}
    var name string;if err=db.Get(&name,"SELECT name FROM sample_records WHERE id=?",id);err!=nil||name!="first"{t.Fatal("read",err,name)}
    if _,err=db.Exec("UPDATE sample_records SET name=? WHERE id=?","second",id);err!=nil{t.Fatal(err)}
    tx,err:=db.Beginx();if err!=nil{t.Fatal(err)}
    if _,err=tx.Exec("INSERT INTO sample_records(name) VALUES (?)","rolled-back");err!=nil{t.Fatal(err)}
    if err=tx.Rollback();err!=nil{t.Fatal(err)}
    if err=db.Get(&count,"SELECT count(*) FROM sample_records");err!=nil||count!=1{t.Fatal("transaction rollback",err,count)}
    if err=db.Get(&name,"SELECT name FROM sample_records WHERE id=?",id);err!=nil||name!="second"{t.Fatal("update",err,name)}
    if _,err=db.Exec("DELETE FROM sample_records WHERE id=?",id);err!=nil{t.Fatal(err)}
    if err=db.Get(&count,"SELECT count(*) FROM sample_records");err!=nil||count!=0{t.Fatal("delete",err,count)}
    if err=Check(dsn);err!=nil{t.Fatal(err)}
}
`

func pureSQLiteRelease312(t *testing.T, old definitions.Release, programs map[string]templateProgram) definitions.Release {
	t.Helper()
	bundles := []definitions.TemplateBundle{}
	for _, id := range []string{"scaffold", "database", "auth", "logging"} {
		body, err := json.Marshal(programs[id])
		if err != nil {
			t.Fatal(err)
		}
		version := "3.1.0"
		if id == "scaffold" || id == "database" {
			version = "3.1.2"
		}
		bundles = append(bundles, definitions.TemplateBundle{ID: id, Version: version, Files: map[string][]byte{"render-program.json": body}})
	}
	release, err := definitions.BuildDefault(definitions.PublishOptions{ContractVersion: "1.2.0", DefinitionVersion: "3.1.2", SkillVersion: "3.1.2", Templates: bundles, Tools: old.ToolBindings, SystemPrompt: old.Definition.SystemPrompt.Content})
	if err != nil {
		t.Fatal(err)
	}
	release.Definition = old.Definition
	release.Definition.DefinitionVersion = "3.1.2"
	release.Definition.AvailableSkills = append([]contracts.SkillReference{}, old.Definition.AvailableSkills...)
	release.Skills = append([]contracts.SkillDefinition{}, old.Skills...)
	for i := range release.Skills {
		skill := &release.Skills[i]
		if skill.SkillID != "add_database" {
			continue
		}
		skill.SkillVersion = "3.1.2"
		for _, dep := range release.TemplateDependencies {
			if dep.TemplateID == "database" {
				skill.TemplateDependencies = []contracts.TemplateDependency{dep}
			}
		}
		skill.SkillHash = chainRehash(t, "SkillDefinition", *skill)
		for j := range release.Definition.AvailableSkills {
			ref := &release.Definition.AvailableSkills[j]
			if ref.SkillID == skill.SkillID {
				ref.SkillVersion, ref.SkillHash = skill.SkillVersion, skill.SkillHash
			}
		}
	}
	release.Definition.DefinitionHash = chainRehash(t, "AgentDefinition", release.Definition)
	return release
}

// Explicit developer publication, never a service fallback. A changed output
// refuses to overwrite already published bytes and requires another version.
func TestRegeneratePublication312(t *testing.T) {
	if os.Getenv("AGENT_REGENERATE_312") != "1" {
		t.Skip("explicit developer publication only")
	}
	requirePublicationTool310(t)
	old, err := definitions.LoadBundledRelease("3.1.1")
	if err != nil {
		t.Fatal(err)
	}
	programs := pureSQLitePrograms312(t, old)
	initial := pureSQLiteRelease312(t, old, programs)
	s, r := publicationRenderer(t, initial)
	_, goBinary, err := controlGoIdentity(r)
	if err != nil {
		t.Fatal(err)
	}
	locks := []map[string]any{}
	for _, orm := range []string{"gorm", "sqlx"} {
		r.Plan = variantPlan310(dependencyVariant310{Name: "sqlite_" + orm, Database: "sqlite", ORM: orm}, "none", "none")
		files, err := s.renderFrozenFiles(r)
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		writeTemplateFiles310(t, dir, files)
		for _, args := range [][]string{{"mod", "tidy"}, {"test", "./..."}, {"build", "./..."}} {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			cmd := exec.CommandContext(ctx, goBinary, args...)
			cmd.Dir = dir
			cmd.Env = append(isolatedGoEnv(), "GOTOOLCHAIN=local", "GOENV=off", "CGO_ENABLED=0", "GOPROXY=https://goproxy.cn", "GOSUMDB=sum.golang.org")
			out, e := cmd.CombinedOutput()
			cancel()
			if e != nil {
				t.Fatalf("%s %v: %v\n%s", orm, args, e, out)
			}
		}
		resolved := readTemplateFiles310(t, dir)
		prefix := "module " + r.Plan.ProjectName + "\n\ngo 1.25.0\n"
		if !strings.HasPrefix(resolved["go.mod"], prefix) {
			t.Fatal("module baseline changed")
		}
		p := programs["database"]
		for i := range p.Rules {
			item := &p.Rules[i]
			if item.When["database"] != "sqlite" || item.When["orm"] != orm {
				continue
			}
			if item.Path == "go.mod" {
				item.Source = strings.TrimPrefix(resolved["go.mod"], prefix)
			}
			if item.Path == "go.sum" {
				item.Source = resolved["go.sum"]
			}
		}
		programs["database"] = p
		locks = append(locks, map[string]any{"variant": "sqlite_" + orm, "go_mod_sha256": contracts.BytesHash([]byte(resolved["go.mod"])), "go_sum_sha256": contracts.BytesHash([]byte(resolved["go.sum"]))})
	}
	release := pureSQLiteRelease312(t, old, programs)
	dir := t.TempDir()
	if err = definitions.ExportReleaseAssets(dir, release); err != nil {
		t.Fatal(err)
	}
	generated := readTemplateFiles310(t, dir)
	// Exact older dependencies are loaded by their existing version, not copied
	// under a new identity or rewritten by this publication.
	for path := range generated {
		if strings.HasPrefix(path, "skills/") && !strings.HasPrefix(path, "skills/add_database/") || strings.HasPrefix(path, "templates/golang/auth/") || strings.HasPrefix(path, "templates/golang/logging/") {
			delete(generated, path)
		}
	}
	body, _ := json.MarshalIndent(map[string]any{"definition_version": "3.1.2", "contract_version": "1.2.0", "definition_hash": release.Definition.DefinitionHash, "source_definition_hash": old.Definition.DefinitionHash, "tool": release.ToolBindings, "cgo_enabled": false, "variants": locks}, "", "  ")
	generated["dependency-locks.json"] = string(append(body, '\n'))
	output := filepath.Join("..", "..", "assets", "releases", "3.1.2")
	for path, body := range generated {
		existing, e := os.ReadFile(filepath.Join(output, path))
		if e == nil && string(existing) != body {
			t.Fatalf("immutable release differs: %s", path)
		}
		if e != nil && !os.IsNotExist(e) {
			t.Fatal(e)
		}
	}
	writeTemplateFiles310(t, output, generated)
	t.Logf("3.1.2 definition_hash=%s", release.Definition.DefinitionHash)
}

func controlSQLiteRun312(t *testing.T, s *Service, version, orm string) Run {
	t.Helper()
	release, err := definitions.LoadBundledRelease(version)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Definitions.Publish(release); err != nil {
		t.Fatal(err)
	}
	d := release.Definition
	frozen, err := s.Definitions.Freeze(d.DefinitionID, d.DefinitionVersion, d.DefinitionHash, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	id := "controlled_sqlite_" + orm
	r := Run{ID: id, Title: id, State: "INTAKE", SourceVersion: 1, Version: 1, StateSequence: 1, FrozenDefinition: &frozen, Definition: legacyDefinition(frozen), Plan: defaultPlan(), ExecutionMode: "local", DecisionMode: "rules", DeliveryMode: "local", Messages: []Message{}, Steps: initialSteps(), Artifacts: []Artifact{}, History: []History{}, Operations: map[string]OperationRecord{}, Processed: map[string]string{}, CreatedAt: now(), UpdatedAt: now()}
	budget, err := governance.NewBudgetLedgerVersion(frozen.ContractVersion, id, frozen.SnapshotHash, frozen.Definition.BudgetPolicy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	r.Control, err = newControlState(r, budget)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Store.Create(r)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ProcessCommand(context.Background(), Command{ID: "requirements_" + orm, RunID: id, Content: fmt.Sprintf("创建 sqlite-%s-service，SQLite %s JWT，添加日志", orm, orm)})
	if err != nil {
		t.Fatal(err)
	}
	r = controlAcceptCommand(t, s, result.Run)
	if r.Plan.Database != "sqlite" {
		t.Fatal("did not create SQLite plan")
	}
	return r
}

func TestPublished312PureGoClosureAndCoexistence(t *testing.T) {
	old, err := definitions.LoadBundledRelease("3.1.1")
	if err != nil {
		t.Fatal(err)
	}
	newRelease, err := definitions.LoadBundledRelease("3.1.2")
	if err != nil {
		t.Fatal(err)
	}
	expected := newRelease.Definition
	expected.DefinitionVersion = old.Definition.DefinitionVersion
	expected.AvailableSkills = old.Definition.AvailableSkills
	expected.DefinitionHash = chainRehash(t, "AgentDefinition", expected)
	if !equalContractValue(expected, old.Definition) {
		t.Fatal("unrelated policy/system/tool changed")
	}
	for _, skill := range old.Skills {
		for _, newer := range newRelease.Skills {
			if skill.SkillID == newer.SkillID && skill.SkillID != "add_database" && !equalContractValue(skill, newer) {
				t.Fatal("unrelated Skill identity/content changed")
			}
		}
	}
	directory := t.TempDir()
	registry, err := definitions.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range []definitions.Release{old, newRelease} {
		if err = registry.Publish(release); err != nil {
			t.Fatal(err)
		}
	}
	registry, err = definitions.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range []definitions.Release{old, newRelease} {
		d := release.Definition
		snapshot, e := registry.Freeze(d.DefinitionID, d.DefinitionVersion, d.DefinitionHash, nil, time.Now().UTC())
		if e != nil {
			t.Fatal(e)
		}
		if e = registry.VerifySnapshot(snapshot); e != nil {
			t.Fatal(e)
		}
		for _, dep := range release.TemplateDependencies {
			files, e := registry.TemplateFiles(snapshot, dep.TemplateID)
			if e != nil {
				t.Fatal(e)
			}
			if string(files["render-program.json"]) != string(release.TemplateContents[dep.Files[0].SHA256]) {
				t.Fatal("restart substituted template bytes")
			}
		}
	}
	oldService, oldRun := publicationRenderer(t, old)
	newService, newRun := publicationRenderer(t, newRelease)
	for _, variant := range dependencyVariants310() {
		oldRun.Plan = variantPlan310(variant, "jwt", "file")
		newRun.Plan = oldRun.Plan
		a, e := oldService.renderFrozenFiles(oldRun)
		if e != nil {
			t.Fatal(e)
		}
		b, e := newService.renderFrozenFiles(newRun)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(b["Dockerfile"], "CGO_ENABLED=1") || !strings.Contains(b["Dockerfile"], "distroless/static-") {
			t.Fatal("native build returned")
		}
		if variant.Database != "sqlite" {
			if !equalContractValue(a, b) {
				t.Fatalf("unrelated variant changed: %s", variant.Name)
			}
			continue
		}
		if !strings.Contains(b["internal/database/database.go"], "modernc.org/sqlite") || strings.Contains(b["internal/database/database.go"], `"sqlite3"`) {
			t.Fatal("native driver selected")
		}
		if !strings.Contains(b["go.mod"], "modernc.org/sqlite v1.40.1") {
			t.Fatal("driver version not frozen")
		}
		if a["internal/database/isolation_test.go"] == b["internal/database/isolation_test.go"] {
			t.Fatal("CRUD/rollback checks absent")
		}
	}
}

// Uses empty Run-owned caches and the frozen 1.3 module environment. This is
// intentionally an explicit network/integration test, never silently skipped
// inside a claimed controlled-tool acceptance run.
func TestControlPureSQLiteColdRuns(t *testing.T) {
	if os.Getenv("CONTROL_SQLITE_REAL_TEST") != "1" {
		t.Skip("explicit cold-cache public dependency integration")
	}
	requirePublicationTool310(t)
	for _, orm := range []string{"gorm", "sqlx"} {
		t.Run(orm, func(t *testing.T) {
			s := testService(t)
			r := controlSQLiteRun312(t, s, "3.2.0", orm)
			// Go intentionally makes extracted module directories read-only.
			// Only restore owner write permission under this disposable Run so
			// testing.TempDir can remove it after every actual process has exited.
			t.Cleanup(func() {
				modCache := filepath.Join(s.Config.DataDir, "workspaces", r.ID, ".control-tools", "cache", "mod")
				_ = filepath.WalkDir(modCache, func(path string, item os.DirEntry, err error) error {
					if err == nil && item.IsDir() {
						return os.Chmod(path, 0700)
					}
					return err
				})
			})
			for i := 0; i < 4; i++ {
				next, err := s.PrepareControlAction(context.Background(), r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if next.Execution == nil {
					t.Fatalf("workspace action %d absent: %+v", i, next)
				}
				var receipt contracts.WorkspaceReceipt
				r, receipt, err = s.ExecuteControlWorkspace(context.Background(), r.ID, *next.Execution)
				if err != nil {
					t.Fatalf("workspace action %d: %v", i, err)
				}
				if receipt.Status != "succeeded" {
					t.Fatal("workspace execution failed")
				}
			}
			before, err := s.controlToolFiles(r, r.Workspace)
			if err != nil {
				t.Fatal(err)
			}
			next, err := s.PrepareControlAction(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if next.Execution == nil {
				t.Fatal("verify request absent")
			}
			kind, _, err := scheduleAction(next.Execution.Action)
			if err != nil || kind != "verify" {
				t.Fatal("expected verify", err)
			}
			cache := filepath.Join(s.Config.DataDir, "workspaces", r.ID, ".control-tools", "cache")
			if _, err = os.Stat(cache); !os.IsNotExist(err) {
				t.Fatal("Run cache is not cold", err)
			}
			start := time.Now()
			result, err := s.ExecuteControlVerification(context.Background(), *next.Execution)
			if err != nil {
				t.Fatalf("real controlled verification: %v\nreceipts=%+v", err, result.Receipts)
			}
			if result.ReapprovalRequired || len(result.Receipts) != 3 {
				t.Fatalf("approved content changed or missing command: %+v", result)
			}
			for _, receipt := range result.Receipts {
				if err = contracts.Validate("ToolExecutionReceipt", receipt); err != nil {
					t.Fatal(err)
				}
				if receipt.Status != "succeeded" || receipt.ExitCode == nil || *receipt.ExitCode != 0 || receipt.OutputManifestRef == nil || receipt.InputManifestRef == nil || receipt.InputManifestRef.Hash != receipt.OutputManifestRef.Hash || !hashMatches("ToolExecutionReceipt", receipt, receipt.ReceiptHash) {
					t.Fatalf("incomplete real tool evidence: %+v", receipt)
				}
			}
			r, err = s.Store.Get(r.ID)
			if err != nil {
				t.Fatal(err)
			}
			after, err := s.controlToolFiles(r, r.Workspace)
			if err != nil || !equalContractValue(before, after) {
				t.Fatal("approved source mutated", err)
			}
			if r.Control.Audit.Budget.Totals.Consumed.Action != 5 || r.Control.Audit.Budget.Totals.Consumed.Tool != 3 || r.Control.Audit.Budget.Totals.Reserved.Action != 0 || r.Control.Audit.Budget.Totals.Reserved.Tool != 0 || r.ActiveStep != "" {
				t.Fatal("budget/active attempt not settled")
			}
			var verified bool
			for _, operation := range r.Control.Audit.Operations {
				if operation.OperationID == next.Execution.OperationID {
					verified = operation.Status == "verified" && len(operation.EvidenceRefs) == 3
				}
			}
			if !verified {
				t.Fatal("verified original operation lacks tool evidence")
			}
			audit, err := s.Store.CanonicalAudit(r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if output := os.Getenv("CONTROL_SQLITE_EVIDENCE_DIR"); output != "" {
				if err = os.MkdirAll(output, 0700); err != nil {
					t.Fatal(err)
				}
				body, e := json.MarshalIndent(map[string]any{"kind": "cold_run_controlled_go_tools", "cgo_enabled": false, "sandbox": true, "orm": orm, "elapsed_ms": time.Since(start).Milliseconds(), "passed": true, "audit": audit}, "", "  ")
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(output, "sqlite_"+orm+".json"), append(body, '\n'), 0600); e != nil {
					t.Fatal(e)
				}
			}
			t.Logf("SQLite %s real cold-Run controlled tidy/test/build passed in %s", orm, time.Since(start))
		})
	}
}
