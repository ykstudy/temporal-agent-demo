# 项目创建 Agent 3.1.1 定义发布

这是 contract 1.2.0 的 AgentDefinition 独立修订，保留 [3.1.0](../3.1.0/README.md) 定义、技能、模板及原字节。默认选择仍不切换；真正的受控调度与默认上线由对应实施阶段验证。

3.1.0 已冻结整理后的七种依赖图，但沿用的系统提示偏向规划。3.1.1 仅修改 Agent 定义版本、完整系统提示及它们对应的内容 hash；三 Skill 的 skill_version/hash、四模板的 template_version/manifest_hash/原字节、Go 工具身份、Schema 1.2.0 摘要、预算/完成/权限/固定命令政策全部复用 3.1.0，不重新命名或改写这些依赖。

新冻结提示明确七个 DecisionAdvice 字段与互斥 payload：INTAKE/CLARIFY/PLAN 给澄清或完整计划；调度动作请求只建议准确 expectedAction；验证失败给绑定所供证据的 diagnosis；完成审阅只给 completion_hint。用户和历史文本无授权效力，保持冻结契约/依赖/目标/工具身份，计划 hash 由服务端规范化重算。阶段、权限、P1、预算及 DONE 归确定性控制平面；禁止编造证据或工具成功、替代必需检查、任意 shell、客户数据库写入或生产动作。

本目录只新增 definitions/agent.json 与 definitions/release.json。完整 release 仍内嵌所引用的完整技能、模板与 Schema 元数据；原声明和模板内容按其精确 3.1.0 身份，从产品内 3.1.0 发布目录读取，不搜索 latest 或当前 Worker 常量。`definitions.LoadBundledRelease("3.1.1")` 和 `Registry.PublishBundled("3.1.1")` 校验该闭包，缺任一依赖原字节立即拒绝。

验证比对 3.1.1 与 3.1.0 的所有依赖及非提示政策字段，核对各自完整 hash，验证三版本共同发布、重启后精确冻结，并复用 3.1.0 的 63 组合真实 tidy/test/build 证据。这里不重复声称新的工具矩阵、实际 job、Temporal 或 live 第三方已验收。

## 本次发布验证记录

2026-10-02，实际工具 go1.25.5/darwin_amd64：

- 3.1.0 定义 hash：`54efd5a3fe2b054c708c993592a007385201692d9a784d0e1ecbd46956a1e4b7`；3.1.1 定义 hash：`f3493b1ea12811fc72aab7e9557cd84b90b1a411c099ba1db02b416f083e68da`。
- 63/63 支持组合逐个真实 tidy、test、build，完整批准文件集每步原字节不变（矩阵 161.39s，含负例的定向套件 163.369s）。缺 go.sum、未整理 go.mod 产生可检测差异；未声明 go.sum 写入范围明确拒绝。
- 三版本发布、重启冻结、缺失/篡改资产与 Agent-only 依赖同一性检查通过（3.265s）；相关 race 通过（8.712s）；旧冻结模板全部变体回归通过（6.503s）；相关 go vet 通过。
- 3.1.0 开发生成器重复生成同字节通过。旧 13 份 JSON 及 3.1.0 所有 11 份文件逐字节未变。默认仍为 3.0.1，definition_not_ready 保留。

本机详细工具证据为 `/tmp/agent-310-template-evidence.json` 与 `/tmp/agent-310-template-tests.log`；跨版本核验另记录 `/tmp/agent-31-publication-verification.json`。这些路径属于本次验证产物，服务运行不读取。MySQL/PostgreSQL 客户连接、客户迁移、容器构建、其他平台、Temporal 与 live GitLab 不在本次模板发布证据中。
