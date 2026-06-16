# 项目完成度与边界

## MVP 已完成

- Gin API、SSE 事件协议、Viper、Zap、请求日志、用户 JWT 与管理员 API Key 鉴权。
- 本地 MySQL、Redis、Qdrant 数据链路；MySQL 不由 Docker 部署。
- 规则快速路由 + LLM 意图分类、LLM Query Rewrite、规则与 LLM 混合 Planner。
- 简单问题走收窄后的 RAG；复杂问题进入受控 Skill 或最多 5 步 ReAct。
- 6 个动态工具通过 MCP `tools/list` / `tools/call` 路径执行，具备 JSON Schema、白名单、超时、重复调用检测、幂等和日志；默认进程内执行，也可通过 HTTP MCP client 接入独立或外部 MCP server。
- 5 个 Skill：商品对比、选购推荐、配件查询、故障诊断、售后判断。
- 故障树状态机、配件兼容三态矩阵、结构化参数优先查询和售后动态事实强制落地。
- Evidence ID、LLM Reflection、确定性 Grounding Review、检索重试和转人工策略。
- Prompt 模板注册、场景化生成、版本历史、切换和回滚。
- 143 篇 mock 文档、200 条评估集、异步 Eval Runner、LLM Judge 和 bad case 分类。
- 真实模型完整 200 条严格通过率 71.5%，对比 82.4%，推荐 80.8%。

## 进阶增强已完成

- OpenAI-compatible LLM 多供应商顺序 fallback、模型级路由和三态熔断器。
- OpenAI-compatible Embedding fallback 到本地哈希向量。
- 已启用 SiliconFlow BGE Reranker，失败时回退本地词法排序/RRF 并记录 metadata。
- 知识文档版本原子激活，旧版本标记为 superseded，并清理旧 Qdrant point。
- 实际 LLM token usage 纳入 Agent token 预算和 Trace。
- Redis 分布式限流，Redis 异常时回退进程内限流。
- OpenTelemetry Span 与可选 OTLP HTTP 导出。
- Naive RAG 与 Agentic RAG 共用评估数据模型，可按系统版本对比。
- Prompt Injection 入口拦截、竞品提及策略和 MySQL 用户数据隔离。
- PDF、DOCX、HTML、Markdown、JSON 与纯文本知识入库解析。
- Prometheus 指标、模型成本估算、熔断器管理 API、Trace 离线分析命令、进程内 MCP 工具服务/客户端和独立 HTTP MCP server。

## 明确未完成或未夸大

- 未接真实电商价格、库存、订单、支付和物流系统，当前内置动态工具使用本地业务数据；HTTP MCP transport 已支持接入外部 MCP server，但不等于已完成真实 ERP/支付/物流联调。
- 已实现 HS256 JWT 与管理员 API Key，但未实现 OIDC、密钥轮换、管理员 RBAC 和可视化管理后台。
- 当前 SSE 是应用层事件分段，不是上游模型 token 直通；OpenAI-compatible 流式客户端已实现首包超时探测。
- LLM 三态熔断已覆盖模型请求，但没有宣称所有基础设施都具备复杂熔断与跨机房容灾。
- Prompt 版本当前由进程内 Registry 管理，尚无数据库持久化和管理 UI。
- 尚未实现超过 5 轮后的异步 LLM 摘要压缩与长期用户画像。
- Cross-Encoder Reranker 依赖外部兼容服务，仓库不内置模型推理服务。
- 已完成真实外部模型完整 200 条回归，但结果是单机串行评测，不代表并发 SLA。
- 评估集与 143 篇知识文档是项目 mock 数据，不是企业真实客服数据。
- 未进行大规模生产压测、真实支付/物流联调或生产 SLA 验证。

## 面试表达

1. 用 Go 把静态知识与价格、库存、订单、保修等动态事实分开治理。
2. 规则负责确定性边界和安全约束，LLM 负责语义分类、改写、动态规划与质量检查。
3. 简单参数问题不进入昂贵 ReAct；复杂推荐、诊断和售后任务进入受控 Skill。
4. 工具调用具备白名单、Schema、超时、日志、幂等、失败降级和结果证据化。
5. 每次回答能回溯到 KB chunk 或工具结果，并由 Reflection 与 Grounding Review 双重检查。
6. 用离线评估、Trace 和 bad case 分类量化改进，不把 mock 工程包装成真实生产系统。
