# CleanCare Agent 前端可视化系统 — 构建指令

---

## 一、身份

你是一名资深全栈工程师，精通 React 18 + TypeScript 前端开发，对 AI Agent 系统的可视化有深入理解。

你的任务是：为 CleanCare Agentic RAG 后端构建一个**完整链路可视化前端**，不只是聊天界面，而是能把 Agent 内部的推理过程、工具调用、检索结果、反思质检全部透明展示出来的调试级前端。

---

## 二、任务

构建一个 React 18 + TypeScript 单页应用（SPA），具备以下四个核心模块：

1. **智能对话界面** — 用户与 Agent 交互的主界面，SSE 流式输出
2. **Agent 执行链路面板** — 实时展示意图识别→改写→规划→检索→工具→反思→生成的完整链路（核心亮点）
3. **Trace 详情查看器** — 查看历史请求的完整 Trace，包含每步耗时和 Token 消耗
4. **管理后台** — 知识库管理、商品查看、评测运行、指标监控

---

## 三、后端背景

### 3.1 后端技术栈

Go + Gin + MySQL + Redis + Qdrant + OpenAI-compatible LLM API

### 3.2 完整 API 清单

后端运行在 `http://localhost:8080`，所有 API 需要在路径前拼接 `/api/v1`。

#### 对话相关

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/conversations` | 创建会话。Body: `{"title":"扫地机器人选购"}` → Response: `{conversation_id, title, created_at}` |
| `GET` | `/conversations/:id/messages?limit=20` | 获取历史消息。Response: `{items:[{id,role,content,trace_id}], next_cursor}` |
| `POST` | `/conversations/:id/messages` | 发送消息（非流式）。Body: `{"content":"T20吸力多大?"}` → Response: `{message_id, answer, evidences:[], trace_id, mode}` |
| `POST` | `/conversations/:id/messages:stream` | 发送消息（SSE流式）。Body: `{"content":"..."}` → SSE 事件流 |

#### SSE 事件格式

后端通过 SSE 推送以下事件类型：

```
event: status
data: {"stage":"planned|retrieving|generating","mode":"naive_rag|agentic|direct|clarify","intent":"product_parameter","confidence":0.95,"trace_id":"tr_xxx"}

event: evidence
data: {"evidence_id":"E1","kind":"kb_chunk|tool_result","source_id":"chk_123","title":"T20核心参数表","content":"...","metadata":{...}}

event: delta
data: {"content":"T20 的额定吸力为"}

event: done
data: {"message_id":"msg_xxx","trace_id":"tr_xxx","finish_reason":"stop","mode":"agentic"}

event: error
data: {"code":"AGENT_TIMEOUT","message":"agent request timeout"}
```

**关键设计**：
- SSE 事件**乱序到达**：`status` 可能穿插在多个 `delta` 之间（比如先在 `planned` 阶段发一个 status，检索完后又发一个 `retrieved` 阶段的 status）
- `evidence` 事件在 `delta` 之前发送（先发证据，再发生成文本），或者在 Reflection 触发重新检索后追加发送
- `done` 事件是最终事件，收到后 SSE 流结束

#### Trace 相关

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/admin/traces/:trace_id` | 获取 Agent 执行 Trace。Response 结构如下 |

```json
{
  "code": "OK",
  "data": {
    "trace_id": "tr_xxx",
    "conversation_id": "cv_xxx",
    "intent": "product_parameter",
    "route_mode": "naive_rag",
    "plan": { "mode":"naive_rag","steps":[...] },
    "steps": [
      {
        "step_id": "step_01",
        "type": "retrieve",
        "status": "success",
        "duration_ms": 82,
        "metadata": {
          "action": "retrieve",
          "reason_code": "collect_focused_static_evidence",
          "result_count": 5
        }
      },
      {
        "step_id": "step_03",
        "type": "finish",
        "status": "success",
        "duration_ms": 1200,
        "metadata": {}
      }
    ],
    "tool_calls": [
      {
        "call_id": "call_xxx",
        "tool_name": "price_query",
        "arguments": {"product_refs":["T20"]},
        "result_summary": {"items":[{"model":"T20","current_price":3599}]},
        "status": "success",
        "error_code": "",
        "latency_ms": 45,
        "created_at": "2026-06-12T10:00:00Z"
      }
    ],
    "status": "success",
    "evidence_ids": ["E1","E2","E3"],
    "input_tokens": 850,
    "output_tokens": 320,
    "latency_ms": 2450,
    "finished_at": "2026-06-12T10:00:02Z"
  }
}
```

**Trace Step 的 type 枚举**：`retrieve` | `call_tool` | `run_skill` | `clarify` | `reflect` | `finish` | `answer_direct`

#### 商品/订单/售后

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/products?category=robot_vacuum` | 商品列表 |
| `GET` | `/products/:product_code` | 商品详情（含 SKU） |
| `GET` | `/orders/:order_no` | 订单详情 |
| `POST` | `/after-sales/tickets` | 创建售后工单 |

#### 管理功能

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/admin/kb/documents` | 上传知识库文档（multipart/form-data） |
| `POST` | `/admin/kb/search` | 搜索知识库。Body: `{"query":"...","mode":"hybrid","filter":{"models":["T20"]}}` |
| `POST` | `/admin/eval/runs` | 触发评测运行。Body: `{"dataset_version":"v1"}` |
| `GET` | `/admin/eval/runs/:run_no?include_failures=true` | 查询评测结果 |
| `GET` | `/admin/metrics/agent` | Agent 运行指标（请求量/成功率/P95延迟/Token消耗） |

#### 健康检查

| 方法 | 路径 |
|------|------|
| `GET` | `/health/live` |
| `GET` | `/health/ready` |

### 3.3 鉴权

所有 `/api/v1` 下的接口需要 Header：`Authorization: Bearer mock-jwt-demo-token`

开发模式下 `auth.enabled=false`，后端默认注入 `user_id=demo-user`。

### 3.4 Agent 执行链路（前端需要可视化的）

一个完整的 Agentic RAG 请求经过以下阶段：

```
用户提问
  ↓
① 意图识别（Intent Router）
  → 规则层快速过滤 + LLM分类兜底
  → 输出：一级意图、二级意图、置信度、实体
  ↓
② 查询改写（Query Rewriter）
  → 指代消解（"那个"→"P400"）
  → 子问题拆分
  → 术语归一化
  ↓
③ 计划生成（Planner）
  → 三种模式：NaiveRAG（简单）/ ReAct（复杂动态）/ PlanExecute（先规划再执行）
  → 输出：步骤列表 [Retrieve, CallTool, RunSkill, Reflect, Finish]
  ↓
④ 步骤执行（ReAct Loop）
  → 每步执行前检查：步数上限 / Token预算 / 重复检测 / 上下文取消
  → Retrieve：多路并行检索（Dense向量 + BM25关键词 → RRF融合 → Rerank精排）
  → CallTool：白名单校验 → JSON Schema参数验证 → 超时控制 → 幂等 → 审计日志
  → RunSkill：5个业务Skill内部多步编排
  ↓
⑤ 反思质检（Self-Reflection）
  → 规则检查（GroundingReflector）+ LLM 7维度检查
  → 不合格 → 重检 / 重生成 / 追问 / 转人工
  ↓
⑥ 流式输出（SSE）
  → evidence事件 → delta事件 → done事件
```

**前端需要把这个链路可视化**：实时展示当前处于哪个阶段、每个阶段的输入输出、每一步的耗时。

---

## 四、前端架构要求

### 4.1 技术选型

- React 18 + TypeScript（严格模式）
- 路由：React Router v6
- 状态管理：React Context + useReducer（不引入 Redux，项目规模不需要）
- HTTP：fetch API（不引入 axios，减少依赖）
- SSE：原生的 `EventSource` 或 `fetch` + `ReadableStream`（因为 EventSource 不支持 POST 和自定义 Header）
- UI 样式：Tailwind CSS 或纯 CSS（保持简洁专业，不要用 UI 组件库）
- 图标：简单的 SVG 内联或 unicode 符号
- 构建：Vite

### 4.2 页面结构

```
┌──────────────────────────────────────────────┐
│  Header: CleanCare Agent | 会话列表 | Admin  │
├──────────────────────────────────────────────┤
│                   主内容区                     │
│  ┌─────────────────────┐ ┌─────────────────┐ │
│  │                     │ │  执行链路面板    │ │
│  │    对话区            │ │  ┌───────────┐  │ │
│  │                     │ │  │ ① 意图识别  │  │ │
│  │  用户: ...          │ │  │   耗时 23ms │  │ │
│  │  Agent: ... [E1]    │ │  ├───────────┤  │ │
│  │                     │ │  │ ② 查询改写  │  │ │
│  │  输入框              │ │  │   耗时 15ms │  │ │
│  │                     │ │  ├───────────┤  │ │
│  │                     │ │  │ ③ 检索      │  │ │
│  │                     │ │  │   耗时 82ms │  │ │
│  │                     │ │  ├───────────┤  │ │
│  │                     │ │  │ ④ 生成      │  │ │
│  │                     │ │  │   耗时 1.2s │  │ │
│  │                     │ │  └───────────┘  │ │
│  └─────────────────────┘ └─────────────────┘ │
└──────────────────────────────────────────────┘
```

### 4.3 路由设计

```
/                      → 重定向到 /chat
/chat                  → 对话主界面（默认新建会话）
/chat/:conversationId  → 对话主界面（加载已有会话）
/admin                 → 管理后台首页（指标看板）
/admin/kb              → 知识库管理
/admin/eval            → 评测管理
/admin/traces/:traceId → Trace 详情页
```

---

## 五、四个核心模块详细要求

### 模块1：智能对话界面（页面主体）

#### 5.1.1 布局

左侧 60%：对话区
右侧 40%：执行链路面板（可折叠）

#### 5.1.2 对话区功能

- **会话列表**：左侧侧边栏或顶部下拉，列出已有会话，支持新建/切换
- **消息气泡**：
  - 用户消息：右对齐，浅色背景
  - Agent 消息：左对齐，白色背景，支持 Markdown 渲染（使用简单的 Markdown 解析库如 `marked` 或 `react-markdown`）
  - 证据引用 `[E1]` 需要渲染为可点击的标签，点击后高亮右侧面板中对应的证据条目
- **流式输出**：Agent 回复通过 SSE 逐字展示，模拟打字效果
- **输入框**：底部固定，支持 Enter 发送、Shift+Enter 换行，发送后显示加载状态
- **状态指示器**：发送消息后在对话区顶部显示当前状态条 — "正在分析意图..." → "正在检索知识库..." → "正在生成回答..."

#### 5.1.3 SSE 流式接入

SSE 不能用浏览器原生 `EventSource`（因为需要 POST + Authorization Header），请用 `fetch` + `ReadableStream` 手动解析：

```typescript
// SSE 流式请求伪代码
const response = await fetch(`/api/v1/conversations/${id}/messages:stream`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${token}`,
    'Accept': 'text/event-stream',
  },
  body: JSON.stringify({ content: userMessage }),
});

const reader = response.body!.getReader();
const decoder = new TextDecoder();
let buffer = '';

while (true) {
  const { done, value } = await reader.read();
  if (done) break;
  buffer += decoder.decode(value, { stream: true });
  // 按 \n\n 分割 SSE 事件，解析 event: xxx\ndata: xxx
  // 触发对应的状态更新
}
```

收到 SSE 事件后的处理逻辑：
- `event: status` → 更新右侧链路面板的阶段状态（如：标记"意图识别"阶段为 completed）
- `event: evidence` → 追加到证据列表，在右侧面板中展示
- `event: delta` → 追加到当前 AI 消息的内容末尾
- `event: done` → 标记对话完成，用 `trace_id` 获取完整 Trace
- `event: error` → 显示错误提示

### 模块2：Agent 执行链路面板（核心亮点）

这是一个**实时状态机可视化面板**，展示当前请求经历的完整链路。这是面试时最有区分度的部分。

#### 5.2.1 视觉设计

采用**纵向时间线**样式，每个阶段是一个节点：

```
┌─────────────────────────────────┐
│  Agent 执行链路  │ trace: tr_xx │
├─────────────────────────────────┤
│                                 │
│  ● 意图识别          23ms ✅   │
│  │  product_parameter           │
│  │  置信度 0.95                 │
│  │  ↓                          │
│  ● 查询改写          15ms ✅   │
│  │  原: T20吸力多大?           │
│  │  改: T20 扫地机器人 吸力    │
│  │  ↓                          │
│  ● 检索              82ms ✅   │
│  │  Hybrid Search              │
│  │  召回 5 条结果              │
│  │  ├─ [E1] T20 核心参数表    │
│  │  ├─ [E2] T20 商品详情      │
│  │  └─ [E3] FAQ: T20常见问题  │
│  │  ↓                          │
│  ● 反思质检          8ms ✅    │
│  │  证据覆盖: 完整             │
│  │  ↓                          │
│  ● 生成回答        1200ms ✅   │
│  │  input: 850t  output: 320t  │
│  │  ↓                          │
│  ● 完成 ✓                     │
│                                 │
└─────────────────────────────────┘
```

#### 5.2.2 阶段状态

每个节点有三种状态：
- **⏳ pending**（灰色，待执行）：步骤尚未开始
- **🔄 running**（蓝色，有脉冲动画）：当前正在执行
- **✅ success**（绿色）/ **❌ failed**（红色）/ **⚠️ degraded**（黄色）：步骤完成

#### 5.2.3 数据来源

面板数据分两个阶段获取：

**阶段1 — 实时**（SSE 流期间）：
- 根据 `event: status` 的阶段字段更新各节点的状态（如 `stage: "retrieving"` → "意图识别"和"查询改写"完成，"检索"运行中）
- 根据 `event: evidence` 追加证据到"检索"节点
- 根据 `event: delta` 的到达时间推算"生成"节点的耗时

**阶段2 — 完成**（SSE 流结束后）：
- 用 `trace_id` 调用 `GET /api/v1/admin/traces/:trace_id` 获取完整 Trace
- 用真实数据替换预估数据：精确的每步耗时、Token 消耗、工具调用详情
- 如果存在 `tool_calls`，在时间线中自动插入"工具调用"节点
- 如果存在 `warnings`，在对应节点显示警告标记

#### 5.2.4 交互

- 点击节点展开/折叠该阶段的详细信息
- 点击证据条目 `[E1]` 滚动到对话区对应引用位置
- 鼠标悬停节点显示详细 Metadata（如检索的 DenseScore、KeywordScore、RerankScore）
- "工具调用"节点展开后显示入参和返回值

### 模块3：Trace 详情查看器

当用户在对话中点击 `trace_id` 或者在管理后台输入 Trace ID 时，进入全屏 Trace 详情页。

#### 5.3.1 页面布局

```
┌─────────────────────────────────────────────────────┐
│  ← 返回    Trace: tr_xxx    Status: ✅ success      │
├─────────────────────────────────────────────────────┤
│                                                      │
│  概览卡片                                            │
│  ┌────────┬────────┬────────┬────────┬────────┐    │
│  │ 意图   │ 模式   │ 总耗时 │ Tokens │ 证据数 │    │
│  │product │naive_  │ 2450ms │ 850/320│   5    │    │
│  │_param  │rag     │        │        │        │    │
│  └────────┴────────┴────────┴────────┴────────┘    │
│                                                      │
│  执行步骤时间线 (同模块2的样式，但是全量精确数据)    │
│  ┌─────────────────────────────────────────────┐    │
│  │ ● step_01  retrieve      82ms  ✅           │    │
│  │   reason: collect_focused_static_evidence    │    │
│  │   result_count: 5                            │    │
│  │                                              │    │
│  │ ● step_02  reflect        8ms  ✅           │    │
│  │                                              │    │
│  │ ● step_03  finish      1200ms  ✅           │    │
│  └─────────────────────────────────────────────┘    │
│                                                      │
│  工具调用记录（如有）                                │
│  ┌─────────────────────────────────────────────┐    │
│  │ price_query  45ms ✅                         │    │
│  │ 入参: {"product_refs":["T20"]}               │    │
│  │ 返回值: {"items":[{"model":"T20","price":..}]}│    │
│  └─────────────────────────────────────────────┘    │
│                                                      │
│  证据列表                                            │
│  ┌─────────────────────────────────────────────┐    │
│  │ [E1] kb_chunk  T20核心参数表                 │    │
│  │ [E2] kb_chunk  T20商品详情                   │    │
│  └─────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

### 模块4：管理后台

#### 5.4.1 指标看板（/admin）

- 4 个指标卡片：总请求数、成功率、P95 延迟、平均 Token 消耗
- 数据来源：`GET /api/v1/admin/metrics/agent`

#### 5.4.2 知识库管理（/admin/kb）

- 文档上传区域（拖拽上传，支持 .json/.md/.txt）
- 已上传文档列表
- 搜索测试面板：输入 query → 展示检索结果（含分数、文档类型、分块内容）
- 数据来源：`POST /api/v1/admin/kb/search`

#### 5.4.3 评测管理（/admin/eval）

- 触发评测运行按钮
- 评测运行列表（历史运行记录）
- 评测结果详情：各指标得分、Bad Case 列表、失败分类统计
- 数据来源：`POST /api/v1/admin/eval/runs` + `GET /api/v1/admin/eval/runs/:run_no`

---

## 六、输出标准

### 6.1 代码质量

1. TypeScript 严格模式（`strict: true`）
2. 所有 API 调用封装在 `src/api/` 目录下，每个资源一个文件（如 `conversations.ts`、`traces.ts`）
3. 自定义 Hook 封装业务逻辑（如 `useSSEStream.ts`、`useTrace.ts`、`usePipeline.ts`）
4. 组件拆分合理：页面组件（`pages/`）、业务组件（`components/`）、通用 UI 组件（`components/ui/`）
5. 错误处理完善：每个 API 调用都有 try-catch，网络错误和业务错误分别处理
6. 加载状态：每个异步操作都有 loading 状态对应 UI

### 6.2 用户体验

1. SSE 流断开时自动重连（最多 3 次）
2. 会话切换时自动取消进行中的 SSE 请求
3. 页面刷新后恢复当前会话
4. Agent 执行链路面板在 SSE 流期间实时更新，流结束后自动拉取 Trace 数据替换
5. 对话历史滚动加载（向下滚动到顶部时加载更早的消息）

### 6.3 视觉风格

- **专业、干净**：白色为主色调，蓝色为主题色
- **信息密度适中**：不要花哨的动画，数据展示清晰第一
- **响应式**：在 1920px 和 1366px 宽度下都能良好展示
- **链路面板的卡片使用微妙阴影和圆角**，节点连线用 SVG 线条
- **不建议使用 UI 组件库**：手写 CSS 以保持轻量和可控

---

## 七、约束

1. **纯前端**：不要引入后端代码，不要做 BFF 层，直接调用 Go 后端 API
2. **轻量依赖**：只用 React + React Router + 一个 Markdown 渲染库（`marked` 或 `react-markdown`），不引入 Ant Design / MUI / Chakra 等重型组件库
3. **单端口部署**：开发时前端 `npm run dev` 在 5173 端口，通过 Vite proxy 代理 `/api` 到 `localhost:8080`
4. **不修改后端 API**：前端必须适配现有 API 格式，不能要求后端改动
5. **开发模式优先**：`auth.enabled=false`，前端固定使用一个 demo 用户
6. **所有中文文本使用前端硬编码**，不做 i18n
7. **SSE 解析必须健壮**：处理不完整事件（一个事件跨两个 chunk）、事件顺序与预期不符的情况

---

## 八、示例

### 示例1：SSE 流解析 Hook 的骨架

```typescript
// src/hooks/useSSEStream.ts

type SSEEventHandler = {
  onStatus?: (data: StatusEvent) => void;
  onEvidence?: (data: EvidenceEvent) => void;
  onDelta?: (content: string) => void;
  onDone?: (data: DoneEvent) => void;
  onError?: (error: SSEErrorEvent) => void;
};

function useSSEStream() {
  const [isStreaming, setIsStreaming] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const startStream = useCallback(async (
    conversationId: string,
    content: string,
    handlers: SSEEventHandler
  ) => {
    setIsStreaming(true);
    const controller = new AbortController();
    abortRef.current = controller;

    try {
      const response = await fetch(
        `/api/v1/conversations/${conversationId}/messages:stream`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer mock-jwt-demo-token`,
            'Accept': 'text/event-stream',
          },
          body: JSON.stringify({ content }),
          signal: controller.signal,
        }
      );

      const reader = response.body!.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || ''; // 最后一个可能不完整，保留到下次
        
        // 解析 SSE 事件: event: xxx\ndata: xxx\n\n
        let currentEvent = '';
        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7);
          } else if (line.startsWith('data: ')) {
            const data = JSON.parse(line.slice(6));
            switch (currentEvent) {
              case 'status': handlers.onStatus?.(data); break;
              case 'evidence': handlers.onEvidence?.(data); break;
              case 'delta': handlers.onDelta?.(data.content); break;
              case 'done': handlers.onDone?.(data); break;
              case 'error': handlers.onError?.(data); break;
            }
          }
        }
      }
    } catch (err) {
      if ((err as Error).name !== 'AbortError') {
        handlers.onError?.({ code: 'NETWORK_ERROR', message: (err as Error).message });
      }
    } finally {
      setIsStreaming(false);
    }
  }, []);

  const abort = useCallback(() => {
    abortRef.current?.abort();
  }, []);

  return { startStream, abort, isStreaming };
}
```

### 示例2：执行链路面板节点的 React 组件骨架

```typescript
// src/components/PipelinePanel.tsx

type StepStatus = 'pending' | 'running' | 'success' | 'failed' | 'degraded';

interface PipelineStep {
  id: string;
  name: string;          // "意图识别" | "查询改写" | "检索" | "工具调用" | "反思质检" | "生成回答"
  status: StepStatus;
  durationMs?: number;
  detail?: string;       // 如 "product_parameter, confidence 0.95"
  subItems?: { label: string; content: string }[];  // 如证据列表
  metadata?: Record<string, unknown>;
}

function PipelinePanel({ steps }: { steps: PipelineStep[] }) {
  return (
    <div className="pipeline-panel">
      <h3>Agent 执行链路</h3>
      <div className="timeline">
        {steps.map((step, i) => (
          <div key={step.id} className={`timeline-node ${step.status}`}>
            <div className="node-indicator">
              {step.status === 'running' && <span className="pulse" />}
              {step.status === 'success' && <span className="check">✓</span>}
              {step.status === 'failed' && <span className="cross">✗</span>}
              {step.status === 'pending' && <span className="dot" />}
            </div>
            <div className="node-content">
              <div className="node-header">
                <span className="node-name">{step.name}</span>
                {step.durationMs && <span className="node-duration">{step.durationMs}ms</span>}
              </div>
              {step.detail && <div className="node-detail">{step.detail}</div>}
              {step.subItems && (
                <div className="node-subitems">
                  {step.subItems.map((item, j) => (
                    <div key={j} className="subitem">
                      <span className="subitem-label">{item.label}</span>
                      <span className="subitem-content">{item.content}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
            {i < steps.length - 1 && <div className="node-connector" />}
          </div>
        ))}
      </div>
    </div>
  );
}
```

### 示例3：Vite 代理配置

```typescript
// vite.config.ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
});
```

---

## 九、检查规则（完成后逐条自检）

### 功能完整性
- [ ] 能否创建新会话并发送消息？
- [ ] SSE 流式输出是否正常（逐字展示）？
- [ ] 执行链路面板是否实时显示各阶段状态？
- [ ] SSE 流结束后是否自动拉取 Trace 更新面板数据？
- [ ] 点击 `[E1]` 引用能否定位到对应证据？
- [ ] Trace 详情页能否正确展示所有步骤、工具调用、Token 消耗？
- [ ] 管理后台指标看板数据是否正常加载？

### 鲁棒性
- [ ] SSE 连接断开后是否会提示用户？
- [ ] 切换会话时是否取消了上一个 SSE 请求？
- [ ] 后端返回错误时前端是否有友好提示（而非白屏）？
- [ ] 空状态（无会话、无消息、无Trace数据）是否有合适的 UI？

### 代码质量
- [ ] `npm run build` 无 TypeScript 错误？
- [ ] 无 console 中的未处理 Promise rejection？
- [ ] API 调用是否有统一的错误处理？
- [ ] 组件是否有合理的拆分（单个组件不超过 200 行）？

### 视觉
- [ ] 对话区的 Markdown 渲染是否正确？
- [ ] 链路面板的节点动画是否流畅？
- [ ] 移动端宽度下是否至少对话区可用？
