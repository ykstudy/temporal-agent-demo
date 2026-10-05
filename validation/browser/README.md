# 浏览器调用验证

此目录是可选的外部 UI 调用器，Agent 不读取或导入它。需 Node.js、Playwright 和已安装的 Chrome；前端与 Agent 分别启动，Vite 的 `/api` 代理必须指向同一 Agent。所有业务操作仅通过界面和公开 HTTP。

```sh
PLAYWRIGHT_MODULE=/绝对路径/playwright/index.mjs AGENT_API_URL=http://127.0.0.1:18091 AGENT_BROWSER_URL=http://127.0.0.1:15173 node validation/browser/smoke.mjs
PLAYWRIGHT_MODULE=/绝对路径/playwright/index.mjs AGENT_API_URL=http://127.0.0.1:18091 AGENT_BROWSER_URL=http://127.0.0.1:15173 node validation/browser/races.mjs
```

如果环境可直接导入 `playwright`，省略 `PLAYWRIGHT_MODULE`。`AGENT_BROWSER_REPORT_DIR` 可指定报告目录，默认 `validation/reports/browser`。这是一个测试调用方，会创建带浏览器测试身份的真实 Run 和本地产物；使用独立验证数据目录。

smoke 验证多轮需求、真实方案/文件预览、确认生成、产物读取、ZIP 下载、历史、1440×1000 桌面与390×844手机可达性、控制台与横向溢出。races 延迟真实 HTTP 回执验证切换Run/切换文件的隔离，并模拟客户端时钟检查审批到期提示；它不宣称模拟时钟证明服务端到期，服务端真实到期由 Python 场景独立验证。

当前实测证据另归档在 `docs/evidence/` 与实现状态册。首次工具默认 Playwright 浏览器缺失，实际采用已安装 Chrome，没有安装浏览器，也没有验证 Docker 或 live 第三方。


只读审查页面与发布来源边界使用独立调用器：

```sh
AGENT_BROWSER_URL=http://127.0.0.1:5173 AGENT_BROWSER_REPORT_DIR=/tmp/agent-review-ui node validation/browser/review.mjs
AGENT_BROWSER_URL=http://127.0.0.1:5173 AGENT_BROWSER_REPORT_DIR=/tmp/agent-review-sources node validation/browser/source-bound.mjs
```

同样可传 `PLAYWRIGHT_MODULE` 指向已有 Playwright。review 覆盖 11 个静态路由、稳定ID点选、真实来源原文、搜索/筛选、主题、缩放、手机目录/说明/返回、错误路由及完整 SVG/PNG 下载和 metadata；source-bound 只在浏览器内拦截公开静态响应注入坏数据，验证发布摘要与来源拒绝，不改仓库发布物、不发 /api 命令。审查计数不能关闭 v2 C01–C18 缺口；最终报告见 docs/evidence/VUE-REVIEW-QA.md。
