# MCP HTTP 回归评测记录

生成时间：2026-06-18 05:40 UTC

最终采用运行：`eval_ebef0f035d6d152e8199d104`

系统版本：`agentic-mcp-aggregate-general-quality-20260618-r2`

数据集：`v2`，完整 200 条，真实前端 + 后端 + 聚合 HTTP MCP + 真实 LLM 链路。

## 摘要

| 指标 | 结果 |
|---|---:|
| Total Cases | 200 |
| Passed Cases | 161 |
| Strict Pass Rate | 80.5% |
| P95 Latency | 10325 ms |
| Average Tokens | 3115.07 |
| Total Tokens | 623015 |
| Average ReAct Steps | 4.76 |

## 链路确认

| 项 | 结果 |
|---|---|
| Frontend entrypoint | `api`, `frontend` |
| MCP mode | `aggregate_http` |
| Real LLM | `true` |
| Frontend trace tokens | input `3327`, output `133`, total `3460` |
| Frontend screenshot | `D:\GoLang\CleanCaregent\.e2e\frontend-real-chain.png` |

## 对比

| Reference | Run | Pass Rate | Delta vs Previous Real | P95 Latency | Avg Tokens |
|---|---|---:|---:|---:|---:|
| previous-real-aggregate | `eval_89f9437d125d6749487f565e` | 60.5% | baseline | 15145 ms | 1776.66 |
| first-general-quality | `eval_22582c6bbc56c28aef7bde4b` | 74.0% | +13.5 pp | 13811 ms | 3731.31 |
| selected-final-r2 | `eval_ebef0f035d6d152e8199d104` | 80.5% | +20.0 pp | 10325 ms | 3115.07 |
| reverted-r3-experiment | `eval_82d5d86520b8918457e9b5bc` | 77.0% | +16.5 pp | 5669 ms | 1076.14 |

`reverted-r3-experiment` 是一次后续收敛执行路径的实验，降低了 token 和延迟，但质量回退到 77.0%，相关补丁已撤回，不作为最终代码结论。

## 失败类型

| Type | Count |
|---|---:|
| `hallucination_or_ungrounded` | 12 |
| `retrieval_miss` | 11 |
| `tool_selection_error` | 6 |
| `clarification_or_rejection_error` | 5 |
| `answer_incomplete_or_incorrect` | 3 |
| `intent_error` | 2 |

## 核心指标

| Metric | Value |
|---|---:|
| `answer_correctness` | 0.9400 |
| `answer_faithfulness` | 0.9785 |
| `answer_grounding_rate` | 0.9363 |
| `clarify_accuracy` | 0.9700 |
| `clarify_reject_accuracy` | 0.9500 |
| `context_precision` | 0.5338 |
| `context_recall` | 0.9721 |
| `false_acceptance_rate` | 0.0200 |
| `false_rejection_rate` | 0.0000 |
| `hit_at_5` | 0.9900 |
| `intent_accuracy` | 0.9900 |
| `mrr` | 0.9200 |
| `multi_step_completion_rate` | 0.9100 |
| `reject_accuracy` | 0.9800 |
| `safety_compliance` | 0.9800 |
| `tool_decision_accuracy` | 0.9700 |
| `tool_parameter_accuracy` | 1.0000 |
| `tool_result_utilization` | 0.8155 |
| `tool_selection_accuracy` | 0.9500 |

## 按意图

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `accessory_compatibility` | 12 | 13 | 92.3% |
| `chitchat` | 1 | 1 | 100.0% |
| `clarification` | 1 | 5 | 20.0% |
| `create_after_sales_ticket` | 7 | 7 | 100.0% |
| `inventory_query` | 10 | 10 | 100.0% |
| `order_query` | 7 | 11 | 63.6% |
| `out_of_scope` | 5 | 8 | 62.5% |
| `price_query` | 11 | 11 | 100.0% |
| `product_comparison` | 15 | 17 | 88.2% |
| `product_parameter` | 17 | 19 | 89.5% |
| `purchase_recommendation` | 23 | 26 | 88.5% |
| `return_eligibility` | 6 | 8 | 75.0% |
| `troubleshooting` | 13 | 23 | 56.5% |
| `usage_instruction` | 29 | 34 | 85.3% |
| `warranty_query` | 4 | 7 | 57.1% |

## 按路径

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `kb_tool` | 21 | 40 | 52.5% |
| `pure_kb` | 94 | 100 | 94.0% |
| `pure_tool` | 34 | 40 | 85.0% |
| `reject_guide` | 12 | 20 | 60.0% |

## 仍需继续改进

主要剩余短板集中在 `clarification`、`troubleshooting`、`warranty_query`、`order_query` 和 `kb_tool` 复合路径。失败主因不是 MCP 传输不可用，而是复杂查询下证据落地、检索精度、工具串联和拒答策略仍有缺口。
