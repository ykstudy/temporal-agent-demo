# 功能：Harness 服务运行

> 实体ID：feature.deploy.service-runtime
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§1.2、§3、§6.1、§11。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-001、R-016、R-018、R-021、R-023。
> 基线日期：2026-10-02。

## 目标

使 Harness 的接口、Worker、Temporal 和持久存储具备明确启动、停止与共享边界。该功能负责产品自身运行，不承担生成项目的生产发布。

## 输入

已验证配置、执行模式、Worker 任务队列与定义版本，持久存储和隔离工作目录，运行身份、端口、资源限制及停止信号。

## 输出

服务就绪状态、可诊断的启动失败、运行组件身份与存储归属、受控停止结果。向健康检查提供实际连接和能力信息，向 Run 保留持久消息、检查点与操作引用。

## 算法 / 流程

先检查目录归属、存储权限与版本兼容。temporal 分支注册 Workflow、受限 Activity 与双方传播器，连接正确命名空间和队列后启动 Worker；local 分支检查自身执行与恢复条件，不要求 Temporal。必要组件可用后才声明就绪。文件存储采用单写者或锁/CAS，跨 Worker 使用共享持久数据。停止时拒绝新任务，给当前处理有限时间并保留可恢复记录。

## 边界与异常

启动失败必须可见，不以 HTTP 进程存活代替编排就绪。Worker 重启可能触发重放和 Activity 重试，执行器仍必须幂等；正在进行的外部写入不能因进程退出被视为回滚。文件不完整时拒绝把缺失数据补成成功。local 模式标注自身恢复限制，不声称提供 Temporal replay。

## 依赖

- [有效配置与模式](configuration-modes.md#输出)：消费公开输出契约。
- [隔离目录](../../06-governance/features/workspace-isolation.md#输出)：消费公开输出契约。
- [持久命令入口](../../01-orchestrator/features/durable-messaging.md#输入)：向公开输入契约提交请求。
- [活动调度契约](../../01-orchestrator/features/activity-scheduling.md#输入)：向公开输入契约提交请求。

## 被谁依赖


- [JSONL 原始消息存储 · 输入](../../05-conversation/features/jsonl-message-store.md#输入)：消费本功能的公开输出契约。
- [健康检查与恢复 · 输入](health-recovery.md#输入)：消费本功能的公开输出契约。
- [幂等与并发控制 · 输入](../../06-governance/features/idempotency-and-concurrency.md#输入)：消费本功能的公开输出契约。

## 验收标准

端口或存储错误阻止就绪并定位原因；temporal 分支额外要求 Temporal 可用，显式 local 不依赖它。停止和重启后原始消息与操作身份可恢复。多 Worker 不通过彼此独立本地文件共享状态；运行记录说明持久卷、权限及数据恢复方式，且不改动其他服务。
