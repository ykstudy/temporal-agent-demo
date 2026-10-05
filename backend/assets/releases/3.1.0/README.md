# 项目创建 Agent 3.1.0 发布设计

本发布采用 contract 1.2.0，执行 [ADR-049](../../../../design/10-decisions/ADR-049-controlled-execution-ledger.md) 与[严格 VERIFY 规则](../../../../design/contracts/releases/1.2.0/README.md#verify-修复与权限生命周期)。它是新的定义、Skill 与模板闭包，旧 3.0.0/3.0.1 文件、hash、恢复解释及默认选择不变。本阶段只准备发布能力与验证；默认切换和运行调度由对应接入阶段完成。

## 依赖文件先批准

已支持参数仍为无数据库，以及 PostgreSQL/MySQL/SQLite 与 GORM/sqlx 的六种组合；不新增数据库、ORM、客户连接或迁移能力。认证 JWT/session、日志 console/file 使用标准库，不引入新的模块依赖。

发布工具在隔离临时目录真实渲染七种依赖图，执行已绑定 Go 工具的固定 `go mod tidy`，将完整整理结果写回 3.1.0 模板：scaffold 明确生成 go.mod 和空 go.sum；database 按既有 database/orm 条件追加已整理的 go.mod 依赖段及 go.sum 原字节。项目名仅替换 module 行和既有项目内导入；依赖版本和校验和不随 Run 改写。模板依旧是 format_version=1 的有限声明数据，没有 shell、远程模板或动态依赖解析。

新 add_database Skill.write_paths 显式包含 go.sum；required_permissions 保持 tool_go/workspace_read/workspace_write，固定 post_commands 及最小权限不增加。scaffold 的输出白名单在新 contract 1.2.0 下包含 go.sum；database 仅允许追加该文件，且必须在冻结 Skill.write_paths 中声明。旧 1.1.0 解释器路径边界不扩大。每个新 Proposal 的文件差异与 manifest 必须包含实际 go.mod/go.sum 内容。

VERIFY 的 tidy 仍真实执行。预先冻结依赖不等于跳过验证；若工具、源码或依赖环境使 tidy 产生任何持久文件差异，按新 source、PLAN、明确重批处理，不能把差异悄悄接受为当前批准范围。

## 发布与运行接口

`definitions.LoadBundledRelease("3.1.0")` 读取本目录完整定义/技能/模板声明，并用对象自身 contract_version 解析产品内离线 Schema，验证所有文件、对象和来源 hash。`Registry.PublishBundled("3.1.0")` 将该完整闭包不可变登记，既有 Run 继续从已登记原始模板字节读取。未知版本、同身份不同内容、缺原字节或 hash 不符直接拒绝；不回退到当前 Worker 模板常量。

本目录保存完整 definitions/agent.json、definitions/release.json、三个 skills/*/skill.json 及 templates/golang/*/3.1.0/render-program.json。嵌入二进制的相同闭包可供 root 显式选择；导出/重新生成仅属于开发发布操作，不属于 Agent Run 或 HTTP 输入。当前工具身份属于本次发布的一部分；不同实际工具身份须明确新发布，不能改写 3.1.0。

## 验收及证据边界

先通过文档结构检查，再生成产品资产。逐项记录七种依赖图的原字节 hash；认证无/JWT/session 与日志无/console/file 的完整 63 个支持组合均真实渲染、tidy、test、build，并比较 tidy 前后完整文件清单及原字节。测试覆盖缺失/改变锁定依赖文件产生差异、缺 go.sum 写入声明被拒绝、原 3.0.1 冻结输出和资产摘要不变，以及新发布注册/重启后继续读取同一闭包。

这些是模板发布和本地真实 Go 工具证据，不代表实际 S2 job、Temporal、Docker 镜像、客户数据库或 live GitLab 已通过。
