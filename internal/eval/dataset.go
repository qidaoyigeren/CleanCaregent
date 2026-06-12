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
	return applyCuratedQueries(cases)
}

type curatedQuery struct {
	Query string
	Tags  []string
}

var curatedQueries = []curatedQuery{
	{Query: "t20吸力多少？家里120平够用不", Tags: []string{"口语化", "小写型号", "组合问"}},
	{Query: "T20一次能扫多大面积啊，90平会不会中途没电", Tags: []string{"口语化", "组合问"}},
	{Query: "把T20主要参数给我捋一下，养猫能用吗", Tags: []string{"口语化", "场景约束"}},
	{Query: "X20pro到底多少Pa？别只说吸力大", Tags: []string{"口语化", "小写型号"}},
	{Query: "我家140㎡，X20 Pro覆盖得过来吗", Tags: []string{"口语化", "面积约束"}},
	{Query: "X20 Pro参数表里续航、越障和集尘分别是多少", Tags: []string{"多参数"}},
	{Query: "p400的CADR是450还是别的数？", Tags: []string{"口语化", "小写型号"}},
	{Query: "P400放50平客厅够不够用", Tags: []string{"口语化", "面积约束"}},
	{Query: "P400噪声、功率、滤芯型号都列一下", Tags: []string{"多参数"}},
	{Query: "w300是400G对吧？一分钟大概能出多少水", Tags: []string{"口语化", "小写型号"}},
	{Query: "一家三口用W300会不会太小", Tags: []string{"口语化", "场景约束"}},
	{Query: "W300的通量、废水比和滤芯配置怎么回事", Tags: []string{"多参数"}},
	{Query: "h100水箱几升，开一夜够吗", Tags: []string{"口语化", "小写型号"}},
	{Query: "卧室20平用H100合适不", Tags: []string{"口语化", "面积约束"}},
	{Query: "H100加湿量、噪声、缺水保护参数发我", Tags: []string{"多参数"}},
	{Query: "T20刚到家，第一次开机从哪几步开始", Tags: []string{"任务式"}},
	{Query: "X20 Pro基站第一次用要装什么、加什么水", Tags: []string{"组合问"}},
	{Query: "R10新机怎么配网？我家只有5G WiFi", Tags: []string{"口语化", "网络约束"}},
	{Query: "P400滤芯外面的袋子是不是得先撕掉", Tags: []string{"口语化", "安全操作"}},
	{Query: "P500第一次开机一直显示红色，安装哪里要检查", Tags: []string{"口语化", "故障混合"}},
	{Query: "W300装好后是不是要先放水冲一会儿", Tags: []string{"口语化", "省略"}},
	{Query: "H100初次使用能直接灌自来水吗", Tags: []string{"口语化", "安全操作"}},
	{Query: "H200刚拆箱，水箱和雾化组件要先洗吗", Tags: []string{"组合问"}},
	{Query: "T20搬家后怎么重新装基站再建图", Tags: []string{"任务式"}},
	{Query: "P400开机前到底有哪些包装必须拆", Tags: []string{"任务式"}},
	{Query: "P400能用哪个滤芯，型号别给错", Tags: []string{"兼容"}},
	{Query: "f400塞P400里能用吧", Tags: []string{"口语化", "小写型号"}},
	{Query: "P500换芯买F400行不行", Tags: []string{"兼容陷阱"}},
	{Query: "F500是不是只配P500", Tags: []string{"口语化", "反向兼容"}},
	{Query: "T20尘袋和边刷分别买什么型号", Tags: []string{"多配件"}},
	{Query: "DB20能不能给T20用，别拿长得像的糊弄我", Tags: []string{"口语化", "兼容"}},
	{Query: "X20 Pro滚刷、尘袋对应哪些配件", Tags: []string{"多配件"}},
	{Query: "rb20装x20 pro上合适吗", Tags: []string{"口语化", "小写型号"}},
	{Query: "W300下一次换芯该买哪根", Tags: []string{"口语化", "省略"}},
	{Query: "C300能直接装W300吗，接口一样不", Tags: []string{"口语化", "兼容"}},
	{Query: "T20换了路由器，APP里怎么重新连网", Tags: []string{"任务式"}},
	{Query: "X20 Pro碰到地毯会抬拖布还是把地毯弄湿", Tags: []string{"场景问"}},
	{Query: "P400滤芯显示30%，现在就得换吗", Tags: []string{"口语化", "状态判断"}},
	{Query: "P500睡眠模式开了以后净化还管用不", Tags: []string{"口语化"}},
	{Query: "W300第一次冲洗要放几分钟水", Tags: []string{"任务式"}},
	{Query: "W500最近出水越来越小，先查哪儿", Tags: []string{"口语化", "故障混合"}},
	{Query: "H100水箱有水垢，能拿白醋洗吗", Tags: []string{"口语化", "安全操作"}},
	{Query: "H200加纯净水还是自来水？家里水很硬", Tags: []string{"组合问"}},
	{Query: "P400滤芯怎么三个月就掉到10%了", Tags: []string{"口语化", "原因问"}},
	{Query: "T20配网老失败，2.4G和5G到底选哪个", Tags: []string{"口语化", "故障混合"}},
	{Query: "T20和X20 Pro差在哪，别光比吸力", Tags: []string{"口语化", "多维对比"}},
	{Query: "两只长毛猫加地毯，T20还是X20 Pro", Tags: []string{"口语化", "场景对比"}},
	{Query: "R10、R20吸力导航续航放一张表比", Tags: []string{"多维对比"}},
	{Query: "小户型没地毯，R20多花的钱值不值", Tags: []string{"口语化", "成本权衡"}},
	{Query: "P400和P500的CADR、噪声、滤芯成本怎么选", Tags: []string{"多维对比"}},
	{Query: "卧室用P400还是P500更安静，缺数据就直说", Tags: []string{"约束对比"}},
	{Query: "W300跟W500除了通量还有啥区别", Tags: []string{"口语化", "多维对比"}},
	{Query: "四口之家早晚集中用水，W300会不会不够", Tags: []string{"口语化", "场景对比"}},
	{Query: "H100和H200哪个晚上更安静", Tags: []string{"单维对比"}},
	{Query: "30平客厅选H100还是H200，主要怕吵", Tags: []string{"口语化", "场景对比"}},
	{Query: "120平、两只猫、客厅地毯，预算5000以内推荐", Tags: []string{"多约束", "口语化"}},
	{Query: "猫毛特别多又不想天天倒尘盒，五千怎么配", Tags: []string{"口语化", "省略", "多约束"}},
	{Query: "复式一共150平有三块地毯，扫地机怎么选", Tags: []string{"多约束"}},
	{Query: "120平预算2500，能接受手动倒尘，推荐哪款", Tags: []string{"多约束"}},
	{Query: "客厅50平而且有人过敏，净化器选哪个", Tags: []string{"多约束"}},
	{Query: "卧室20平睡觉很轻，想要安静的净化器", Tags: []string{"口语化", "多约束"}},
	{Query: "四口之家每天做饭，400G净水器够不够", Tags: []string{"口语化", "多约束"}},
	{Query: "早晚用水扎堆，还要接管线机，选多大通量", Tags: []string{"口语化", "省略", "多约束"}},
	{Query: "婴儿房18平，晚上开加湿器要安静好洗", Tags: []string{"多约束"}},
	{Query: "大客厅35平，想少加水，H100还是H200", Tags: []string{"口语化", "多约束"}},
	{Query: "t20今天到手价多少？我账号有啥券", Tags: []string{"口语化", "小写型号", "动态数据"}},
	{Query: "X20 Pro现在是不是4699，优惠后呢", Tags: []string{"口语化", "价格核验"}},
	{Query: "P400多少钱，别拿商品详情里的旧价格", Tags: []string{"动态数据"}},
	{Query: "p500有活动没，最终要付多少", Tags: []string{"口语化", "小写型号"}},
	{Query: "我上礼拜买的那个净化器，替换滤芯多少钱还有券吗", Tags: []string{"口语化", "指代", "多跳"}},
	{Query: "T20现在能直接下单吗，库存几台", Tags: []string{"口语化", "动态数据"}},
	{Query: "x20pro还有货不，没货别给我报价格", Tags: []string{"口语化", "小写型号"}},
	{Query: "P400白色款库存还有多少", Tags: []string{"动态数据"}},
	{Query: "CC20260603001帮我看看现在啥状态", Tags: []string{"口语化", "订单"}},
	{Query: "上周买的净化器具体是哪款来着", Tags: []string{"口语化", "省略", "指代"}},
	{Query: "订单cc20250522008里买的是T20吗", Tags: []string{"口语化", "小写订单"}},
	{Query: "CC20260603001这单的P400保修到哪天", Tags: []string{"订单", "时间推理"}},
	{Query: "去年买的T20是不是已经过保了，单号CC20250522008", Tags: []string{"口语化", "时间推理"}},
	{Query: "CC20260603001按今天算还在保内不", Tags: []string{"口语化", "时间推理"}},
	{Query: "CC20250522008拆封用了二十多天，现在还能退吗", Tags: []string{"口语化", "政策多条件"}},
	{Query: "CC20260603001包装配件都在，想退要满足啥条件", Tags: []string{"口语化", "政策多条件"}},
	{Query: "CC20250522008机器有质量问题，超过7天还能换不", Tags: []string{"口语化", "政策例外"}},
	{Query: "我确认提交，给CC20260603001建维修工单：P400异响", Tags: []string{"副作用", "明确确认"}},
	{Query: "确认创建售后单，订单CC20250522008，T20充不上电", Tags: []string{"副作用", "明确确认"}},
	{Query: "CC20260603001麻烦现在建工单，我确认，问题是传感器报错", Tags: []string{"副作用", "明确确认"}},
	{Query: "T20趴充电座一晚上还是0%，咋排查", Tags: []string{"口语化", "故障"}},
	{Query: "充电座灯是亮的，触点也擦了，T20还是充不进", Tags: []string{"口语化", "省略", "多轮"}},
	{Query: "X20 Pro滚刷全是猫毛还咔咔响，先关机吗", Tags: []string{"口语化", "安全"}},
	{Query: "毛发清完了还异响，下一步检查啥", Tags: []string{"口语化", "省略", "多轮"}},
	{Query: "P400一开三档就嗡嗡响，低档正常", Tags: []string{"口语化", "故障细节"}},
	{Query: "滤芯重装过，P400还是响，接着怎么查", Tags: []string{"口语化", "省略", "多轮"}},
	{Query: "P500显示的PM2.5一直是0，屋里明明有烟", Tags: []string{"口语化", "故障细节"}},
	{Query: "传感器窗口擦过了数值还不变，能拆机看吗", Tags: []string{"口语化", "安全", "多轮"}},
	{Query: "W300底下漏水，地上已经湿了，要不要先断电关阀", Tags: []string{"口语化", "安全红线"}},
	{Query: "阀门关了也断电了，W300接口还是滴水，下一步呢", Tags: []string{"口语化", "多轮"}},
	{Query: "这个现在多少钱", Tags: []string{"口语化", "省略", "歧义"}},
	{Query: "它能装上吗？", Tags: []string{"口语化", "指代", "歧义"}},
	{Query: "用了些日子不想要了，能退不", Tags: []string{"口语化", "省略", "歧义"}},
	{Query: "给我推荐个拍照好的手机，预算四千", Tags: []string{"越界"}},
	{Query: "把我公司上个月的增值税发票调出来", Tags: []string{"越界", "权限边界"}},
}

func applyCuratedQueries(cases []Case) []Case {
	if len(cases) != len(curatedQueries) {
		panic(fmt.Sprintf("curated eval query count %d does not match case count %d", len(curatedQueries), len(cases)))
	}
	for index := range cases {
		cases[index].Query = curatedQueries[index].Query
		cases[index].Tags = append(cases[index].Tags, curatedQueries[index].Tags...)
	}
	return cases
}
