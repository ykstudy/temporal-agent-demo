package agent

import (
	"strings"
	"testing"

	"agentstudio/internal/contracts"
)

func TestControlToolsFrozenModuleEnvironment(t *testing.T) {
	policy := contracts.GoModulePolicy{ProxyURL: "https://goproxy.cn", Sumdb: "sum.golang.org", DownloadMode: "proxy_only"}
	r := Run{FrozenDefinition: &contracts.DefinitionSnapshot{ContractVersion: "1.3.0", Definition: contracts.AgentDefinition{ContractVersion: "1.3.0", GoModulePolicy: policy}}}
	module, err := controlFrozenModuleEnvironment(r)
	if err != nil || module.proxy != policy.ProxyURL {
		t.Fatal("new frozen source not honored", err)
	}
	t.Setenv("GOPROXY", "https://unapproved.invalid,direct")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("HTTP_PROXY", "http://secret.invalid")
	for _, command := range []string{"go.mod_tidy", "go.test", "go.build"} {
		env := map[string]string{}
		for _, item := range controlToolEnvironment(controlToolPaths{}, "/fixed/go", command, module) {
			key, value, _ := strings.Cut(item, "=")
			env[key] = value
		}
		if env["CGO_ENABLED"] != "0" || env["GOTOOLCHAIN"] != "local" || env["GOVCS"] != "*:off" || env["GOENV"] != "off" || env["HTTP_PROXY"] != "" {
			t.Fatal("environment isolation changed")
		}
		if command == "go.mod_tidy" {
			if env["GOPROXY"] != policy.ProxyURL || env["GOSUMDB"] != policy.Sumdb {
				t.Fatal("frozen public source changed")
			}
		} else if env["GOPROXY"] != "off" || env["GOSUMDB"] != "off" {
			t.Fatal("test/build allowed dependency downloads")
		}
	}
	for _, mutate := range []func(*contracts.GoModulePolicy){
		func(p *contracts.GoModulePolicy) { p.ProxyURL = "https://unapproved.invalid" },
		func(p *contracts.GoModulePolicy) { p.ProxyURL = "https://goproxy.cn,direct" },
		func(p *contracts.GoModulePolicy) { p.Sumdb = "off" },
		func(p *contracts.GoModulePolicy) { p.DownloadMode = "direct" },
		func(p *contracts.GoModulePolicy) { p.AllowDirect = true },
		func(p *contracts.GoModulePolicy) { p.InheritEnvironment = true },
		func(p *contracts.GoModulePolicy) { *p = contracts.GoModulePolicy{} },
	} {
		r.FrozenDefinition.Definition.GoModulePolicy = policy
		mutate(&r.FrozenDefinition.Definition.GoModulePolicy)
		if _, err = controlFrozenModuleEnvironment(r); err == nil {
			t.Fatal("invalid frozen policy accepted")
		}
	}
	r.FrozenDefinition.ContractVersion = "1.2.0"
	r.FrozenDefinition.Definition.ContractVersion = "1.2.0"
	r.FrozenDefinition.Definition.GoModulePolicy = policy
	module, err = controlFrozenModuleEnvironment(r)
	if err != nil || module.proxy != "https://proxy.golang.org" || module.sumdb != "sum.golang.org" {
		t.Fatal("old Run source silently changed", err)
	}
	r.FrozenDefinition.ContractVersion = "1.4.0"
	r.FrozenDefinition.Definition.ContractVersion = "1.4.0"
	module, err = controlFrozenModuleEnvironment(r)
	if err != nil || module.proxy != policy.ProxyURL || module.sumdb != policy.Sumdb {
		t.Fatal("published 1.4 frozen source changed", err)
	}
	// 1.4 is now a published release. Keep the unknown-version rejection
	// assertion against an actually unpublished identity.
	r.FrozenDefinition.ContractVersion = "9.9.9"
	r.FrozenDefinition.Definition.ContractVersion = "9.9.9"
	if _, err = controlFrozenModuleEnvironment(r); err == nil {
		t.Fatal("unknown contract defaulted to old source")
	}
}
