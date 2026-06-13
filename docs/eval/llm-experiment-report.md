# CleanCare Agent 真实模型 200 条评测报告

## 结论

2026-06-13 使用真实 DeepSeek LLM、SiliconFlow Embedding/Reranker、
Qdrant、MySQL FULLTEXT 和 LLM-as-Judge 完成三轮完整 200 条评测。

| 运行 | 严格通过率 | P95 | 平均 Token |
|---|---:|---:|---:|
| `eval_b5d0110346cd680678539c83` | 31.0%（62/200） | 26,760 ms | 9,838 |
| `eval_d183d66b64a70f5d32698382` | 61.0%（122/200） | 21,018 ms | 4,531 |
| `eval_67abf7a0bdfe419fbdf92a68` | **71.5%（143/200）** | **17,077 ms** | **4,007** |

严格通过要求意图、检索、工具、语义正确性、忠实度、Grounding 和安全等
阻断指标同时通过，不等同于“回答看起来合理”。

## 实际链路

| 环节 | 实际配置 |
|---|---|
| Chat / Planner / Judge | DeepSeek `deepseek-chat` |
| Embedding | SiliconFlow `BAAI/bge-large-zh-v1.5`，1024 维 |
| Reranker | SiliconFlow `BAAI/bge-reranker-v2-m3` |
| Dense 检索 | Qdrant `clean_care_kb` |
| Keyword 检索 | MySQL FULLTEXT / 应用层 BM25 |
| 融合 | RRF |

Embedding 与 Reranker 的调用可从 Trace、结果 metadata 和熔断器状态验证。
SiliconFlow 对部分模型可能采用免费或赠送额度，因此余额未变化不能推断为未调用。

## 最终结果

运行编号：`eval_67abf7a0bdfe419fbdf92a68`

| 指标 | 结果 |
|---|---:|
| Strict Pass Rate | 71.5% |
| Intent Accuracy | 96.0% |
| Hit@5 | 94.5% |
| Context Recall | 96.04% |
| Answer Correctness | 92.10% |
| Answer Faithfulness | 97.35% |
| Answer Grounding Rate | 92.25% |
| Tool Decision Accuracy | 96.0% |
| Tool Selection Accuracy | 93.5% |
| Tool Parameter Accuracy | 99.5% |
| Multi-step Completion | 88.0% |
| P95 Latency | 17,077 ms |
| Average Tokens | 4,007 |

重点意图：

| 意图 | 最终通过 |
|---|---:|
| 商品对比 | 14/17（82.4%） |
| 购买推荐 | 21/26（80.8%） |
| 创建售后工单 | 7/7（100%） |
| 商品参数 | 16/19（84.2%） |
| 价格查询 | 11/11（100%） |
| 库存查询 | 9/10（90.0%） |

## 本轮关键修复

1. 启用远程 Cross-Encoder Reranker，并保留本地词法降级。
2. 对比/推荐使用场景指南与型号参数双路检索，修复指南路由错误携带
   单型号 Metadata Filter 的问题。
3. 跨品类组合保留全部 category，例如 P400 + H100 同时检索净化器和加湿器。
4. 补齐“哪个便宜、两款到手价、实时价、总价”等意图与工具映射。
5. 售后建单保持明确确认、订单号、幂等键和工具结果校验；成功建单答案不再
   被后续 LLM 反思覆盖。
6. 确定性动态结果优先直接输出，减少无必要生成和反思。
7. 评测增加 case ID 定向运行、等价证据、工具结果 Grounding 和 Bad Case 分类。

## 边界与剩余问题

- 动态价格、库存、订单、保修和售后数据仍为本地 Mock，并标记
  `data_scope=mock`；未接真实 ERP、支付或物流。
- P95 17.1 秒仍偏高，主要来自复杂故障/售后链路和多次真实模型调用。
- 故障诊断 8/23、配件兼容 6/13、保修 3/7，仍是下一轮主要优化方向。
- 最终失败分类仍有 retrieval 14、grounding 18、intent 8、tool selection 8。
- 结果来自单机串行评测，不代表生产并发 SLA。
