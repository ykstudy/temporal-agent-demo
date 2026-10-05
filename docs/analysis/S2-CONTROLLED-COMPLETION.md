# S2 受控打包与完成证据

日期：2026-10-02。适用于具有完整冻结定义、批准和动作链的受控 Run。本说明先冻结本轮实现边界；不修改旧 1.2 契约字节，不代表 S2 已通过，也不改变默认未就绪保护。

## 打包来源和不可变字节

`package` 必须消费服务端签发的完整 ExecutionRequest。开始前复校当前来源、批准、权限、manifest、动作顺序和预算；action 预留与 operation=sent 在同一持久化事务内完成。工作目录只能是请求绑定的 Run 内批准目录。读取使用受限根目录，拒绝路径穿越、符号链接、特殊文件、不合法 UTF-8、凭据和超过冻结文件/字节预算的内容。实际文件集合、每个文件的原始 UTF-8 字节、大小和 SHA-256 必须同时等于完整 Proposal 和当前 manifest，不能用 legacy 检查状态代替。

ZIP 包含 `<project_name>/` 下的每一份已批准源文件，以及单独的 `AGENT-MANIFEST.json` 完整公开 manifest。该文件是交付元数据，不冒称已批准源文件；批准路径与该保留名冲突时拒绝打包。manifest 使用 JCS 编码，源文件保持原始字节。条目按路径排序，使用固定时间、常规文件权限和 Store 方法；不存在额外文件、重复条目、目录条目、链接或绝对路径。生成后必须重新读取每个条目，核对大小、CRC、实际字节摘要和元数据原文。

ZIP 位于同一 Run 根目录的 `.control-packages/`，不污染批准项目目录。文件名只由原 operation 身份摘要派生，不能由模型或请求任意指定。先写唯一临时文件、fsync、核验，再以不覆盖已有目标的原子发布登记最终文件。任何已开始且结果不确定的写保留原请求、attempt 和 operation，结算 unknown 并 BLOCKED；不得重发写入。成功重放只返回准确原回执，读取与恢复必须核对原文件字节。

1.3 发布 `PackageReceipt`，绑定 Run/source/definition/plan/proposal/approval/execution/operation/request/attempt/workspace/manifest、原始 ZIP 引用及实际字节摘要/长度/源文件数、时间和状态。原始 ZIP 的 `package` 引用版本为 1，hash 为实际 ZIP 字节 SHA-256；回执 hash 为排除自身 hash 后的完整 JCS。包与回执均须可通过精确对象引用读取。私有调度结果只保存索引，不能代替公开回执。1.2 没有此载体，不能声称其私有结果已经满足公开交付审计。

## 完成报告与确定性裁决

完成入口首先验证完整动作链已耗尽且没有 sent/unknown、待核实证据、活跃写、暂停或取消。完整 `VerificationReport` 按冻结 CompletionPolicy 的当前 delivery_mode 逐项生成 `RequiredCheck`；保留所有未就绪/失败项。已知安全基线不能通过删除检查、改变 kind/evidence_types、重复 ID 或仅传递一个绿色结果来削弱。新增但没有执行器的检查保持 not_ready。

本地检查分别核对完整批准源、完整 manifest、三个固定 Go 命令的完整持久回执、四份必需文件、实际字节秘密扫描和 ZIP 完整性。每个通过项均带准确同源对象引用；工具回执还须绑定原验证请求、命令、参数、工具身份、父 action、child attempt 和已结算预算。旧文本、Step 标签、legacy RequiredChecks 或外部 validation 报告不提供完成权限。

GitLab 交付另要求 1.3 `RemoteExecutionReceipt` 的完整原项目/目标/归属证据、准确批准提交树与 CI 配置，以及准确原 pipeline 的非 allow_failure test/build jobs、真实解析的覆盖率和构建工件。缺载体或缺事实保持 not_ready；只凭项目 URL、commit 字符串或 success 状态不能通过。local 模式按冻结策略不包含远程检查，不伪造远程执行。

1.3 S2AuditBundle 公开当前 nullable verification_report 与全部 verification_reports、package_receipts、remote_receipts，精确对象接口提供不可变正文。报告只声明该次实际检查时点的事实。首次报告归档后，completion 使用共享语义建议入口：rules 才生成规则建议，model 必须真实调用冻结 provider、记录完整 Context/Decision/Validation 和预算结算。模型只能建议，不能设置 DONE。建议之后重新读取当前批准源和 ZIP，并在同一 Run 持久化边界复校来源、权限、完整链、报告及全部证据；全部满足才写入 DONE。拒绝、预算耗尽、并发控制变化或证据漂移均不完成。

## 验证与尚待接入

新专有模块提供 `ExecuteControlPackage` 和 `FinalizeControlCompletion`；dispatcher、公开精确原始 ZIP 读取、对象登记和 Run 默认版本由主集成负责。新契约生成完成前先验证纯 ZIP 和完成规则，不能用 synthetic 回执冒称真实 job。测试覆盖实际打包/重读、精确源集合、路径/链接/秘密/CRC/篡改、失败与 unknown 结算、原请求重放、冻结检查保留、伪造或异源证据、模型 completion 真调用及无直接完成权限、暂停取消与来源变化。代码检查、产品执行、独立 HTTP、真实第三方和 Temporal 分别登记。
