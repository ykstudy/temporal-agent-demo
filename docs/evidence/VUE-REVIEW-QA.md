# Vue 迁移与只读审查验收

2026-10-02。已完成 ADR-047 授权的 Vue 等价迁移、11 类审查视图、来源追溯及同源 SVG/PNG 导出。产品仍消费 legacy API；S1–S5、目标 v2 C01–C18、live 模型/GitLab Runner 和生产发布未因本阶段通过而变成已实现。

## 当前结果与时点

最终生产构建预检时间 UTC 09:40:59，最终审查记录 UTC 09:40:59–09:44；Vue 3.5.43、Vite 7.3.6、TypeScript 5.9.3，`vue-tsc --noEmit && vite build` 通过。工作区入口约 201.83 kB，审查组件按路由加载约 322.53 kB；[预检与构建资产摘要](vue-review/preflight.json)记录实际文件。浏览器采用现有 Playwright 与已安装 Chrome；Browser 插件未提供。根任务另用 Codex 内置浏览器打开页面、查看截图及 Schema 原文，并确认关闭后焦点回原来源按钮。

| 验证分类 | 实际结果 | 证据 |
| --- | --- | --- |
| Vue 创建路径 | 主线 10/10，竞态 5/5；UTC 09:12 生产构建，local/rules/local | [主线](vue-review/migration/browser-summary.json)、[竞态](vue-review/migration/browser-races.json) |
| 最终只读审查 | 187/187；11 路由、普通指针点选、来源/筛选/主题/缩放、手机/返回、手势、真实下载 | [审查结果](vue-review/review/review-summary.json) |
| 最终发布来源边界 | 15/15 主用例（1 控制、14 拒绝），额外 Schema-only 探针 1/1；无 /api 请求 | [边界结果](vue-review/source-bound/source-bound-summary.json) |
| 发布数据 | 11 模型 / 128 来源；12 数据负例 + 2 摘要负例拒绝，离线 Draft 2020-12 / format / 来源与语义检查通过 | [发布检查](vue-review/bundle-check.json)、[完整数据报告](vue-review-data-check.json) |
| 文档认知模型 | 137 实体 / 529 关系 / 47 ADR；7 self-test + 4 guard 通过 | [模型报告](vue-review-design-check.json) |
| 迁移保护 | 21 个记录的后端文件、2 份冻结规范及原 API/types/style 三份文件摘要均未变 | [26 份文件逐字节核对](vue-review/preservation.json) |
| 长文本呈现 | 状态/泳道显示有界，选中仍保留完整来源；4/4 定向断言 | [文字布局](vue-review/text-layout/summary.json) |

创建路径未在最后只读复验中重复生成；其报告保留自己的代码/时点。最后修改仅审查发布校验、图排版和手势，最终复验另确认返回工作区及异步加载。不同分类不合成“全部功能完成率”。[最终索引](vue-review/qa-index.json)记录原始报告、脚本、资产摘要和证明边界；原始报告保留运行时临时路径用于来源追溯，稳定副本在本目录。

最终发布 bundle SHA-256：`dd674b3239c1c6c9a3d8949a0c72905715605b313cc37256d9c9ba7417de3ab6`；Schema：`aedd4f32c50d9827cae27962e964a32f265e500ba702a54ec2d98a144921ca81`，生成 UTC 09:23:49.476116。浏览器先核对编译摘要，再验证编译 Schema、冻结基线、来源原文字节与绑定；删覆盖格、篡改终态边、重签来源或放松网络 Schema 都拒绝显示。

## 11 类视图

状态与门禁、职责与权限、交互时序、契约对象、来源与证据、模块边界、验收覆盖、身份与版本、文档关系、Skill 能力、上下文与摘要均有独立 `?review=模型ID` 链接和专用原生 SVG 渲染器。类型、语义及来源见 [MODELS](../../design/review/MODELS.md)。参考组件复制与本地适配见 [PROVENANCE](../../frontend/src/review/diagrams/PROVENANCE.json)；没有参考项目的构建/运行路径依赖。

默认契约图呈现 8 个核心对象和 7 条字段绑定，全部开关保留 108 个 Schema 对象与 397 条关系（379 引用、18 字段绑定）。文档图保留 137 个登记实体、529 条关系，用领域/类型/关系与搜索收敛；验收矩阵 21 行 × 5 类，18 个 v2 场景仍 planned。身份时间线是设计场景，非真实 Run 历史。模块图保留 12 节点，三条边界为产品、离线设计工具与独立调用器。

## 概念与实际截图比对

依据 [概念图](../design/review-workbench-concept.png)和 [实施规范](../design/REVIEW-SPEC.md)逐项检查；实际用 [1440×1000 契约页](vue-review/review/review-contracts-desktop.png)、[模块页](vue-review/review/review-architecture-desktop.png)、[泳道页](vue-review/review/review-responsibility-desktop.png)、[状态页](vue-review/review/review-state-desktop.png)、[手机页](vue-review/review/review-mobile.png)与其余逐图截图核对。

| 维度 | 验证与允许差异 | 结论 |
| --- | --- | --- |
| 配色与层级 | 冷灰外壳/白画布/海军蓝文字/靛蓝强调；主题切换实测颜色变化且刷新保留 | 通过 |
| 组织与密度 | 头部、11 项目录、完整 SVG、来源栏、底部工具；默认核心链及知识领域筛选，全部数据保留 | 通过 |
| 文案与契约 | 实际标题来自模型；字段按发布 Schema，概念 approved_scope/verified_version/artifact_version 不进入对象；箭头按真实引用方向 | 有意适配，已登记 |
| 节点/关系操作 | 7 核心边均由稳定ID普通点选，标签先避节点/其他标签；移动标签带虚线引线；完整说明和来源可读 | 通过 |
| 图文字与边界 | 状态/泳道长文字换行或缩略且完整绑定保留；模块三列；知识全量横向边界包括全部圆；大图用放大/平移阅读 | 通过 |
| 移动与键盘 | 390×844 无页面横溢出；目录、说明关闭、返回均可达；来源焦点限定/恢复；画布外松开后不继续平移 | 通过 |
| 导出 | 原生完整 viewBox，样式内联；来源/baseline/版本/时点/当前筛选 metadata；SVG/PNG 实际下载并解码 | 通过 |

概念用于视觉方向；正式规范的对象、关系和状态不会为贴合示意图而变造。全量大图初始适应提供概览，细节需筛选或缩放；这不是把每个字段塞入初始视口的验收。

![实际契约审查页面](vue-review/review/review-contracts-desktop.png)

## 导出文件

- [核心字段链 SVG](vue-review/review/exports/contracts-core.svg)与[PNG](vue-review/review/exports/contracts-core.png)：8 节点、7 关系，PNG 2480×1716。
- [完整 Schema 图 SVG](vue-review/review/exports/contracts.svg)与[PNG](vue-review/review/exports/contracts.png)：108 节点、397 实际关系路径、351 标签引线；PNG 782×8192 保持完整比例，详细放大推荐 SVG。
- [搜索后的文档关系 SVG](vue-review/review/exports/knowledge-searched.svg)：metadata 记录非空搜索文本及当前筛选，导出与实际子集一致。

PNG 长边上限 8192，全量长图按比例缩放；没有用屏幕截图截掉画布来制造完整导出。PNG 的 AgentStudio 文本 metadata 与 SVG metadata 对应，实际 PNG 解码与非空像素、图形范围和关系路径数均通过。

## 复现与范围

先按 [审查发布说明](../../design/review/README.md)生成数据和清单，`npm run check`、`npm run build`；浏览器命令见 [调用器说明](../../validation/browser/README.md)。最终验证在 localhost UI15182、API18111，以隔离验收数据和 local/rules/local 运行；测试结束已关闭生产 preview15182，开发审查15181/API18111保留给当前用户查看。

没有验证 live 模型、真实 GitLab Runner、容器/生产、长期持久卷或物理手机；没有据此实施或关闭 v2 S1–S5。最终 Console 错误、审查运行 API 写入及来源负例 /api 请求均为空。失败诊断或未执行项不计入上表通过数，私有 launcher/controller 数据未归档。
