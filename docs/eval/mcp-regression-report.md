# MCP HTTP 回归评测记录

生成时间：2026-06-19 12:14:41 UTC
运行编号：`eval_72051bf5bee3a2f98ee302b0`
系统版本：`agentic-aftersales-loop-20260619`
数据集：`v2`
状态：`completed`

## 摘要

| 指标 | 结果 |
|---|---:|
| Total Cases | 200 |
| Passed Cases | 161 |
| Strict Pass Rate | 80.5% |
| Tool Accuracy | 98.0% |
| False Action Rate | 0.0% |
| PII Leak Rate | 0.0% |
| P95 Latency | 10386 ms |
| Average Tokens | 3221.72 |
| Average ReAct Steps | 4.75 |

## 变化参考

参考基线来自历史报告，若模型、Embedding、Reranker 或数据集不同，只能用于观察趋势，不能作为严格 A/B 结论。

| Reference | Pass Rate | Pass Delta | P95 Latency | P95 Delta | Avg Tokens | Token Delta |
|---|---:|---:|---:|---:|---:|---:|
| previous local MCP regression | 62.5% | +18.0 pp | 0 ms | n/a | 0.00 | n/a |

## 失败类型

| Type | Count |
|---|---:|
| `hallucination_or_ungrounded` | 16 |
| `intent_error` | 9 |
| `retrieval_miss` | 6 |
| `tool_selection_error` | 4 |
| `clarification_or_rejection_error` | 3 |
| `tool_parameter_error` | 1 |

## 核心指标

| Metric | Value |
|---|---:|
| `answer_correctness` | 0.9430 |
| `answer_faithfulness` | 0.9718 |
| `answer_grounding_rate` | 0.9447 |
| `clarify_accuracy` | 0.9900 |
| `clarify_reject_accuracy` | 0.9850 |
| `context_precision` | 0.5343 |
| `context_recall` | 0.9571 |
| `efficiency_score` | 0.3642 |
| `false_acceptance_rate` | 0.0050 |
| `false_action_rate` | 0.0000 |
| `false_rejection_rate` | 0.0000 |
| `hit_at_5` | 0.9600 |
| `intent_accuracy` | 0.9500 |
| `latency_ms` | 4010.5650 |
| `mrr` | 0.9005 |
| `multi_step_completion` | 0.9150 |
| `multi_step_completion_rate` | 0.9150 |
| `pii_leak_rate` | 0.0000 |
| `react_steps` | 4.7550 |
| `reject_accuracy` | 0.9950 |
| `safety_compliance` | 0.9950 |
| `safety_violation_rate` | 0.0050 |
| `self_correction_success_rate` | 1.0000 |
| `token_count` | 3221.7200 |
| `tool_accuracy` | 0.9800 |
| `tool_decision_accuracy` | 0.9900 |
| `tool_parameter_accuracy` | 1.0000 |
| `tool_result_utilization` | 0.8165 |
| `tool_selection_accuracy` | 0.9800 |

## 按意图

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `accessory_compatibility` | 11 | 13 | 84.6% |
| `chitchat` | 1 | 1 | 100.0% |
| `clarification` | 2 | 5 | 40.0% |
| `create_after_sales_ticket` | 5 | 7 | 71.4% |
| `inventory_query` | 10 | 10 | 100.0% |
| `order_query` | 7 | 11 | 63.6% |
| `out_of_scope` | 8 | 8 | 100.0% |
| `price_query` | 10 | 11 | 90.9% |
| `product_comparison` | 14 | 17 | 82.4% |
| `product_parameter` | 18 | 19 | 94.7% |
| `purchase_recommendation` | 20 | 26 | 76.9% |
| `return_eligibility` | 7 | 8 | 87.5% |
| `troubleshooting` | 18 | 23 | 78.3% |
| `usage_instruction` | 27 | 34 | 79.4% |
| `warranty_query` | 3 | 7 | 42.9% |

## 按难度

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `hard` | 40 | 50 | 80.0% |
| `medium` | 54 | 70 | 77.1% |
| `simple` | 67 | 80 | 83.8% |

## 按路径

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `kb_tool` | 26 | 40 | 65.0% |
| `pure_kb` | 89 | 100 | 89.0% |
| `pure_tool` | 30 | 40 | 75.0% |
| `reject_guide` | 16 | 20 | 80.0% |

## 失败样例

| Case | Error Type | Intent | Tools | Latency | Trace |
|---|---|---|---|---:|---|
| `EVAL-021` | `intent_error` | `product_parameter` | - | 3083 ms | `tr_edd06ae8153e3644b27509b7` |
| `EVAL-037` | `intent_error` | `product_parameter` | - | 2416 ms | `tr_d8ede8138e60b93f20fa5a8d` |
| `EVAL-040` | `intent_error` | `product_parameter` | - | 24078 ms | `tr_1b2556d520ee7290f5102371` |
| `EVAL-049` | `intent_error` | `product_parameter` | - | 3673 ms | `tr_f68ec54092a1e3c7e1120a20` |
| `EVAL-070` | `tool_parameter_error` | `accessory_compatibility` | user_purchase_history, user_purchase_history, price_query, price_query | 30000 ms | `tr_30f91f79814e1b50853bcd28` |
| `EVAL-086` | `hallucination_or_ungrounded` | `troubleshooting` | - | 2355 ms | `tr_258ef70a04408951e0d075a1` |
| `EVAL-089` | `hallucination_or_ungrounded` | `troubleshooting` | - | 511 ms | `tr_61e1cfa5e3bb2c89430fe03c` |
| `EVAL-103` | `intent_error` | `product_parameter` | - | 2844 ms | `tr_12eda14def26285a44ec0224` |
| `EVAL-108` | `intent_error` | `product_parameter` | - | 2972 ms | `tr_18485c8f2a65a6465f070221` |
| `EVAL-111` | `hallucination_or_ungrounded` | `usage_instruction` | - | 3273 ms | `tr_08fc7f7f78e624d5e2fce9c5` |
| `EVAL-119` | `hallucination_or_ungrounded` | `troubleshooting` | - | 412 ms | `tr_9c6ca24e58f4ecccb40c2c49` |
| `EVAL-122` | `retrieval_miss` | `product_comparison` | - | 6718 ms | `tr_9a4da7b59ec09531960e907b` |
| `EVAL-126` | `tool_selection_error` | `price_query` | user_purchase_history, price_query | 24 ms | `tr_a13bbbb6feea0c524c585687` |
| `EVAL-132` | `hallucination_or_ungrounded` | `warranty_query` | warranty_check | 510 ms | `tr_c37b1c10816e1866d89f633a` |
| `EVAL-133` | `hallucination_or_ungrounded` | `create_after_sales_ticket` | create_after_sales_ticket | 23 ms | `tr_cc89a77e9f62480a98fe9c3d` |
| `EVAL-138` | `intent_error` | `usage_instruction` | - | 12538 ms | `tr_3a77003189ea5696d9a37b5f` |
| `EVAL-139` | `hallucination_or_ungrounded` | `order_query` | order_lookup | 3620 ms | `tr_9cf044a60787b72da1f755e4` |
| `EVAL-140` | `hallucination_or_ungrounded` | `order_query` | user_purchase_history | 8228 ms | `tr_19d8fb6729e10440ed9b9069` |
| `EVAL-141` | `hallucination_or_ungrounded` | `warranty_query` | warranty_check | 891 ms | `tr_cc32c8fc57200f09538e6449` |
| `EVAL-146` | `hallucination_or_ungrounded` | `create_after_sales_ticket` | create_after_sales_ticket | 20 ms | `tr_0eb17cc4d1b2f972c7b96d17` |
| `EVAL-150` | `hallucination_or_ungrounded` | `warranty_query` | user_purchase_history, warranty_check, warranty_check | 2780 ms | `tr_71be438aa6a58dbed6a67514` |
| `EVAL-151` | `hallucination_or_ungrounded` | `order_query` | user_purchase_history, user_purchase_history | 10294 ms | `tr_c16e3ff7ccb45044bc875607` |
| `EVAL-153` | `hallucination_or_ungrounded` | `purchase_recommendation` | price_query | 3728 ms | `tr_a3947d2597a0fd69fade0425` |
| `EVAL-154` | `retrieval_miss` | `product_parameter` | inventory_check | 4020 ms | `tr_10fa0a71e74ccddacbb73ee6` |
| `EVAL-156` | `intent_error` | `product_parameter` | inventory_check | 3032 ms | `tr_afed47e84215da694a7e6dbe` |
| `EVAL-158` | `hallucination_or_ungrounded` | `accessory_compatibility` | inventory_check, price_query | 428 ms | `tr_47a3c90dc9b3e2093f8baf2a` |
| `EVAL-159` | `tool_selection_error` | `inventory_query` | inventory_check | 18678 ms | `tr_e0c981f2b03064d0f4963fce` |
| `EVAL-160` | `hallucination_or_ungrounded` | `usage_instruction` | price_query | 5040 ms | `tr_dd1f048b53b5df603342faf3` |
| `EVAL-162` | `tool_selection_error` | `usage_instruction` | - | 2927 ms | `tr_fb9504f19d9b570d194b15c2` |
| `EVAL-164` | `intent_error` | `inventory_query` | inventory_check | 14086 ms | `tr_d14149df5bf72ad8a7086e16` |
| `EVAL-173` | `retrieval_miss` | `purchase_recommendation` | price_query, inventory_check | 6133 ms | `tr_3a10032e0d97799b1fe824e1` |
| `EVAL-175` | `tool_selection_error` | `troubleshooting` | warranty_check | 495 ms | `tr_2c182cb52e199b0f8816b375` |
| `EVAL-178` | `retrieval_miss` | `purchase_recommendation` | price_query, inventory_check | 6019 ms | `tr_a024036321db6c38e264d032` |
| `EVAL-180` | `retrieval_miss` | `return_eligibility` | order_lookup, warranty_check | 534 ms | `tr_9533467aff5ed969728c3202` |
| `EVAL-181` | `retrieval_miss` | `warranty_query` | user_purchase_history, warranty_check, warranty_check | 593 ms | `tr_21c793e128b7e1a681731fac` |
| `EVAL-187` | `clarification_or_rejection_error` | `clarification` | - | 1422 ms | `tr_870e4892a0fb4387c4cd0728` |
| `EVAL-188` | `hallucination_or_ungrounded` | `clarification` | - | 1433 ms | `tr_a0265e95b246717aac5e8e87` |
| `EVAL-190` | `clarification_or_rejection_error` | `clarification` | - | 3905 ms | `tr_8e373a1e4aeeed73cf85229b` |
| `EVAL-198` | `clarification_or_rejection_error` | `troubleshooting` | - | 537 ms | `tr_2be7be74335b9802dedae0e0` |

## 复现

```powershell
python3 scripts/eval-regression-report.py --base-url http://127.0.0.1:8080 --system-version agentic-aftersales-loop-20260619 --max-cases 200 --output docs\eval\mcp-regression-report.md
```
