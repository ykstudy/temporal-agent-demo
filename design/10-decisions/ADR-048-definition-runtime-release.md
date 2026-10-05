# ADR-048 完整冻结定义的内容引用与运行预算发布

> 实体ID：adr.ADR-048
> 来源：2026-10-02 用户授权按提示词1–8分阶段实施；S1结构与运行链核对。

## 状态

accepted（2026-10-02）。先发布contract 1.1.0再实施S1；不修订冻结PRODUCT-GUIDE或INVARIANTS。

## 背景

S0的TemplateFile引用与template清单hash语义不清，会导致清单包含自身内容引用的循环。BudgetPolicy只有五类计数，未完整表达现有效功能要求的工具期限、业务总时限和未知结果核实额度。现有legacy接口也没有读取完整冻结包的公开路径，独立S1验收无法只凭私有类型成立。

## 决策

按[1.1.0发布说明](../contracts/releases/1.1.0/README.md)保留旧发布，新增完整Schema包。新增template_content原字节对象引用，template继续表示清单；定义依赖先登记内容再按无循环顺序绑定。BudgetPolicy新增单次/累计期限和读取配额，先冻结再由受控执行器落实。发布S1只读定义/内容观察接口和legacy创建的精确定义选择，不提前声称完整v2 Run已提供。

## 理由

冻结必须包含真实可读取的内容，hash身份需要唯一主体；同版内容不可被“修复”覆盖。预算同样必须是Run创建时可核对的政策。公开完整对象允许外部oracle独立检验，保留产品与场景调用器边界。

## 备选方案

直接用文件sha冒充模板manifest hash会改变既有身份语义，拒绝。只存源码hash而执行时读取当前模板无法保证升级后不漂移，拒绝。只补DTO而不接运行调用链不能满足S1。采用版本化内容存储、纯渲染程序及明确发布迁移。

## 影响

新增1.1.0发布源、派生Go/外部Schema和完整定义注册表；旧1.0.0与旧legacy来源保持原事实。规则和模板版本变更只能影响明确选择的新Run。S1保存预算不表示S2执行预算/权限或S4完整v2已完成，分别留验收证据。

## 关联

- 域：[agent](../02-agent/overview.md)、[skills](../03-skills/overview.md)、[governance](../06-governance/overview.md)。
- 功能：[definition-versioning](../02-agent/features/definition-versioning.md)、[skill-contract](../03-skills/features/skill-contract.md)、[template-versioning](../03-skills/features/template-versioning.md)、[budget-limits](../06-governance/features/budget-limits.md)。
- 规则基线：[有效不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：按用户阶段实施授权核对S0遗漏，发布1.1.0；原1.0.0和历史证据保留，不提升实现完成状态。
