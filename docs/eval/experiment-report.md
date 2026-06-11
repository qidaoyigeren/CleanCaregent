# CleanCare Agent 历史本地基线评估报告

评估日期：2026-06-11  
数据集：`v1`，100 条  
知识资产：57 篇生成式 mock 文档  
运行环境：本地 MySQL、Redis、Qdrant，`local_hash` Embedding，抽取式 Generator

> 本报告记录的是改造前的可复现基线，不代表当前 LLM 意图分类、查询改写、受控 ReAct、远程 Rerank 和 LLM Reflection 的效果。当前版本需要使用新的 `system_version` 重跑完整 100 条评估后再做正式对比。

## 路径分布

| 路径 | 数量 |
|---|---:|
| 纯 KB 查询 | 45 |
| KB 多文档检索 | 20 |
| KB + Tool | 20 |
| 故障诊断 | 10 |
| 拒答 / 澄清 | 5 |

## 实测对比

| 指标 | Naive RAG | Agentic RAG v4 |
|---|---:|---:|
| Intent Accuracy | 1.000 | 1.000 |
| Hit@5 | 0.790 | 0.840 |
| MRR | 0.659 | 0.718 |
| Context Recall | 0.790 | 0.860 |
| Context Precision | 0.156 | 0.306 |
| Tool Decision Accuracy | 0.800 | 1.000 |
| Tool Selection Accuracy | 0.800 | 1.000 |
| Tool Parameter Accuracy | 0.870 | 1.000 |
| Answer Faithfulness | 0.890 | 1.000 |
| Answer Correctness | 0.468 | 0.495 |
| Multi-step Completion | 0.690 | 0.860 |
| Clarify / Reject Accuracy | 0.950 | 1.000 |
| P95 Latency | 73 ms | 59 ms |
| Average Tokens（近似） | 131.50 | 135.28 |
| Average Steps | 1.00 | 3.92 |
| Strict Pass Rate | 0.260 | 0.400 |

运行记录：

- Naive RAG：`eval_781d2fbc16d3b2b1473961b2`
- Agentic RAG v4：`eval_78b9826262d6919098de9023`

## 结论

Agentic 版本的主要收益来自动态工具和受控 Skill。价格、订单、保修和配件购买链路不再依赖静态文本猜测；工具决策、选择和参数指标达到 1.0。多文档任务的 Hit@5、MRR 和 Context Precision 也有提升。

严格通过率仍只有 0.40，主要失败集中在 `answer_correctness`、`context_precision` 和部分 `hit_at_5`。这是合理的初版结果：本地哈希向量和抽取式生成器只能验证工程链路，不能代表 BGE + Qwen/DeepSeek 的真实语义效果。

## Bad Case

1. 近义问法会召回同品类多个文档，导致 Context Precision 偏低。
2. 抽取式回答能保持忠实，但对推荐结论和多条件归纳能力有限。
3. 当前故障评估以单条 query 表示多轮节点，尚未计算逐轮状态迁移准确率。
4. Answer Correctness 使用确定性 token overlap，不是 LLM Judge，适合作为回归指标但不等于人工评分。

## 后续优化

1. 接入 BGE/text-embedding-v3，重新调节 dense/keyword 权重和分文档类型 TopK。
2. 接入 bge-reranker/gte-rerank，并对表格、政策、故障树使用不同候选规模。
3. 使用 Qwen/DeepSeek 生成结构化回答，再由 Grounding Review 校验。
4. 增加真实多轮故障 case、政策边界 case 和工具失败注入测试。
5. 将评估结果按 Prompt、模型、知识库版本做长期对比。

> 本报告数值来自本地 mock 实测，不应写成线上生产指标，也不能直接作为当前版本的结果。

## 当前版本 LLM 冒烟回归

运行日期：2026-06-11  
系统版本：`agentic-llm-v5-smoke`  
运行记录：`eval_81f43ea12966cc0860b6dbd4`

本次只取评估集前 3 条验证异步 Eval Runner、LLM 组件、检索、生成和 Judge 链路：

| 指标 | 结果 |
|---|---:|
| 通过数 | 3 / 3 |
| Intent Accuracy | 1.000 |
| Hit@5 | 1.000 |
| Context Recall | 1.000 |
| Context Precision | 0.333 |
| Answer Faithfulness | 1.000 |
| Answer Correctness | 1.000 |
| P95 Latency | 5982 ms |
| Average Tokens | 1445 |
| Average ReAct Steps | 4 |

评估创建接口在本次调用中约 125 ms 返回 `202 Accepted`，计算在后台继续执行。该结果仅证明当前链路可运行，不具备统计代表性；正式版本对比仍需重跑完整 100 条评估集。
