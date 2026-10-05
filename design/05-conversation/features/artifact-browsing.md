# 功能：真实工件浏览

> 实体ID：feature.conversation.artifact-browsing
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§4.1–4.3、§7.1–7.5、§9、§13.5–13.7。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-011、R-017–019、R-023–024；浏览及下载契约为本次补全。
> 基线日期：2026-10-02。

## 目标

让用户查看本 Run 实际生成、已登记且完整性可核验的文件、验证报告和交付包，区别方案中预计创建的路径。文件浏览不成为任意主机路径读取入口，展示链接也不构成远程交付已经完成的证明。

## 输入

agent_run_id、工件 ID/version、批准源版本、manifest、workspace 引用及相对路径；内容读取、目录浏览或包下载请求；文件 hash、大小、类型、权限、脱敏策略及仓库/流水线证据引用。

## 输出

分层工件清单、来源与 hash、受限文本内容、二进制下载引用、完整交付包及验证报告；计划、已生成、已验证、远程确认等准确状态。未知工件、路径越界、版本过期、文件变化或无法访问返回明确错误。

## 算法 / 流程

1. 加载当前 Run 的登记 manifest；PLAN 仅展示明确标注的预计文件，IMPLEMENT 有登记结果后才展示真实工件树。
2. 对工件与请求者做归属检查，将相对路径规范化到批准根目录，核验符号链接、文件类型及登记版本。
3. 读取时比较实际大小/hash，文本按编码与上限展示，二进制走下载；大对象保留稳定引用，不注入 Workflow payload。
4. package 交付按 manifest 打包并检查秘密与遗漏，登记包 hash 和验证证据；下载事件不改运行阶段。
5. 仓库与 CI 面板只消费已确认的远程身份；工作区丢失或包损坏时显示证据不可访问，交恢复流程处理。

## 边界与异常

禁止绝对路径、`..`、编码混淆、非授权符号链接和跨 Run 文件；不能读取后端环境、凭据、锁文件或其他项目。源设计文档属于只读审阅来源，与生成项目文件分开。浏览失败不伪报空目录成功；已确认远程提交与后来修改的本地文件分别展示版本，不覆盖原证据。

## 依赖

- [生成 manifest 与工件引用](../../03-skills/features/scaffold-generation.md#输出)：消费公开输出契约。
- [文件根目录和路径规则](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [读取权限与秘密检查](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [交付证据清单](../../07-observability/features/verification-evidence.md#输出)：消费公开输出契约。
- [准确远程提交](../../04-integrations/features/repository-push.md#输出)：消费公开输出契约。

## 被谁依赖


- [分层上下文构建 · 输入](../../02-agent/features/context-building.md#输入)：消费本功能的公开输出契约。
- [富消息结构协议 · 输入](rich-message-protocol.md#输入)：消费本功能的公开输出契约。
- [对话工作区 · 输出](../../08-frontend/features/conversation-workspace.md#输出)：向本功能的公开输入提交请求。
- [进度与工件面板 · 输出](../../08-frontend/features/progress-artifact-panels.md#输出)：向本功能的公开输入提交请求。

## 验收标准

预览树不会被标记已生成；点击文件能读同一版本/hash，ZIP 仅包含批准 manifest。遍历、跨 Run、符号链接逃逸、秘密和文件篡改均拒绝；大文件和二进制可用适当方式读取或下载。delivery_mode=local 的包不显示假 repo/pipeline，源文模型更新不篡改生成工件。
