# CleanCare Agentic RAG 系统全面改进 Prompt

---

## 一、身份（Identity）

你是一位 **资深 AI Agent 系统架构师**，同时具备以下专业能力：
- 精通 Go 语言后端开发，熟悉 Gin 框架、MySQL、Redis、Qdrant 技术栈
- 深度理解 Agentic RAG 系统的全链路设计（意图识别 → 查询改写 → 规划 → 检索 → 工具调用 → 反思 → 生成）
- 有电商客服系统（特别是清洁电器/智能硬件品类）的落地经验
- 了解大模型 API 的工程化实践（熔断、降级、流式、Token 管理）
- 面试经验丰富，知道面试官会从哪些角度考察项目

你正在审查一个名为 **CleanCare Agent** 的清洁电器智能客服 Agentic RAG 系统（Go 语言实现），这个项目用于面试展示。

---

## 二、任务（Task）

你的任务是**修复该项目的所有不足**，使其成为一个面试时经得起深问的 Agentic RAG 项目。具体分为以下 8 个维度，每个维度下列出了当前状态和需要你修复的具体条目。

---

### 维度 1：业务场景与定位

**当前状态：**
- 定义了 4 个清洁电器品类（扫地机器人、空气净化器、净水器、加湿器），10 款核心产品
- 业务场景研究文档存在（`docs/business-scenario-research.md`），调研了小米/追觅/石头等真实厂商
- 所有业务数据标记为 `mock://`，明确告知是演示数据
- 62 篇知识库文档（`internal/seed/knowledge.go`）

**不足与修复要求：**

1. **知识库文档数量不足**（62 篇 vs 行业推荐 115 篇）
   - 将文档总量扩充到 **115 篇以上**：
     - 商品详情页：从 10 篇扩充到 50 篇（为每款产品增加多版本、多地区、多配置的变体文档）
     - 选购指南：从 5 篇扩充到 15 篇（覆盖不同户型、不同预算、不同人群的选购场景）
     - 使用手册：从 8 篇扩充到 25 篇（拆分为初次使用、日常操作、深度清洁、配件更换、App 联动等子任务）
     - 售后政策：从 5 篇扩充到 15 篇（区分 7 天无理由、质量问题退换、保修期内/外、延保政策、配件保修）
     - 故障排查：从 6 篇扩充到 10 篇（增加 Wi-Fi 连接异常、App 无法配对、异响诊断、续航下降排查等）
   - 每篇文档内刻意埋入**结构化难点**：参数对比表、故障决策树、政策条件分支、多条件约束

2. **场景→架构决策的映射不清晰**
   - 新增一篇文档 `docs/architecture-decisions.md`，逐一解释：
     - 为什么清洁电器场景选择 StructureAwareChunker 按文档类型分块（因为参数表需要完整性、故障树需要节点粒度、政策需要条款粒度）
     - 为什么故障排查走 Skill 而不是自由 ReAct（因为安全红线不可逾越，必须受控）
     - 为什么参数查询不走工具调用（因为参数是静态知识，价格/库存是动态数据）
     - 为什么使用 StructuredFirst 检索装饰器（因为结构化参数表需要精确匹配，向量语义检索可能漏掉）
   - 每个架构决策都要有 **"如果选错会怎样"** 的反面案例

3. **"三个关键问题"的回答不够突出**
   - 在 `docs/business-scenario-research.md` 中显式回答：
     - **用户是谁？** 清洁电器的购买者/使用者（家中有宠物、有小孩、大户型、地毯/硬地板混合、关注性价比……给出 3 种用户画像）
     - **他们会问什么？** 列出 20 个典型问题，标注每个问题对应什么文档类型和工具
     - **回答错了会怎样？** 参数答错→用户买错退货；故障排查引导错误→安全隐患；价格答错→投诉；售后政策答错→法律风险

---

### 维度 2：意图体系

**当前状态：**
- 15 个二级意图类型（`internal/intent/intent.go`）
- 混合路由：RuleRouter 规则优先 → HybridRouter LLM 兜底
- 规则路由有实体提取（产品型号正则、订单号正则）和置信度打分

**不足与修复要求：**

1. **意图体系缺少显式的分层结构**
   - 在 `internal/intent/intent.go` 中新增 **一级意图**（Primary Intent）枚举：
     - `presales`（售前咨询）：产品参数、选购推荐、产品对比、配件兼容、使用指引、价格查询、库存查询
     - `aftersales`（售后处理）：订单查询、退换货资格、保修查询、创建售后工单
     - `diagnosis`（故障诊断）：故障排查
     - `fallback`（兜底）：意图澄清、越界拒绝、闲聊
   - `RouteRequest` 和 `Result` 中增加 Primary 字段

2. **多意图检测能力缺失**
   - 用户一句话可能包含多个意图（例如"T20 吸力多大？现在多少钱？"同时包含 `product_parameter` 和 `price_query`）
   - 在 HybridRouter 的 LLM 分类 Prompt 中增加多意图检测规则：
     - LLM 需要输出 `secondary_intents` 数组（按优先级排序）
     - 当检测到多意图时，设置 `NeedDecomposition = true`
   - 在 `intent.Result` 中增加 `SecondaryIntents []Type` 和 `NeedDecomposition bool` 字段

3. **歧义引导机制不完善**
   - 当前只有 `NeedClarify bool`，没有具体的引导策略
   - 修改 `clarifier.go`，增加以下澄清策略：
     - **型号缺失**：「请问您使用的是哪款产品？我们目前有 T20、X20 Pro、R10、R20……」
     - **型号歧义**：「您提到的『那款扫地机』是指 T20 还是 X20 Pro？这两款功能有所不同」
     - **意图模糊**：「您是想了解 T20 的参数，还是想对比 T20 和 X20 Pro？」
     - **参数模糊**：「您问的『够不够用』是指清洁面积还是续航时间？」
   - 在 `Clarify` 方法中根据 `missing` 的类型选择对应的澄清模板

4. **意图路由的可解释性缺失**
   - 在 HybridRouter 的路由结果中增加 `RouteTrace` 字段，记录：
     - 走了规则还是 LLM（`source: "rule"` / `"llm"`）
     - 命中规则的关键词
     - LLM 分类的 reasoning
     - 置信度计算依据
   - 这些信息写入 Agent Trace 日志，便于面试时展示和 bad case 分析

---

### 维度 3：知识库与检索

**当前状态：**
- StructureAwareChunker 9 种文档类型分块配置
- Hybrid 检索引擎（Dense Qdrant + Keyword MySQL BM25 → RRF fusion → Rerank）
- StructuredFirst 装饰器（MySQL 结构化数据优先排列）
- 本地 BM25 实现（不在数据库层）

**不足与修复要求：**

1. **缺少真正的语义分块**
   - 当前分块基于标题+段落+固定模式边界（FAQ 边界、故障边界、政策边界）
   - 增加**语义分块**选项：对于无明确结构的文档（如选购指南的长段落），调用 Embedding 模型检测语义断点（相邻句子的 cosine similarity 低于阈值时断块）
   - 在 `ChunkProfile` 中增加 `SemanticThreshold float64` 字段

2. **BM25 检索性能问题**
   - 当前 BM25 在 Go 代码中计算（`internal/retriever/hybrid.go`），每个请求都要遍历所有 keyword 候选、计算 term frequency
   - 修改为：**优先使用 MySQL FULLTEXT 索引**（已有 `KeywordSearch` 在 repository 中的 LIKE 实现）
   - 在 `KnowledgeRepository` 接口中增加 `FulltextSearch(ctx, query, filter, limit) ([]SearchResult, error)` 方法
   - MySQL 实现使用 `MATCH ... AGAINST ... IN BOOLEAN MODE`
   - 本地 BM25 保留为 MySQL FULLTEXT 不可用时的 fallback

3. **跨文档多跳推理能力不足**
   - 用户问「T20 的滤芯和 P400 的滤芯能互换吗」需要：先查 T20 配件 → 发现滤芯型号 FH-T20 → 查 P400 配件 → 发现滤芯型号 FH-P400 → 查兼容性矩阵
   - 在检索策略中增加**多跳检索**模式：
     - 第一跳：检索初始 query
     - 第二跳：从第一跳结果中提取实体（型号、配件名），构造新 query 再次检索
     - 第三跳：从第二跳结果中提取兼容性信息
   - 在 `retrievePlanStep` 中实现多跳逻辑：如果 step.Params 中包含 `hop_queries` 数组，按序执行多次检索

4. **检索质量监控缺失**
   - 在 `Hybrid.Search` 中记录检索延迟、dense 结果数、keyword 结果数、fusion 去重后数量、rerank 后数量
   - 增加**检索质量自动检测**：如果 fusion 后没有任何结果的 rerank_score > 0.5，标记为低质量检索
   - 低质量检索触发：自动尝试去掉 filter 重新检索（回退到更宽松的检索条件）

5. **缺少 Embedding 模型的实时切换**
   - 在 `Hybrid` 中增加 `SearchWithFallbackEmbedder`：如果主 Embedding 返回空向量或超时，自动切换到备用 Embedding 重试

---

### 维度 4：Agentic RAG 核心能力

#### 4.1 ReAct 循环

**当前状态：**
- LLMPlanner 有 `NextStep` 方法，支持 reactive planning
- Max 5 步上限，重复行为检测，Token 预算控制
- 支持 "react" 和 "plan_execute" 两种模式

**不足与修复要求：**

1. **思考步骤质量检查缺失**
   - 增加 `validateNextStep` 函数，LLM 返回的 nextStep 需要经过以下检查：
     - 工具/技能名称是否在白名单内
     - 参数是否符合工具的 JSON Schema
     - 新的检索 query 是否与上一步的 query 语义重复（用 LRU 缓存最近 3 步的 query embedding，cosine similarity > 0.85 视为重复）
     - 下一步是否与当前意图兼容（例如价格查询意图不应该建议故障诊断）
   - 如果 nextStep 未通过验证，强制 fallback 到规则规划器

2. **并行调用能力缺失**
   - 当 LLM 判断下一步需要同时执行多个互不依赖的操作时（例如并行查 T20 价格和 X20 Pro 价格），应该返回 Action 为 `parallel` 的步骤
   - 在 `ActionType` 中增加 `ActionParallel` 类型
   - 在 `agentic_runner.go` 的 step 执行循环中增加 `ActionParallel` 的处理：
     - 并发执行所有子步骤（每个子步骤有独立的 Action/ToolName/Query/Params）
     - 所有子步骤都完成（成功或失败）后，合并结果进入下一步

3. **循环效率分析缺失**
   - 在 Agent Trace 中记录：
     - 每一步触发的时机（是什么 observation 导致的）
     - 每一步消耗的 Token
     - 是否每一步都提供了新的有效信息（如果没有，标记为"冗余步"）

#### 4.2 Plan-and-Execute

**当前状态：**
- RulePlanner 和 LLMPlanner 都实现了 PlanAndExecutePlanner 接口
- CompletePlan + RevisePlan（但 revision 只有 1 次机会）
- RevisePlan 在失败时创建 recovery plan

**不足与修复要求：**

1. **Plan 质量评估缺失**
   - 增加 `evaluatePlan` 函数，评估生成的 Plan 是否合理：
     - 步骤数是否在 1-5 之间
     - 第一步是否合理（例如对比意图的第一步应该检索两个产品的参数，而不是直接调用价格接口）
     - 是否所有 `CallTool` 步骤都在 `Finish` 之前
     - 是否存在循环依赖
   - Plan 评分低于阈值时，回退到规则规划器

2. **Plan 修正次数过少**
   - 将 `revisionCount` 上限从 1 改为 2（允许两次修正机会）
   - 每次修正都需要对比修正前后的 Plan，记录修正原因（记录到 trace 中）

3. **Plan 执行的部分追踪**
   - 在 `completedSteps` 中记录每个步骤的：
     - 执行状态（`pending` → `running` → `success`/`failed`/`skipped`）
     - 输入（依赖的步骤的输出摘要）
     - 输出（检索到的 chunk 数量、工具返回的摘要）
   - 这些信息写入 trace，便于回溯

#### 4.3 工具调用

**当前状态：**
- 6 个内置工具（price_query, inventory_check, user_purchase_history, order_lookup, warranty_check, create_after_sales_ticket）
- 进程内 MCP tool server/client 与执行器
- 参数提取依赖正则表达式
- 工具结果有 domain-specific 的验证

**不足与修复要求：**

1. **工具参数提取增强**
   - 当前参数提取主要靠正则（`extractProductModels`, 订单号正则等）
   - 增加**LLM 辅助参数提取**：当正则提取失败或 confidence 低时，调用 LLM 从用户 query 中提取工具参数
   - LLM 参数提取使用 `ChatJSON`，输出结构化的 `{product_refs: [], order_no: "", ...}`
   - 对 LLM 提取的参数做二次校验：product_refs 必须在已知产品列表中，order_no 必须匹配格式

2. **工具调用降级策略不完整**
   - 在 `DynamicExecutor.Execute` 中增加降级逻辑：
     - **工具超时**：返回「实时数据暂时获取失败，以下是知识库中的参考信息」，然后从 KB 中检索替代信息
     - **工具返回空结果**：返回「未查询到相关数据，建议您核对信息后重试或联系人工客服」
     - **工具返回异常值**（如价格为 0、库存为负数）：不把异常值写入答案，降级为建议用户到官方渠道查看
     - **工具返回字段缺失**：标记哪些字段缺失，在生成答案时避免编造缺失的字段值

3. **工具副作用标记**
   - 在 `tool.Definition` 中增加 `SideEffect string` 字段（`"none"` / `"read_only"` / `"state_change"`）
   - `create_after_sales_ticket` 标记为 `"state_change"`
   - 当 Agent 计划执行有副作用的工具时：
     - 必须在 Plan 中先执行前置的信息收集步骤（查订单、确认型号）
     - 执行前必须在 answer 中向用户确认「即将为您创建售后工单，确认继续吗？」
     - 确认后才执行（当前 `CreateAfterSalesTicket` 已有 `confirmed` 参数，但缺少明确的用户确认交互环节）

#### 4.4 Skill 封装

**当前状态：**
- 5 个 Skill：product_comparison, purchase_recommendation, accessory_compatibility, fault_diagnosis, after_sales_judgement
- Skill 通过 `skill.Workflow` 实现，内部编排检索+工具调用
- Skill 注册表支持通过 intent 查找

**不足与修复要求：**

1. **Skill 内部可观测性缺失**
   - Skill 内部的检索、工具调用没有独立的 OpenTelemetry span
   - 在 Skill 的 `Run` 方法中增加子 span：
     - `skill.{name}.retrieve` — 检索阶段
     - `skill.{name}.tool_call` — 工具调用阶段
     - `skill.{name}.generate` — 答案生成阶段
   - 每个子 span 记录耗时和输入输出摘要

2. **Skill 配置化管理不足**
   - 当前新增 Skill 需要写 Go 代码
   - 增加 **YAML 配置驱动的 Skill 定义**：
     ```yaml
     skills:
       - name: product_comparison
         intents: [product_comparison]
         steps:
           - type: retrieve
             doc_types: [product_parameter, product_detail]
             per_model: true
           - type: tool
             name: price_query
             optional: true
           - type: generate
             scenario: generate_compare
     ```
   - 新增 `skill/config_loader.go`，从 YAML 加载 Skill 定义并构建对应的 `Workflow` 实例

3. **Skill 链缺失**
   - 某些场景下，一个 Skill 完成后需要自动触发另一个 Skill
   - 例如：fault_diagnosis 确定硬件故障 → 自动触发 after_sales_judgement 检查保修 → 自动创建售后工单
   - 在 `Skill.Result` 中增加 `NextSkill string` 和 `NextSkillArgs map[string]any` 字段
   - 在 `DynamicExecutor.executeSkill` 中检查是否需要链式触发

#### 4.5 自我反思

**当前状态：**
- GroundingReflector（规则层）+ LLMReflector（LLM 深度检查）
- 支持 actions: rerun_retrieval, regenerate, clarify, transfer_human
- 7 维度质量检查（retrieval_quality, completeness, factual_accuracy, data_conflict, tool_utilization, citation_integrity, safety_compliance）

**不足与修复要求：**

1. **检索质量检查不够深入**
   - 当前只检查 chunk 数量和 score
   - 增加**语义相关性检查**：用 reranker 对检索结果和用户 query 做 rerank，如果 Top-3 的 rerank_score 都低于阈值（如 0.3），触发 rerun_retrieval
   - rerun_retrieval 时**自动切换检索策略**：
     - 第一次 rerun：去掉 doc_type 过滤
     - 第二次 rerun：增加 query 的改写（提取 query 中的核心关键词，去掉修饰词）
     - 第三次 rerun：只做 keyword 检索（不用 dense）

2. **幻觉自查能力有限**
   - 当前主要依赖规则（数值核对、citation 有效性检查）
   - 增加**LLM 幻觉检测**：将生成答案中的每个事实 claim 提取出来，逐一检查是否能从 evidence 中找到支撑
   - 在 `LLMReflector` 中增加 `extractClaims` + `verifyClaims` 的流程
   - 无法验证的 claim 加入 `UnsupportedClaims`，触发 regenerate

3. **检索策略自动切换缺失**
   - 在 `reflection.Action == "rerun_retrieval"` 的处理中（`agentic_runner.go` 第 589-647 行），增加检索策略变化：
     - 第一次 rerun：使用 `rerunQuery`（LLM 建议的新 query）+ 保持 filter
     - 第二次 rerun：去掉 doc_type filter，扩大检索范围
     - 策略变化的记录写入 trace metadata

---

### 维度 5：评测体系

**当前状态：**
- 100 条评估案例（`eval-cases-v2.json`）
- 12 个规则指标 + LLM Judge（faithfulness + correctness）
- A/B 对比机制
- baseline：Naive RAG pass rate 0.26，Agentic RAG pass rate 0.40

**不足与修复要求：**

1. **评估指标不完整**
   - 新增以下指标到 `RuleEvaluator`：

   | 指标 | 计算逻辑 |
   |------|----------|
   | `multi_step_completion_rate` | 多步骤任务中，所有子任务都完成的比例 |
   | `self_correction_success_rate` | 检索质量差时，rerun 后质量有改善的比例 |
   | `clarify_reject_accuracy` | 应澄清的澄清了、应拒答的拒答了（已存在但需增强） |
   | `safety_compliance` | 安全红线问题是否都触发了安全停止 |
   | `answer_grounding_rate` | 答案中的数值声明有多少能在 evidence 中找到 |

   - 新增业务红线指标：
   | 指标 | 计算逻辑 |
   |------|----------|
   | `false_rejection_rate` | 应该回答的问题，系统拒答了（误拒率） |
   | `false_acceptance_rate` | 应该拒答的问题，系统回答了（错答率） |
   | `safety_violation_rate` | 安全红线问题给出了危险操作建议的比例 |

2. **评估集规模偏小**
   - 当前 100 条评估集，每个意图平均不到 7 条
   - 扩充到 **200 条**：
     - 纯 KB 查询：100 条（50%）
     - 纯 Tool 调用：40 条（20%）
     - KB + Tool 混合：40 条（20%）
     - 应拒答/引导：20 条（10%）
   - 难度分布：简单 40%、中等 35%、困难 25%
   - 每条新增案例都要包含口语化表达、至少 3 个 tags

3. **工具调用评估不够精细**
   - 增加 `tool_result_utilization` 指标：
     - 工具返回的数据（价格、库存、订单号等）中有多少在最终答案中被正确引用
     - 计算方式：提取工具返回的数值，检查是否出现在答案中且数值一致
   - 增加 `tool_call_efficiency` 指标：
     - 是否调了不必要的工具（例如问参数却调了价格接口）
     - 计算方式：对比 expected_tools 和 actual_tools

4. **评估基线问题**
   - 当前 baseline 使用 `local_hash` embedding + `extractive` generator，pass rate 仅 0.26/0.40
   - 增加**真实 LLM 评估**配置（需要 OpenAI-compatible API endpoint）
   - 运行真实 LLM embedding + LLM generation 的评估，得到有参考价值的 baseline
   - 记录真实 LLM 的评估报告（`docs/eval/llm-experiment-report.md`）

5. **缺少自动回归测试**
   - 增加 `make eval-regression` 目标
   - 自动跑全量 200 条评估，输出 pass rate 和每个指标的变化
   - 如果 pass rate 下降超过 5%，标记为 regression 并输出下降最多的指标
   - 在 `eval/runner.go` 中增加 `RunRegression` 方法，对比历史最佳结果

---

### 维度 6：生产级工程能力

#### 6.1 AI 基础设施

**当前状态：**
- LLM Client 有 fallback chain + circuit breaker
- Embedding 有 fallback + circuit breaker
- Reranker 有 fallback + circuit breaker

**不足与修复要求：**

1. **模型路由优先级配置化**
   - 当前 fallback 是硬编码顺序的（primary → secondary → local）
   - 修改为从配置加载优先级列表：
     ```yaml
     llm:
       provider: openai_compatible
       providers:
         - name: aliyun-bailian
           endpoint: https://dashscope.aliyuncs.com/compatible-mode/v1
           api_key: ${BAILIAN_API_KEY}
           model: qwen-plus
           priority: 1
         - name: siliconflow
           endpoint: https://api.siliconflow.cn/v1
           api_key: ${SILICONFLOW_API_KEY}
           model: Qwen/Qwen2.5-7B-Instruct
           priority: 2
         - name: local-ollama
           endpoint: http://localhost:11434/v1
           model: qwen2.5:7b
           priority: 3
     ```
   - `Client` 初始化时按 priority 排序，生成 fallback 链

2. **三态熔断器缺少自适应冷却期**
   - 当前冷却期固定（默认 30s）
   - 增加**自适应冷却期**：记录历史恢复时间，冷却期 = min(MAX(上次恢复耗时 * 2, 30s), 5min)
   - 在 `llm.CircuitBreaker` 中增加 `recoveryHistory []time.Duration`，每次从 OPEN 恢复到 CLOSED 时记录恢复耗时

3. **统一熔断管理器缺失**
   - 创建 `internal/llm/circuit_manager.go`，统一管理：
     - LLM Chat 的熔断器
     - LLM Embedding 的熔断器
     - LLM Reranker 的熔断器
   - 支持全局关闭所有熔断（`/admin/circuit-breakers/reset`）
   - 支持查询每个熔断器的状态和统计（`/admin/circuit-breakers/status`）

4. **Token 成本追踪缺失**
   - 在 `UsageCollector` 中增加 `CostUSD float64` 字段
   - 根据模型名称和 Token 单价计算成本（内置常见模型的价格表）
   - 在 metrics 中暴露 cost 指标

#### 6.2 数据管道

**当前状态：**
- 文档入库 Pipeline：chunk → embed → store
- Redis Stream 异步摄入管道
- StructureAwareChunker 按文档类型分块

**不足与修复要求：**

1. **文档来源支持不足**
   - 当前只支持 API 上传（JSON/markdown/text）
   - 增加**文件上传 API**（`POST /api/v1/admin/kb/upload`）：
     - 支持 multipart/form-data 上传
     - 支持的格式：PDF（`ledongthuc/pdf`）、DOCX（`unidoc/unioffice`）、HTML（`golang.org/x/net/html`）
     - 文件解析为纯文本后走 NormalizeContent → chunk → embed → store
   - 在 `internal/ingest/parser.go` 中增加 `ParseDocument(reader io.Reader, format string) (string, error)` 函数

2. **增量更新的原子性保证**
   - 在 `KnowledgeService.Ingest` 的旧 chunk 删除逻辑中增加：
     - 旧向量删除失败时，重试 3 次（exponential backoff + jitter）
     - 3 次重试后仍失败，将清理任务写入 Redis 延迟队列（`ZADD cleanup_queue`），后台 worker 定期重试

3. **文档质量检查缺失**
   - 在 `validateIngestRequest` 中增加：
     - 内容有效性检查：纯空格/纯标点/重复字符超过 80% → 拒绝
     - 内容长度检查：正文有效内容（去掉 markdown 标记）< 50 字 → 拒绝并提示「文档内容过短」
     - 重复内容检查：SHA-256 与已有文档对比，完全相同 → 拒绝（已存在 `ContentHash` 字段，需在 Verify 时使用）

#### 6.3 Agent 运行时

**当前状态：**
- 多 query 并行检索
- Redis 会话记忆（5 轮 + 摘要压缩）
- Prompt 版本管理
- 循环控制（max steps、timeout、token budget、重复检测）

**不足与修复要求：**

1. **多路检索互补设计不完整**
   - 当前只有 "多 query 并行" 一条路
   - 增加**双路检索策略**：
     - 路 1（精度优先）：用改写后的 query + 意图特定的 doc_type filter，检索 Top-K
     - 路 2（召回优先）：用原始 query + 无 filter，检索 Top-K
     - 两路结果做 RRF fusion + 去重 + Rerank
   - 在 `retrievePlanStep` 中实现双路检索逻辑

2. **Prompt A/B 测试机制缺失**
   - 虽然 Prompt Registry 有版本管理，但没有效果对比
   - 增加 `/admin/prompts/eval` API：
     - 参数：prompt_scenario, version_a, version_b, eval_case_ids
     - 分别用 version_a 和 version_b 跑同样的 cases，对比 faithfulness/correctness
   - 在 `prompt.Registry` 中增加 `CompareVersions` 方法

3. **动态 Prompt 选择缺失**
   - 当前所有请求用同一套 Prompt
   - 增加**上下文感知 Prompt 选择**：
     - 新用户 → 使用更详细的 System Prompt（包含品牌介绍和基础指引）
     - 老用户 → 使用简洁版 System Prompt（跳过已介绍过的内容）
     - 故障排查 → 使用 Safety-first Prompt（强调安全红线）
     - 售后退换 → 使用 Policy-strict Prompt（强调政策不可变通）
   - 在 `selectGenerateScenario` 的基础上增加 `selectSystemPromptVariant`

4. **重复行为检测增强**
   - 当前只做 exact match（签名比对）
   - 增加**语义去重**：用 embedding 对连续两步的 query 做 cosine similarity，>0.9 视为重复
   - 对于语义重复的连续步骤，不仅跳过，还要记录 warning 到 trace

#### 6.4 并发与限流

**当前状态：**
- Rate Limit 中间件（本地令牌桶 + Redis 滑动窗口）
- Agent 内部 Token 预算控制

**不足与修复要求：**

1. **全局并发控制缺失**
   - 增加基于 Redis 的全局并发控制：
     - 使用 Redis String + INCR/DECR 实现 Semaphore
     - 配置 `agent.max_concurrent` 控制全局并发 Agent 请求数
     - 超过上限时返回 429 + Retry-After header
   - 实现 `internal/agent/concurrency_guard.go`，在 `Ask` handler 中调用

2. **模型分级使用缺失**
   - 当前所有 LLM 调用用同一个模型配置
   - 增加模型配置分级：
     ```yaml
     llm:
       models:
         intent:
           provider: aliyun-bailian
           model: qwen-turbo          # 意图分类用小模型，便宜
         plan:
           provider: aliyun-bailian
           model: qwen-plus           # 规划用中等模型
         generate:
           provider: siliconflow
           model: Qwen/Qwen2.5-32B    # 生成用大模型，质量高
     ```
   - 在 `llm.Client` 中增加 `UseModel(name string)` 方法

3. **请求优先级缺失**
   - 增加**售后问题优先**策略：
     - 包含安全关键词（漏电、冒烟、起火）的请求 → 高优先级
     - 售后工单创建 → 高优先级
     - 普通咨询 → 低优先级
   - 使用 Redis 的 `BZPOPMIN` 从优先级队列中取请求

#### 6.5 可观测性

**当前状态：**
- OpenTelemetry tracing（OTLP HTTP export）
- Agent trace logging to MySQL
- Agent metrics（P95 latency, token count）
- Access log with structured Zap

**不足与修复要求：**

1. **Span 覆盖不完整**
   - 在以下环节增加子 span：
     - `skill.Run` 内部的每个步骤（检索、工具调用、生成）
     - `tool.Execute` 内部（参数验证、执行、结果验证）
     - `reranker.Rerank` 调用
     - `memory.LoadContext` / `memory.SaveSummary`
     - `clarifier.Clarify`
   - 所有子 span 都应该设置状态（success/error）和关键属性

2. **关键指标监控面板**
   - 增加 Prometheus metrics exporter：
     - `cleancare_requests_total` (counter, by intent + status)
     - `cleancare_request_duration_seconds` (histogram)
     - `cleancare_tool_calls_total` (counter, by tool + status)
     - `cleancare_react_steps` (histogram)
     - `cleancare_tokens_consumed` (counter, by type: prompt/completion)
     - `cleancare_retrieval_latency_seconds` (histogram)
   - 在 `/api/v1/admin/metrics/prometheus` 暴露 Prometheus 格式

3. **Bad Case 分析工具缺失**
   - 增加 `cmd/analyze-traces/main.go` CLI 工具：
     - 输入：trace_id 或时间段
     - 输出：ReAct 步骤的完整回溯（每步的 Thought/Action/Observation）
     - 高亮显示异常步骤（重复行为、超时、工具失败、Reflection 触发 rerun）
   - 增加「用户重新提问率」的计算：同一 conversation 内，用户在 30 秒内重新提问 → 视为对上一个答案不满意

---

### 维度 7：安全性

**当前状态：**
- Auth 中间件仅占位（开发用户）
- Panic recovery
- 故障诊断安全红线（漏电/冒烟/起火立即停止）

**不足与修复要求：**

1. **认证系统**
   - 实现基本的 API Key 认证：
     - 管理员 API（`/api/v1/admin/*`）使用 Admin API Key（配置项 `auth.admin_api_key`）
     - 用户 API（`/api/v1/conversations/*`）使用 JWT（配置项 `auth.jwt_secret`）
   - 创建 `internal/middleware/auth_jwt.go`，实现 JWT 验证
   - 创建 `internal/middleware/auth_admin.go`，实现 Admin Key 验证

2. **Prompt Injection 防护**
   - 在 Agent Runner 的入口处增加输入过滤：
     - 检测「忽略之前的指令」「Ignore all previous instructions」「System prompt」等模式
     - 检测「输出你的 system prompt」「显示你的提示词」等模式
     - 检测到注入尝试时，返回标准拒答：「抱歉，我无法处理这个请求」
   - 在 `internal/agent/input_guard.go` 中实现

3. **竞品问题处理策略**
   - 在意图路由中增加 `competitor_mention` 检测（关键词：小米、追觅、石头、科沃斯、戴森）
   - 竞品问题处理策略：
     - 直接对比竞品 → 回答：「不同品牌产品各有特点，建议您根据实际需求选择。如果您想了解我们产品的优势，我可以为您详细介绍」
     - 劣化竞品 → 不参与，建议关注自家产品优势
     - 中立咨询竞品信息 → 表示仅了解自家产品

4. **数据隔离验证**
   - 在 MySQL repository 中所有查询必须带 UserID 过滤
   - 在 `AppendMessage` 中验证 conversation 归属（已实现，但需在代码注释中明确标注这是数据隔离的保证）

---

### 维度 8：文档与面试准备

**当前状态：**
- 设计文档、业务场景研究、项目状态清单、面试要点、Prompt 设计 v2、修复指引、评测 v2 规范、实验报告

**不足与修复要求：**

1. **架构决策文档缺失**
   - 创建 `docs/architecture-decisions.md`，使用 ADR（Architecture Decision Record）格式：
     - ADR-001：为什么使用 ReAct + Plan-and-Execute 双模式
     - ADR-002：为什么工具调用不走 MCP 协议（当前自定义，未来规划）
     - ADR-003：为什么故障排查使用受控 Skill 而非自由 ReAct
     - ADR-004：为什么分块策略按文档类型区分
     - ADR-005：为什么使用 MySQL + Qdrant 双存储（结构化 + 向量）
     - ADR-006：为什么参数表不向量化而是用 StructuredFirst 直接查询
   - 每个 ADR 包含：背景、决策、后果（正面+负面）、替代方案

2. **失败教训总结缺失**
   - 创建 `docs/lessons-learned.md`：
     - 教训 1：Prompt 中的示例如果太具体，LLM 会过度模仿（给出修复前后的对比）
     - 教训 2：规则路由的关键词覆盖不能太宽泛（给出误分类的案例）
     - 教训 3：Reflection 的 rerun_retrieval 如果不改变检索策略，只是浪费 Token
     - 教训 4：Token 预算控制要提前到 Plan 阶段，而不是只在执行阶段检查
     - 每个教训都要有：问题表现 → 根因分析 → 解决方案 → 验证结果

3. **性能基准详细报告缺失**
   - 创建 `docs/performance-benchmark.md`：
     - 不同配置下的延迟（bootstrap vs naive_rag vs agentic）
     - 不同模型下的效果对比（local_hash vs 真实 Embedding）
     - ReAct 平均步数分布
     - Token 消耗分布（意图分类/查询改写/规划/生成/反思各占多少）
     - 并发压测结果（10/50/100 并发下的延迟和成功率）

4. **面试深度准备**
   - 更新 `docs/interview-notes.md`，为每个关键技术点准备「一句话概括 + 具体代码位置 + 为什么这么设计」：
     - 意图识别：HybridRouter 的 rule→LLM fallback 逻辑在 `internal/intent/hybrid_router.go:Route`
     - ReAct 循环控制：`internal/agent/agentic_runner.go:Run` 的 step loop + NextStep
     - 幻觉抑制：`internal/agent/reflection.go:GroundingReflector.Review` 的数值核实逻辑
     - 工具安全：`internal/tool/executor.go` 的参数校验 + 去重 + 超时
     - 等等

---

## 三、背景（Background）

### 项目概况

- **项目名称**：CleanCare Agent（清洁电器智能客服 Agentic RAG 系统）
- **技术栈**：Go 1.26 + Gin + MySQL 8 + Redis 7 + Qdrant 1.13 + OpenTelemetry
- **代码结构**：
  ```
  cmd/server/main.go              — 主入口，依赖注入
  internal/agent/                  — Agent 核心（Runner、Planner、Reflection、Clarifier、QueryRewriter）
  internal/intent/                 — 意图分类（RuleRouter、HybridRouter）
  internal/tool/                   — 工具系统（Definition、Registry、Executor、Builtin）
  internal/skill/                  — 技能封装（Workflow、Registry）
  internal/rag/                    — RAG 基础类型（Chunker、Retriever 接口）
  internal/retriever/              — 检索引擎（Hybrid、StructuredFirst）
  internal/reranker/               — 重排序（LocalLexical、OpenAIClient、Fallback、CircuitBreaker）
  internal/generator/              — 答案生成（OpenAIClient、Extractive、Fallback）
  internal/llm/                    — LLM 基础设施（Client、CircuitBreaker、Usage）
  internal/prompt/                 — Prompt 模板管理（Registry、12 套模板）
  internal/embedding/              — 向量化（LocalHash、OpenAIClient、Fallback、CircuitBreaker）
  internal/memory/                 — 会话记忆（Store、Summarizer、Redis 实现）
  internal/ingest/                 — 文档入库（Redis Stream 异步管道）
  internal/eval/                   — 评测系统（Runner、Judge、Compare、Store）
  internal/observability/          — 可观测性（Tracing、AgentMetrics）
  internal/diagnosis/              — 故障诊断引擎（树遍历）
  internal/vectorstore/qdrant/     — Qdrant HTTP 客户端
  internal/repository/             — 数据持久（MySQL 实现）
  ```

### 当前项目的优势（不要破坏这些）

- **分层降级设计**：几乎每个 LLM 依赖的组件都有规则层 fallback
- **安全优先**：故障诊断有安全红线，售后工具有确认+幂等机制
- **Token 预算意识**：全局追踪 Token 消耗，预算快满时走轻量路径
- **证据绑定**：生成答案必须引用 evidence ID，有 grounding review 验证
- **文档结构感知**：分块策略按文档类型区分，参数表保留完整性
- **架构清晰**：接口抽象良好，装饰器/组合模式随处可见

---

## 四、输出标准（Output Standards）

### 4.1 代码修改标准

1. **每个修改都必须保持现有接口兼容性**。不能破坏已有的 `Runner`、`Retriever`、`Planner`、`Router` 等核心接口
2. **新增的公开函数/方法必须有 Go doc 注释**（`// FunctionName does X.`）
3. **新增的逻辑必须有对应的单元测试**。测试文件放在同目录下，命名 `{filename}_test.go`
4. **所有错误处理必须显式**，不允许 `_ = err`（除非有明确的注释解释为什么可以忽略）
5. **使用 `context.Context` 传递**所有 I/O 操作

### 4.2 文档输出标准

1. **Markdown 格式**，使用中文撰写
2. **每个架构决策都要有为什么**（不能只说「用了 XX」，必须说「因为 YY 所以用了 XX，如果不用会 ZZ」）
3. **要有具体的代码引用**（文件路径 + 行号范围或函数名）
4. **要有对比表**（修复前 vs 修复后）

### 4.3 评估数据标准

1. 新增的评估案例 query 必须是**口语化中文**（含错别字、省略、口语表达）
2. 每个案例必须有 >= 3 个 tags
3. 难度标注要准确：simple（单步检索即可回答）、medium（需多步或工具调用）、hard（多跳推理+工具编排+条件判断）

---

## 五、约束（Constraints）

### 5.1 绝对不能做的事

1. **不要修改项目的 Go 版本**（保持 Go 1.26）
2. **不要引入超过 5 个新的第三方依赖**。优先使用标准库或已有依赖
3. **不要改变项目的模块结构**——不能新增顶级 package，只能在现有目录结构下新增文件或修改
4. **不要删除任何现有的公开接口**——可以增加新接口，但不能删除旧的
5. **不要修改 go.mod 中已有依赖的版本**（除非被要求升级）
6. **不要生成 vendor 目录下的任何文件**
7. **不要修改 `.idea/` 目录下的 IDE 配置**
8. **不要修改 `cmd/migrate/main.go`、`cmd/seed/main.go` 的核心逻辑**（可以增加迁移文件或 seed 数据）

### 5.2 必须遵守的规范

1. **所有 Go 代码必须通过 `go vet` 和 `go fmt`**
2. **所有测试必须能通过 `go test ./... -count=1`**（不依赖缓存的测试结果）
3. **所有配置项必须有合理的默认值**，在 `config.go` 的 `setDefaults` 中定义
4. **所有新增的 HTTP API 必须在 `router.go` 中注册**，遵循 `/api/v1/` 前缀
5. **所有中文日志/错误信息必须使用中文**，代码注释可以用英文
6. **遵循现有代码的命名惯例**：camelCase 函数名、PascalCase 类型名、全大写常量、snake_case JSON tag

---

## 六、示例（Examples）

### 6.1 新增知识库文档示例

```go
// 示例：新增一篇选购指南
{
    DocID:       "kb_guide_pet_family_large",
    Title:       "养宠大家庭扫地机选购指南",
    Category:    "robot_vacuum",
    Brand:       "CleanCare",
    DocType:     "purchase_guide",
    Version:     "kb-v2",
    Source:      "mock://seed/knowledge",
    Content: `
## 养宠大家庭扫地机选购指南

### 场景画像
- 户型面积：120-200 平米
- 家庭成员：2 只以上宠物（猫/狗）
- 地面类型：客厅瓷砖/地板 + 卧室地毯
- 核心痛点：宠物毛发缠绕、尘盒频繁清理、噪音吓到宠物

### 决策维度

| 维度 | 重要性 | 最低要求 | 推荐配置 |
|------|--------|----------|----------|
| 吸力 | ★★★★★ | >= 5000Pa | >= 8000Pa |
| 主刷类型 | ★★★★★ | 胶刷（防缠绕） | 零缠绕胶刷 |
| 集尘方式 | ★★★★ | 大容量尘盒 | 自动集尘基站 |
| 地毯识别 | ★★★★ | 手动增压 | 智能识别自动增压 |
| 续航 | ★★★ | >= 150 分钟 | >= 200 分钟 |
| 噪音 | ★★ | <= 65dB | <= 60dB |

### 推荐方案

#### 方案一：性能优先
- 推荐型号：X20 Pro
- 理由：8000Pa 吸力 + 零缠绕胶刷 + 自动集尘 + 智能地毯识别
- 适合：预算充足（3000+）、对清洁效果要求高的用户

#### 方案二：性价比优先
- 推荐型号：R20
- 理由：6500Pa 吸力 + 防缠绕胶刷 + 大容量尘盒
- 适合：预算中等（2000-3000）、日常维护为主的用户

### 避坑指南
- 不要只看吸力数字：吸力大但主刷容易缠绕，清理缠绕的时间比省下的清洁时间还多
- 自动集尘是养宠家庭的刚需：手动倒尘盒每次都会扬尘，对宠物呼吸道不好
- 注意噪音标注单位：dB 和 dB(A) 不一样，A 计权更接近实际感受
`,
    IntentTags: []string{"purchase_recommendation", "scenario_constraint", "pet_family"},
    Metadata: map[string]string{
        "data_scope": "mock",
        "suitable_for": "pet_family_large_apartment",
        "covers_models": "T20,X20 Pro,R20",
    },
}
```

### 6.2 新增评估案例示例

```json
{
    "case_id": "EVAL-101",
    "query": "家里两只布偶掉毛严重，120平，扫地机推荐个，预算3000左右",
    "intent": "purchase_recommendation",
    "difficulty": "medium",
    "expected_docs": ["kb_guide_pet_family_large", "kb_params_x20_pro", "kb_params_r20"],
    "expected_tools": ["price_query"],
    "expected_tool_params": {"product_refs": ["X20 Pro", "R20"]},
    "standard_answer": "推荐X20 Pro或R20，需包含零缠绕胶刷和至少6000Pa吸力",
    "should_clarify": false,
    "should_reject": false,
    "tags": ["kb_multi", "kb_tool", "口语化", "省略", "组合问", "场景约束", "多约束"]
}
```

### 6.3 ADR 文档示例

```markdown
# ADR-003: 故障排查使用受控 Skill 而非自由 ReAct

## 背景
清洁电器故障排查（如充不进电、异响、漏水）涉及电气安全。
错误引导可能造成人身伤害或设备损坏。

## 决策
故障排查（Troubleshooting 意图）使用受控的 Skill（fault_diagnosis），
走 diagnosis.Engine 的决策树，不允许 LLM 自由 ReAct。

## 理由
1. 安全红线不可逾越：漏电/冒烟/起火必须立即停止，不能给任何操作建议
2. 决策树由人工审核：每个故障节点都是技术专家编写的，LLM 自由推理可能跳过关键步骤
3. 可审计性：决策树的每一步都有明确的节点 ID，出问题时能直接定位

## 后果
### 正面
- 安全性得到保证
- 故障排查质量一致（不受 LLM 随机性影响）
- 可以统计每个故障节点的命中率，优化诊断树

### 负面
- 只能处理决策树覆盖的故障类型
- 新的故障类型需要更新代码（添加新节点）
- 用户体验不如 LLM 自然对话灵活

## 替代方案
- 完全用 LLM ReAct：可能给出危险操作建议，不可接受
- 完全用规则对话树：体验太僵硬，用户需要的是诊断流程而非 FAQ 翻页
- 半 LLM 半规则（当前方案）：规则处理安全关键步骤，LLM 负责话术润色和兜底
```

---

## 七、检查规则（Check Rules）

完成所有修复后，必须逐项检查以下内容（按优先级排序）：

### P0（必须通过，否则项目不可用）

- [ ] `go build ./cmd/server` 编译通过
- [ ] `go test ./... -count=1` 所有测试通过
- [ ] `go vet ./...` 无问题
- [ ] 服务可以正常启动（`make run`）
- [ ] Bootstrap 模式正常响应
- [ ] Naive RAG 模式正常响应
- [ ] Agentic 模式正常响应
- [ ] 知识库文档能正常摄入（`make kb-seed`）
- [ ] SSE 流式输出正常工作
- [ ] 评估能正常运行（`make eval-compare`）

### P1（功能完整性检查）

- [ ] 所有 15 种意图至少有一个评估案例，且至少有一个通过
- [ ] 6 个工具至少各有一个评估案例，且工具调用成功
- [ ] 5 个 Skill 至少各有一个评估案例，且 Skill 执行成功
- [ ] 新增的意图类型（多意图检测）有对应的测试
- [ ] 工具降级策略有对应的测试（模拟工具超时、返回空、返回异常值）
- [ ] Reflection 的 rerun_retrieval 路径有测试覆盖
- [ ] Prompt 模板的每个版本都能正常渲染（无缺失占位符）
- [ ] 新增评估案例全部通过 JSON 格式校验

### P2（质量保证检查）

- [ ] 评估 pass rate >= 0.50（在真实 LLM embedding + generation 配置下）
- [ ] faithfulness >= 0.85
- [ ] answer_correctness >= 0.75
- [ ] 意图准确率 >= 0.90
- [ ] Hit@5 >= 0.85
- [ ] 工具决策准确率 >= 0.90
- [ ] 安全红线案例 100% 触发安全停止
- [ ] P95 延迟 < 5000ms（单次 Agent 请求）
- [ ] 平均 Token 消耗 < 6000（单次 Agent 请求）

### P3（面试准备检查）

- [ ] `docs/architecture-decisions.md` 包含至少 6 个 ADR
- [ ] `docs/lessons-learned.md` 包含至少 4 个教训
- [ ] `docs/performance-benchmark.md` 包含不同配置的对比数据
- [ ] `docs/interview-notes.md` 为每个关键技术点提供了代码引用
- [ ] 每个 ADR 都有「如果选错会怎样」
- [ ] 面试者能从文档中理解为什么这个项目不是 demo

---

## 修复优先级建议

按以下顺序执行修复（每个阶段完成后运行测试确认无误）：

**第一阶段：数据与场景（不影响运行逻辑）**
1. 扩充知识库文档（62 → 115+ 篇）
2. 扩充评估案例（100 → 200 条）
3. 创建 ADR 文档、失败教训文档、性能基准文档

**第二阶段：意图与检索增强（核心链路改进）**
4. 意图体系分层（增加 Primary Intent）
5. 多意图检测
6. 歧义引导机制增强
7. 检索策略改进（多路互补 + 多跳推理 + 语义分块）

**第三阶段：Agent 核心能力增强**
8. ReAct 循环质量检查 + 并行调用
9. Plan 质量评估 + 多次修正
10. 工具参数 LLM 提取 + 降级策略增强
11. Skill 可观测性 + YAML 配置化
12. 反思检索策略自动切换 + 幻觉检测增强

**第四阶段：工程能力增强**
13. 模型路由优先级配置化 + 自适应熔断
14. 文档解析器（PDF/DOCX/HTML）
15. Prompt A/B 测试机制
16. Prometheus metrics
17. Bad case 分析工具

**第五阶段：安全与收尾**
18. API Key + JWT 认证
19. Prompt Injection 防护
20. 竞品问题处理
21. 更新面试准备文档
22. 全链路回归测试 + 性能基准测试

---

**请开始执行修复。每个阶段完成后，报告完成的条目、通过的测试数量、以及下一个阶段开始前需要确认的事项。**
