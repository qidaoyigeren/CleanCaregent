# 面试讲解提纲

## 60 秒项目介绍

我用 Go 实现了一个清洁电器垂直客服后端，范围只覆盖扫地机器人、空气净化器、净水器和加湿器。商品参数、兼容表、故障树和售后条款走混合 RAG；价格、库存、订单、保修和工单走动态工具。简单问题直接检索，比较、推荐、配件、诊断和售后判断进入固定 Skill，整个 Agent 最多执行 5 步，并记录 Trace 和 evidence。

## 可写入简历

- 基于 Go、Gin、MySQL、Redis、Qdrant 构建清洁电器 Agentic RAG 后端，实现混合检索、RRF、Rerank 与 SSE 流式回答。
- 设计 6 个带白名单、超时、幂等和日志的动态工具，以及 5 个业务 Skill，完成购买记录、配件兼容和实时价格的多步编排。
- 实现受控 Planner、重复调用检测、Grounding Review、Evidence ID 和 OpenTelemetry Trace，支持 Agent bad case 回放。
- 构建 57 篇 mock 知识文档和 100 条分层评估集；本地同集测试中 Agentic 版本 Tool Selection 从 0.80 提升到 1.00，Multi-step Completion 从 0.69 提升到 0.86。

## 高频追问

### 为什么不用普通 RAG

普通 RAG 可以回答 T20 吸力，但不能可靠处理实时价格、用户订单、保修期和副作用工单。多约束推荐、配件购买和售后判断还需要多文档与工具组合。

### 为什么 Tool 和 Skill 分开

Tool 是原子动态能力，例如查订单；Skill 是 Retriever、多个 Tool 和固定业务规则的组合，例如先查购买记录，再检索兼容表，最后查滤芯价格。

### 如何防止 Agent 失控

规则先决定简单或复杂路径；工具按意图白名单开放；最大 5 步；同一 Trace 的相同参数调用只允许一次；每个工具独立超时；最终回答还要过 Grounding Review。

### 为什么 MySQL 不存向量

MySQL 保存文档、chunk 元数据和审计记录；Qdrant 负责向量近邻检索。关键词检索继续走 MySQL FULLTEXT/LIKE 路径，再用 RRF 融合。

### 评估结果为什么不高

当前使用本地哈希 Embedding 和抽取式生成器，目标是验证工程链路。严格通过率 0.40 暴露了 Context Precision 和生成归纳能力问题，不能包装成线上模型效果。
