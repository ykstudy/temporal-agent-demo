# 独立场景验证

该目录是项目创建 Agent 的外部验证调用器，不是 Agent 的运行依赖。业务操作只通过公开 HTTP、SSE 和交付 ZIP；不导入或调用内部 Service、Workflow、模板或其他实验模块。启动辅助、provider fixture 与场景调用器分开。

## 验证层次

| 层 | 入口 | 证明范围 |
|---|---|---|
| 启动辅助 | `start.py` | 编译并启动本项目 backend，只控制自己启动的进程 |
| 业务黑盒 | `scenarios.py --base-url` | 需求、修改、未确认预览、确认、版本/过期/幂等、暂停/重做/阻塞、SSE、ZIP |
| 生成项目 | 业务场景下载的 ZIP | 安全解包后实际构建、启动生成项目并请求 `/healthz` |
| 协议 fixture | `fixtures.py` | 明确的 mock model/GitLab HTTP 协议，不证明真实模型或真实 Runner |
| Temporal | 相同业务场景连接 Temporal 模式 Agent | 真实 Query、Continue-As-New、上下文压缩和并发；显式指定自建 launcher 后验证运行中 VERIFY 崩溃恢复 |

## 本地完整核心场景

从项目根启动独立 Agent，保持此终端运行：

```sh
python3 validation/start.py --execution-mode local --decision-mode rules --delivery-mode local --port 18090 --approval-ttl 1m
```

另一个终端运行：

```sh
python3 validation/scenarios.py --base-url http://127.0.0.1:18090 --suite core --expect-execution-mode local
```

也可以直接用 `--base-url` 连接已启动的 Agent，不需要启动辅助。过期验证使用公开的 `issued_at/expires_at`，不能由客户端覆盖期限；默认最长等待 90 秒。现有服务默认 30 分钟期限时，该项会明确记为 `not_run`，总体不完整，退出码 2；其他可执行场景继续记录。

## 明确 mock 的全套场景

以下命令的 token/key 是本地 fixture 占位值，不是外部服务凭据。三个服务分别在终端中保持运行：

```sh
python3 validation/fixtures.py --port 18101
GITLAB_TOKEN=validation-fixture-token python3 validation/start.py --execution-mode local --decision-mode rules --delivery-mode gitlab --port 18092 --gitlab-base-url http://127.0.0.1:18101 --gitlab-namespace-id 1 --approval-ttl 1m
AI_API_KEY=validation-fixture-key python3 validation/start.py --execution-mode local --decision-mode model --delivery-mode local --port 18093 --ai-base-url http://127.0.0.1:18101/malformed/v1 --ai-model validation-mock --approval-ttl 1m
```

主 Agent 18090 同样按核心场景命令启动，然后运行：

```sh
python3 validation/scenarios.py --base-url http://127.0.0.1:18090 --suite full --gitlab-agent-url http://127.0.0.1:18092 --malformed-agent-url http://127.0.0.1:18093 --fixture-url http://127.0.0.1:18101
```

fixture 的 `/fixture/health` 与 `/fixture/state` 提供明确身份和公开协议证据。模型固定响应和损坏响应都是 mock；GitLab 的提交、文件/tree、精确 SHA/ref 流水线以及必需 `test/build` job、`coverage.out/bin/server` artifact 都是固定协议响应，未启动真实 CI Runner。build artifact 仅提供明确 mock 的 ELF header，不能当作真实编译产物。

## 真实 Temporal

先独立准备 Temporal 服务，再启动 Agent：

```sh
python3 validation/start.py --execution-mode temporal --decision-mode rules --delivery-mode local --port 18094 --temporal-address 127.0.0.1:17333 --approval-ttl 1m
python3 validation/scenarios.py --base-url http://127.0.0.1:18094 --suite core --expect-execution-mode temporal
```

Temporal 核心场景额外发送 21 条消息，通过公开 `/workflow` 核验相同业务 Run/Workflow 下真实 execution run ID 切换；继续验证压缩保留原始消息、跨 Continue-As-New 幂等及 11 个并发请求（9 个唯一命令）。

要包含真实故障恢复，把启动输出的 `launcher_state` 路径显式传入：

```sh
python3 validation/scenarios.py --base-url http://127.0.0.1:18094 --suite core --expect-execution-mode temporal --launcher-state 'validation/reports/agent-<本次启动ID>/launcher.json'
```

故障场景先通过公开 Run 观察到 `VERIFY` 正在执行，然后调用启动辅助的 loopback controller，杀死并重启它自己的确切 `Popen` 进程；数据、编译 binary 和 task queue 保持相同。controller 随机凭据只保存在权限 0600 的 launcher 文件，不写进 HTTP 日志、summary 或启动输出；controller 不接受 PID、命令或路径参数。未指定 launcher 时明确记录 crash `not_selected`，连接已有 Agent 不会控制其进程。

启动辅助不启动、停止或删除 Temporal 容器。真实 Temporal 的昂贵集成与进程故障注入统一协调运行，避免重复。本目录的调用器不把 `local` 标成 Temporal，不把 rules 标成模型调用，也不把本地交付标成 GitLab CI 成功。

## 失败后稳定业务身份重试

这是独立的定向套件，验证模型 HTTP 503 后同一 Create 的恢复、成功重放，以及同一 Run ID 不同初始 payload 的冲突。它不替代完整 core/full 套件。

```sh
python3 validation/fixtures.py --port 18102 --model-fail-first
AI_API_KEY=validation-fixture-key python3 validation/start.py --execution-mode temporal --decision-mode model --delivery-mode local --port 18095 --temporal-address 127.0.0.1:17333 --ai-base-url http://127.0.0.1:18102/v1 --ai-model validation-mock --approval-ttl 1m
python3 validation/scenarios.py --base-url http://127.0.0.1:18095 --suite model-retry --expect-execution-mode temporal --fixture-url http://127.0.0.1:18102
```

需要新的 fail-first fixture；报告必须实际观察第一次 `model_unavailable`/503 与随后两次模型请求、未确认 PLAN、成功重放没有额外请求和异 payload 的 409。provider 始终是 mock；Temporal 的 Query/Update 重试路径是真实服务。

## 证据与退出码

默认稳定保存到 `validation/reports/<UTC时间>-<唯一ID>/`，可以用 `--report-dir` 选择报告父目录。报告目录被本目录 `.gitignore` 忽略，源码与验证入口独立保留。

- `summary.json`：实际服务模式、执行结果、逐场景 `passed/failed/not_run`、Run 身份、证明范围。
- `http.jsonl`：公开请求和响应，保留稳定命令与对象身份，不保存认证请求头。
- `artifacts/`：下载 ZIP、校验摘要及解包后的实际生成项目。
- `project-runtime.log`：生成项目的真实构建/启动输出和 health 证据。
- 启动辅助另存私有 `launcher.json`、`build.log`、`agent.log`、`control-events.jsonl` 与隔离数据目录；分享报告时排除 launcher 凭据文件。

退出码 0 表示所选套件全部执行通过；1 表示场景断言失败；2 表示前置条件不满足或存在 `not_run`，不声称完整通过。需要 Go 工具链启动 Agent 和构建交付项目；fixture 与调用器自身只使用 Python 标准库。

`scripts/integration.py` 保留为新的场景入口兼容包装；不再隐式构建 Agent、依赖 Docker CLI 或读取临时证据目录。进程启动请显式使用 `validation/start.py`。
