# 3.2.0 完整受控交付定义发布

2026-10-02，依 contract 1.3.0 / ADR-050 新发布 Agent 与三个 Skill 定义。该定义引用 3.1.2 的 scaffold/database 和 3.1.0 的 auth/logging 模板原字节，不重新渲染、tidy 或重写旧发布；SQLite 使用已发布纯 Go 驱动。参数、权限和功能选择范围保持原值，Skill 仅将完整发布契约/Schema hash/自身版本与摘要迁移到 1.3.0 / 3.2.0。

GoModulePolicy 明确固定 https://goproxy.cn + sum.golang.org，禁止 direct fallback 和环境继承；Go 的 CGO=0/隔离/校验边界保持。Agent 的 required_tools 全局包含 Go、git、git_remote_http，三轴下都需要本机实际身份匹配；并非仅选择 GitLab 后才推断工具。Git 版本/平台/原二进制摘要来自此次受信开发发布观察，不允许不同机器根据当前 PATH 重写同版本。

GitExecutionPolicy 精确冻结六条命令模板、上限和凭据隔离规则。模型提示沿 3.1.1/3.1.2，模型仍无权选择任意命令、跳过验证或批准。真实工具、受控 push/CI、修复重批与完整完成门分别验收；本发布不切换运行默认，不移除未就绪门禁，也不代表 live 第三方或 S2 job 已通过。

发布时记录 agent/skill/tool/schema/template 身份并由产品 Registry 实际 Publish/Freeze 校验闭包；若原字节或固定工具改变，必须创建新版本。
