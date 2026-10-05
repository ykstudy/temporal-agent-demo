# 公共对象规范化与摘要

日期：2026-10-02。目标契约1.0.0。此处规定算法；各对象参与/排除字段由 [Agent契约](AGENT-CONTRACTS.md) 的hash表列明，消息引用使用 [消息协议](MESSAGE-PROTOCOL.md) 的权威对象摘要。当前代码的普通json.Marshal摘要不是本规范已实现的证据。

## 逻辑对象与文件摘要

逻辑对象使用 `SHA256(UTF8(JCS(hash_payload)))`，小写64位hex。JCS遵循 [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785)：使用规范JSON序列化及确定的属性排序。对象的自身hash字段按固定清单移除，不能按任意字段名递归删hash；子对象已经登记的hash保留并另外核验。数组顺序保留，语义为集合的数组由对象契约先规定稳定顺序后冻结。源文本不作Unicode重排。

文件、模板、Schema发布文件、ZIP内文件和原消息JSONL来源范围使用确切原始字节的SHA256，不经过JCS，不把重新pretty-print的JSON当成原字节。JSONL来源范围按消息记录实际追加顺序拼接原始UTF8行（包含原换行）；SummaryRecord保留完整message_id范围和来源摘要，缺原字节不得自造可核实范围。

Definition的逻辑hash与绑定依赖包的snapshot_hash有不同输入，不能互换。提案snapshot_hash覆盖未应用方案及固定确认要求/授权范围，排除可变status和未来签发的Approval实体；Approval单向绑定既有提案hash。ActionValidation的hash排除后生成execution_request，ExecutionRequest单向绑定已冻结validation_hash，避免循环。完整固定排除清单见Agent契约的hash表。Message ObjectRef.hash只引用已登记对象，不把最新对象覆盖旧hash。普通state_sequence变化不改变已冻结批准内容的hash，业务参数变化形成新source/proposal版本。

## 编解码要求

在Schema及hash处理前拒绝重复key、非法Unicode代理字符、NaN/Infinity与超限输入。用于身份/版本/预算的整数范围为0至2^53−1，正版本从1开始；高精度业务数据另用明确字符串类型，不让Go与JS解析产生不同hash。`date-time`字段使用UTC的Z结尾表示，启用真实格式检查，不以尾缀正则代替合法日期。

规范化参数由可信注册Skill的默认值和Schema产生，再冻结同形对象。验证器的`default`注释不自动生成业务参数。模型不能更改hash输入清单、完成条件、预算或秘密引用解析规则。

## 独立验收

后续实现至少验证：Go/TS/Python同一发布向量得到相同hash；属性顺序与空白变化不改变逻辑hash；数组顺序和提示内容变化改变hash；模板原文件空白变化改变文件hash；只改自身hash不影响hash_payload而仍要求结果比对；嵌套依赖hash改动会改变上层hash；重复key/非法日期/大整数拒绝；旧消息指向原对象。算法符合性与SHA外形检查分开记录。

本轮只发布规则和结构样例，不声称已实现JCS或跨语言hash验收。示例中的占位hash（如64个零或a）仅用于Schema形状演示；不是任何真实对象/批准/工件的证据。
