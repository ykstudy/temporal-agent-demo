# 3.1 发布准备证据

2026-10-02 的 3.1.0 模板与 3.1.1 Agent 定义发布准备证据从临时目录原样复制到本目录。它们证明版本化闭包、原字节保护及本机真实 Go 命令结果，不代表 S2 实际 job 验收或默认切换。

- `template-matrix.json`：63 种技能组合的真实 `go mod tidy`、`go test`、`go build` 结果及逐命令文件原字节比较。
- `template-tests.log`：矩阵、缺失锁文件与未声明写路径负例的测试日志。
- `publication-verification.json`：发布身份、原资产保护、定向测试及源摘要。
- `docs-check.json`：该时点的设计文档结构检查结果。

矩阵使用 Go 1.25.5 / darwin_amd64 与本机默认 CGO 环境。SQLite 的 GORM 和 sqlx 模板使用 `mattn/go-sqlite3`；本结果不能证明它们在受控执行器固定 `CGO_ENABLED=0` 的环境能实际打开数据库并完成隔离迁移测试。该运行环境差异是 S2 待完成项，必须通过新版本的精确工具链或模板发布解决；不得重写已经冻结的 3.1.0 / 3.1.1 字节或复用本矩阵声称等价通过。
