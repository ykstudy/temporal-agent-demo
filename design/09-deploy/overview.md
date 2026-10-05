# Harness 运维域

> 实体ID：domain.deploy
> 基线日期：2026-10-02。
> 来源：[原设计](../../docs/PRODUCT-GUIDE.md)：§1.2、§3、§8、§11、§13；审校：[裁决记录](../../docs/analysis/REVIEW-DECISIONS.md)：R-012、R-018、R-021、R-023、R-024。

## 域职责

定义 Harness 自身的配置、运行组件、持久存储、健康与故障恢复，以及交付包的可复核运行条件。明确 Temporal/local、规则/模型和 local/gitlab 交付的差异，使部署环境与产品能力对应。

## 域边界

产品范围是 Go 项目生成、GitLab 仓库与首次 CI 验证，本域名称不代表生成项目的生产部署。不得隐式发布、自动合并或把环境缺失伪装成成功；配置只能声明能力，不能代替每次外部副作用授权。

## 与其他域的关系

从 Agent 域取得定义版本，从治理域取得安全、预算和目录边界，向编排提供可靠 Worker 与持久命令入口；交付引用技能产物和集成证据，健康恢复消费可观测性与检查点。

## 按改动节奏划域的理由

运行地址、存储卷、资源限制、工具版本与恢复流程随环境和运维要求变化，比业务阶段和技能行为更频繁，也不同于前端交互迭代。独立本域可在改变部署方式时保留稳定的业务契约与明确模式标签。

## 功能清单

- [配置与执行模式](features/configuration-modes.md)
- [Harness 服务运行](features/service-runtime.md)
- [可复核的工件交付](features/reproducible-delivery.md)
- [健康检查与恢复](features/health-recovery.md)

## 规范与证据

功能契约规定目标行为；原设计代码示例、参考项目与本轮历史实现不自动成为完整行为证明。已执行与尚未执行的验证范围见[实现状态](../../docs/analysis/IMPLEMENTATION-STATUS.md)，本轮设计校验只证明文档结构、来源摘要和认知模型的一致性。
