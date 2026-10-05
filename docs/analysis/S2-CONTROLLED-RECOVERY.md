# S2 受控验证恢复与工作区代际

日期：2026-10-02。适用于完整受控 Run；本说明细化既有状态机、固定工具、预算与批准规则，不修改冻结不变量，也不声明当前 S2 已退出。

## 原批准内的有界安全重试

只有准确原 verify 请求的 action 已结算为 failed、operation 为 rejected，且本次已启动工具均有完整终止回执、准确原输入/输出 manifest、原子预算结算，才可能重试。重新读取批准目录并逐字节核对当前 Proposal/Manifest；候选差异、缺回执、unknown/interrupted、未结算尝试或其他 sent/unknown 操作均不能作为可重试依据。模型 diagnosis 是建议，不能覆盖上述判定。

满足条件时先登记 diagnosis（model 真实调用冻结 provider，rules 才生成规则建议），然后以原 source、批准、ExecutionRequest 与 operation 回到 IMPLEMENT 的安全重试边界。operation 恢复 ready；下一次调度重新进入 VERIFY，执行器复校当前权限、工具、来源和预算后分配新的 action/tool attempt。旧失败回执和结算保留，次数与执行总期限不刷新。预算耗尽由确定性控制面终止为 FAILED；未知事实继续 BLOCKED。暂停、取消、权限撤销或来源变化不能被重试覆盖。

## 内容变更与新批准

工具只写隔离候选。候选不是已批准 manifest，不能复制回旧目录、直接 resume 或将旧 manifest 改写 source。候选修订必须具有完整原请求/尝试/回执/输入 manifest、候选文件及摘要来源；这部分公开载体随后续契约发布，不向已发布 1.2 闭合对象临时加字段。

验证修订先展示新 Proposal 全部文件、原/后摘要与原验证证据，增加业务 source_version，关闭旧批准并等待明确 P1。无候选文件变更的控制恢复不增加 source。被批准的新 source 使用新的 Run 内目录代际，旧目录、manifest、回执、operation 和预算全部保留。新目录由新批准动作重新生成全量内容，不继承旧 manifest 作为已经实施的证据。代际身份由 Run/source/批准身份确定；文件及子进程仍限于同一 Run 根目录。

2026-10-03实施细化：完整候选载体已由契约1.3.0发布。自动修订与显式`revise`共用入口；显式命令必须绑定当前原提案ID、版本、source及hash。候选采用前重新检查原批准目录的真实字节、每个工具的固定身份/参数/环境、父子attempt及真实预算结算，不能仅验证回执hash。计划中的候选引用必须来自当前Run已经登记的证据，且只允许VERIFY修订边界使用。建议失败或拒绝时保持BLOCKED/VERIFY、原source与批准，不回到可执行状态；成功形成新提案时才增加source并关闭当前批准。命令回执与新提案在同一持久提交中完成，成功重放不重复生成提案或调用模型。

## 调度边界

local 与 Temporal Activity 调用相同的受控调度/执行服务；调度提示不提供写权限。每次副作用前必须使用完整 ExecutionRequest 重新接纳。失响应/重启后只读核实原 operation，不因 Activity 自动重试重复外部写。动作耗尽后还须完成固定内部检查与完整报告；模型 completion 不能直接设置 DONE。

取得数据目录独占锁后的进程重启，核对所有未结算预留。已原子登记的终止回执与结算继续复用；缺结果的原预留按unknown消费，不释放、不生成工具回执。未终止的动作转unknown并清除失效ActiveStep，Run进入原阶段的BLOCKED；暂停和终态保留。远程写只读核实原身份，本地缺回执的工具不能因Temporal重试盲重放，用户可取消后以新Run重做。此分支的验收应证明安全阻塞及新身份恢复，而不能将缺结果描述成原尝试成功。

ExecuteStep的已知业务拒绝、结构/授权/预算失败通过Temporal Activity边界转换为不可自动重试的明确错误；不能由SDK重试重新调用模型来覆盖原拒绝。worker中断及未返回结果仍使用Temporal原Activity恢复机制，随后由持久原请求、unknown计账和只读核实判定安全行为。ProcessCommand原有failed Update显式同身份重试保持原规则。

明确的进度查询（进度、状态、查询进度、查看进度、status、progress）只登记原输入、命令消费和普通进度，不重写计划description，不增加source_version、不刷新提案或批准。它们在CAN和并发重放中保持同一业务方案。

## 2026-10-04 参数修订与期限重提

继续第2阶段时补齐无文件候选的已知VERIFY失败路径。带准确原提案身份的`revise`先核对真实失败回执、预算结算和原批准目录，关闭当前批准并进入CLARIFY；后续原文形成完整新计划，内容或范围变化时增加source并重批。缺回执、unknown、活跃执行、用户暂停/取消和权限撤销均不能借此逃过核实。旧文件/manifest/执行请求仅保留历史，不能作为新source已实施的事实。存在真实候选时仍走原候选采用入口，不自动丢弃候选。

未开始执行的过期批准不能resume或开始动作。允许用户明确重提同一完整计划，签发新proposal_version及服务端期限；方案和授权范围相同不增加source。原批准/提案和计账保留，旧确认不能用于新版本。终态重做始终新Run、新预算和新批准，受控Run不经legacy简化Plan直接恢复执行。

受控终态重做绑定准确原完整计划及定义选择，原重做命令的JSON作为新Run的真实输入，原计划只作为明确标注的重做来源进入上下文。model模式仍需真实建议；rules模式建议完整原计划，均重新经过治理、渲染和新P1。旧Run的批准、workspace、candidate、operation、预算和回执不继承；引用旧验证候选的采用权限也不跨Run继承，新Run从冻结模板重新生成。

## 验证要求

覆盖已知失败后原请求的新尝试、失败计账/预算耗尽、目录或候选变化拒绝重试、缺/伪回执拒绝、unknown 不重放、模型 diagnosis 真调用及无权限、暂停取消竞态、原批准/新 source 目录代际和重启。产品测试、独立 HTTP oracle、真实 Temporal 和第三方 live 分开登记。
