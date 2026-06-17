# CleanCare Agent

面向扫地机器人、空气净化器、净水器和加湿器场景的电商客服 Agentic RAG 项目。后端使用 Go 构建，前端提供 React 演示控制台，覆盖商品问答、多约束推荐、配件兼容、故障诊断、订单查询、退换货判断和售后工单等完整链路。

项目重点不是简单封装大模型接口，而是演示如何把静态知识、动态业务事实、受控 Agent 编排、证据校验、离线评测和可观测性组合成一套可运行的工程系统。

> 当前仓库使用 mock 商品与业务数据，适合本地开发、架构验证和面试展示，不代表已接入真实电商生产系统。

## 项目展示

![CleanCare Agent 对话与执行流水线](docs/images/agent-pipeline.jpg)

图中展示了 SSE 对话、Evidence ID 引用和 Agent Pipeline。系统在证据不足时会明确说明缺失信息，检索命中后可展示意图识别、查询改写、计划生成、知识检索、工具调用、反思质检和回答生成过程。

## 核心能力

- **三种运行模式**：支持 `bootstrap`、`naive_rag` 和 `agentic`，便于基线对比与渐进式开发。
- **混合 Agent 编排**：简单事实问题走快速 RAG，复杂任务进入受控 Skill、ReAct 或 Plan-and-Execute，ReAct 最多执行 5 步。
- **混合检索链路**：MySQL 关键词检索 + Qdrant 向量检索 + RRF 融合 + 可降级 Rerank，并支持结构化参数优先查询。
- **动静数据隔离**：商品参数、故障树和售后政策进入知识库；价格、库存、订单和保修状态通过 Tool 实时查询。
- **安全与可信回答**：包含 Prompt Injection 拦截、安全停止条件、Evidence ID、数值证据检查、LLM Reflection 和确定性 Grounding Review。
- **工程化工具调用**：6 个动态工具通过 MCP `initialize`、`tools/list` / `tools/call` 执行，支持进程内、Streamable HTTP、stdio 和多 server 聚合，并具备 JSON Schema、白名单、超时、重复调用检测、幂等、结果校验和审计日志。
- **可配置业务 Skill**：内置商品对比、选购推荐、配件查询、故障诊断和售后判断 5 类工作流。
- **模型容错**：支持 OpenAI-compatible LLM、Embedding 和 Reranker，多供应商 fallback、三态熔断与本地降级。
- **评测闭环**：内置 200 条 v2 分层评测集、异步 Eval Runner、LLM-as-Judge、bad case 分类和系统版本对比。
- **可视化演示**：React 前端提供流式对话、执行流水线、Trace、知识库、评测和 Prompt 管理页面。

## 系统架构

```mermaid
flowchart LR
    UI["React Demo Console"] --> API["Gin HTTP / SSE API"]
    API --> SESSION["Session & Memory"]
    SESSION --> ROUTER["Hybrid Intent Router"]
    ROUTER --> FAST["Fast RAG"]
    ROUTER --> AGENT["ReAct / Plan-and-Execute"]
    AGENT --> SKILL["5 Business Skills"]
    AGENT --> TOOL["6 Dynamic Tools"]
    FAST --> RETRIEVER["Hybrid Retriever"]
    SKILL --> RETRIEVER
    RETRIEVER --> MYSQL["MySQL Keyword / Metadata"]
    RETRIEVER --> QDRANT["Qdrant Vector Search"]
    TOOL --> BUSINESS["Price / Inventory / Order / Warranty"]
    FAST --> REVIEW["Reflection & Grounding Review"]
    AGENT --> REVIEW
    REVIEW --> ANSWER["Answer + Evidence + Trace"]
    SESSION -. cache .-> REDIS["Redis"]
```

核心执行链路：

```text
HTTP/SSE -> Session -> Intent Router -> Query Rewrite
         -> Fast RAG / Skill / ReAct / Plan-and-Execute
         -> Retriever / Tool Executor -> Evidence Collector
         -> Reflection / Grounding Review -> Answer -> Agent Trace
```

## 当前资产

| 项目 | 当前规模 |
|---|---:|
| Mock 知识文档 | 143 篇 |
| v2 评测案例 | 200 条 |
| 评测难度分布 | simple 80 / medium 70 / hard 50 |
| 动态工具 | 6 个 |
| 业务 Skill | 5 个 |
| Prompt 场景模板 | 12 类 v3 模板 |
| 前端页面 | 对话、Trace、概览、知识库、评测、Prompt |

支持入库的文档格式包括纯文本、Markdown、HTML、JSON、CSV、Excel、DOCX 和 PDF。

## 技术栈

| 层级 | 技术 |
|---|---|
| Backend | Go 1.26、Gin、Viper、Zap |
| Frontend | React 19、TypeScript 6、Vite 8 |
| Database | MySQL 8.4 |
| Cache / Memory | Redis 7.4 |
| Vector Store | Qdrant 1.13 |
| Model Protocol | OpenAI-compatible Chat / Embedding / Rerank |
| Observability | OpenTelemetry、Prometheus、Agent Trace |
| Quality | Go Test、Race Test、Vet、ESLint、GitHub Actions |

## 快速开始

### 1. 环境要求

- Go 1.26
- Node.js 22+
- Docker Desktop

### 2. 启动基础设施

```powershell
docker compose up -d mysql redis qdrant
docker compose ps
```

Compose 默认创建以下本地服务：

| 服务 | 地址 | 默认账号 |
|---|---|---|
| MySQL | `127.0.0.1:3306` | `cleancare / cleancare` |
| Redis | `127.0.0.1:6379` | 无密码 |
| Qdrant | `http://127.0.0.1:6333` | 无 API Key |

### 3. 创建本地配置

```powershell
Copy-Item configs/config.local.example.yaml configs/config.local.yaml
$env:CLEANCARE_MYSQL_DSN = "cleancare:cleancare@tcp(127.0.0.1:3306)/cleancare?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=true"
```

`config.local.example.yaml` 默认启用 MySQL、Redis、Qdrant 和 `agentic` 模式。真实 API Key 应通过 `CLEANCARE_*` 环境变量注入，不要提交到仓库。

### 4. 初始化数据

```powershell
go run ./cmd/migrate
go run ./cmd/seed
go run ./cmd/kb-seed
```

也可以使用：

```powershell
make migrate
make seed-all
```

### 5. 启动后端

```powershell
go run ./cmd/server
```

健康检查：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health/ready
```

可选：将内置业务工具拆到独立 MCP 进程运行：

```powershell
go run ./cmd/mcp-server
```

主服务连接独立 MCP server 时配置：

```yaml
tool:
  mcp:
    transport: http
    endpoint: http://127.0.0.1:8090/mcp
    api_key: change-me
```

stdio MCP server 示例：

```yaml
tool:
  mcp:
    transport: stdio
    stdio_command: go
    stdio_args: ["run", "./cmd/mcp-server"]
    stdio_env:
      CLEANCARE_TOOL_MCP_SERVER_TRANSPORT: stdio
```

多个 MCP server 可通过 `tool.mcp.servers` 聚合；当配置多于一个 server 时，暴露给 Agent 的工具名会加上 `<server>/<tool>` 前缀，避免命名冲突。独立 MCP server 支持 `initialize` / `notifications/initialized` 生命周期、`Mcp-Session-Id`、`MCP-Protocol-Version`、SSE notification stream、Bearer / `X-MCP-API-Key` 校验和 OAuth protected-resource metadata；本仓库不内置完整 OAuth 授权码发放服务器，可通过 `authorization_servers` 指向外部授权服务器。

### 6. 启动前端

打开另一个终端：

```powershell
Set-Location clean-care-frontend
npm ci
npm run dev
```

访问：

- 前端控制台：`http://127.0.0.1:5173`
- 后端 API：`http://127.0.0.1:8080`
- Qdrant Dashboard：`http://127.0.0.1:6333/dashboard`

Vite 开发服务器会把 `/api` 请求代理到 `8080`。本地配置默认关闭鉴权；启用鉴权后，用户接口使用 JWT，管理员接口使用 `X-Admin-API-Key`。

## Docker 启动后端

`app` 服务位于可选 profile 中，可同时启动 MySQL、Redis、Qdrant 和 Go 后端：

```powershell
docker compose --profile app up -d --build
```

该命令会自动建表，但首次运行仍需执行 `go run ./cmd/seed` 和 `go run ./cmd/kb-seed` 写入演示数据。前端当前作为独立 Vite 工程运行，不包含在 Compose 中。

## 模型配置

默认本地配置无需外部模型：

```yaml
embedding:
  provider: local_hash

reranker:
  provider: local_lexical

llm:
  provider: extractive
```

接入真实模型时可配置任意 OpenAI-compatible 服务：

```powershell
$env:CLEANCARE_LLM_PROVIDER = "openai_compatible"
$env:CLEANCARE_LLM_ENDPOINT = "https://example.com/v1/chat/completions"
$env:CLEANCARE_LLM_API_KEY = "your-api-key"
$env:CLEANCARE_LLM_MODEL = "your-model"
$env:CLEANCARE_PROMPT_ENABLE_LLM_COMPONENTS = "true"
```

Embedding 与 Reranker 也支持独立端点和 fallback。同一供应商域名下，Reranker
未单独配置 Key 时可安全复用 Embedding Key；跨域名不会复用。完整字段见
[`configs/config.example.yaml`](configs/config.example.yaml)。

## 主要 API

### 用户接口

```text
POST /api/v1/conversations
GET  /api/v1/conversations/{id}/messages
POST /api/v1/conversations/{id}/messages
POST /api/v1/conversations/{id}/messages:stream

GET  /api/v1/products
GET  /api/v1/products/{product_code}
GET  /api/v1/orders/{order_no}
POST /api/v1/after-sales/tickets
```

SSE 使用 `status`、`evidence`、`delta`、`done` 和 `error` 事件。创建售后工单必须显式确认，并提供幂等键。

### 管理接口

```text
POST /api/v1/admin/kb/documents
POST /api/v1/admin/kb/upload
POST /api/v1/admin/kb/search
GET  /api/v1/admin/traces/{trace_id}

POST /api/v1/admin/eval/runs
POST /api/v1/admin/eval/comparisons
GET  /api/v1/admin/eval/comparisons/{comparison_id}
GET  /api/v1/admin/eval/runs/{run_no}

GET  /api/v1/admin/prompts
POST /api/v1/admin/prompts/{scenario}/activate
POST /api/v1/admin/prompts/eval

GET  /api/v1/admin/circuit-breakers/status
POST /api/v1/admin/circuit-breakers/reset
GET  /api/v1/admin/metrics/agent
GET  /api/v1/admin/metrics/prometheus
```

## 评测与可观测性

重新生成内置 v2 数据集：

```powershell
go run ./cmd/eval-dataset -output docs/eval/eval-cases-v2.json
```

启动 200 条 Agentic 回归或 Naive RAG 对比：

```powershell
make eval-regression
make eval-compare
```

生成当前 MCP HTTP 链路的 200 条回归报告：

```powershell
make e2e-agentic-mcp-eval SYSTEM_VERSION=agentic-mcp-http-20260617
```

若服务已经启动，也可以只调用评测 API 并生成报告：

```powershell
make eval-regression-report SYSTEM_VERSION=agentic-mcp-http-20260617
```

定位特定 Bad Case 时可在 `POST /api/v1/admin/eval/runs` 中传入
`"case_ids":["EVAL-046","EVAL-047"]`，无需重复执行无关用例。

评测任务通过 API 异步执行，使用返回的 `run_no` 查询进度和结果。Trace 可记录意图、检索、工具、步骤、Token、延迟、模型和估算成本；Prometheus 接口输出请求、Token、工具和模型相关指标。

历史 100 条本地基线仅用于验证确定性链路；真实模型 200 条串行评测见
`docs/eval/llm-experiment-report.md`。当前 MCP HTTP 版本回归记录见
`docs/eval/mcp-regression-report.md`；真实模型或外部 MCP server 配置变化后，应使用新的
`system_version` 重新生成报告。大规模并发压测仍待执行。

## 项目结构

```text
.
├── cmd/                       # server、mcp-server、迁移、数据种子、评测与 Trace 分析
├── configs/                   # 本地配置、完整配置示例与 Skill 配置
├── internal/
│   ├── agent/                 # ReAct、Plan-and-Execute、Reflection、安全护栏
│   ├── api/                   # Gin API、SSE、管理接口
│   ├── eval/                  # 数据集、评测器、LLM Judge、对比与存储
│   ├── ingest/                # 多格式文档解析
│   ├── intent/                # 规则与 LLM 混合意图路由
│   ├── retriever/             # Hybrid Retrieval、RRF、缓存与重检
│   ├── skill/                 # 5 类业务工作流
│   ├── tool/                  # MCP 工具服务/客户端、Executor 与 6 个内置工具
│   └── trace/                 # Agent Trace
├── clean-care-frontend/       # React + TypeScript 演示控制台
├── docs/                      # 设计、ADR、评测、实验与性能文档
├── compose.yaml
├── Dockerfile
└── Makefile
```

## 验证

后端：

```powershell
go test ./...
go test -race ./...
go vet ./...
```

前端：

```powershell
Set-Location clean-care-frontend
npm run lint
npm run build
```

端到端链路：
```powershell
make e2e-agentic-mcp
```

GitHub Actions 会执行模块文件检查、后端测试与构建、前端 lint 与构建、Docker 镜像构建，以及基于 Docker Compose 的 Agentic MCP 端到端链路测试。

## 项目边界

- 动态价格、库存、订单和售后数据均为本地 mock 数据，工具结果和 Trace 会标记
  `data_scope=mock`，未接真实 ERP、支付或物流系统。
- 工具发现和调用已走 MCP `initialize`、`tools/list` / `tools/call` 抽象；默认使用进程内 MCP server，也可通过 Streamable HTTP、stdio 或多 server 聚合接入独立/外部 MCP server。当前内置工具仍使用本地业务数据，未接真实 ERP、支付或物流系统。
- React 界面是开发与演示控制台，不是具备完整 RBAC、OIDC 和审计能力的生产管理后台。
- Prompt 版本由进程内 Registry 管理，重启后不会持久化激活状态。
- Cross-Encoder Reranker 依赖外部兼容服务，仓库不内置模型推理服务。
- 尚未完成生产级 SLA、跨机房容灾、真实支付物流联调和大规模压力测试。

## 相关文档

- [系统设计](docs/clean-care-agentic-rag-design.md)
- [架构决策记录](docs/architecture-decisions.md)
- [项目完成度与边界](docs/project-status.md)
- [性能基准](docs/performance-benchmark.md)
- [MCP HTTP 回归评测记录](docs/eval/mcp-regression-report.md)
- [真实模型 200 条评测报告](docs/eval/llm-experiment-report.md)
- [历史评测报告](docs/eval/experiment-report.md)
- [面试说明](docs/interview-notes.md)
