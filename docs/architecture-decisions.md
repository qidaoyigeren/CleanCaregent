# CleanCare Agent 架构决策记录

本文记录业务场景如何约束架构选择。代码位置使用函数名，避免重构后行号失效。

## 修复前后对比

| 维度 | 修复前 | 当前决策 |
|---|---|---|
| Agent 模式 | 复杂问题统一逐步执行 | 快速 RAG、ReAct、Plan-and-Execute 分流 |
| 故障诊断 | 可由通用生成回答 | 受控 Skill + 故障树 + 安全停止 |
| 分块 | 统一长度 | 九类文档采用结构感知策略 |
| 参数检索 | 仅依赖语义召回 | StructuredFirst 精确优先 |
| 数据存储 | 单一检索视角 | MySQL 管结构与审计，Qdrant 管向量 |
| 动静态数据 | 容易混用 | 静态 KB 与动态 Tool 明确分界 |

## ADR-001：使用 ReAct 与 Plan-and-Execute 双模式

### 背景

单一事实问题只需一次检索；多约束推荐、跨型号比较和售后判断需要多步依赖。所有请求都走 ReAct 会增加延迟和 Token，所有复杂请求都预生成完整计划又难以及时吸收工具失败。

### 决策

- 简单事实走快速 RAG。
- 依赖关系明确的复杂任务走 Plan-and-Execute。
- 需要根据观察动态决定下一步的任务走 ReAct。
- ReAct 最多 5 步。

代码：`internal/agent/agentic_runner.go` 的 `AgenticRunner.Run`，`internal/agent/planner.go` 的 Planner 接口。

### 后果

正面：简单问题成本低，复杂问题可审计，动态失败仍能调整。

负面：路由和 Planner 质量成为新的关键点，需要评测不同模式。

### 如果选错会怎样

若所有问题都走 ReAct，“T20 吸力多少”也会产生多次 LLM 调用；若只用固定计划，价格工具失败后仍可能继续使用无效结果。

### 替代方案

仅 Naive RAG 无法处理订单和副作用操作；仅自由 ReAct 难以控制安全和成本。

## ADR-002：工具调用采用 MCP 抽象

### 背景

当前 6 个工具均为项目内 mock 动态能力，部署边界、鉴权模型和审计格式一致。为了让 Agent 主循环不绑定本地工具实例，同时保留本地可测性，工具发现和执行需要统一到 MCP 形态。

### 决策

使用 `internal/tool/mcp` 提供 MCP tool server/client。业务工具挂到 MCP server，`tool.Executor` 只依赖 MCP client 的 `tools/list` 和 `tools/call` 能力，并继续负责白名单、Schema 校验、超时、幂等、审计和结果校验。默认配置为进程内 client/server；`tool.mcp.transport=http` 时通过 HTTP JSON-RPC 连接独立或外部 MCP server。

代码：`internal/tool/tool.go`、`internal/tool/mcp/server.go`、`internal/tool/mcp/http.go`、`internal/tool/executor.go`、`cmd/mcp-server`。

### 后果

正面：工具发现/调用路径与 MCP 对齐，Skill 和 Agent 不依赖本地 registry；超时和审计仍统一；测试可完全本地化；需要跨进程部署时可用独立 MCP server，远程连接支持 Bearer/API key 鉴权、工具列表缓存和瞬时故障重试。

负面：当前 HTTP transport 覆盖 `tools/list` / `tools/call` 和基础 SSE 响应读取，尚未实现完整 MCP 生命周期、OAuth 授权服务器、server-to-client notification 订阅和多 MCP server 聚合命名冲突处理。

### 如果选错会怎样

若工具仍直接经本地 registry 调用，未来接外部 MCP Server 会侵入 Agent 和 Skill；若强制所有环境都走远程传输，会在 mock 数据阶段增加不必要的网络故障点。因此默认进程内、远程按配置开启。

### 替代方案

直接 HTTP 调用会把鉴权、超时、校验散落在 Skill 中；继续使用本地 registry 会让工具发现协议与真实 MCP 脱节。当前采用 MCP client 抽象统一进程内和 HTTP 远程传输。

## ADR-003：故障排查使用受控 Skill

### 背景

漏水、冒烟、焦糊味和触电风险属于安全红线。LLM 自由推理可能跳过断电、关阀或禁止拆机步骤。

### 决策

`troubleshooting` 意图进入 `fault_diagnosis` Skill，按故障树节点推进；规则层先处理安全停止，LLM 只用于话术和证据整理。

代码：`internal/skill/workflow.go` 的 `Workflow.Run`，`internal/diagnosis`，`internal/agent/reflection.go` 的 `GroundingReflector.Review`。

### 后果

正面：安全动作稳定、节点可审计、可统计每个故障分支。

负面：未覆盖的新故障需要补树或转人工，自由度低于通用对话。

### 如果选错会怎样

自由 ReAct 可能在漏水场景建议带压拆接头，或在冒烟时继续试机，风险不可接受。

### 替代方案

纯 FAQ 缺少多轮状态；纯规则话术生硬。当前采用规则决策、LLM 表达的组合。

## ADR-004：按文档类型使用 StructureAwareChunker

### 背景

商品参数表、任务手册、故障树和售后政策的最小完整语义单元不同。统一按字符数切分会破坏表格行、步骤顺序和政策例外。

### 决策

九类文档分别设置 ChunkProfile。参数表保留整表，手册按任务，故障树按节点，政策按条款和条件，FAQ 一问一答一块。

代码：`internal/rag/chunker.go` 的 `StructureAwareChunker`，`configs/config.example.yaml` 的 `rag.chunk_profiles`。

### 后果

正面：证据可读，减少条件丢失和错误拼接。

负面：分块配置更多，新增文档类型必须定义边界规则。

### 如果选错会怎样

若统一切块，“七天无理由”可能与“已拆封耗材例外”分离，系统会给出错误退货承诺；故障步骤也可能丢失前置断电要求。

### 替代方案

固定 Token 窗口实现简单但不满足业务完整性，因此只作为无结构文本兜底。

## ADR-005：使用 MySQL 与 Qdrant 双存储

### 背景

订单、文档版本、政策生效时间、审计记录需要事务和结构化过滤；自然语言相似召回需要高效向量索引。

### 决策

MySQL 保存文档、Chunk 元数据、业务数据和审计；Qdrant 保存向量点并执行 Dense Search。两者通过稳定 DocID/ChunkID 关联。

代码：`internal/repository/mysql`、`internal/vectorstore/qdrant`、`internal/retriever/hybrid.go`。

### 后果

正面：结构化约束和语义召回各用合适引擎。

负面：写入链路需要处理双写一致性、重试和清理。

### 如果选错会怎样

只用 Qdrant 难以可靠处理订单归属和政策时间；只用 MySQL LIKE 会让口语、错别字和近义表达召回显著下降。

### 替代方案

单库简化部署但牺牲至少一种核心能力，故不采用。

## ADR-006：参数表使用 StructuredFirst

### 背景

型号和数值参数要求精确。向量检索可能因描述相似同时召回多个型号，Rerank 也不能保证错误型号绝不排在前面。

### 决策

当 query 包含明确型号和参数意图时，优先读取结构化参数文档，再补充混合检索结果。价格和库存不进入参数表。

代码：`internal/retriever/structured_first.go` 的 `StructuredFirst.Search`。

### 后果

正面：型号命中稳定，数值证据口径一致。

负面：需要维护型号归一化和结构化记录。

### 如果选错会怎样

若只做向量检索，问“P400 CADR”可能混入 P500；若把实时价格也结构化在参数表，会产生过期报价。

### 替代方案

仅关键词精确匹配无法覆盖口语和拼写变体，因此 StructuredFirst 后仍保留 Hybrid 召回。
