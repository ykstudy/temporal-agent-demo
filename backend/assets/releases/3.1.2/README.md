# 项目创建 Agent 3.1.2 纯 Go SQLite 发布

本次发布继续使用 contract 1.2.0，新增 AgentDefinition 3.1.2、数据库 Skill 3.1.2，以及 scaffold/database 模板 3.1.2；认证和日志 Skill、模板保留 3.1.0 的精确版本与原字节。3.1.1 的完整系统提示、Go 1.25.5/darwin_amd64 身份、权限、预算、固定命令及完成政策不变。旧发布不得覆写，默认选择和 S2 完成状态不由本次发布修改。

SQLite 的实际 database/sql 驱动改为 `modernc.org/sqlite v1.40.1`，GORM 保留官方 `gorm.io/driver/sqlite v1.6.0` 与 `gorm.io/gorm v1.31.0`，通过官方 `sqlite.Config{DriverName:"sqlite"}` 显式选用纯 Go 驱动。sqlx 同样注册并选择 `sqlite`。官方 GORM dialector 仍声明 mattn 的传递依赖，但 `CGO_ENABLED=0` 只编译其未选用的 stub；任何连接都不得使用 `sqlite3`。不引入 C 编译器、SDK 权限或任意 shell。

SQLite GORM 与 sqlx 的 go.mod/go.sum 原字节由真实 Go 工具整理后冻结。其他四种数据库/ORM 依赖及源码保持原字节。SQLite 隔离测试保留重复迁移，并增加真实插入、读取、更新、事务回滚和删除检查；全部只访问测试临时数据库。Dockerfile 统一以 CGO_ENABLED=0 编译和 static 运行镜像，不再为 SQLite 选择原生 C 运行库。

发布验收必须使用完整冻结定义、真实公开命令语义、P1 批准、scaffold/Skill 执行及实际受控 Go tidy/test/build；两种 SQLite ORM 每条工具回执均需绑定同一批准、manifest 与父 attempt，approved 工作区原字节不变。开发生成器整理依赖、非沙箱模板测试或旧 63 组合证据均不能替代受控工具验收。生成和验收结果另行附于本文件，未获得真实结果前不声明通过。

驱动选择依据：[官方 GORM SQLite 自定义 DriverName](https://github.com/go-gorm/sqlite/blob/v1.6.0/sqlite.go)、[modernc SQLite 文档](https://pkg.go.dev/modernc.org/sqlite)。纯 Go 驱动的适配仅改变冻结模板内容，未增加公共 Schema 字段；跨版本引用仍由产品 Registry 完整校验。

## 生成及验证记录

2026-10-02，`go1.25.5/darwin_amd64`，`AGENT_REGENERATE_312=1 go test ./internal/agent -run '^TestRegeneratePublication312$' -count=1 -v` 通过（23.022s）。两种 ORM 均真实执行 CGO_ENABLED=0 的 tidy/test/build；SQLite 测试完成重复迁移、CRUD、生成主键与事务回滚。生成器显式选用公开 `https://goproxy.cn` 和 `sum.golang.org`，这是开发发布环境证据，未改变 contract 1.2 的运行环境。完整依赖摘要记录在 [dependency-locks.json](dependency-locks.json)。定义 hash 为 `816e847d34581158c3743c96a91554a7f541db53eb8e2d4c7f1e476838e14c5b`。

本机原固定 `proxy.golang.org:443` 在受控沙箱外也连接超时，因此 1.2 旧 Run 的代理策略保持原样，不能借本次发布静默换源。冷缓存受控运行验收等待 contract 1.3 的显式 GoModulePolicy 及绑定它的新 Agent 3.2.0；此处不把开发生成器通过写成旧 1.2 Run 受控执行通过。其他平台、容器镜像、客户数据库和 live GitLab 均不在这份证据内。

完整新旧发布闭包、Registry 重启后精确冻结、无关政策和 auth/logging 身份不变、非 SQLite 五组渲染原字节不变的定向测试通过（6.792s），相同边界的 race 检查通过（27.849s）。这些是版本和模板边界证据，不替代实际 Run 工具执行。
