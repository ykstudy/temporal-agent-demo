# 功能：CI 配置与首跑触发

> 实体ID：feature.integrations.ci-config-trigger
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.1、§7.4、§8.3。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-004、R-006–008、R-011、R-020。
> 基线日期：2026-10-02。

## 目标

提供与批准项目版本一致、可实际执行的 GitLab CI 配置，并只启动或复用对应提交的首跑。CI 的文件、工具版本、构建路径和报告声明必须与真实命令产物一致，不能以 YAML 合法代替 Runner 验收。

## 输入

冻结 AgentDefinition、CI/项目模板版本、语言与 Skill 配置、批准 manifest、目标 project_id/ref、确认提交 SHA；自动生成流水线信息、稳定触发 operation_id、触发权限和预算。是否启用远程 CI 是方案范围，不由模型临时改变。

## 输出

版本化 `.gitlab-ci.yml` 工件及 hash、固定工具/镜像版本、构建与报告清单；远程首跑输出 pipeline_id、project_id、ref、commit SHA、复用或触发来源及结果状态。生成配置与已启动流水线是不同输出事实。

## 算法 / 流程

1. 在方案预览中登记 CI 模板与固定工具版本；IMPLEMENT 渲染 CI 文件，使其参与原批准工件包和本地检查。
2. 构建指定可执行包并明确输出路径；覆盖率登记真实 coverage.out，只有真实转换后才声明 Cobertura；安全工具禁止使用漂移的 latest。
3. 在 DELIVER/remote_ci 消费确认推送的 SHA，核对该提交中的配置 hash；先查由本次提交自动产生的匹配流水线。
4. 匹配的既有首跑按操作记录复用；明确没有可复用首跑时，在批准预算内触发并登记返回的 pipeline_id 和 SHA/ref。
5. 触发响应丢失先登记未知并核实原操作。不得因为查询晚到或没有“最新”结果就重复触发。

## 边界与异常

触发 API 不接受期望 SHA 时，仍须核实实际 pipeline SHA；ref 在触发期间变化导致不匹配，该流水线不能验收本次产物。Runner 缺失、配置错误和令牌不足明确失败或 BLOCKED。重新生成配置属于方案修订，不在远程交付中静默增加未批准提交。delivery_mode=local 只展示本地配置文件；execution_mode=local 若选择 GitLab 交付仍须真实触发及核验。

## 依赖

- [冻结定义](../../02-agent/features/definition-versioning.md#输出)：消费公开输出契约。
- [模板与工件版本](../../03-skills/features/template-versioning.md#输出)：消费公开输出契约。
- [确认远程提交](repository-push.md#输出)：消费公开输出契约。
- [触发授权](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [触发预算](../../06-governance/features/budget-limits.md#输出)：消费公开输出契约。
- [原触发核实](unknown-results.md#输出)：消费公开输出契约。

## 被谁依赖


- [匹配流水线验证 · 输入](ci-verification.md#输入)：消费本功能的公开输出契约。

## 验收标准

真实 Runner 能生成所登记的构建与覆盖率工件；工具版本可复现。自动流水线可复用，触发重试不增加第二条首跑；丢响应或 ref 改变不误报成功。输出必须包含准确 pipeline ID/SHA/ref，不能用项目最近流水线替代。
