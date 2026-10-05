# Independent public-contract oracle

This separate Python process consumes the released public schemas and HTTP/content bytes. It never imports product packages, private templates, Run workspaces, or the design checker. The standard-library legacy caller uses an explicit legacy-only adapter; canonical v2 uses this full-schema oracle. Passing this oracle does not decide product DONE.

The default release is `contract_version=1.1.0`, copied byte-for-byte from `design/contracts/releases/1.1.0`. `schemas/publication.json` records the source, identity, size and digest of all three files. The original 1.0.0 publication remains unchanged. The full `jsonschema` Draft 2020-12 validator uses an explicit local registry and `FormatChecker`; unregistered references cannot fetch the network. Strict parsing rejects duplicate keys, invalid UTF-8/scalars, unsafe integer literals, nonfinite numbers and trailing documents before validation. Published resource bounds are 16 MiB, value depth 128 (root 0), 500000 values excluding object keys, 128 bytes per number literal, and exponent magnitude 400. Exact Decimal parsing preserves fractional integer-schema failures such as `1.00000000000000000001`; the complete standard validator uses an exact integer type predicate, while JCS converts valid decimals to binary64 as RFC 8785 requires. `1.0` and `1e0` remain valid integers. Schema-valid numbers, date-time, and cross-object semantics remain separate checks.

Install into an isolated environment:

```sh
python3 -m venv /tmp/temporal-agent-contract-oracle-env
/tmp/temporal-agent-contract-oracle-env/bin/python -m pip install -r validation/contracts/requirements.txt
/tmp/temporal-agent-contract-oracle-env/bin/python validation/contracts/publish.py --check
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.self_test --report validation/reports/contracts-s1/oracle-self-test.json
node validation/contracts/jcs_vectors.mjs
```

`publish.py --source PATH --destination PATH` explicitly publishes another contract directory. `--check` never rewrites files to conceal drift. `Oracle(release=..., source=...)` supports separate releases without combining their registries or field aliases.

## 1.3 public-export structure and hash slice

`release13_check` consumes exported **public S2AuditBundle** objects under an `audit` key. The product-test export records whether its initial proposal was test input; this reader does not use private Run fields, templates or directories. It checks the full 1.3 Schema, every enumerated public object's JCS hash, and publication-derived structural negatives for new policies, candidates, package/remote receipts and reports actually present. This is an exported-object check, not an HTTP collection or complete cross-object semantic oracle. Missing remote receipts and unexercised branches remain explicitly uncovered.

Product tests export these public objects only when `CONTROL_RECOVERY_EVIDENCE_DIR` or `CONTROL_SQLITE_EVIDENCE_DIR` is explicitly set. Use an absolute output path: Go runs tests from the package directory, so a path relative to the shell's `backend/` directory would write to a different location. Moving an already exported file without changing its bytes does not require repeating the tool execution.

```sh
python3 validation/contracts/publish.py --source design/contracts/releases/1.3.0 --destination validation/contracts/releases/1.3.0
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.release13_check \
  --audit-export docs/evidence/s2-candidate-revision/rules-reapproval-done.json \
  --audit-export docs/evidence/s2-candidate-revision/model-revision-plan.json \
  --report validation/reports/contracts-s2/release13-exports.json
```

Use `publish.py` with the same paths and `--check` to verify the derived release without modifying it. Old releases, fixtures and 1.2 semantic oracles stay separate; this slice does not turn their historical success into 1.3 completion.

`fixtures/definition-snapshot.valid.json` is a **synthetic oracle fixture**. Its shape starts from the public S0 example, explicitly migrated to 1.1.0. Hashes bind independent placeholder template bytes and the released schemas. It is not an Agent output and cannot prove template generation. Regenerate only with `python -m validation.contracts.build_fixtures` after an intentional release change; `content-manifest.json` records provenance. The self-test derives required/type/enum/const/pattern/bounds/unknown-field negatives from the public schema, validates real embedded parameter schemas, and separately tests coherent forged snapshots with recalculated outer hashes. Semantic tests cover prompt bytes, dependency closure, immutable versions, template content, defaults, command/tool bindings, canonical collection order and schema digest identity.

The seven JCS vectors have explicit canonical reference strings covering UTF-16 property sorting, ECMAScript number formatting, escaping and unchanged Unicode. Their source is [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785). Python uses `rfc8785==0.1.4`; JavaScript uses the RFC's object-key sorting plus native JSON serialization. Product Go/TS implementations must additionally consume the same vectors; the standalone JavaScript check does not claim product TypeScript validation.

Read a real Run's complete S1 snapshot and frozen byte dependencies through the published read-only legacy perimeter:

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.cli \
  --base-url http://127.0.0.1:18090 --run-id actual-run-id \
  --expected-decision-mode rules --report-dir validation/reports/contracts-s1/http
```

Repeat `--run-id` to check immutable definition/skill/template versions across Runs. The client calls only `GET /api/definitions`, `GET /api/runs/{id}/definition`, and the exact content route; it validates original bytes, digest headers, complete bindings, invalid reads and stable repeat reads. Add `--legacy-run-id ID` to verify the S1 definition perimeter's explicit source-incomplete 409 without calling it complete v2 migration. It does not create or approve Runs. Use the product's normal public creation and approval path independently. Full v2 RunSnapshot, live models, Skill execution, drift after restart, and C06 v2 legacy migration still require their explicit scenario evidence.

An exported public closure may be checked with `--export DIR`: provide `definition-snapshot.json` and `content/<sha256>` for every template and schema dependency. Do not point this option at private product directories. `--snapshot FILE --definition NAME` checks only a standalone public object's strict structure and says so in its report. All commands save evidence to the specified report directory and return nonzero on rejection; no response shape is relaxed to make the product pass.

For C03 actual request binding, `provider_fixture` supplies a separate mock `/v1/chat/completions` endpoint. Its only successful advice is a fully schema-validated clarification. It saves actual role/content inputs and their byte digests, never authentication headers. It does not generate a Plan or claim a real model succeeded:

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.provider_fixture \
  --port 18128 --output validation/reports/contracts-s1/provider-fixture
```

Configure an isolated Agent with that fixture, explicitly create a model-mode Run using a unique user-message marker, and compare the public request record to the public frozen snapshot:

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.provider_check \
  --base-url http://127.0.0.1:18127 --fixture-url http://127.0.0.1:18128 \
  --run-id actual-model-run --request-marker unique-test-marker \
  --report validation/reports/contracts-s1/provider-binding.json
```

`--minimum-requests 2` can check repeated input after a controlled restart. Merely reading the same prompt twice is not an upgrade-drift test; publishing a new definition and proving that the old Run still requests its original prompt is separate evidence. The fixture can inject one explicit HTTP 503 with `--fail-first`, but that branch is not counted as verified until an actual product retry scenario observes it.

## S2 独立对象链基础自检

`decision_semantics.py` 只消费发布的完整对象，核对五类建议、实际上下文的 JCS hash、输入消息引用、计划/提案/批准/权限绑定，以及 `DecisionProposal → ActionValidation → ExecutionRequest` 的单向内容链。首次执行期限与已启动动作链的生命周期分开检查；重新计算 hash 不能使跨 Run、扩大范围、撤销权限或重用不可变身份合法。

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.decision_self_test \
  --report validation/reports/contracts-s2/oracle-self-test.json
```

`decision_fixtures.py` 产生的对象都是独立 synthetic 样例，不是产品输出；该入口不访问 Agent、不调用 provider、不执行工具，也不能作为 C07–C10 的运行验收。此入口保留验证 1.1.0 对象的基础语义。`--release` 仅选择带来源清单的 Schema 副本，切换版本时必须另行发布匹配的 fixtures，不能把旧对象静默改名为新契约。

## S2 契约 1.2.0

`releases/1.2.0/` 是从 `design/contracts/releases/1.2.0/` 独立复制的完整发布包，使用单独 registry；不覆盖原 `schemas/` 的 1.1.0 字节。来源摘要、502 个离线引用与完整 Draft 2020-12 均由同一严格 oracle 核对。新增 hash 的排除字段固定来自 ADR-049 和发布 README，不从产品输出反推。

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python validation/contracts/publish.py \
  --source design/contracts/releases/1.2.0 --destination validation/contracts/releases/1.2.0 --check
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.s2_self_test \
  --report validation/reports/contracts-s2/oracle-1.2-self-test.json
```

`s2_fixtures.py` 产生新的 synthetic 身份、定义/技能/模板版本和内容，保存到 `fixtures/s2/`。它包含七类动作（含三次不同技能应用）、完整提案/批准/权限/输入消息/上下文、操作、预算预留/结算、工具与工作区回执。文件携带 synthetic 来源说明，不能作为旧 Run 升级、真实工具执行或 provider 证据。`s2_self_test` 覆盖新对象逐字段结构负例和重新计算合法 hash 后仍违反语义的对象；尤其区分 unknown 消耗、尚未结算预留、累计期限、稳定 scope、精确 argv、UTF-8 字节上限及未批准的输出差异。

只读采集器消费已发布的三个 S2 接口，并使用 S1 冻结内容接口检查原字节：

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.s2_collect \
  --base-url http://127.0.0.1:18127 --run-id actual-s2-run \
  --samples 2 --report-dir validation/reports/contracts-s2/public-http
```

每轮保存 `/decision-audit` 的原始响应，独立验证全部可枚举历史对象并以五字段精确引用读取 `/objects`。`/budget` 的 `Link: <...>; rel="canonical"` 提供该账本版本真实登记来源；采集器不拿当前 Run source 猜预算 source，且只允许同源同 Run 的精确对象地址。预算在两个读取间推进时，新版本另记为结构/hash/引用核验，只有与完整 audit 同版的账本才算跨对象语义验证。重复 `--run-id` 可加跨 Run 引用拒绝检查。

`--prior-observation PATH` 可加载此前保存的公开 audit，用于重启前后 immutable 对象、预算窗口与账目追加性比较。观察到的 `state_sequence → state` 只在同一序号匹配请求签发序号时用于合法阶段验证；未观察到的历史阶段明确记为未覆盖，不从 ContextRecord 文本推导。当前生命周期和未知外部写的真实结果，还须通过专用运行场景、provider 请求次数及副作用产物验收；通过 audit 不证明这些事实。

离线传入明确导出的 `audit-bundle.json` 与 `content/<sha256>`：

```sh
/tmp/temporal-agent-contract-oracle-env/bin/python -m validation.contracts.s2_collect \
  --export validation/contracts/fixtures/s2 --audit-name audit-bundle.valid.json \
  --report-dir validation/reports/contracts-s2/synthetic-export
```

该命令报告 `supplied_export_no_live_http`，不会把 synthetic 文件当作实际 Agent HTTP 证据。接口采集始终只发 GET，不创建、批准、恢复、取消或推进 Run。

`s2_transport_self_test --report-dir validation/reports/contracts-s2/transport-mock` 用当前进程持有的临时 loopback HTTP fixture 验证采集器，退出时关闭自己创建的监听。它显式传 `--endpoint-kind mock`，包括“当前 source=2、原预算引用 source=1”的读取；结果为 `synthetic_http_collector_self_test` / `mock_http_contract_transport`，不能作为实际 job 验收。

## 2026-10-04 S2公开1.3验收

受控公开创建显式选择定义3.2.0。采集器用`--release validation/contracts/releases/1.3.0 --source design/contracts/releases/1.3.0`加载独立副本；不把1.2样本改version冒充1.3。新增候选、Git工具、远程逐GET与CI、包及完整报告按发布1.3对象及原文/hash/预算/批准链校验，原1.2负例仍保留。

S2模型fixture先按1.3 `DecisionAdvice`验证合法样本，再明确注入协议或授权破坏，保存实际请求和响应摘要；不导入产品。HTTP场景分别记录C07–C10、local真实生成、真实Temporal Update失败后同身份重试、CAN与并发、launcher拥有进程的VERIFY崩溃。业务拒绝后还观察原decision和预算预留不增加。崩溃后缺工具结果必须安全BLOCKED并保留unknown消费，不能要求盲重放原工具；取消后新Run须重新P1批准并完成原完整方案，原Run保持取消。ZIP、coverage及可执行artifact的原字节验证与仅对象结构验证分开计数，mock provider/CI不记作live模型或Runner。

时间按RFC3339的纳秒精度比较。远程回执的started_at绑定原dispatch预留；本地包回执记录实际包任务开始，必须在action预留之后且在结算之前，不要求其时间字符串等于预留时间。批准前读取不延长服务端期限；过期后必须显式重提案，再使用新签发版本批准。

## S5 当前 1.4 与 runtime/1.0 验收入口

在仓库根使用 `validation/.contract-env/bin/python`（`requirements.txt` 的隔离环境）。以下消费者不导入后端私有实现：

```sh
validation/.contract-env/bin/python -m validation.contracts.schema_sweep --base-url http://127.0.0.1:18170 --run-id <已有完整RunID> --report-dir validation/reports/<新的目录>
validation/.contract-env/bin/python -m validation.contracts.shape_self_test --report validation/reports/<新的目录>/synthetic-shapes.json
python3 -m validation.contracts.legacy_self_test --report validation/reports/<新的目录>/legacy.json
python3 -m validation.contracts.document_examples --report validation/reports/<新的目录>/document-examples.json
validation/.contract-env/bin/python -m validation.contracts.container_delivery --delivery-report validation/reports/runtime-final-local-delivery --report-dir validation/reports/<新的容器目录>
python3 validation/live_preflight.py --report validation/reports/<新的目录>/live-prerequisites.json
```

`schema_sweep` 对完整公开 Run/建议审计/原文及摘要/runtime 输出先做结构和跨对象核验，再按发布 Schema 生成逐字段负例。`--public-export` 可另消费已保存的原始公开响应与 `content/<sha256>`；报告明确其导出来源，不冒充现场 HTTP。缺少正常 DONE 观察的语言分支由 `shape_self_test` 独立 synthetic 身份覆盖，不能把结构样例升级为产品历史或 live 成功。`s5-schema-coverage.json` 分列两类覆盖，不用路径计数推导结构完成。

`scenarios.py` 仅处理原 legacy 输出，经 `legacy_adapter.py` 核对固定 11/14 检查与准确 manifest；`passed/status` 冲突、缺 size、hash 别名、字典形式文件列表均拒绝。完整 v2 场景仍用 `v2_public`。`s2_provider_fixture --contract-version 1.4.0` 明确选择发布版本，先验证完整 DecisionAdvice/SummaryAdvice，再注入指定攻击；`conversation_public --decision-mode model` 记录真实产品模型路径，但 provider 仍为 mock。

容器消费者校验原公开 ZIP 的全部字节后，使用其中原始 Dockerfile 构建并请求隔离容器 health；只清理自己的标签/容器。依赖镜像下载失败保留原失败日志并在阶段索引标记 external_blocked，禁止更换原 Dockerfile 或借缓存的其他 Go 版本宣称通过。`live_preflight` 只输出配置存在与否和精确测试目标绑定，缺项退出 2；它不调用模型、不访问 GitLab、不签发或替代产品批准。
