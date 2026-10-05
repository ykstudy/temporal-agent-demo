# Product definition publication assets

These files belong to the standalone Agent product. Runtime loading does not read
`design/`, another project, or `validation/`.

- `definitions/agent.json` is the complete published `AgentDefinition`.
- `skills/*/skill.json` are the complete three published `SkillDefinition` objects.
- `definitions/release.json` binds those definitions to Schema digests, template
  manifests and concrete tool identities.
- `templates/golang/*/*/render-program.json` preserve the exact source bytes of
  each scaffold/Skill program. Their hashes cover bytes, not parsed JSON.

The current assets use contract 1.1.0 and definition/Skill/template 3.0.1. They
contain actual hashes, including the tool identity of this publication. A changed
prompt, skill, template, or bound tool identity requires a new publication;
same-version different-content publication is rejected by the registry.

`definitions.BuildDefault` validates the complete input declarations and their
hashes, resolves the selected exact dependency bytes, and produces the complete
release. `Registry.Publish` verifies the whole closure before writing immutable
version records and content-addressed blobs. `Registry.Freeze` binds a Run to one
release, and all later reads verify the same persisted closure and revocations.
`ExportReleaseAssets` is a developer publishing operation, never an Agent Run
operation or HTTP endpoint. It exports a verified release for review.

Template `content_ref` identifies `template_content` raw bytes. The containing
`TemplateDependency.manifest_hash` hashes the full manifest separately, avoiding
self-reference. A missing, modified, or revoked dependency blocks resolution;
there is no fallback to current Worker code.
