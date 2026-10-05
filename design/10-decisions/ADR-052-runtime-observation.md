# ADR-052 运行观察、实际就绪与持久审计

> 实体ID：adr.ADR-052
> 来源：2026-10-05 用户要求依序完成创建Agent及实际Job的全部阶段。

## 状态

accepted（2026-10-05），先发布运行协议再实施。

## 背景

既有1.4业务快照不能临时加入审计、活动和health字段。固定健康200及缓存client不能证明依赖就绪；历史记录没有实际Activity尝试时不能从文本重建。

## 决策

发布独立[运行协议1.0.0](../contracts/runtime/1.0.0/README.md)，只读引用既有业务对象。新Run事务追加关联事件，实际调度记录Activity尝试，指标从持久身份去重。Workflow Query与心跳各自带来源及时间。readiness检查实际存储、Temporal与自有Worker。恢复先保全原身份和副作用，再由控制面判定合法阶段；单写者停止边界内验证完整副本。

## 理由

独立版本保留冻结业务Schema和旧来源；完整采集范围与不可用标记防止把推断当事实。探针和观察只读业务，不能触发模型或交付。

## 备选方案

不采用修改1.4闭合对象、把日志转换成历史真实尝试、把心跳当完成或备份运行中不一致目录。

## 影响

新增派生完整Go/TS/外部Schema、审计事务及运行观察接口，运行工作区展示真实诊断。既有三轴、权限、预算和完成规则不变；不扩展生产部署或多写者。

## 关联

- 领域：[orchestrator](../01-orchestrator/overview.md)、[observability](../07-observability/overview.md)、[deploy](../09-deploy/overview.md)。
- 功能：[audit-traceability](../07-observability/features/audit-traceability.md)、[workflow-observability](../07-observability/features/workflow-observability.md)、[failure-attribution](../07-observability/features/failure-attribution.md)、[health-recovery](../09-deploy/features/health-recovery.md)、[service-runtime](../09-deploy/features/service-runtime.md)。

## 历史

- 2026-10-05：冻结首版完整观察和恢复边界。
