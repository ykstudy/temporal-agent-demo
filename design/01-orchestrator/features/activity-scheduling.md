# 功能：Activity 调度、重试与心跳

> 实体ID：feature.orchestrator.activity-scheduling
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§3、§4.3、§5.2、§8、§10.1；L139、L181–235、L354–365、L1153–1200、L1239–1241。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-001、R-006、R-009、R-020、R-021。
> 基线日期：2026-10-02。

## 目标

把已获准的业务步骤交给 Activity 执行，区分尝试失败、业务失败与结果未知，并在时限内可取消、可观察、可恢复。保持 Workflow 纯控制，外部副作用由受约束执行器承担。

## 输入

合法动作、稳定 operation_id、批准来源版本、工作区/目标引用、工具授权与预算；Activity 输入输出 Schema、单次/总超时、heartbeat 及有限重试策略。

## 输出

ActivityInvocation、尝试/心跳记录、结构化结果与工件引用；成功、永久失败、可重试读取、超时、取消、部分完成和结果待核实分别输出。结果不等于 Run 已完成或权限已扩大。

## 算法 / 流程

1. 核对动作、授权、参数与工作区，拒绝非法任务后才调度；大对象使用版本化引用。
2. 设置 StartToClose 或 ScheduleToClose 并定义单次/总等待，长步骤发送心跳，传递取消信号与预算停止。
3. 执行模板、验证或集成操作；只把安全读取等基础设施失败交有限退避重试，保存原调用身份。
4. 永久输入/权限失败立即反馈；业务失败交诊断/修订或终止；未知写入先交核实契约，核实前不重复执行。

## 边界与异常

取消、超时与 Worker 消失不能自动证明回滚。重试耗尽有确定出口，不能无限调用模型或工具。已完成步骤按绑定 manifest 核实后复用，不能以单个 marker 声称成功；部分产物与已发生副作用必须披露。Activity 不得将执行器密钥写入返回值或历史。

## 依赖

- [动作允许性](../../02-agent/features/action-validation.md#输出)：消费公开输出契约。
- [工具和秘密边界](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [可执行预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。
- [写操作核实结果](../../04-integrations/features/unknown-results.md#输出)：消费公开输出契约。
- [必需检查要求及证据](../../07-observability/features/verification-evidence.md#输出)：消费公开输出契约。

## 被谁依赖


- [Harness 服务运行 · 输出](../../09-deploy/features/service-runtime.md#输出)：向本功能的公开输入提交请求。
- [失败归因与恢复提示 · 输入](../../07-observability/features/failure-attribution.md#输入)：消费本功能的公开输出契约。
- [工作流可观测性 · 输入](../../07-observability/features/workflow-observability.md#输入)：消费本功能的公开输出契约。
- [状态机 · 输入](state-machine.md#输入)：消费本功能的公开输出契约。

## 验收标准

注入可重试读取、权限失败、业务验证失败、取消、心跳失联和未知写入，断言各类别不同出口。杀 Worker 后按相同业务操作恢复，成功步骤不重复；重试次数与时限有上界，任何 Activity 成功都不能单独使 Run 完成。

