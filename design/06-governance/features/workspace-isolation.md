# 功能：工作区与工件隔离

> 实体ID：feature.governance.workspace-isolation
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.1–4.3、§7.1–7.3、§9.2。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-003、R-011、R-017–020。
> 基线日期：2026-10-02。

## 目标

把每个 Run 的生成、Skill 修改、命令和文件浏览限制在明确根目录与批准路径集合，形成可原子登记的工件包。隔离保护当前宿主代码与其他任务，不让模板参数、符号链接或并发操作跨越业务范围。

## 输入

稳定 Run/operation 身份、批准 source_version、可信 workspace 根、模板/Skill 版本和参数 hash、计划写路径、文件内容/类型；固定命令 cwd、文件读写或 package 请求，以及已有 manifest 和工作区所有者记录。

## 输出

Run 独占 WorkspaceRef、规范化允许路径、文件访问检查、隔离 staging 及原子登记 manifest；manifest 记录文件 hash、大小、模板/配置来源和操作身份。越界、并发持有、内容不符和部分提交返回明确结果。

## 算法 / 流程

1. 由可信配置建立 Run 独立根与操作 staging，不根据项目名任意拼宿主路径；检查归属和独占写权。
2. 对每个模板/请求路径规范化，拒绝绝对路径、`..`、非法编码和非授权符号链接，实际文件解析仍不得逃出根目录。
3. 在 staging 渲染和运行白名单命令，检查所有写入和进程结果；命令只使用过滤环境及批准 cwd，不对宿主项目执行；数据库只在隔离测试中验证模板，不接入或迁移客户真实数据库。
4. 对完整文件集合计算 hash，校验批准版本、参数与秘密边界；全部通过后提交文件包与 manifest，失败不写成功 marker。
5. 普通阶段进度只改 state_sequence，不改变 manifest 的批准 source_version；恢复时核验实际文件，不仅检查标记存在；读文件、推送及打包再次验证归属和完整性。

## 边界与异常

符号链接在校验后被替换也要由实际打开边界拦截，不能只做字符串前缀判断。多文件更新说明 staging 提交边界与崩溃修复，不声称任意文件系统都支持跨文件事务。已写部分文件保留失败证据，不自动删除用户目录；暂停、取消和清理分别授权。摘要文件和持久消息有受限存储根，不能混入项目 ZIP。

## 依赖

- [固定写路径与工具声明](../../03-skills/features/skill-contract.md#输出)：消费公开输出契约。
- [冻结模板及配置 hash](../../03-skills/features/template-versioning.md#输出)：消费公开输出契约。
- [操作独占及源版本](idempotency-and-concurrency.md#输出)：消费公开输出契约。
- [环境与秘密边界](permissions-and-secrets.md#输出)：消费公开输出契约。

## 被谁依赖


- [真实工件浏览 · 输入](../../05-conversation/features/artifact-browsing.md#输入)：消费本功能的公开输出契约。
- [JSONL 原始消息存储 · 输入](../../05-conversation/features/jsonl-message-store.md#输入)：消费本功能的公开输出契约。
- [Harness 服务运行 · 输入](../../09-deploy/features/service-runtime.md#输入)：消费本功能的公开输出契约。
- [批准工件提交与推送 · 输入](../../04-integrations/features/repository-push.md#输入)：消费本功能的公开输出契约。
- [数据库技能 · 输入](../../03-skills/features/database-skill.md#输入)：消费本功能的公开输出契约。
- [日志技能 · 输入](../../03-skills/features/logging-skill.md#输入)：消费本功能的公开输出契约。
- [项目骨架生成 · 输入](../../03-skills/features/scaffold-generation.md#输入)：消费本功能的公开输出契约。
- [模板版本与快照 · 输入](../../03-skills/features/template-versioning.md#输入)：消费本功能的公开输出契约。

## 验收标准

相邻 Run 写入、绝对路径、遍历、编码绕过与符号链接逃逸均拒绝。磁盘满、命令失败及提交期间崩溃不会登记完整成功；恢复以文件 hash 核实。参数改变不能复用旧 marker；同批 manifest 只含批准文件，宿主原项目和其他 Run 的 hash 保持不变。
