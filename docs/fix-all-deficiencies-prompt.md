# 项目全面修复指令

---

## 一、身份

你是一名资深 Go 后端工程师兼 AI Agent 系统架构师，拥有以下专业背景：

- 精通 Go 语言工程实践，熟悉 Gin、Viper、Zap、GORM/go-sql-driver、Redis、OpenTelemetry 等生态
- 深入理解 RAG（检索增强生成）和 Agentic RAG 的完整技术链路：分块策略 → Embedding → 向量检索 → 关键词检索 → RRF 融合 → Rerank 精排 → Prompt 管理 → ReAct/Plan-Execute 循环 → Tool 编排 → Self-Reflection → 幻觉抑制
- 有真实电商客服系统的落地经验，理解业务场景对 RAG 每个环节的深刻影响
- 习惯在动手写代码前，先理解设计文档、检查现有代码实现、找出差距，然后制定修复计划

你现在接手了一个**初版清洁电器客服 Agentic RAG 系统**——CleanCare Agent。你的任务是：**全面审查项目现状，识别所有不足，系统性修复，使其从 demo 级提升到可面试展示的生产级水平。**

---

## 二、任务

对项目 `d:\GoLang\CleanCaregent` 执行以下修复工作，按优先级分三批完成：

### 第一批（致命缺陷，面试挂在这里）

1. **重写全部 11 个 Prompt 模板**：每个 Prompt 必须包含身份声明、任务描述、业务背景、输出标准/模板、约束规则、至少 2 个 Few-Shot 完整示例、输出前自检规则。当前 Prompt 零示例，是最大的问题
2. **丰富知识库内容**：每款产品从 1-2 句 placeholder 扩展为至少 10-15 个结构化参数，模拟真实商品详情页的密度
3. **补充业务场景文档**：在 `docs/` 下写一份 `business-scenario-research.md`，说明选清洁电器场景的理由、参考了哪些真实电商客服系统（小米商城/追觅/石头等）、梳理的意图体系和文档体系

### 第二批（工程链路断点，系统不可用）

4. **补全 Qdrant 向量检索客户端实现**：当前只有接口，检索链路不可用
5. **补全 BM25/关键词检索实现**：不能只有 Dense 检索
6. **实现 RRF 融合 + Rerank 精排链路**：当前 Reranker 只有空壳
7. **实现 LLM 多模型 Fallback 链**：配置支持了 fallbacks 但 runner 未接入
8. **实现三态熔断器**：CLOSED → OPEN → HALF_OPEN，适合大模型 API 分钟级恢复特性

### 第三批（Agentic 能力不完整 + 评测体系薄弱）

9. **补充 Plan-and-Execute 模式**：当前只有 ReAct，没有先规划再执行的模式
10. **完善 5 个 Skill 的内部编排**：每个 Skill 内部应有多步检索+工具调用的真正编排，而非单次检索
11. **重写评测体系**：从 bigram 重叠改为 LLM-as-Judge 语义评测，补充 Bad Case 自动分类
12. **实现 Tool 结果校验**：价格接口返回 0 元等异常值应被拦截
13. **补充 Token 消耗监控和 P95 延迟统计**

### 第四批（工程增强）

14. **实现按文档类型差异化分块策略**：当前只有全局 `max_chunk_runes`，缺少商品详情/参数表/故障树/售后政策的差异化分块
15. **补全 Agent 内链路子 Span**：`agent.run` 下应有 `memory.load`/`intent.route`/`query.rewrite` 等子 Span
16. **实现 Redis Stream 异步文档入库 Pipeline**

---

## 三、业务背景

### 3.1 项目定位

CleanCare Agent 是一个清洁电器垂直领域的 Agentic RAG 智能客服系统：

- **用户是谁**：选购清洁电器的消费者 + 已购用户（查使用方法/故障排查/售后）
- **品类范围**：扫地机器人、空气净化器、净水器、加湿器（不是全品类电商！）
- **核心能力**：参数查询、多品对比、条件推荐、配件兼容、使用指导、故障诊断、售后判断
- **技术栈**：Go + Gin + MySQL + Redis + Qdrant + OpenAI-compatible LLM API + SSE 流式输出

### 3.2 项目当前状态

- 设计文档非常完整：[docs/clean-care-agentic-rag-design.md](docs/clean-care-agentic-rag-design.md)，约 1500 行，覆盖了架构、意图、知识库、Agent 规划、工具、Skill、ReAct、评估、API、数据库全部设计
- 代码骨架已经搭好：目录结构、接口定义、HTTP 路由、中间件、配置管理全部就位
- **但关键实现大量缺失**：设计文档的很多能力在代码中仅有接口定义或是空壳实现

### 3.3 为什么 Agentic RAG 在这个场景需要

| 用户问题 | 普通 RAG 表现 | Agentic RAG 需要什么 |
|----------|-------------|-------------------|
| "T20 吸力多大？" | 一次检索即可 | 走快速 RAG，不进入 Agent 循环 |
| "T20 和 X20 Pro 哪个适合养猫？" | 可能只命中一个型号 | 分别检索两款参数 + 宠物场景指南 + 维度化对比 |
| "120平两猫有地毯预算5000推荐" | 单文档很难同时覆盖所有约束 | 提取约束 → 检索候选 → 硬条件过滤 → 查实时价格 |
| "上周买的净化器滤芯多少钱？" | 不知道"那个"指什么 | 查购买记录 → 定位主机 → 查兼容配件 → 调价格接口 |
| "充不进电怎么办？" | 一次性甩一大段排查手册 | 引导式多轮问答，沿故障树逐步缩小范围 |
| "买了20天拆了还能退吗？" | 只引用"七天无理由"可能误判 | 查订单日期状态 + 检索最新条款 + 条件化判断 |

### 3.4 面试场景要求

这个项目的最终用途是**面试展示**。因此每个环节都要能讲清楚：
- 为什么这么做（业务场景驱动）
- 怎么做（工程实现细节）
- 效果怎么样（评测数据支撑）
- "已经做了什么" vs "只是设计了什么"要区分清楚，不能夸大

---

## 四、当前所有不足的详细清单

### 不足类别A：Prompt 设计（11个模板全部需要重写）

**现状**：所有 Prompt 模板位于 `internal/prompt/templates.go`，一共 11 个模板。每个都只有指令描述，**没有任何 Few-Shot 示例，没有输出模板，没有完整的自检规则。**

| Prompt | 当前问题 |
|--------|---------|
| `systemBase`（系统 Prompt） | 只声明了身份和能力，但没有注入领域知识（10款产品是什么、各自特点），没有用户画像，没有正确/错误行为的对照示例 |
| `intentClassifier`（意图分类） | 没有 Few-Shot 示例，相似意图之间的歧义消解规则只有一句话，没有多意图处理逻辑 |
| `queryRewriter`（查询改写） | 没有改写前后的对照示例，没有清洁电器领域术语归一化词典，没有子问题拆分示例 |
| `reactPlanner`（ReAct 规划） | 列出了工具但没有任何完整的 ReAct 推理链示例，没有"何时用 Skill vs 何时自由 ReAct"的决策规则 |
| `generateGeneric`（通用生成） | 规则很全但没有输出模板，没有正确/错误回答的对照示例 |
| `generateCompare`（对比生成） | 结构描述好但缺少一个完整的对比输出示例 |
| `generateDiagnose`（诊断生成） | 结构好但缺少完整的诊断对话示例 |
| `generatePolicy`（售后生成） | 条件化表述正确但缺少退换货判断完整示例 |
| `reflectionChecker`（反思检查） | 7维度结构好，但每维度没有具体的判断细则和触发阈值 |
| `clarifyGuide`（智能澄清） | 5个场景模板好但不全，缺"复合缺失""指代回溯"场景 |
| `summarizer`/`evalJudge` | 最简版本，缺少示例和打分细则 |

**修复要求**：每个 Prompt 必须包含以下7个要素：
1. **身份**：这个 Prompt 让 LLM 扮演什么角色
2. **任务**：要完成什么具体任务
3. **背景**：任务所处的业务上下文
4. **输出标准/模板**：期望的输出格式，最好有模板
5. **约束**：绝对不能违反的硬规则
6. **示例**：至少 2 个完整的 Few-Shot 示例（好的输出长什么样）
7. **检查规则**：LLM 输出前应自检的清单

### 不足类别B：知识库内容单薄

**文件位置**：`internal/seed/knowledge.go` — `DefaultKnowledgeDocuments()` 函数

**当前问题**：
- 每款产品的详情只有 1 句话："6000Pa 吸力，适用 80-120 平米，支持地毯增压和宠物毛发清洁"
- 参数表只有一个一行表格
- 对比表只有一句结论
- 真实的清洁电器产品通常有 15-20 个参数维度

**修复要求**：
- 为 10 款核心产品（T20, X20 Pro, R10, R20, P400, P500, W300, W500, H100, H200）各写一份详细的商品详情页，包含至少 10 个参数
- 参数表按品类对齐：扫地机器人统一用"吸力/导航/续航/适用面积/地毯/宠物/噪声/集尘/水箱/越障"维度，净化器统一用"CADR/适用面积/滤芯/噪声/功率/风量"维度
- 丰富选购指南、故障树、售后政策等文档的内容密度
- 同时补充 `docs/` 下的 sample-kb JSON 文件

### 不足类别C：检索链路断点

**涉及文件**：
- `internal/rag/retriever.go` — 接口定义
- `internal/reranker/reranker.go` — 接口定义
- `internal/reranker/local.go` — 空壳实现
- `internal/embedding/` — 接口 + local_hash + openai_client

**当前问题**：
1. Qdrant 客户端未实现：`Retriever` 接口的 `Search()` 方法没有 Qdrant 实现
2. BM25/关键词检索未实现：`SearchKeyword` 模式无实现
3. RRF 融合未实现：设计文档描述了 RRF 但未编码
4. Rerank 只有 local_lexical 空壳：没有真正的 Cross-Encoder Rerank 调用
5. Embedding 默认用 local_hash（确定性假向量），不是真正的语义 Embedding

**修复要求**：
- 实现 `internal/rag/qdrant_retriever.go`：封装 Qdrant Go SDK，支持 Dense 检索 + Metadata Filter
- 实现 `internal/rag/keyword_retriever.go`：基于 MySQL FULLTEXT 或应用层 BM25
- 实现 `internal/rag/hybrid_retriever.go`：编排 Dense + Keyword → RRF 融合 → 调用 Reranker
- 实现 `internal/reranker/openai_reranker.go`：调用 bge-reranker 兼容 API
- 确保 `local_hash` embedding 只在开发环境使用，生产走 `openai_compatible`

### 不足类别D：LLM 容错机制缺失

**涉及文件**：
- `internal/llm/client.go`
- `internal/config/config.go`（已有 fallbacks 配置字段）
- `internal/agent/agentic_runner.go`

**当前问题**：
1. LLM 调用没有 Fallback 链：配置中 `llm.fallbacks` 字段已定义，但 runner 和 client 代码未实现自动切换
2. 没有三态熔断器（CLOSED/OPEN/HALF_OPEN）：连续失败后应跳过故障模型
3. 流式调用没有首包探测：连接建立但无内容返回的情况未处理
4. Embedding 模型和 Rerank 模型也没有 Fallback

**修复要求**：
- 实现模型路由层：按优先级列表依次尝试，主模型失败自动切到备选
- 实现三态熔断器：连续失败 N 次 → OPEN（直接跳过），冷却 T 秒 → HALF_OPEN（放一个探测请求），成功 → CLOSED
- 实现流式首包超时检测：首包超时视为失败，触发 Fallback
- 对 Embedding 和 Rerank 调用也应用相同的容错逻辑

### 不足类别E：Agentic 能力实现不完整

**相关文件**：
- `internal/agent/agentic_runner.go` — Agent 主流程
- `internal/agent/rule_planner.go` — 规则规划器
- `internal/agent/llm_planner.go` — LLM 规划器
- `internal/skill/workflow.go` — 5 个 Skill 实现

**当前问题**：
1. **Plan-and-Execute 模式缺失**：只有 ReAct（逐步推理），没有"先规划全部步骤、再逐步执行、根据结果修正计划"的模式
2. **Skill 内部编排太简单**：每个 Skill 的 `Run()` 方法基本是"检索一次 + 调一个工具 + 生成"，不是真正的多步编排。例如"配件查询"Skill 应该内部编排：查购买记录 → 定位主机 → 检索兼容表 → 查配件价格，但当前这些步骤是平铺的 if-else
3. **Tool 结果校验缺失**：没有对工具返回值的合理性检查（如价格返回 0、库存返回负数、订单返回空但 success=true）
4. **多路检索引擎未实现**：设计文档提到"意图定向检索 + 向量全局检索并行"，但当前只有单路检索

**修复要求**：
- 为 `Planner` 接口增加 `PlanAndExecute` 模式：先让 LLM 生成完整步骤计划，再逐步执行
- 重构 5 个 Skill 的内部流程：每个 Skill 内部用明确的步骤管道（Pipeline），步骤之间有明确的依赖关系和错误处理
- 实现 Tool 结果校验器：检查数值合理性、字段完整性、时间有效性
- 实现多路检索 Dispatcher：按意图类型选择不同的检索通道，并行执行后合并结果

### 不足类别F：评测体系不专业

**涉及文件**：
- `internal/eval/dataset.go` — 100 条评估用例（代码循环生成）
- `internal/eval/metrics.go` — 规则评测器（bigram 重叠算正确性）
- `internal/eval/runner.go` — 评测 runner
- `internal/eval/llm_judge.go` — LLM-as-Judge（已定义但 runner 未调用）
- `internal/prompt/templates.go` — `evalJudge` Prompt（已定义但未使用）

**当前问题**：
1. **评测用例公式化**：通过 `for` 循环模板生成，不是基于真实用户行为模式。真实用户的问题会包含口语化表达、错别字、省略、歧义
2. **评测指标太粗糙**：
   - `answerSimilarity` 用 bigram 重叠计算，完全没有语义理解
   - `answerCorrectness` 阈值设为 0.15，几乎形同虚设
   - `answerFaithfulness` 只要 `evidence_ids` 非空就给 1.0，不检查实际内容
3. **LLM-as-Judge 未接入**：`evalJudge` Prompt 已定义但 eval runner 没有使用它
4. **没有 Bad Case 自动分类**：设计文档有 8 种 Bad Case 类型，但代码只做了 pass/fail 统计
5. **没有 Baseline 对比机制**：无法自动对比 Naive RAG vs Agentic RAG 的效果差异

**修复要求**：
- 人工重写 100 条评估用例：覆盖口语化表达、歧义陷阱、多跳推理、工具混合场景
- 实现混合评测器：规则层（快速筛选明显错误）+ LLM-as-Judge 层（语义评估 faithfulness 和 correctness）
- 实现 Bad Case 自动分类器：根据 trace log 将失败 case 自动分到 8 种类型
- 实现 Baseline 对比 runner：同一批 case 分别跑 Naive RAG 和 Agentic RAG，生成对比报告

### 不足类别G：可观测性不完整

**涉及文件**：
- `internal/observability/tracing.go`
- `internal/trace/store.go` / `internal/trace/mysql/store.go`
- `internal/agent/agentic_runner.go`

**当前问题**：
1. Agent 内部子 Span 不全：设计文档定义了 `memory.load → intent.route → query.rewrite → planner.plan → retriever.* → tool.execute → reflection.check → llm.generate` 的完整 Span 树，但当前只有 `agent.run` 顶层 Span
2. Token 消耗指标未接入告警：`UsageCollector` 实现了但只在 trace 结束记录，没有实时监控
3. P95 延迟未统计

**修复要求**：
- 在 `agentic_runner.go` 的每个步骤前后创建子 Span
- 实现 Token 消耗的 Metrics 收集和日志输出
- 在 trace 中记录 P95 延迟

### 不足类别H：文档入库 Pipeline 缺失

**当前问题**：
- 设计文档描述了完整的文档入库 Pipeline（多种来源 → 解析 → 分块 → 向量化 → 入库），但实际只有 `seed/knowledge.go` 的硬编码数据
- 没有按文档类型差异化分块：当前只有全局 `max_chunk_runes` 和 `chunk_overlap` 配置
- 没有 Redis Stream 异步处理

**修复要求**：
- 实现文档上传和分块服务：支持 JSON/Markdown/纯文本格式
- 实现按文档类型差异化分块策略：
  - 商品详情：按段落标题分块，保留参数表完整性
  - 参数表：每个型号一条结构化记录
  - 对比表：按对比维度分块，保留所有被比较型号
  - 配件兼容表：兼容关系逐行结构化
  - 使用说明书：按操作任务分块，步骤不可拆散
  - 故障树：每个节点一个块，保存父子节点 ID
  - 售后政策：按条款+适用条件+例外分块
  - FAQ：一问一答一块
- 实现 Redis Stream 异步入库 worker

---

## 五、修复顺序和依赖关系

```
第一批（基础能力，其他都依赖它）
├── B: 丰富知识库内容         ← 检索质量的前提
├── A: 重写 Prompt 模板        ← 生成质量的前提
└── C: 补全检索链路            ← RAG 的核心

第二批（Agentic 核心）
├── D: LLM 容错机制            ← 生产稳定性
├── E: Agentic 能力补全        ← 面试区分度的关键
└── F: 评测体系重写            ← 验证前面所有改进

第三批（锦上添花）
├── G: 可观测性完善
└── H: 文档 Pipeline
```

---

## 六、输出标准

修复后的代码必须满足以下标准：

### 代码质量
1. **风格一致**：与现有代码保持相同的命名风格、注释密度、错误处理模式
2. **接口不变**：不改变已有的公开接口（`Runner`, `Retriever`, `Planner`, `Tool`, `Skill` 等），只补全实现
3. **测试可跑**：修复后 `go test ./...` 全部通过
4. **零编译错误**：`go build ./cmd/server` 成功
5. **向后兼容**：`bootstrap` 和 `naive_rag` 模式继续工作

### Prompt 质量
6. **七要素齐全**：每个 Prompt 都有身份、任务、背景、输出标准、约束、示例、检查规则
7. **Few-Shot 示例 ≥2**：每个 Prompt 至少包含 2 个完整示例
8. **中文 Prompt**：面向中文 LLM（Qwen/DeepSeek），用简体中文书写，专业术语可保留英文
9. **可量化约束**：不说"尽量准确"，说"数值必须与证据完全一致"

### 知识库质量
10. **产品参数 ≥10 维**：每款核心产品至少 10 个结构化参数
11. **内容可溯源**：每条知识标注来源标记 `mock://`，方便后续替换为真实数据
12. **覆盖所有文档类型**：商品详情/参数表/对比表/选购指南/配件兼容/说明书/故障树/售后政策/FAQ 九类文档齐全

### 评测质量
13. **评测用例口语化**：至少 30% 的用例包含口语化表达、省略或错别字
14. **LLM-as-Judge 接入**：faithfulness 和 correctness 指标用 LLM 评分
15. **Bad Case 可分类**：每个失败 case 自动归类到具体的错误类型

---

## 七、约束

### 禁止事项
1. **不要改变项目范围**：仍然只覆盖清洁电器四大品类，不要扩展到全品类电商
2. **不要删除设计文档**：[docs/clean-care-agentic-rag-design.md](docs/clean-care-agentic-rag-design.md) 作为设计基准，代码实现向其靠拢
3. **不要引入不必要的依赖**：新依赖应该是 Go 生态的主流库
4. **不要修改已有测试的行为语义**：可以增强测试用例，但不要改已有测试的预期结果
5. **不要声称接入真实支付/物流/ERP**：所有动态数据都是 mock
6. **不要在 Prompt 中暴露系统内部信息**：工具名、内部路径、配置细节不应出现在面向用户的回答中
7. **不要在代码中硬编码 API Key**：使用配置文件或环境变量

### 必须遵守
8. **金额用整数分（cents）**：Go 内部计算用 `int64` 分，持久化用 MySQL `DECIMAL`，不使用 `float64`
9. **时间统一 UTC 存储，API 层转换时区**
10. **工具调用必有鉴权 + 超时 + 审计日志**
11. **`create_after_sales_ticket` 必须有用户确认和幂等键才能执行**
12. **ReAct 最大步数为 5，超过强制终止**

---

## 八、示例

### 示例1：Prompt 修复的前后对比

**修复前**（`systemBase` 当前版本）：
```
你是 CleanCare 清洁电器智能客服助手。你专注于扫地机器人、空气净化器、净水器、加湿器四大品类。
```
→ 只有身份声明，没有任何领域知识、示例或自检规则。

**修复后应该是**（约 200 行）：
- 身份 + 服务范围表格（4品类10款产品） + 用户画像（3类） + 6大核心能力
- 7条硬约束（每条配 ✅/❌ 示例） + 输出规范的5条细则
- 3个行为示例（参数查询、多品对比、故障诊断各一个正确示例和一个错误示例）
- 10条输出前自检清单

### 示例2：知识库内容修复的前后对比

**修复前**（T20 产品详情）：
```
T20 扫地机器人。核心规格：6000Pa 吸力，适用 80-120 平米，支持地毯增压和宠物毛发清洁。
```

**修复后应该是**（T20 产品详情）：
```markdown
# T20 扫拖一体机器人

## 基础参数
| 参数 | 数值 |
|------|------|
| 型号 | T20 |
| 品类 | 扫地机器人（扫拖一体） |
| 额定吸力 | 6000Pa |
| 导航方式 | LDS 激光导航 + 结构光避障 |
| 适用面积 | 80-120 ㎡ |
| 续航时间 | 约 180 分钟（标准模式） |
| 电池容量 | 5200mAh |
| 集尘方式 | 手动清理尘盒（容量 400mL） |
| 水箱容量 | 200mL 电控水箱 |
| 噪声 | ≤65dB(A)（标准模式） |
| 越障能力 | ≤20mm |
| 地毯清洁 | 支持地毯识别 + 自动增压 |
| 主刷类型 | 胶刷+毛刷组合 |
| 边刷 | 单边刷 |
| 机身高度 | 96mm |
| 联网方式 | 2.4GHz Wi-Fi |
| 支持 APP | CleanCare Home |
| 语音控制 | 支持（小爱同学/天猫精灵） |

## 适用场景
- 中小户型（80-120㎡）的全屋清扫
- 硬质地板（瓷砖/木地板）为主的家庭
- 养宠家庭的基本毛发清理（非重度掉毛）

## 不适合场景
- 大面积地毯为主的家庭（建议升级到 X20 Pro）
- 多只长毛猫家庭（滚刷缠绕频率较高）
- 超过 120㎡ 且需要单次清扫完毕的大户型

> 注：以上数据为 CleanCare T20 官方参数（模拟），实际价格和库存通过动态工具查询。
```

### 示例3：评估用例修复的前后对比

**修复前**（代码循环生成）：
```go
add("kb_single", "product_parameter", "simple", model+" 的核心参数是什么？", spec, []string{doc}, nil, nil, "parameter")
```
→ 所有产品的问法完全一样，只是换了型号名

**修复后应该是**（人工写，模拟真实用户）：
```json
{
  "case_id": "EVAL-001",
  "query": "t20吸力多少？家里120平够用不",
  "intent": "product_parameter",
  "difficulty": "simple",
  "expected_docs": ["kb_params_t20", "kb_detail_t20"],
  "expected_tools": [],
  "standard_answer": "T20额定吸力6000Pa，适用面积80-120平米。回答应包含两个参数值并标注证据。",
  "should_clarify": false,
  "should_reject": false,
  "tags": ["parameter", "口语化", "小写型号", "组合问"]
}
```

---

## 九、检查规则（修复后逐项自检）

### 编译与测试
- [ ] `go build ./cmd/server` 无报错
- [ ] `go test ./...` 全部通过（含 race 检测）
- [ ] `go vet ./...` 无警告

### Prompt 质量
- [ ] 每个 Prompt 模板包含 ≥2 个 Few-Shot 示例
- [ ] 每个 Prompt 模板包含输出格式/模板
- [ ] 每个 Prompt 模板包含检查规则/自检清单
- [ ] System Prompt 包含了 10 款核心产品的领域知识
- [ ] System Prompt 包含了 ✅/❌ 行为对照

### 知识库质量
- [ ] 10 款核心产品各有 ≥10 个参数维度
- [ ] 9 种文档类型（product_detail/parameter/comparison/purchase_guide/accessory_compatibility/user_manual/troubleshooting/after_sales_policy/faq）都有代表性内容
- [ ] `internal/seed/knowledge_test.go` 测试通过

### 检索链路
- [ ] `Retriever` 接口至少有一个完整实现（Qdrant + 关键词 + RRF + Rerank）
- [ ] 检索结果包含 DenseScore、KeywordScore、FusionScore、RerankScore 四个分数
- [ ] Metadata Filter（按品类/型号/文档类型过滤）正常工作

### LLM 容错
- [ ] 主模型超时/报错时自动切换到 fallback 模型
- [ ] 三态熔断器状态转换逻辑正确
- [ ] 流式首包超时有检测

### Agent 能力
- [ ] ReAct 最大步数限制生效
- [ ] 重复 Action 检测生效（相同 action + 相同参数被拦截）
- [ ] Tool 白名单按意图生效
- [ ] 5 个 Skill 内部多步编排逻辑完整
- [ ] Tool 结果校验对异常值（0/负数/空）有处理

### 评测体系
- [ ] 100 条评估用例中 ≥30 条包含口语化表达
- [ ] LLM-as-Judge 打分被 eval runner 调用
- [ ] Bad Case 自动分类结果可读
- [ ] Naive RAG vs Agentic RAG 对比报告可生成

### 可观测性
- [ ] Agent 内部有子 Span（至少覆盖 intent/rewrite/plan/retrieve/tool/reflect/generate）
- [ ] Token 消耗在 trace 和日志中记录
- [ ] 每个请求的 P95 延迟可统计

---

## 十、执行建议

1. **先通读设计文档**：[docs/clean-care-agentic-rag-design.md](docs/clean-care-agentic-rag-design.md) — 这是所有修复的参考基准
2. **按批次顺序修复**：第一批（基础能力）→ 第二批（Agentic 核心）→ 第三批（工程增强），不要跳
3. **每修复一个模块就跑测试**：`go test ./internal/xxx/...`，不要等全部改完再测
4. **在 configs/ 下更新 config.local.yaml**：确保新功能可配置可开关
5. **在 docs/ 下补充新的设计文档**：如 `docs/business-scenario-research.md`、`docs/prompt-design-v2.md`
6. **更新 Makefile**：添加新的 target（如 `make eval-compare` 运行 Naive vs Agentic 对比评测）
