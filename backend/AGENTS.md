# 后端维护约定

继承根目录 `AGENTS.md`。本目录是可独立运行的项目创建 Agent，不是文档认知模型服务，也不是外部场景验证器。

实施以 `../design/contracts/README.md` 和 `../design/IMPLEMENTATION-PLAN.md` 的阶段输入/退出条件为准；目标模块见 `../design/contracts/MODULE-MAP.md`。先引入完整公共对象和legacy adapter，再移动职责。系统提示、参数/动作Schema、技能依赖和完成条件必须进入冻结发布包；只存版本标签及模板hash不足以满足定义冻结。模型建议、规则裁决、执行请求分别持久绑定身份，不能用Plan+Execute布尔值替代公开契约。

## 实现与依赖

Workflow 只用 Temporal SDK 确定性原语，文件、网络、模型和进程进入 Activity。业务 agent_run_id 与稳定 operation_id 跨 Continue-As-New 不变；Temporal execution/attempt 仅追踪。消息、快照和工作区明确单写者持久化边界，不宣称多节点事务。

不导入 `validation/` 或其他项目；不读取参考 harness 源文件；不把 `design/_model/check.py` 作为启动或 DONE 条件。产品内部真实 test/build、manifest/秘密检查及远程提交/CI 绑定验证必须保留。公共依赖由三轴配置和 readiness 明确，缺依赖失败或 BLOCKED，不能制造成功。

## 版本、批准和副作用

把业务 source_version 与进度 state_sequence 分开；proposal_version 不原地修改。接受和应用复核服务端时间、proposal/source 版本、快照 hash、定义和动作范围；重复成功请求复用原结果。批准期限默认30分钟，可配1分钟至24小时，不接受客户端延长。

外部写超时或丢响应先核实原 project/ref/SHA/pipeline 身份。禁止强推、接管他人项目、重复触发首跑以及取消后谎称撤销副作用。数据库技能只生成模板与隔离测试；普通创建确认不授权真实客户数据库或生产发布。

## 检查和说明

针对变化运行必要 Go tests，涉及并发、批准或恢复时运行 race；完成后执行 vet 和 build。通过的单元/契约测试不能代替 live GitLab/模型、真实 Temporal、浏览器或运维验收。更新公开 API、环境变量和输出字段时同步 `backend/README.md` 与 `docs/AGENT-GUIDE.md`，不在说明中暴露秘密。
