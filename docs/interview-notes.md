# CleanCare Agent 面试讲解提纲

## 60 秒介绍

CleanCare Agent 是 Go 实现的清洁电器垂直 Agentic RAG 后端，只覆盖扫地机器人、空气净化器、净水器和加湿器。143 篇 mock 知识文档承载参数、对比、说明书、故障树和政策，200 条评测覆盖纯 KB、纯 Tool、KB+Tool 与拒答引导。简单问题走快速 RAG，复杂问题走 ReAct 或 Plan-and-Execute，动态价格、库存、订单、保修和工单通过受控工具完成。

## 关键技术点

| 技术点 | 一句话概括 | 代码位置 | 为什么这样设计 |
|---|---|---|---|
| 意图识别 | 规则处理高确定性请求，低置信度再交给 LLM | `internal/intent/hybrid_router.go` 的 `HybridRouter.Route` | 降低成本并保留口语泛化能力 |
| 查询改写 | 用会话摘要补全指代，同时保留原始 query | `internal/agent/query_rewriter.go` | 避免“那个滤芯”丢失购买上下文 |
| 混合检索 | Dense + Keyword 经 RRF 融合后 Rerank | `internal/retriever/hybrid.go` 的 `Hybrid.Search` | 兼顾语义召回与型号、数字精确命中 |
| 参数精确检索 | 参数文档在混合结果前精确优先 | `internal/retriever/structured_first.go` 的 `StructuredFirst.Search` | 防止相似型号参数串线 |
| 差异化分块 | 九类文档按表格、任务、节点和条款切分 | `internal/rag/chunker.go` 的 `StructureAwareChunker` | 保留参数表、步骤和政策例外完整性 |
| ReAct 控制 | 最大 5 步、重复检测、Token 预算和超时 | `internal/agent/agentic_runner.go` 的 `AgenticRunner.Run` | 防止循环失控和成本不可预测 |
| Plan-and-Execute | 先生成依赖明确的计划，失败后有限修正 | `internal/agent/llm_planner.go` | 适合推荐、比较和售后多步骤任务 |
| Tool 安全 | 白名单、参数校验、超时、结果校验和审计 | `internal/tool/executor.go` 的 `Executor.Execute` | 动态数据不能由 LLM 猜测 |
| Skill | 固定业务编排组合 Retriever 与多个 Tool | `internal/skill/workflow.go` 的 `Workflow.Run` | 故障和售后需要受控流程 |
| 幻觉抑制 | 数值、引用和安全规则在生成后复核 | `internal/agent/reflection.go` 的 `GroundingReflector.Review` | 让答案中的事实可回到 evidence |
| 模型容错 | Fallback + CLOSED/OPEN/HALF_OPEN 熔断 | `internal/llm`、`internal/embedding`、`internal/reranker` | 外部模型分钟级故障时保持可用 |
| 可观测性 | Trace 记录步骤，Metrics 统计 Token 与 P95 | `internal/observability/agent_metrics.go` | 可定位慢在哪一步、贵在哪个模型 |
| 入口安全 | Prompt Injection 在 Runner 前拦截，用户与管理员使用不同凭证 | `internal/agent/safety_guard.go`、`internal/middleware/auth.go` | 避免恶意输入进入模型，也避免后台接口复用用户身份 |
| 文档入库 | 上传后按格式解析，再进入差异化分块与异步索引 | `internal/ingest/parser.go`、`internal/ingest/redis_stream.go` | 将知识库从硬编码 seed 扩展为可维护 Pipeline |
| 评测闭环 | 200 条数据集同时统计自纠正、安全、有据率和工具利用率 | `internal/eval/metrics.go`、`internal/eval/runner.go` | 不只看答案相似度，还定位 Agent 链路失败位置 |

## 高频追问

### 为什么不是普通 RAG

普通 RAG 能回答吸力，但无法可靠处理实时价格、当前用户订单、保修期和创建工单。多约束推荐还需要先过滤参数，再查询动态数据。

### 为什么 Tool 和 Skill 分开

Tool 是原子动态能力，例如查订单；Skill 是带业务顺序和安全约束的编排，例如购买记录定位主机、检索兼容表、查询滤芯价格。

### 为什么故障诊断不能自由 ReAct

漏水、冒烟和漏电必须先触发固定安全动作。受控故障树可审计，LLM 只负责理解表达和组织回答。

### 当前效果怎样

历史 100 条本地基线中，Agentic Strict Pass Rate 从 0.26 提升到 0.40，Multi-step Completion 从 0.69 提升到 0.86。该结果使用 local_hash 和 extractive Generator，只证明工程链路，不代表真实 LLM 线上效果。3 条 LLM 冒烟 P95 为 5982 ms，样本不足，完整 200 条和并发基准仍待测。

### 项目哪里仍不是生产完成态

所有业务数据是 mock；没有接入真实支付、物流或 ERP。真实模型全量基线、50/100 并发压测和长期线上告警尚未执行，文档明确区分“已实现”和“待验证”。

### 第二轮改进如何定位代码

- 模型优先级与自适应熔断：`internal/llm/client.go`、`internal/llm/circuit_breaker.go`、`internal/llm/circuit_manager.go`。
- Plan-and-Execute 与有限计划修正：`internal/agent/agentic_runner.go`、`internal/agent/llm_planner.go`。
- Skill 配置化与链式调用：`internal/skill/config.go`、`internal/skill/workflow.go`。
- Reflection 三策略重检：`internal/agent/agentic_runner.go` 的 `reflection.check` 阶段。
- Prompt A/B：`internal/prompt/compare.go` 与 `/api/v1/admin/prompts/eval`。
- Trace 分析：`cmd/analyze-traces`，用于慢步骤、重复步骤、工具失败和反思触发率统计。
