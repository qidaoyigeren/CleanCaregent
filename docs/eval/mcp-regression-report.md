# MCP HTTP 回归评测记录

生成时间：2026-06-18 04:31:36 UTC
运行编号：`eval_ebef0f035d6d152e8199d104`
系统版本：`agentic-mcp-aggregate-general-quality-20260618-r2`
数据集：`v2`
状态：`completed`

## 摘要

| 指标 | 结果 |
|---|---:|
| Total Cases | 200 |
| Passed Cases | 161 |
| Strict Pass Rate | 80.5% |
| P95 Latency | 10325 ms |
| Average Tokens | 3115.07 |
| Average ReAct Steps | 4.76 |

## 变化参考

参考基线来自历史报告，若模型、Embedding、Reranker 或数据集不同，只能用于观察趋势，不能作为严格 A/B 结论。

| Reference | Pass Rate | Pass Delta | P95 Latency | P95 Delta | Avg Tokens | Token Delta |
|---|---:|---:|---:|---:|---:|---:|
| first-general-quality-74.0 | 74.0% | +6.5 pp | 13811 ms | -3486.0 ms | 3731.31 | -616.2 |

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
| `efficiency_score` | 0.3673 |
| `false_acceptance_rate` | 0.0200 |
| `false_rejection_rate` | 0.0000 |
| `hit_at_5` | 0.9900 |
| `intent_accuracy` | 0.9900 |
| `latency_ms` | 4391.6200 |
| `mrr` | 0.9200 |
| `multi_step_completion` | 0.9100 |
| `multi_step_completion_rate` | 0.9100 |
| `react_steps` | 4.7650 |
| `reject_accuracy` | 0.9800 |
| `safety_compliance` | 0.9800 |
| `safety_violation_rate` | 0.0200 |
| `self_correction_success_rate` | 1.0000 |
| `token_count` | 3115.0750 |
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

## 按难度

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `hard` | 36 | 50 | 72.0% |
| `medium` | 54 | 70 | 77.1% |
| `simple` | 71 | 80 | 88.8% |

## 按路径

| Group | Passed | Total | Pass Rate |
|---|---:|---:|---:|
| `kb_tool` | 21 | 40 | 52.5% |
| `pure_kb` | 94 | 100 | 94.0% |
| `pure_tool` | 34 | 40 | 85.0% |
| `reject_guide` | 12 | 20 | 60.0% |

## 失败样例

| Case | Error Type | Intent | Tools | Latency | Trace |
|---|---|---|---|---:|---|
| `EVAL-086` | `hallucination_or_ungrounded` | `troubleshooting` | - | 8826 ms | `tr_8a8d819f95ebc463d50ca790` |
| `EVAL-087` | `hallucination_or_ungrounded` | `troubleshooting` | - | 12726 ms | `tr_d2b4e49d0d79811da7d02a1d` |
| `EVAL-089` | `hallucination_or_ungrounded` | `troubleshooting` | - | 5548 ms | `tr_b653b699536fbcc7b1c8a5d6` |
| `EVAL-093` | `retrieval_miss` | `troubleshooting` | - | 12367 ms | `tr_78f7e48958052aa67c105d4c` |
| `EVAL-097` | `answer_incomplete_or_incorrect` | `clarification` | - | 3612 ms | `tr_92beafd91ba1cfc317ada391` |
| `EVAL-099` | `clarification_or_rejection_error` | `out_of_scope` | - | 10325 ms | `tr_d24530061f49870b4c66177f` |
| `EVAL-119` | `hallucination_or_ungrounded` | `troubleshooting` | - | 396 ms | `tr_e4bd56bb3ef84f99482016ae` |
| `EVAL-122` | `retrieval_miss` | `product_comparison` | - | 6882 ms | `tr_44d1ffe334564f18db08ce1f` |
| `EVAL-131` | `tool_selection_error` | `order_query` | user_purchase_history, primary/user_purchase_history | 5966 ms | `tr_69b4e1c3ceab25908f21ea76` |
| `EVAL-138` | `intent_error` | `usage_instruction` | - | 13421 ms | `tr_2a64a200a67c58db90839d62` |
| `EVAL-140` | `hallucination_or_ungrounded` | `order_query` | user_purchase_history | 4548 ms | `tr_15cce34b5063c4969aaa7502` |
| `EVAL-141` | `hallucination_or_ungrounded` | `warranty_query` | order_lookup, warranty_check | 416 ms | `tr_ff38265faaded1495b54e2b4` |
| `EVAL-150` | `tool_selection_error` | `warranty_query` | - | 2824 ms | `tr_59a4023dfda82b9b72bd0236` |
| `EVAL-151` | `hallucination_or_ungrounded` | `order_query` | user_purchase_history | 7461 ms | `tr_931907bef49e625553e3b5df` |
| `EVAL-154` | `retrieval_miss` | `product_parameter` | inventory_check | 745 ms | `tr_40b5dec6551211692a152851` |
| `EVAL-157` | `hallucination_or_ungrounded` | `product_parameter` | price_query | 445 ms | `tr_80b06c10a1988813cae964b5` |
| `EVAL-158` | `hallucination_or_ungrounded` | `accessory_compatibility` | inventory_check, price_query | 454 ms | `tr_a57e20f3765f21b58cfcfee2` |
| `EVAL-159` | `retrieval_miss` | `usage_instruction` | inventory_check | 365 ms | `tr_9823ffac413d2a176a5d9483` |
| `EVAL-160` | `hallucination_or_ungrounded` | `usage_instruction` | price_query | 893 ms | `tr_68679b1348555609f43dd4c5` |
| `EVAL-162` | `tool_selection_error` | `usage_instruction` | - | 3401 ms | `tr_69bbb517292b651a7e62dd3c` |
| `EVAL-163` | `hallucination_or_ungrounded` | `product_comparison` | price_query | 7921 ms | `tr_efa36713ef0fd47adaa76fe3` |
| `EVAL-166` | `answer_incomplete_or_incorrect` | `usage_instruction` | order_lookup | 385 ms | `tr_d7c6ed081623c0771db3e6d0` |
| `EVAL-170` | `retrieval_miss` | `return_eligibility` | order_lookup | 384 ms | `tr_ae579ead4f551615773a144c` |
| `EVAL-171` | `hallucination_or_ungrounded` | `usage_instruction` | price_query | 3277 ms | `tr_3c252b7217d72c2c73888966` |
| `EVAL-173` | `retrieval_miss` | `purchase_recommendation` | price_query, inventory_check | 7159 ms | `tr_351fd4493e118d0cf743ad90` |
| `EVAL-176` | `tool_selection_error` | `troubleshooting` | user_purchase_history, user_purchase_history | 420 ms | `tr_5ad859141663595ea17ea3f2` |
| `EVAL-177` | `retrieval_miss` | `troubleshooting` | - | 409 ms | `tr_36535eff8cd867a75b61ce83` |
| `EVAL-178` | `retrieval_miss` | `purchase_recommendation` | price_query, inventory_check | 7295 ms | `tr_0a641608572975fbca2c494e` |
| `EVAL-179` | `intent_error` | `usage_instruction` | price_query, price_query, inventory_check | 4853 ms | `tr_ea4c91994d9d99137758f578` |
| `EVAL-180` | `retrieval_miss` | `return_eligibility` | order_lookup, warranty_check | 368 ms | `tr_59d350e7309997d80937de80` |
| `EVAL-181` | `retrieval_miss` | `warranty_query` | user_purchase_history, user_purchase_history | 385 ms | `tr_5cb5abfd8779eadfa67e76e6` |
| `EVAL-182` | `tool_selection_error` | `troubleshooting` | user_purchase_history, user_purchase_history | 416 ms | `tr_196111bbdc3a73c17f18391e` |
| `EVAL-185` | `retrieval_miss` | `troubleshooting` | - | 799 ms | `tr_cf61b01eba0b3009c2debf03` |
| `EVAL-187` | `clarification_or_rejection_error` | `clarification` | - | 2191 ms | `tr_3df2e0349ba0b550d3863154` |
| `EVAL-188` | `answer_incomplete_or_incorrect` | `clarification` | - | 2449 ms | `tr_776a3c30f011581a1b41fd35` |
| `EVAL-190` | `clarification_or_rejection_error` | `clarification` | - | 5615 ms | `tr_c21f3e341aec1cf73917063e` |
| `EVAL-194` | `clarification_or_rejection_error` | `out_of_scope` | - | 52018 ms | `tr_eed1719716bbc79fc52aea94` |
| `EVAL-195` | `tool_selection_error` | `out_of_scope` | user_purchase_history | 2053 ms | `tr_3ef273be76de664fd6489689` |
| `EVAL-198` | `clarification_or_rejection_error` | `troubleshooting` | - | 45284 ms | `tr_1ae75af90bc7126cff3b35b8` |

## 复现

```powershell
python3 scripts/eval-regression-report.py --base-url http://127.0.0.1:18080 --system-version agentic-mcp-aggregate-general-quality-20260618-r2 --max-cases 200 --output docs/eval/mcp-regression-report.md
```
