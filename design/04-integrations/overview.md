# GitLab 与 CI 集成域

> 实体ID：domain.integrations
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§4.4、§7.3–7.5、§8、§9。
> 审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-004–009、R-019、R-021、R-023。
> 基线日期：2026-10-02。

## 职责

把已确认且本地验证通过的项目工件交付到指定 GitLab 实例、namespace 和仓库，生成可执行的 CI 配置并核验首条匹配流水线。所有外部写操作经过 Activity，登记稳定业务操作身份、准确目标和结果证据；Worker 重试或执行链切换不会成为再次建仓、推送或触发的理由。本域产生外部结果，不决定 Workflow 阶段或最终完成。

## 边界

三项模式独立记录：execution_mode 为 temporal/local，decision_mode 为 model/rules，delivery_mode 为 gitlab/local。远程交付发生在 DELIVER 的 remote_repo、remote_ci 子步骤；delivery_mode=local 以 package 交付，明确没有远程仓库与 CI 事实。execution_mode=local 不阻止明确选择的真实 GitLab 集成，也不能证明 Temporal replay。项目创建确认只授权具体创建、推送及首跑，不授权覆盖他人项目、强制推送、生产合并或发布。集成域不读取模型自然语言作为授权，不把同名仓库、任意远程提交或最近成功流水线当作本次交付证据。未知结果交回编排为 BLOCKED，核实成功后才能恢复。

## 与其他领域的关系

编排提供当前阶段、调度身份和版本绑定批准；技能提供定义、CI 模板及经过确认的文件 manifest；治理提供目标权限、凭据使用、并发与预算约束；可观测性保留操作和验证证据；对话与前端展示仓库、流水线及阻塞原因。依赖在各功能页中限定为公开输入或输出，外部客户端、HTTP 实现和 git 子进程细节不是跨域契约。

## 按改动节奏划域的理由

GitLab 接口、认证方式、Runner 环境和外部错误语义会独立于对话表达与项目模板演化，把这些适配与核实规则集中维护可避免改变 UI 时触及远程写入行为。仓库身份与操作幂等保持稳定；CI 模板随 Go 和工具版本升级；未知结果恢复跟随各接口可获得的证据变化。把三者放在同域便于一致维护身份链，又按功能拆开其独立验收。

## 功能列表

| 功能 | 公共结果 | 变化关注点 |
| --- | --- | --- |
| [GitLab 项目身份与创建](features/gitlab-project.md) | 精确项目身份、所有权与创建结果 | namespace、可见性及 API 适配 |
| [批准工件提交与推送](features/repository-push.md) | manifest 绑定提交及远程确认 | git 传输与分支冲突 |
| [CI 配置与首跑触发](features/ci-config-trigger.md) | 配置版本、提交与 pipeline 绑定 | Runner、报告和固定工具版本 |
| [匹配流水线验证](features/ci-verification.md) | 固定必需检查和流水线证据 | 终态与工件完整性 |
| [外部结果未知核实](features/unknown-results.md) | 原操作核实结论及恢复条件 | 丢响应、超时与部分副作用 |
