# Agent Studio 界面设计

原始图：[workspace-concept.png](workspace-concept.png)。使用内置 Image Gen，设计提示为完整三栏 Chinese conversation-first durable project workspace，pure white main / cool gray rails / violet #7462df。图为布局和视觉基线，真实状态决定文案。

- 主内容纯白 #fff；侧栏 #f7f8fb；标题 #202332；辅助 #858b9d；分割线 #e5e7ef；强调 #7462df。无背景照片或装饰图。
- 1536×1024 原始三栏：左 232~280，主栏自适应，右约 330；主栏最大阅读宽度 780。响应式在 1180 隐藏右栏由按钮打开、800 左栏抽屉，移动端一次只呈现主要任务。
- 字体 Inter/system sans + PingFang SC；标题 22/600、正文 15/1.7、控件 14/500、辅助 12。SVG Lucide 18/1.6，几何立方体作为统一品牌。
- 当前页面架构：Sidebar / Conversation / Composer / ProposalCard / Inspector（Progress、Artifact、History），Workspace.vue组合并处理权威legacy快照与命令结果。提案、文件树、版本和动作由真实 API 数据渲染。
- 当前首屏文案：项目创建 Agent、新建对话、项目任务、运行与产物；会话标题和状态动态；理解需求、方案确认、项目生成、交付验证；产物、历史、当前运行；Enter 发送、Shift + Enter 换行。早期图中的认知模型和设置导航不进入本期产品工作区。
- 有意适配：生成图的轻微按钮渐变改为提示中明确指定的纯色；未创建对话时显示真实空状态；不种入不存在的“创建订单服务”任务；文件树仅在实际生成后显示，预览阶段明确标记计划；运行模式、确认门禁、错误提示随真实业务状态。
- 核心路径：创建会话、自然语言多轮输入、方案预览、约束与版本校验、接受/拒绝/修订、实际生成、验证、工件预览下载、暂停/恢复/重做；设计结构模型独立离线检查。

## 本轮实现适配

2026-10-02迁移前历史：React工作区实现任务列表、对话/方案、产物/历史三栏，数据来自独立项目创建Agent的公开API。设计图中的“认知模型”是早期概念，当前不建立运行认知API，不把文档审阅apply放入项目批准路径；当前design结构模型只用于离线检查。组件按真实Run、提案版本/hash、批准目标及当前阶段呈现，三轴独立显示；完整内部required_checks和manifest由公开服务与工件证明。1440×1000桌面保持白/冷灰/紫三栏，390×844窄屏把侧栏和产物区变为抽屉。真实截图与主线/竞态结果见[浏览器验收](../evidence/BROWSER-QA.md)及[实现状态](../analysis/IMPLEMENTATION-STATUS.md)，图本身不提供完成结论。


## Vue迁移与只读审查实现

2026-10-02按[ADR-047](../../design/10-decisions/ADR-047-vue-readonly-review.md)实施：Vue 3 + TypeScript + Vite等价保留工作区。`App.vue`选择工作区/审查入口，`Workspace.vue`拥有任务选择、请求隔离、SSE与控制状态，`components/Sidebar.vue`、`ProposalCard.vue`、`Inspector.vue`、`MarkdownMessage.vue`负责公开数据呈现，`lib/api.ts`和`types.ts`继续对应legacy接口。前端路径迁移不表示MessageEnvelope/Part或其他v2对象完成。

独立`?review`入口提供目录、画布、详情、来源正文、主题、缩放/移动及同源SVG/PNG导出；十一类为状态、职责、时序、契约、血缘、架构、覆盖、版本场景、知识、Skill依赖、上下文数据流。契约默认展示核心对象，完整Schema和字段通过全对象切换及来源查看；知识支持领域/实体/关系筛选。覆盖使用离散状态，历史证据标出原时点，版本时间线明确为设计场景。

审查运行只读取本项目静态bundle/Schema，入口先用Ajv Draft2020-12及formats校验，再核对来源SHA-256；不连接运行命令、设计apply或参考项目目录。导出保留模型标识、筛选、基线摘要及生成时间。视觉沿用白/冷灰/紫的可读风格，但审查不是早期概念图中的在线治理编辑器。

新Vue迁移浏览器回归15/15已通过；审查与导出浏览器检查仍在收尾，最终证据随本轮最终QA登记。构建/数据检查通过不代替这部分交互和图片验证，也不代替S1–S5目标契约实施。
