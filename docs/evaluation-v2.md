# CleanCare Evaluation v2

## 目标

v2 评测用于同一批真实化问法上比较 Naive RAG 与 Agentic RAG。它不把历史 mock 基线包装成当前线上效果；正式数值必须在指定模型、知识库和配置版本下重新运行后记录。

## 数据集

- 默认版本：`v2`
- 用例数：300
- split：原 200 条为 `regression`，新增 75 条为 `tuning`，新增 25 条为 `holdout`
- 至少 30 条带有口语化、省略、大小写混用、指代或歧义
- 覆盖纯知识查询、多文档对比、知识加工具、多轮故障诊断、澄清和拒答
- 生成命令：`make eval-dataset`
- 输出文件：`docs/eval/eval-cases-v2.json`

`tuning` 用于 Prompt、检索参数、规则和编排策略迭代；`holdout` 只用于阶段性最终测量，避免把留出集反馈进调参。默认不传 `split` 时会跑所选范围内的全部 case；传 `split=regression/tuning/holdout` 时只跑对应子集。

## 混合评测

规则层负责可确定验证的指标：

- Intent Accuracy、Hit@5、MRR
- Context Recall / Precision
- Tool Decision / Selection / Parameter Accuracy
- 多步完成度、澄清和拒答
- 延迟、Token、步骤数

LLM-as-Judge 负责语义指标：

- `answer_faithfulness`：答案事实是否由检索上下文或工具证据支持
- `answer_correctness`：答案是否覆盖标准答案的核心结论和必要条件

只要配置 `llm.provider=openai_compatible`，Eval Runner 就会调用 Judge；Judge 暂时不可用时才退化到保守的字面事实检查，并且不再使用 bigram 重叠作为语义正确性。

## Bad Case 分类

失败用例会结合指标和 trace 自动归入：

1. `intent_error`
2. `retrieval_miss`
3. `retrieval_noise`
4. `tool_selection_error`
5. `tool_parameter_error`
6. `hallucination_or_ungrounded`
7. `answer_incomplete_or_incorrect`
8. `clarification_or_rejection_error`

工具错误码、失败步骤和 Agent 顶层错误码优先于单纯指标名，便于定位链路根因。

## Baseline 对比

启动 agentic 模式后执行：

```bash
make eval-compare
```

接口为 `POST /api/v1/admin/eval/comparisons`，会立即返回 `comparison_id`；通过 `GET /api/v1/admin/eval/comparisons/{comparison_id}` 查询状态和报告。后台先用 Naive RAG 跑同一批用例，再用 Agentic RAG 跑候选版本，最终报告包含两次运行记录以及 Pass Rate、P95、平均 Token和各项指标的 delta。

历史本地基线见 `docs/eval/experiment-report.md`，仅作为改造前链路基线；2026-06-13 真实模型 200 条串行评测见 `docs/eval/llm-experiment-report.md`。代码、Prompt 或 MCP transport 变更后，应使用新的 `system_version` 重跑再引用当前效果；新增 holdout 结果需要单独标注 `split=holdout`。
