# ADR-047 Vue 等价迁移与独立只读可视化审查

> 实体ID：adr.ADR-047
> 来源：2026-10-02 用户“开始推进所有项”，包括 Vue 迁移与可视化审查；[有效规范](../README.md)。
> 审校：[审查模型规范](../review/MODELS.md)。

## 状态

accepted（2026-10-02）。用户已授权实施前端等价迁移和全部十一类审查视图。先发布本决定、模型语义与数据 Schema，再推进实现和验证。不修订冻结规则，不将本轮等价迁移宣布为 S1–S5 全部完成。

## 背景

现有前端工作区规模较小，React 对话与控制功能已有路径证据；增加设计审查需要图目录、画布、详情、来源和可复核导出。参考 process-diagram 的 Vue/SVG 组件设计可降低重复工作，但审查内容必须来自本项目有效规范，不能直接搬示例业务或依赖另一项目的绝对路径。

## 决策

统一采用 Vue 3、TypeScript、Vite。等价保留现有任务、消息、具体提案确认、文件预览、控制、SSE、下载和窄屏体验；暂停/恢复、批准有效期、请求隔离及幂等身份继续消费权威服务响应。

增加独立只读设计审查入口，发布 [十一类模型](../review/MODELS.md) 与 [ReviewBundle Schema](../review/review-bundle.schema.json)。离线生成器从本项目文档、公共 Schema、登记关系、差异与历史证据派生静态 bundle。运行前端只读该发布物，不触发 checker，不读其他项目目录，不参与 Run 批准或完成判定。

审查采用目录、画布、可收起详情与来源；每个元素携带稳定 ID、来源 ID 和摘要。按 design/current/evidence 区分范围，目标、现状与证据不能混为绿色完成。SVG/PNG 从同一前端数据与渲染结果导出，保留基线摘要、生成时间与模型标识，不用 AI 生成技术图替代结构真值。

公共 v2 与 legacy 继续按 ADR-046 分离。Vue 迁移不得凭 UI 改写目标 Schema、宣称历史资料可信、自动将 v2 写请求降级到 legacy。审查不提供设计 apply、运行批准、Shell、生产部署或客户数据库迁移入口；原功能文档描述的设计变更能力仍为后续目标。

## 理由

Vue 可以直接复用本项目内固定版本的 SVG 图组件；等价迁移避免同时改变产品协议。静态来源包支持独立部署、离线复核和同源图片导出。交互页便于查看高密度关系与证据，图片便于归档，两者共享真值。

## 备选方案

仅生成 PNG：无法筛选或追溯高密度关系，拒绝。保留 React 并嵌入 Vue 子应用：增加双运行时和维护边界，拒绝。复制外部样例为产品状态：来源与证据不真实，拒绝。将只读审查直接扩为治理/设计应用后端：超出本阶段验收，拒绝。

## 影响

前端构建与测试依赖迁移；既有路径需要新浏览器证据，不能继承旧通过。新增离线生成、Schema/语义检查与来源摘要比对。历史 20/20、15/15 保留其时点和 legacy 标签；C01–C18 仍依契约阶段执行。文档模型继续只有四种实体类型，审查视图不是新的运行实体。

## 关联

- 域：[frontend](../08-frontend/overview.md)、[observability](../07-observability/overview.md)、[deploy](../09-deploy/overview.md)。
- 功能：[model-review](../08-frontend/features/model-review.md)、[conversation-workspace](../08-frontend/features/conversation-workspace.md)、[state-stream-sync](../08-frontend/features/state-stream-sync.md)、[progress-artifact-panels](../08-frontend/features/progress-artifact-panels.md)、[accessibility](../08-frontend/features/accessibility.md)、[verification-evidence](../07-observability/features/verification-evidence.md)、[reproducible-delivery](../09-deploy/features/reproducible-delivery.md)。
- 规则：[冻结不变量](../00-invariants/INVARIANTS.md)。

## 历史

- 2026-10-02：先发布审查数据规范，再按授权实施 Vue 等价迁移、十一视图与同源导出。
- 本决定补充 ADR-045/046，不修改旧规范来源、不增加数据库主外键含义、不把知识图关系转成运行调度。
