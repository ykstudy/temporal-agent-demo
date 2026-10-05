# 界面方向材料

[workspace-concept.png](workspace-concept.png) 与 [DESIGN-SPEC.md](DESIGN-SPEC.md) 是早期生成的视觉方向，帮助实现对话、方案、进度和产物的布局。它们不是当前服务的运行依赖，也不能代替前端运行证据。迁移前React工作区的桌面/手机及竞态结果保留为[历史浏览器证据](../evidence/BROWSER-QA.md)。当前按[ADR-047](../../design/10-decisions/ADR-047-vue-readonly-review.md)已等价迁移Vue，并落地独立只读设计审查页、十一类静态视图及同源SVG/PNG导出。迁移浏览器回归15/15通过；最终审查187/187、发布来源边界15/15及1探针通过，准确时点、截图和导出见[本阶段验收](../evidence/VUE-REVIEW-QA.md)。

当前产品规范来自 [根 design](../../design/README.md)，现有 Agent 公共契约见 [AGENT-GUIDE](../AGENT-GUIDE.md)。设计认知模型只用于结构审阅和离线检查；早期参考副本和实验认知 API 的移除见 [ADR-045](../../design/10-decisions/ADR-045-agent-model-validation-separation.md)。

前端功能与运行证据分开登记：交互目标见 [frontend 功能契约](../../design/08-frontend/overview.md)，真实完成范围见 [实现状态](../analysis/IMPLEMENTATION-STATUS.md)。后续界面以权威 Agent 响应为准，不把文档 apply 当作项目批准或交付完成。


当前审查规范与数据发布见 [design/review](../../design/review/README.md)。工作区继续消费legacy公开API；只读审查消费本项目bundle和Schema发布物，不触发Agent批准/执行，也不把图中的目标对象宣称为已实现v2。


审查页的视觉方向与实现适配见[REVIEW-SPEC](REVIEW-SPEC.md)，仅解释目录/画布/详情及密度筛选，不成为Agent运行规则。
