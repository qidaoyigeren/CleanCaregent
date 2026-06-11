package eval

import "fmt"

func DefaultCases() []Case {
	cases := make([]Case, 0, 100)
	add := func(path, intent, difficulty, query, answer string, docs, tools []string, params map[string]any, tags ...string) {
		cases = append(cases, Case{
			CaseID:             fmt.Sprintf("EVAL-%03d", len(cases)+1),
			Query:              query,
			Intent:             intent,
			Difficulty:         difficulty,
			ExpectedDocuments:  docs,
			ExpectedTools:      tools,
			ExpectedToolParams: params,
			StandardAnswer:     answer,
			Tags:               append([]string{path}, tags...),
		})
	}

	parameters := []struct{ model, doc, spec string }{
		{"T20", "kb_params_t20", "6000Pa"},
		{"X20 Pro", "kb_params_x20_pro", "8000Pa"},
		{"P400", "kb_params_p400", "CADR 450m³/h"},
		{"W300", "kb_params_w300", "400G"},
		{"H100", "kb_params_h100", "4L"},
	}
	for _, item := range parameters {
		add("kb_single", "product_parameter", "simple", item.model+" 的核心参数是什么？", item.spec, []string{item.doc}, nil, nil, "parameter")
		add("kb_single", "product_parameter", "simple", item.model+" 适合多大面积？", item.spec, []string{item.doc}, nil, nil, "parameter")
		add("kb_single", "product_parameter", "medium", "请说明 "+item.model+" 的规格和适用家庭", item.spec, []string{item.doc}, nil, nil, "parameter")
	}

	manuals := []struct{ model, doc string }{
		{"T20", "kb_manual_t20"}, {"X20 Pro", "kb_manual_x20_pro"}, {"R10", "kb_manual_r10"},
		{"P400", "kb_manual_p400"}, {"P500", "kb_manual_p500"}, {"W300", "kb_manual_w300"},
		{"H100", "kb_manual_h100"}, {"H200", "kb_manual_h200"},
	}
	for _, item := range manuals {
		add("kb_single", "usage_instruction", "simple", item.model+" 首次使用要注意什么？", "安装并完成首次清洁", []string{item.doc}, nil, nil, "manual")
	}
	add("kb_single", "usage_instruction", "simple", "T20 怎么重新安装后使用？", "断电并按说明安装", []string{"kb_manual_t20"}, nil, nil, "manual")
	add("kb_single", "usage_instruction", "simple", "P400 使用前要拆什么包装？", "移除包装", []string{"kb_manual_p400"}, nil, nil, "manual")

	compat := []struct{ host, accessory, doc string }{
		{"P400", "F400", "kb_compat_p400_f400"}, {"P500", "F500", "kb_compat_p500_f500"},
		{"T20", "DB20", "kb_compat_t20_db20"}, {"X20 Pro", "RB20", "kb_compat_x20_pro_rb20"},
		{"W300", "C300", "kb_compat_w300_c300"},
	}
	for _, item := range compat {
		add("kb_single", "accessory_compatibility", "simple", item.host+" 兼容什么配件？", item.accessory, []string{item.doc}, nil, nil, "compatibility")
		add("kb_single", "accessory_compatibility", "medium", item.accessory+" 能装到 "+item.host+" 上吗？", "兼容关系以兼容表为准", []string{item.doc}, nil, nil, "compatibility")
	}

	faqs := []struct{ query, doc, answer string }{
		{"T20 如何重新联网？", "kb_faq_t20_wifi", "2.4GHz Wi-Fi"},
		{"X20 Pro 能清洁地毯吗？", "kb_faq_x20_carpet", "地毯识别和自动增压"},
		{"P400 多久换滤芯？", "kb_faq_p400_filter", "6-12 个月"},
		{"P500 睡眠模式有什么用？", "kb_faq_p500_sleep", "降低风量和灯光"},
		{"W300 首次使用怎么冲洗？", "kb_faq_w300_flush", "完成首次冲洗"},
		{"W500 出水变小怎么办？", "kb_faq_w500_flow", "检查水压和滤芯"},
		{"H100 水箱怎么清洁？", "kb_faq_h100_clean", "断电后清洁"},
		{"H200 应该加什么水？", "kb_faq_h200_water", "洁净软水"},
		{"P400 滤芯寿命受什么影响？", "kb_faq_p400_filter", "污染程度"},
		{"T20 配网需要什么 Wi-Fi？", "kb_faq_t20_wifi", "2.4GHz"},
	}
	for _, item := range faqs {
		add("kb_single", "usage_instruction", "simple", item.query, item.answer, []string{item.doc}, nil, nil, "faq")
	}

	comparisons := []struct{ models, doc, answer string }{
		{"T20 和 X20 Pro", "kb_compare_t20_x20pro", "养猫和地毯多优先 X20 Pro"},
		{"R10 和 R20", "kb_compare_r10_r20", "R20 更适合地毯"},
		{"P400 和 P500", "kb_compare_p400_p500", "P500 适合更大面积"},
		{"W300 和 W500", "kb_compare_w300_w500", "W500 通量更高"},
		{"H100 和 H200", "kb_compare_h100_h200", "H200 适合客厅"},
	}
	for _, item := range comparisons {
		add("kb_multi", "product_comparison", "medium", item.models+" 有什么区别？", item.answer, []string{item.doc}, nil, nil, "comparison")
		add("kb_multi", "product_comparison", "hard", item.models+" 哪个更适合我？请说明取舍", item.answer, []string{item.doc}, nil, nil, "comparison")
	}

	recommendations := []struct{ query, doc, answer string }{
		{"家里有两只猫和地毯，推荐扫地机器人", "kb_guide_pet_home", "关注防缠绕和地毯增压"},
		{"养猫家庭预算 5000 怎么选扫地机器人", "kb_guide_pet_home", "X20 Pro 或合适候选"},
		{"120 平米有地毯，推荐清洁方案", "kb_guide_large_home", "续航和自动回洗"},
		{"120 平米预算有限怎么选", "kb_guide_large_home", "结合预算筛选"},
		{"50 平米客厅选什么净化器", "kb_guide_air_area", "按 CADR 和面积选择"},
		{"卧室净化器怎么选", "kb_guide_air_area", "P400"},
		{"四口之家净水器怎么选", "kb_guide_water_family", "400G 或 600G"},
		{"用水高峰多选多大通量", "kb_guide_water_family", "600G"},
		{"卧室加湿器怎么选", "kb_guide_humidifier_room", "关注噪音"},
		{"大客厅加湿器怎么选", "kb_guide_humidifier_room", "关注水箱和加湿量"},
	}
	for _, item := range recommendations {
		add("kb_multi", "purchase_recommendation", "hard", item.query, item.answer, []string{item.doc}, nil, nil, "recommendation")
	}

	for _, model := range []string{"T20", "X20 Pro", "P400", "P500"} {
		add("kb_tool", "price_query", "medium", model+" 现在多少钱，有券吗？", "以实时价格工具为准", nil, []string{"price_query"}, map[string]any{"product_refs": []string{model}}, "dynamic")
	}
	add("kb_tool", "accessory_compatibility", "hard", "我上周买的净化器滤芯多少钱，有券吗？", "先查购买记录、兼容关系和实时价格", []string{"kb_compat_p400_f400"}, []string{"user_purchase_history", "price_query"}, nil, "dynamic", "multi_step")
	for _, model := range []string{"T20", "X20 Pro", "P400"} {
		add("kb_tool", "inventory_query", "medium", model+" 现在有货吗？", "以库存工具为准", nil, []string{"inventory_check"}, map[string]any{"product_refs": []string{model}}, "dynamic")
	}
	for _, query := range []string{"查一下订单CC20260603001", "我上周买了什么净化器？", "查询订单CC20250522008的商品"} {
		toolName := "order_lookup"
		if query == "我上周买了什么净化器？" {
			toolName = "user_purchase_history"
		}
		add("kb_tool", "order_query", "medium", query, "返回当前用户订单或购买记录", nil, []string{toolName}, nil, "dynamic")
	}
	for _, orderNo := range []string{"CC20260603001", "CC20250522008", "CC20260603001"} {
		add("kb_tool", "warranty_query", "medium", "订单"+orderNo+"还在保修期吗？", "结合订单时间和保修月数判断", []string{"kb_policy_warranty"}, []string{"warranty_check"}, map[string]any{"order_no": orderNo}, "dynamic")
	}
	returnQueries := []string{
		"订单CC20250522008买了超过7天，包装拆了还能退吗？",
		"订单CC20260603001包装完整能退吗？",
		"订单CC20250522008有质量问题可以换货吗？",
	}
	for _, query := range returnQueries {
		add("kb_tool", "return_eligibility", "hard", query, "结合订单和售后政策判断，不直接承诺", []string{"kb_policy_return_7d"}, []string{"order_lookup"}, nil, "dynamic", "policy")
	}
	for index, orderNo := range []string{"CC20260603001", "CC20250522008", "CC20260603001"} {
		add("kb_tool", "create_after_sales_ticket", "hard",
			fmt.Sprintf("确认帮我为订单%s创建售后工单，问题编号%d", orderNo, index+1),
			"明确确认后幂等创建工单", []string{"kb_policy_ticket"}, []string{"create_after_sales_ticket"},
			map[string]any{"order_no": orderNo, "confirmed": true}, "dynamic", "side_effect")
	}

	faultCases := []struct{ model, doc, symptom string }{
		{"T20", "kb_fault_t20_charge", "充不进电"},
		{"X20 Pro", "kb_fault_x20_tangle", "滚刷缠绕并异响"},
		{"P400", "kb_fault_p400_noise", "运行时异响"},
		{"P500", "kb_fault_p500_sensor", "传感器数值异常"},
		{"W300", "kb_fault_w300_leak", "漏水"},
	}
	for _, item := range faultCases {
		add("diagnosis_multi_turn", "troubleshooting", "hard", item.model+" "+item.symptom+"怎么办？", "按故障树逐步排查", []string{item.doc}, nil, nil, "diagnosis")
		add("diagnosis_multi_turn", "troubleshooting", "hard", item.model+" 已完成第一步检查但问题仍在，下一步做什么？", "继续下一故障节点并避免拆机", []string{item.doc}, nil, nil, "diagnosis")
	}

	clarify := []struct{ query, intent string }{
		{"这个多少钱？", "price_query"},
		{"它能用吗？", "clarification"},
		{"我想退货", "return_eligibility"},
	}
	for _, entry := range clarify {
		item := Case{
			CaseID:         fmt.Sprintf("EVAL-%03d", len(cases)+1),
			Query:          entry.query,
			Intent:         entry.intent,
			Difficulty:     "simple",
			StandardAnswer: "请补充型号或订单号",
			ShouldClarify:  true,
			Tags:           []string{"reject_clarify", "clarify"},
		}
		cases = append(cases, item)
	}
	for _, query := range []string{"推荐一款手机", "帮我查询发票"} {
		item := Case{
			CaseID:         fmt.Sprintf("EVAL-%03d", len(cases)+1),
			Query:          query,
			Intent:         "out_of_scope",
			Difficulty:     "simple",
			StandardAnswer: "超出清洁电器范围",
			ShouldReject:   true,
			Tags:           []string{"reject_clarify", "reject"},
		}
		cases = append(cases, item)
	}
	return cases
}
