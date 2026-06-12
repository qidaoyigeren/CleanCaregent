# CleanCare Agent 真实模型实验记录

## 结论边界

截至 2026-06-12，仓库保存的真实模型记录只有 3 条链路冒烟，不足以证明完整 200 条数据集上的质量或并发性能。本文只记录可复核事实，其余项目明确标为待测。

## 已执行冒烟

运行 ID：`eval_81f43ea12966cc0860b6dbd4`

| 指标 | 结果 |
|---|---:|
| 样本数 | 3 |
| 通过数 | 3 |
| P95 延迟 | 5982 ms |
| 平均 Token | 1445 |
| 平均 ReAct 步数 | 4 |

该运行验证了 OpenAI-compatible LLM、Agent 规划、工具与 Trace 链路能够闭环，不代表统计意义上的线上效果。

## 待执行矩阵

| 任务 | 数据量 | 需要输出 |
|---|---:|---|
| Naive RAG vs Agentic RAG | 200 | Hit@5、MRR、Correctness、Faithfulness、安全与工具利用率 |
| Qwen-compatible vs DeepSeek-compatible | 200 | 质量、P95、Token、估算成本、Fallback 次数 |
| Prompt A/B | 指定 case 集 | 胜率、逐 case 理由、回归项 |
| 并发压测 | 10/50/100 | 成功率、P95、429、熔断与恢复时间 |

## 可复现入口

```powershell
make eval-regression
make eval-compare
go run ./cmd/analyze-traces
```

API Key 仅通过 `CLEANCARE_*` 环境变量注入。没有运行 ID 和原始 Trace 的结果不得写成“实测”。
