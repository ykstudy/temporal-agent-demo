# ADR-022 CI 配置与触发身份去重

> 实体ID：adr.022
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§7.4–7.5、§8.3；L1010–1073、L1093–1112、L1183–1200。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-006、R-007、R-008、R-009。

## 状态

accepted。生效日期为 2026-10-02；这是本次有效设计基线，不将来源示例、已有代码或测试结果追认为过去的批准。

## 背景

原文允许重复产生流水线并只检查最新结果，CI 模板还声明未真实生成的 bin 和覆盖率 XML。最新成功可能来自别的提交，YAML 合法也不能证明命令、工具和报告可执行。

## 决策

CI 配置与批准 manifest 一同提交，固定工具及输出路径；流水线绑定 project_id、ref、确认 SHA 和 pipeline ID。优先复用相同身份且已核实的自动流水线，必要触发使用稳定操作账本；响应丢失先核实，不重复创建第二条。

## 理由

版本绑定避免并发提交污染完成判断，身份去重让重连和重试不会平白增加 CI 成本。真实报告和固定 required_checks 可以使缺输入、缺工件和未终态明确阻止 DONE。

## 备选方案

只查最新 pipeline 简单但混淆来源；每次都主动触发可能重复自动流水线；安装 latest 工具无法复现。覆盖率 XML 只有真实转换且验证后才可声明，否则仅登记原始报告。

## 影响

验收固定 runner 执行配置、显式构建产物、报告和工具版本，并覆盖触发响应丢失、自动触发复用、SHA/ref 不匹配及缺 pipeline ID。所有证据关联本次提交，未配置 runner 或真实 CI 不声明验证通过。

## 关联

- [GitLab 与 CI 域](../04-integrations/overview.md)。
- [CI 配置与触发](../04-integrations/features/ci-config-trigger.md)。
- [CI 首跑核验](../04-integrations/features/ci-verification.md)。
- [INV-015 结果未知先核实](../00-invariants/INVARIANTS.md#inv-015-结果未知先核实)。
- [INV-017 验证绑定来源版本](../00-invariants/INVARIANTS.md#inv-017-验证绑定来源版本)。
- [INV-018 完成需要固定检查集合](../00-invariants/INVARIANTS.md#inv-018-完成需要固定检查集合)。

## 历史

2026-10-02：依据保留的原文章节和本次审校建立此决策。没有可证实的更早接受记录；未来替换须新增 ADR、说明旧新差异并更新关联，不改写本条历史。
