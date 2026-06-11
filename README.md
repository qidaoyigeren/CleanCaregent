# CleanCare Agent

面向扫地机器人、空气净化器、净水器和加湿器的清洁电器电商客服 Agentic RAG 后端。项目使用 Go 实现，重点覆盖商品对比、多约束推荐、配件兼容、故障诊断、退换货判断和动态订单工具调用。

当前仓库是可运行的本地工程项目，不宣称已接入真实电商生产系统。

## 已实现

- Gin HTTP API 与 SSE `status/evidence/delta/done/error` 流式协议
- MySQL 会话、商品、订单、知识库、Agent Trace、工具日志和评估结果
- Redis 最近消息、诊断上下文和可选分布式限流
- Qdrant 向量检索，MySQL 关键词检索，RRF 融合和可降级的远程 Rerank
- 规则快速路由 + LLM 意图分类、LLM Query Rewrite、受控 Planner 和最多 5 步 ReAct
- 6 个动态工具和 5 个可复用 Skill
- Prompt 模板注册、版本切换、回滚和参数/对比/故障/售后分场景生成
- 故障树状态机、配件兼容三态矩阵、结构化参数优先查询
- 最终 Grounding Review、LLM Reflection、数值证据检查、低置信度和转人工策略
- LLM 多供应商 fallback、三态熔断、Embedding/Rerank fallback 与 OpenTelemetry Trace
- 57 篇 mock 知识文档生成器和 100 条分层评估集
- 知识版本原子激活、旧向量清理和异步 Eval Runner

## 核心链路

```text
HTTP/SSE -> Session -> Hybrid Intent Router -> LLM Query Rewriter
         -> Guarded Planner / ReAct -> Retriever / Skill / Tool Executor
         -> Evidence Collector -> LLM Reflection / Grounding Review
         -> Answer -> Agent Trace
```

静态商品参数、兼容表、故障树和售后条款进入 RAG；价格、库存、订单、保修状态和工单进入 Tool。动态数据不会写进向量库作为事实来源。

## 本地依赖

- Go 1.26
- 本机 MySQL 8，数据库名 `cleancare`
- Docker Desktop，仅运行 Redis 和 Qdrant

```powershell
docker compose up -d redis qdrant
Copy-Item configs/config.local.example.yaml configs/config.local.yaml
```

在 `configs/config.local.yaml` 中填写本机 MySQL DSN。MySQL 不由 Docker 创建。

初始化并启动：

```powershell
go run ./cmd/migrate
go run ./cmd/seed
go run ./cmd/kb-seed
go run ./cmd/server
```

健康检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health/ready
```

## 主要 API

```text
POST /api/v1/conversations
POST /api/v1/conversations/{id}/messages
POST /api/v1/conversations/{id}/messages:stream
GET  /api/v1/conversations/{id}/messages

GET  /api/v1/products
GET  /api/v1/products/{product_code}
GET  /api/v1/orders/{order_no}
POST /api/v1/after-sales/tickets

POST /api/v1/admin/kb/documents
POST /api/v1/admin/kb/search
GET  /api/v1/admin/traces/{trace_id}

POST /api/v1/admin/eval/runs
GET  /api/v1/admin/eval/runs/{run_no}?include_failures=true
```

评估运行接口返回 `202 Accepted`，任务在后台执行；使用返回的 `run_no` 查询进度和结果。

创建会话并提问：

```powershell
$base = "http://127.0.0.1:8080/api/v1"
$cv = (Invoke-RestMethod -Method Post -Uri "$base/conversations" `
  -ContentType "application/json" -Body '{"title":"选购咨询"}').data.conversation_id

Invoke-RestMethod -Method Post -Uri "$base/conversations/$cv/messages" `
  -ContentType "application/json; charset=utf-8" `
  -Body '{"content":"我上周买的净化器滤芯多少钱，有券吗？"}'
```

## 评估

重新生成评估集：

```powershell
go run ./cmd/eval-dataset -output ./docs/eval/eval-cases-v1.json
```

运行完整 100 条评估：

```powershell
Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:8080/api/v1/admin/eval/runs `
  -ContentType "application/json" `
  -Body '{"dataset_version":"v1","system_version":"agentic-local","max_cases":0}'
```

历史本机基线结果见 [实验报告](docs/eval/experiment-report.md)。基线使用 `local_hash` Embedding 和抽取式生成器，只用于验证链路与评估框架；更换 BGE、Reranker、Qwen 或 DeepSeek 后必须以新的 `system_version` 重新运行评估，不能沿用旧结果。

## 可选应用容器

`compose.yaml` 的 `app` 服务属于可选 profile，仍连接宿主机 MySQL：

```powershell
$env:CLEANCARE_MYSQL_DSN = "user:password@tcp(host.docker.internal:3306)/cleancare?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=true"
docker compose --profile app up -d --build
```

默认 `docker compose up -d` 不启动 Go 应用，也不创建 MySQL。

## 验证

```powershell
go test ./...
go test -race ./internal/agent ./internal/tool ./internal/retriever ./internal/middleware ./internal/memory/redis
go vet ./...
```

设计文档见 [clean-care-agentic-rag-design.md](docs/clean-care-agentic-rag-design.md)，完成度与边界见 [project-status.md](docs/project-status.md)。
