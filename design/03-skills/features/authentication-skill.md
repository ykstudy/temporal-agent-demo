# 功能：认证技能

> 实体ID：feature.skills.authentication-skill
> 来源：[原设计](../../../docs/PRODUCT-GUIDE.md)：§2.3、§7.2、§9；L72、L877–920、L1205–1216。
> 审校：[裁决记录](../../../docs/analysis/REVIEW-DECISIONS.md)：R-009、R-011、R-019、R-020。
> 基线日期：2026-10-02。

## 目标

生成明确拒绝未认证请求的 JWT 或 session 基础框架，提供可测试的签发、校验、过期与撤销边界。基础认证不等于完整用户体系或生产授权策略，不能种入默认账号/秘密。

## 输入

已确认 auth_method、token_ttl、算法/claims、cookie/session 存储策略、秘密引用与权限要求；Skill/模板版本、scaffold 与工作区、需要保护的接口及安全验收清单。

## 输出

认证模块、中间件、配置和使用说明、测试/依赖 manifest 与分类结果。记录 JWT/session 的能力和持久性边界；未配置、无凭据、无身份校验与验证通过可区分。

## 算法 / 流程

1. 校验方法、TTL、支持算法、秘密强度与 cookie/session 策略，把保护范围和配置缺口列入方案。
2. 生成固定版本的签发/验证与拒绝路径，身份确认留给明确用户接口，不以任意输入直接代表可信登录。
3. JWT 核验签名、算法、必要 claims 和有效期；session 核验高熵身份、过期、撤销与存储状态，cookie 使用明确安全属性。
4. 执行无/错凭据、篡改、过期、撤销和配置缺失测试，确认保护接口行为，再登记 manifest 与适用限制。

## 边界与异常

无 secret 或无有效身份不得放行；不使用硬编码 key、示例密码或默认管理员。不得从客户端 claims 自增权限，算法切换和 TTL 变更须重新预览。内存 session 重启失效需明示；生产持久会话、多实例和细粒度授权若未验收，不能声称完成。秘密不得进生成文件或日志。

## 依赖

- [认证参数 Schema](skill-contract.md#输出)：消费公开输出契约。
- [审核模板版本](template-versioning.md#输出)：消费公开输出契约。
- [服务入口和工件](scaffold-generation.md#输出)：消费公开输出契约。
- [秘密/身份/授权边界](../../06-governance/features/permissions-and-secrets.md#输出)：消费公开输出契约。
- [安全约束](../../06-governance/features/constraint-validation.md#输出)：消费公开输出契约。

## 被谁依赖


当前基线无其他功能消费本契约；入口/外部使用者在该功能输入与验收中说明。

## 验收标准

JWT/session 都能通过基础编译与正反例测试；保护接口在缺失、错误、篡改、过期及撤销凭据下拒绝。核验算法与 TTL 绑定、cookie 属性和配置缺失；扫描工件/日志无真实秘密。用户系统与生产会话能力的未验证部分明确展示。

