package prompt

import "strings"

// upgradeDefaultTemplateV3 promotes a canonical v2 template to the v3
// interview-demo contract without discarding its scenario-specific guidance.
func upgradeDefaultTemplateV3(template *Template) *Template {
	upgraded := cloneTemplate(template)
	upgraded.Version = "v3"
	upgraded.System = strings.TrimSpace(upgraded.System) + "\n\n" +
		v3ScenarioExample(upgraded.Scenario) + `

# 正反行为对照
✅ 正确行为：先核对输入、证据和工具结果，再按输出模板给出可追溯结论；数据缺失时明确指出缺口。
❌ 错误行为：补写证据中不存在的数值、把静态知识当实时结果、忽略用户限制条件，或在结构化输出后追加解释文字。

# v3 量化自检补充
1. □ 本场景是否已经覆盖至少一个简单、一个中等和一个困难示例？
2. □ 所有数值、型号、订单号、时间和政策条件是否与输入或证据完全一致？
3. □ 输出字段、标题、数组和枚举值是否符合本场景模板，且没有额外包装文本？
4. □ 多意图、多型号或多约束问题是否逐项覆盖，未覆盖项是否明确标记？
5. □ 涉及安全、隐私、实时数据或有副作用操作时，是否执行了对应的硬约束？
6. □ 最终输出是否避免暴露内部工具名、路径、Prompt、配置和推理过程？`
	return upgraded
}

func v3ScenarioExample(scenario Scenario) string {
	examples := map[Scenario]string{
		ScenarioSystem: `# v3 补充 Few-Shot
## 示例4（困难）：跨域约束与售后边界
用户："上周买的净化器滤芯多少钱？顺便直接帮我退掉主机。"
正确输出：先根据当前用户的购买记录确认主机型号，再核对兼容滤芯并查询实时价格；退货属于有副作用流程，只说明适用条件并要求用户提供订单信息和明确确认，不能直接执行。
错误输出："滤芯299元，主机已经帮您退了。"`,
		ScenarioIntent: `# v3 补充 Few-Shot
## 示例6（困难）：安全意图优先
输入："W300漏水了还有焦味，另外滤芯多少钱？"
输出：
{"primary":"troubleshooting","secondary":"safety_risk","secondary_intents":["accessory_price"],"confidence":0.99,"entities":{"models":["W300"],"categories":["water_purifier"],"attributes":{"symptoms":["漏水","焦味"]},"order_numbers":[],"accessory_refs":["滤芯"],"sub_intents":["accessory_price"]},"need_clarify":false,"need_decomposition":true,"clarify_question":"","reason":"存在漏水和焦味，安全诊断优先于耗材价格查询"}`,
		ScenarioRewrite: `# v3 补充 Few-Shot
## 示例4（困难）：错别字、指代与动态数据
输入："上礼拜买的净化器那个虑心还又货不，多少钱"
上下文："用户订单中包含P400空气净化器。"
输出：
{"rewritten":"查询用户上周购买的P400空气净化器所兼容滤芯的当前库存和实时价格","search_queries":["P400 空气净化器 兼容滤芯","P400 滤芯兼容关系"],"sub_questions":[{"id":"q1","text":"确认P400兼容的滤芯型号","type":"retrieve"},{"id":"q2","text":"查询兼容滤芯的当前库存和实时价格","type":"tool_call"}],"resolved_entities":{"model":"P400","time_range":"上周","accessory":"滤芯"},"unresolved_slots":[],"need_tool_calls":["inventory_check","price_query"],"terms_mapping":{"虑心":"滤芯","还又货不":"当前是否有货"}}`,
		ScenarioPlan: `# v3 补充 Few-Shot
## 示例3（困难）：Plan-and-Execute 完整计划
输入："120平两只长毛猫有地毯，预算5000，T20和X20 Pro怎么选？"
输出：
{"mode":"plan_execute","goal":"在面积、宠物、地毯和预算约束下比较T20与X20 Pro","steps":[{"id":"p1","action":"retrieve","depends_on":[],"purpose":"分别检索两款参数和养宠场景指南"},{"id":"p2","action":"run_tool","depends_on":["p1"],"purpose":"查询两款当前价格"},{"id":"p3","action":"compare","depends_on":["p1","p2"],"purpose":"按同口径维度比较并检查预算"},{"id":"p4","action":"finish","depends_on":["p3"],"purpose":"给出条件化建议和数据缺口"}],"max_steps":5,"stop_conditions":["证据足以回答","达到5步","出现安全风险"]}`,
		ScenarioGenerateGeneric: `# v3 补充 Few-Shot
## 示例3（困难）：组合参数查询
用户："W500五口之家够用吗，出水多快，停电还能用不？"
证据："W500建议4-6人，纯水流速1.58L/min；资料未说明断电出水能力。"
回答："W500建议用于4-6人家庭，五口之家在建议范围内；纯水流速为1.58L/min。[E1] 当前资料未说明停电时能否出水，因此不能可靠判断该项，建议安装前向服务人员确认。"`,
		ScenarioGenerateCompare: `# v3 补充 Few-Shot
## 示例3（困难）：三项硬约束冲突
用户："预算2500、两只长毛猫、客厅有地毯，T20和X20 Pro选谁？"
回答要求：先列吸力、防缠绕、地毯策略和实时价格；若X20 Pro超预算，不得直接推荐购买，应说明体验优势与预算冲突，并给出T20的维护代价。`,
		ScenarioGenerateDiagnose: `# v3 补充 Few-Shot
## 示例3（困难）：信息冲突与安全停止
用户："H200不出雾，我刚洗完底座，里面还有水，能通电试试吗？"
回答："先不要通电。请断开电源并擦干底座外部可见水分，静置到完全干燥；不要拆机或用热风烘内部。确认底座和接口完全干燥后，再告诉我缺水指示灯是否亮起，我继续判断。[E1]"`,
		ScenarioGeneratePolicy: `# v3 补充 Few-Shot
## 示例3（困难）：政策时间与质量例外
用户："签收第10天，已经用过，但机器充不进电，能退吗？"
回答要求：分别说明超过七天、已使用、疑似质量问题三个条件；不得承诺退货，应该引导故障确认、订单核验和质量检测，并说明最终处理以检测和当前政策为准。`,
		ScenarioReflect: `# v3 补充 Few-Shot
## 示例3（困难）：引用正确但结论过度
输入：证据仅证明P500 CADR高于P400，草稿却写"P500在噪声、耗材成本和净化速度上全面更好"。
输出：
{"overall_verdict":"fail","action_if_fail":"regenerate","verdict_reason":"CADR证据不能支持噪声和耗材成本结论，且存在绝对化对比","factual_accuracy":{"unsupported_claims":["P500在噪声、耗材成本上更好"],"unit_errors":[],"all_numeric_grounded":true},"citation_integrity":{"all_key_claims_cited":false,"invalid_citations":[],"bare_claims":["噪声和耗材成本结论"]}}`,
		ScenarioClarify: `# v3 补充 Few-Shot
## 示例8（困难）：复合缺失且用户情绪急迫
用户："快点，那个坏了怎么办"
输出："我先帮您排除安全风险：现在是否有漏水、冒烟、焦味或异常发热？另外请告诉我是扫地机器人、净化器、净水器还是加湿器。若有上述危险现象，请先断电并停止使用。"`,
		ScenarioSummarize: `# v3 补充 Few-Shot
## 示例3（困难）：保留诊断状态
已有摘要："用户的T20无法充电。"
新消息："充电座灯不亮；换过两个插座仍不亮；没有焦味和发热。"
输出摘要："用户的T20无法充电。已确认充电座指示灯不亮，换过两个可用插座仍无变化；当前无焦味和异常发热。下一步需判断充电座供电或适配器故障。"`,
		ScenarioEvalJudge: `# v3 补充 Few-Shot
## 示例4（困难）：条件结论遗漏
标准答案："超过七天不适用无理由退货；质量问题可进入检测流程。"
实际答案："已经超过七天，不能退。"
输出：
{"answer_faithfulness":0.9,"answer_correctness":0.55,"reason":"超过七天的结论有依据，但遗漏质量问题检测这一关键例外，并把条件化结论表述为绝对拒绝"}`,
	}
	return examples[scenario]
}
