# MCP HTTP 回归评测记录

生成时间：2026-06-18 08:16:52 UTC
运行编号：`eval_1f78637258282add3a72ef35`
系统版本：`agentic-multiturn-context-20260618`
数据集：`v2`
状态：`completed`

## 摘要

| 指标 | 结果 |
|---|---:|
| Total Cases | 200 |
| Passed Cases | 153 |
| Strict Pass Rate | 76.5% |
| P95 Latency | 12855 ms |
| Average Tokens | 3144.49 |
| Average ReAct Steps | 4.72 |

## 变化参考

参考基线来自历史报告，若模型、Embedding、Reranker 或数据集不同，只能用于观察趋势，不能作为严格 A/B 结论。

| Reference | Pass Rate | Pass Delta | P95 Latency | P95 Delta | Avg Tokens | Token Delta |
|---|---:|---:|---:|---:|---:|---:|
| eval_ebef0f035d6d152e8199d104 | 80.5% | -4.0 pp | 10325 ms | +2530.0 ms | 3115.07 | +29.4 |

## 失败类型

| Type | Count |
|---|---:|
| `hallucination_or_ungrounded` | 17 |
| `retrieval_miss` | 10 |
| `intent_error` | 9 |
| `tool_selection_error` | 6 |
| `clarification_or_rejection_error` | 3 |
| `answer_incomplete_or_incorrect` | 2 |

## 核心指标

| Metric | Value |
|---|---:|
| `answer_correctness` | 0.9377 |
| `answer_faithfulness` | 0.9785 |
| `answer_grounding_rate` | 0.9321 |
| `clarify_accuracy` | 0.9750 |
| `clarify_reject_accuracy` | 0.9600 |
| `context_precision` | 0.5346 |
| `context_recall` | 0.9671 |
| `efficiency_score` | 0.3521 |
| `false_acceptance_rate` | 0.0150 |
| `false_rejection_rate` | 0.0000 |
| `hit_at_5` | 0.9800 |
| `intent_accuracy` | 0.9550 |
| `latency_ms` | 4885.4900 |
| `mrr` | 0.8865 |
| `multi_step_completion` | 0.9100 |
| `multi_step_completion_rate` | 0.9100 |
| `react_steps` | 4.7200 |
| `reject_accuracy` | 0.9850 |
| `safety_compliance` | 0.9850 |
| `safety_violation_rate` | 0.0150 |
| `self_correction_success_rate` | 1.0000 |
| `token_count` | 3144.4900 |
| `tool_decision_accuracy` | 0.9700 |
| `tool_parameter_accuracy` | 1.0000 |
| `tool_result_utilization` | 0.8138 |
| `tool_selection_accuracy` | 0.9550 |

## 按意图

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `accessory_compatibility` | 12 | 13 | 92.3% |
| `chitchat` | 1 | 1 | 100.0% |
| `clarification` | 4 | 5 | 80.0% |
| `create_after_sales_ticket` | 7 | 7 | 100.0% |
| `inventory_query` | 10 | 10 | 100.0% |
| `order_query` | 7 | 11 | 63.6% |
| `out_of_scope` | 6 | 8 | 75.0% |
| `price_query` | 11 | 11 | 100.0% |
| `product_comparison` | 16 | 17 | 94.1% |
| `product_parameter` | 11 | 19 | 57.9% |
| `purchase_recommendation` | 18 | 26 | 69.2% |
| `return_eligibility` | 7 | 8 | 87.5% |
| `troubleshooting` | 12 | 23 | 52.2% |
| `usage_instruction` | 28 | 34 | 82.4% |
| `warranty_query` | 3 | 7 | 42.9% |

## 按难度

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `hard` | 32 | 50 | 64.0% |
| `medium` | 52 | 70 | 74.3% |
| `simple` | 69 | 80 | 86.2% |

## 按路径

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `kb_tool` | 20 | 40 | 50.0% |
| `pure_kb` | 84 | 100 | 84.0% |
| `pure_tool` | 33 | 40 | 82.5% |
| `reject_guide` | 16 | 20 | 80.0% |

## 失败样例

| Case | Error Type | Intent | Tools | Latency | Trace |
|---|---|---|---|---:|---|
| `EVAL-001` | `intent_error` | `purchase_recommendation` | - | 3568 ms | `tr_8fd4be3a81406f6732ddf732` |
| `EVAL-002` | `intent_error` | `purchase_recommendation` | - | 2937 ms | `tr_05daf45180765c5e5ea09dd3` |
| `EVAL-003` | `intent_error` | `purchase_recommendation` | - | 4992 ms | `tr_730872e8fadc823619060f1f` |
| `EVAL-005` | `intent_error` | `purchase_recommendation` | - | 4206 ms | `tr_f8189df8608fa71b66609acd` |
| `EVAL-008` | `intent_error` | `purchase_recommendation` | - | 3577 ms | `tr_63cb2084e0fa9ed3d7f4dc8e` |
| `EVAL-014` | `intent_error` | `purchase_recommendation` | - | 5020 ms | `tr_7fe861ff0c72fa21043735af` |
| `EVAL-037` | `intent_error` | `purchase_recommendation` | - | 3204 ms | `tr_cf1e73345d8c9e96a1c874db` |
| `EVAL-059` | `retrieval_miss` | `purchase_recommendation` | - | 10357 ms | `tr_7d9d09bf02c082c2b369a438` |
| `EVAL-060` | `hallucination_or_ungrounded` | `purchase_recommendation` | - | 15353 ms | `tr_8d21d375d6254472e6b8737c` |
| `EVAL-086` | `hallucination_or_ungrounded` | `troubleshooting` | - | 9726 ms | `tr_a708d920c38f08028f2ceb54` |
| `EVAL-087` | `hallucination_or_ungrounded` | `troubleshooting` | - | 11109 ms | `tr_e34bc409378ebbc243702b15` |
| `EVAL-089` | `hallucination_or_ungrounded` | `troubleshooting` | - | 6929 ms | `tr_00a6e9bdefaf899738188c92` |
| `EVAL-093` | `retrieval_miss` | `troubleshooting` | - | 11064 ms | `tr_91373e12dee473ba5470ed3d` |
| `EVAL-094` | `answer_incomplete_or_incorrect` | `troubleshooting` | - | 4886 ms | `tr_2a8be46f54cf17d15a7035eb` |
| `EVAL-119` | `hallucination_or_ungrounded` | `troubleshooting` | - | 495 ms | `tr_293d422901851af48505386e` |
| `EVAL-122` | `retrieval_miss` | `product_comparison` | - | 5834 ms | `tr_7c1fac03d760685ee015d1d0` |
| `EVAL-132` | `hallucination_or_ungrounded` | `warranty_query` | warranty_check | 621 ms | `tr_4efbd1f41c2771d7f527821e` |
| `EVAL-138` | `intent_error` | `usage_instruction` | - | 10755 ms | `tr_1f952fa823c47437799f91b1` |
| `EVAL-139` | `hallucination_or_ungrounded` | `order_query` | order_lookup | 4980 ms | `tr_cbb97654f72e4459cef81f82` |
| `EVAL-140` | `hallucination_or_ungrounded` | `order_query` | user_purchase_history | 4439 ms | `tr_afd811f548d49aa76f021c9d` |
| `EVAL-141` | `hallucination_or_ungrounded` | `warranty_query` | order_lookup, warranty_check | 571 ms | `tr_ccf191d66b9d21d4cbab7c83` |
| `EVAL-150` | `tool_selection_error` | `warranty_query` | - | 2871 ms | `tr_32dbed0d54ff6a7fe8c0e15d` |
| `EVAL-151` | `hallucination_or_ungrounded` | `order_query` | user_purchase_history, user_purchase_history | 12855 ms | `tr_d71aa666ba9dce031daefc3d` |
| `EVAL-153` | `hallucination_or_ungrounded` | `purchase_recommendation` | price_query | 3169 ms | `tr_0aa13e70fe01a0a7be8a61b5` |
| `EVAL-154` | `retrieval_miss` | `product_parameter` | inventory_check | 980 ms | `tr_7f378969a5c9f97382990f8e` |
| `EVAL-156` | `hallucination_or_ungrounded` | `purchase_recommendation` | inventory_check | 2936 ms | `tr_c61a187546a8e34f21a5ed03` |
| `EVAL-157` | `hallucination_or_ungrounded` | `product_parameter` | price_query | 469 ms | `tr_c75b9a8316613bcb874a2c7e` |
| `EVAL-158` | `hallucination_or_ungrounded` | `accessory_compatibility` | inventory_check, price_query | 461 ms | `tr_2e08172bdffab6a93bdeb102` |
| `EVAL-159` | `retrieval_miss` | `usage_instruction` | inventory_check | 638 ms | `tr_30218050c26128cc49ad399b` |
| `EVAL-160` | `hallucination_or_ungrounded` | `usage_instruction` | price_query | 911 ms | `tr_c95a48124e43c11f10d8968e` |
| `EVAL-162` | `tool_selection_error` | `usage_instruction` | - | 2755 ms | `tr_d543985c06259ba9693b05f7` |
| `EVAL-166` | `answer_incomplete_or_incorrect` | `usage_instruction` | order_lookup | 360 ms | `tr_69665adbcec8162ee50f5106` |
| `EVAL-171` | `hallucination_or_ungrounded` | `usage_instruction` | price_query | 2536 ms | `tr_338bc7ca75a3318d2db9a9fc` |
| `EVAL-173` | `retrieval_miss` | `purchase_recommendation` | price_query, inventory_check | 6895 ms | `tr_8f6c56e9f8a895aa87c4576c` |
| `EVAL-176` | `tool_selection_error` | `troubleshooting` | user_purchase_history, user_purchase_history | 644 ms | `tr_b18546dd3a89417d9c79bad9` |
| `EVAL-177` | `tool_selection_error` | `troubleshooting` | - | 548 ms | `tr_9b073e96138e4c13e8ee1780` |
| `EVAL-178` | `retrieval_miss` | `purchase_recommendation` | price_query, inventory_check | 3955 ms | `tr_f0a612c70177387b80bbd15c` |
| `EVAL-179` | `intent_error` | `usage_instruction` | price_query, price_query, inventory_check | 4022 ms | `tr_0903b37ff1a314883317ff33` |
| `EVAL-180` | `retrieval_miss` | `return_eligibility` | warranty_check, order_lookup | 630 ms | `tr_2bac46afe163070648188296` |
| `EVAL-181` | `retrieval_miss` | `warranty_query` | user_purchase_history, user_purchase_history | 584 ms | `tr_00990f1da3d2d656968ae791` |
| `EVAL-182` | `tool_selection_error` | `troubleshooting` | user_purchase_history, user_purchase_history | 559 ms | `tr_bf28f0531c285ebeb5822001` |
| `EVAL-184` | `hallucination_or_ungrounded` | `purchase_recommendation` | price_query, inventory_check | 3184 ms | `tr_cbfb83c0d05dc4abd985a9ed` |
| `EVAL-185` | `retrieval_miss` | `troubleshooting` | - | 1214 ms | `tr_d6ac9364a7ab1031e16b347f` |
| `EVAL-187` | `clarification_or_rejection_error` | `clarification` | - | 1562 ms | `tr_14c383ca9f7a6e1b7ecc165b` |
| `EVAL-194` | `clarification_or_rejection_error` | `out_of_scope` | - | 18009 ms | `tr_c505c89ed4180e9ce3467ec1` |
| `EVAL-195` | `tool_selection_error` | `out_of_scope` | user_purchase_history | 1291 ms | `tr_daaa2ec47a3845384f696fea` |
| `EVAL-198` | `clarification_or_rejection_error` | `troubleshooting` | - | 30002 ms | `tr_aaa8337c593688f86f9bcb86` |

## 复现

```powershell
python3 scripts/eval-regression-report.py --base-url http://127.0.0.1:8080 --system-version agentic-multiturn-context-20260618 --max-cases 200 --output docs/eval/mcp-regression-report.md
```
