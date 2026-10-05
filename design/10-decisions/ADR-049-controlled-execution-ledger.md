# ADR-049 受控执行、完整来源与预算证据账本

> 实体ID：adr.ADR-049
> 来源：2026-10-02 用户授权按提示词1–8实施，S1退出后S2契约核对。

## 状态

accepted（2026-10-02）。先发布 contract 1.2.0 并通过设计检查，再生成派生Schema/Go类型并实施S2；不修订冻结PRODUCT-GUIDE或INVARIANTS。

## 背景

1.1.0已有完整建议、校验、批准、受控请求和运行预算上限，但Usage没有预留/结算或跨恢复累计期限的载体；evidence/workspace引用缺准确公开回执主体。DecisionProposal必须引用真实MessageEnvelope，不能因为S3尚未完成而填假来源。未知GitLab目标的初始Run不能伪造权限目标。

## 决策

按[1.2.0发布说明](../contracts/releases/1.2.0/README.md)发布BudgetReservation、BudgetSettlement、BudgetWindow、BudgetLedger及计量汇总，ContextRecord、ToolExecutionReceipt、WorkspaceReceipt和完整S2AuditBundle。保留原核心对象和固定hash排除，增加新对象的唯一hash输入及单向依赖，预留单向绑定已冻结请求，结算单向绑定预留。

新输入先保存完整text MessageEnvelope作为S3前置切片，旧消息不补造历史。固定七种动作按公开阶段/最小权限表验证；local没有远程执行请求。PostCommand合并声明且固定argv，工具执行范围、来源和证据与批准互相核对。操作target_hash统一对准确action.parameters计算，legacy历史不改写。

首次实际执行前持久预算起点，实际尝试前预留，失败和unknown消耗计账；相同attempt回执不双扣，窗口和总deadline跨恢复不重置。VERIFY改变批准内容或依赖元数据须隔离候选、新source回PLAN重批，不能以tidy或impact文本获得隐式派生写权限。未绑定精确计划时权限可null，进入PLAN及执行阶段须完整绑定。三个只读S2观察接口提供真实完整对象，不冒充完整v2 Run。

## 理由

授权、来源、计量和副作用必须具有可独立核对的主体。计数显示不能证明预算消费，临时map或legacy文本不能代替完整引用；模型建议不具备批准和完成权。版本化发布避免新Schema破坏已冻结1.1 Run的读取与恢复。

## 备选方案

放宽DecisionProposal.input_message_refs或补造旧消息会制造来源事实，拒绝。修改1.1源字节会破坏冻结身份，拒绝。把预算放在未发布私有字段、只展示当前累计数不能验证结算幂等，拒绝。扩展自由shell或隐式tidy写范围会超出具体批准，拒绝。

## 影响

新增1.2发布包、ADR登记及按contract_version选择的离线codec；1.0/1.1原字节保留，新旧包分别校验。S2落地新增预算、输入及执行证据持久链；后续S3消息/摘要、S4完整v2/API/前端与S5独立验收继续分别推进，发布不证明它们已实现。

## 关联

- 域：[agent](../02-agent/overview.md)、[orchestrator](../01-orchestrator/overview.md)、[governance](../06-governance/overview.md)、[observability](../07-observability/overview.md)、[conversation](../05-conversation/overview.md)。
- 功能：[model-decision](../02-agent/features/model-decision.md)、[action-validation](../02-agent/features/action-validation.md)、[human-gates](../01-orchestrator/features/human-gates.md)、[state-machine](../01-orchestrator/features/state-machine.md)、[budget-limits](../06-governance/features/budget-limits.md)、[permissions-and-secrets](../06-governance/features/permissions-and-secrets.md)、[verification-evidence](../07-observability/features/verification-evidence.md)、[jsonl-message-store](../05-conversation/features/jsonl-message-store.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：S1退出后核对S2结构缺口，先发布1.2及兼容边界；冻结源文件、历史证据和原批准范围保持原事实。
