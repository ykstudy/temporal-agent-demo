# 项目创建工作区浏览器验收

日期：2026-10-02。当前React工作区通过公开Agent API呈现对话、方案、实际生成、产物和历史。实测执行/决策/交付为local/rules/local；它不提供运行认知模型或生产发布界面。报告分别为[主线10项](browser-summary.json)与[定向5项](browser-races.json)，均passed且无记录的控制台错误。

## 环境与实际命令

Browser plugin不可用，默认Playwright浏览器未安装；实际通过Playwright使用已安装Chrome，未安装新的浏览器。桌面viewport为1440×1000，手机窄屏viewport为390×844；后者是浏览器视口验证，不是物理手机认证。Agent与Vite分别启动，数据根独立：

```sh
python3 validation/start.py --execution-mode local --decision-mode rules --delivery-mode local --port 18091 --data-dir .data/browser-final --approval-ttl 30m
AGENT_API_URL=http://127.0.0.1:18091 npm --prefix frontend run dev -- --port 15173
node /tmp/project-agent-browser.mjs
node /tmp/project-agent-browser-races.mjs
```

最后两条为首轮实际临时脚本命令。可复用版保存在[独立浏览器调用器](../../validation/browser/README.md)，入口为[smoke](../../validation/browser/smoke.mjs)与[races](../../validation/browser/races.mjs)；它们只调用界面与公开HTTP，不是产品依赖。

最终已于新binary再次运行保存版smoke 10/10与races 5/5，最终主线报告recorded_at为2026-10-02T02:07:13Z，业务Run为`e82104d1-778e-4bd0-93cf-190f06cd67d4`。Agent采用独立`.data/browser-current`数据根，02:06:53Z启动，API仍18091，Vite15173代理到它。两条实际最终命令分别执行一次：

```sh
PLAYWRIGHT_MODULE=/Users/admin/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.mjs AGENT_BROWSER_REPORT_DIR='/Users/admin/git/temporal- agent-demo/docs/evidence' node validation/browser/smoke.mjs
PLAYWRIGHT_MODULE=/Users/admin/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.mjs AGENT_BROWSER_REPORT_DIR='/Users/admin/git/temporal- agent-demo/docs/evidence' node validation/browser/races.mjs
```

其他环境按调用器README提供自己的模块位置和API/UI地址即可；上述模块路径属于本次机器的公共Playwright依赖，不是产品依赖，也没有把模块拷入Agent。根`npm run dev`另实际启动到API18097/UI5173就绪，默认明确local/rules/local；后端独立默认仍temporal。

## 主用户链路

| 主线检查 | 结果及证明范围 |
| --- | --- |
| 页面身份与非空状态 | 当前产品标题和真实初始入口存在 |
| 需求与预览 | 创建SQLite/GORM、JWT和JSON日志项目，在PLAN确认前公开artifacts为空；能预览真实README计划内容 |
| 多轮修改 | 改为session认证与text日志，界面及方案随权威响应更新 |
| 确认与实际生成 | 绑定当前方案确认，真实Run到DONE；rules明确未调用模型 |
| 产物与ZIP | 读取登记README、实际下载项目ZIP，不以文件按钮存在代替生成 |
| 历史 | 可见该Run实际阶段与操作记录 |
| 窄屏 | 390×844无横向页面溢出；主会话和侧面板入口仍可见 |
| 控制台 | 主线报告console为空，检查通过 |

以上主线合计10个断言，报告包含业务Run身份。确认前检查针对公开产物列表，浏览器未检查私有工作区，不能据此推断全部隐藏文件系统行为。

## 定向负例与修复理由

| 定向检查 | 修复与结果 |
| --- | --- |
| 迟到Run快照 | 手动刷新后切换Run，延迟旧响应不能覆盖已选任务；通过 |
| 迟到文件响应 | 连续选择README/main.go，延迟旧文件不能覆盖最新选择；通过 |
| 文件跨Run | 读取后切Run，旧响应不能污染新任务产物；通过 |
| 客户端时钟到期 | 模拟客户端时钟，卡片主动禁用确认并显示到期；通过 |
| 定向控制台 | 无记录的页面异常；通过 |

对应修复使用selected引用与逐Run状态序号、文件请求序号与Run引用、1秒到期时钟；批准卡展示准确GitLab目标和日志参数。pause/resume新循环的命令键包含当前状态序号，避免复用上一轮意图。该段说明实现理由；5项自动断言不单独证明所有循环故障、断流或权限场景。客户端模拟时钟不证明服务端TTL，真实服务端过期由独立Python场景验证。

## 截图与视觉适配

已实际查看[桌面DONE截图](browser-done-desktop.png)与[手机DONE截图](browser-done-mobile.png)，另留[方案预览](browser-plan-desktop.png)。桌面保留[视觉方向](../design/workspace-concept.png)的纯白主区、冷灰侧栏、紫色强调与细分割线，三栏分别为任务、对话方案和运行/产物/历史。窄屏改为单主会话与侧面板抽屉，仍可进入产物，不强塞三栏。

有意适配包括：当前产品名称与真实状态替换概念文案；移除早期认知模型和营销导航；没有确认时不展示已生成工件；local远程仓库/CI显示不适用，不伪造远程成功。布局与状态来源见[设计说明](../design/DESIGN-SPEC.md)，完整规范仍在design，截图不能替代运行契约。

## 未覆盖范围

本轮没有live模型/GitLab/Runner、完整键盘与读屏验收、物理手机或生产发布验证。容器镜像manifest查询网络超时，未执行Docker build；固定Go镜像标签不等于容器可运行。产品内部检查、真实Temporal和mock协议证据另在[实现状态](../analysis/IMPLEMENTATION-STATUS.md)登记。
