# CleanCare Agent 性能基准

## 数据可信度声明

本文只把已保存运行记录标为“实测”。没有对应运行 ID 的并发压测、当前代码版本重跑或外部服务组合结果均标为“待测”，不使用设计目标冒充结果。

## 已有实测基线

来源：`docs/eval/experiment-report.md`，日期 2026-06-11，100 条 v1 数据集，本地 MySQL/Redis/Qdrant，`local_hash` Embedding + extractive Generator。

| 指标 | Naive RAG | Agentic RAG v4 |
|---|---:|---:|
| Strict Pass Rate | 0.260 | 0.400 |
| Hit@5 | 0.790 | 0.840 |
| Context Precision | 0.156 | 0.306 |
| Multi-step Completion | 0.690 | 0.860 |
| P95 延迟 | 73 ms | 59 ms |
| 平均 Token（近似） | 131.50 | 135.28 |
| 平均步骤 | 1.00 | 3.92 |

运行 ID：

- Naive RAG：`eval_781d2fbc16d3b2b1473961b2`
- Agentic RAG：`eval_78b9826262d6919098de9023`

这些数据只能说明本地确定性链路的相对变化。Agentic P95 更低可能受缓存、样本顺序和本地抽取器影响，不能据此断言复杂 Agent 天然更快。

## LLM 冒烟实测

2026-06-11 的 3 条链路冒烟运行 `eval_81f43ea12966cc0860b6dbd4`：

| 指标 | 结果 |
|---|---:|
| 通过数 | 3 / 3 |
| P95 延迟 | 5982 ms |
| 平均 Token | 1445 |
| 平均 ReAct 步数 | 4 |

样本只有 3 条，不具备统计代表性。

## 真实模型 200 条串行实测

来源：`docs/eval/llm-experiment-report.md`，日期 2026-06-13，200 条 v2 数据集，DeepSeek LLM、SiliconFlow Embedding/Reranker、Qdrant、MySQL FULLTEXT 和 LLM-as-Judge。

| 运行 ID | Strict Pass Rate | P95 延迟 | 平均 Token |
|---|---:|---:|---:|
| `eval_67abf7a0bdfe419fbdf92a68` | 71.5% | 17,077 ms | 4,007 |

该结果是单机串行评测记录，不代表并发 SLA；MCP HTTP transport、CI 和工具链后续变更后的当前版本指标需要重新运行后记录。

## 当前资产规模

| 项目 | 当前值 |
|---|---:|
| 知识文档 | 143 |
| 商品详情 | 50 |
| 选购指南 | 15 |
| 使用手册 | 25 |
| 售后政策 | 15 |
| 故障排查 | 10 |
| 评测案例 | 200 |
| 评测难度 | simple 80 / medium 70 / hard 50 |

## 待测矩阵

| 维度 | 配置 | 状态 | 验收指标 |
|---|---|---|---|
| 运行模式 | bootstrap / naive_rag / agentic | 待按当前版本重跑 200 条 | P50/P95、成功率、Token |
| Embedding | local_hash / OpenAI-compatible | 已有 local_hash 与 SiliconFlow 串行记录；其他端点待测 | Hit@5、MRR、召回延迟 |
| Rerank | local_lexical / BGE-compatible | 已有 local_lexical 与 SiliconFlow BGE 串行记录；其他端点待测 | Context Precision、P95 |
| Generator | extractive / Qwen / DeepSeek compatible | 已有 extractive 与 DeepSeek 串行记录；其他端点待测 | Faithfulness、Correctness |
| 并发 | 10 / 50 / 100 | 待压测环境 | 成功率、P95、429 比例 |

## Token 分布采集方案

需要按以下阶段写入 trace：意图分类、查询改写、规划、生成、反思。当前实现已记录请求级 Prompt/Completion Token、模型名和估算成本，并通过 `internal/observability/prometheus.go` 暴露计数器与延迟直方图。2026-06-13 的 200 条真实模型评测已记录运行级平均 Token；分阶段 Token 成本拆分仍需在后续当前版本回归中补齐和验证。

## ReAct 步数分布

当前历史基线只有平均步骤 3.92，没有完整分桶。正式报告应至少输出 `1/2/3/4/5` 步数量、重复步骤比例、Reflection rerun 比例和超预算终止比例。

## 可复现命令

```powershell
go test ./... -count=1
go run ./cmd/eval-dataset -output docs/eval/eval-cases-v2.json
make eval-compare
make eval-regression
```

真实 LLM 测试前必须通过环境变量配置 API Key，不得写入仓库。若 MySQL、Redis、Qdrant 或远程模型不可用，报告应记录为“未执行”，不能填入估算值。
