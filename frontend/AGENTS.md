# Vue 前端边界

先遵守根 AGENTS.md、ADR-047、design/review 及 docs/design/REVIEW-SPEC.md。Workspace.vue 和 components/*.vue 承载现有 legacy 创建流程；lib/api.ts、types.ts 的公开格式保持现有合同，不能以迁移宣称 v2 完成。

review/ 为独立只读文档审查。数据由 scripts/build-review-data.py 离线生成；publication.generated.json 是已验证 bundle/Schema 原字节摘要清单，不手写、不为迁就错误响应更新。修改来源后重新发布，再重新构建。运行时不得读其他项目目录、运行 checker、访问验证器内部结果或提交批准。

图内可以换行、缩略和筛选，但来源对象、字段、稳定ID及完整绑定保留；导出来源是同一原生 SVG，范围不随视口变换裁剪。避免长标签溢出、路径抢占标签点击和窄屏抽屉遮挡。复制组件的本地调整写入 diagrams/PROVENANCE.json，不复制参考示例为真实业务数据。

产品调用、文档检查和外部浏览器验收独立。检查用 npm run check/build；浏览器脚本置于 ../validation/browser，只消费公开页面/API/下载；按根约定记录具体范围、时点和未执行项。
