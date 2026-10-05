# ADR-050 固定交付工具、远程证据与修复候选

> 实体ID：adr.ADR-050
> 来源：2026-10-02 按已授权阶段2继续受控七动作实施。

## 状态

accepted（2026-10-02）。先发布 contract 1.3.0，通过结构检查后生成完整派生类型并实施；保留冻结规则及旧发布原字节。

## 背景

1.2 的完整工具回执与预算工具范围只有三条 Go 命令，不能合法执行既有推送规范要求的固定 git 命令。私有远程结果缺少必需 job/工件来源，隔离 VERIFY 候选及 ZIP 也缺完整公开不可变主体。当前受控环境还需要明确冻结可用公开 Go 模块代理，不能凭环境或网络失败静默变更。

## 决策

发布 [1.3.0完整契约](../contracts/releases/1.3.0/README.md)，增加固定 GitExecutionPolicy、GoModulePolicy、Git 工具回执分支、RemoteExecutionReceipt/CI观察、PackageReceipt 和 VerificationCandidate；S2审计完整公开这些事实及验证报告。原 Go post_commands 保留 VERIFY 职责，Git 限制在批准 push_repository；冻结二进制与参数、环境、预算，不增加任意 shell。push 通过ci.skip避免无归属首跑，随后trigger按原operation/request变量唯一关联，未知只读核实。

修复候选只记录真实隔离差异；采用时新source/PLAN/P1，不能充当原批准工件。交付包和CI工件按原字节登记，完成检查独立核对完整实际证据。旧版本按原Schema和hash读取，新字段不能补造历史。3.2定义重新发布完整闭包，复用已发布模板原字节并显式冻结代理和Git身份，不重写3.1历史。

## 理由

完整公开对象及单向来源允许独立复核真实授权与完成，不依赖validation内部实现。明确reconcile控制意图使终态原副作用也能按原身份只读核实，不用GET产生网络副作用或恢复终态。

## 备选方案

把Git伪装Go、改用commits API规避本地SHA规范、以pipeline绿色代替job和报告、把私有差异当批准文件，均拒绝。代理故障必须按冻结策略失败，不能静默direct或关闭验签。

## 影响

新增发布源、完整生成类型及旧版兼容、Git工具预算语义与版本化3.2资产。受控push/CI、候选重批和完成门各自有真实检查，默认切换仍等完整闭环，文档检查只代表docs_ready。

## 关联

- 域：[agent](../02-agent/overview.md)、[integrations](../04-integrations/overview.md)、[governance](../06-governance/overview.md)、[observability](../07-observability/overview.md)。
- 功能：[repository-push](../04-integrations/features/repository-push.md)、[ci-config-trigger](../04-integrations/features/ci-config-trigger.md)、[ci-verification](../04-integrations/features/ci-verification.md)、[unknown-results](../04-integrations/features/unknown-results.md)、[budget-limits](../06-governance/features/budget-limits.md)、[permissions-and-secrets](../06-governance/features/permissions-and-secrets.md)、[verification-evidence](../07-observability/features/verification-evidence.md)、[human-gates](../01-orchestrator/features/human-gates.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：S2实现核对后发布1.3，保留1.0–1.2、旧批准及原工具网络策略。
