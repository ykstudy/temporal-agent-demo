# 场景调用器边界

本目录独立于 Agent 的内部实现。现有 Python 场景调用器只使用标准库；后续完整Schema验证采用下述独立进程及显式依赖边界。业务验证通过公开 HTTP、事件流、工件清单与 ZIP，不得导入 backend/internal、直接调用 Service、读取内部模板或私有工作区来使验证通过。

后续目标契约验收按 `../design/contracts/README.md` 和 `../design/IMPLEMENTATION-PLAN.md`。现有调用器保持标准库边界；需要完整Draft2020-12时使用独立、显式依赖的契约验证进程，不临时拼一个仅检查required字段的validator。它消费冻结的公开Schema发布物并离线解析引用，不消费产品私有类型；格式检查/负例与跨对象语义分别执行。不得接受字段别名、未知字段或passed/status OR等宽松判定来迁就当前实现，fixtures也须先通过目标Schema。本轮只补这些约束，尚未新增运行契约验证器。

`start.py` 只负责本项目 backend 的独立启动，`fixtures.py` 只负责明确 mock 的 provider 协议，`scenarios.py` 只负责公开场景。Agent 服务本身不得导入本目录或等待场景完成才能工作。

证据写入被忽略的 `reports/`，保留 stable command/run/proposal/source/hash 身份。失败与未执行分开；缺模型、GitLab、Go 或 Temporal 前置条件不能静默降级或制造通过结果。任何模拟都必须标明 mock，不作为 live provider、Runner、浏览器或生产证明。

只控制自己创建的进程，不停止已有服务或容器。真实 Temporal 与昂贵生成项目验证先与主任务协调；不要为了改动调用器反复重跑无变化的昂贵集成。

崩溃验证必须显式指定本目录启动辅助的 `--launcher-state`。仅用随机凭据认证 loopback controller；控制器只能重启自己持有的确切 `Popen`、原 binary/数据/task queue，不接受外部 PID/命令/路径。不将 control token 写进公开证据、summary 或命令输出。
