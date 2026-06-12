package seed

import (
	"strings"

	"CleanCaregent/internal/service"
)

func DefaultKnowledgeDocuments() []service.IngestDocumentRequest {
	products := defaultProducts()
	var documents []service.IngestDocumentRequest
	add := func(docID, title, content, category, docType string, models []string, tags ...string) {
		metadata := map[string]any{
			"data_scope":            "mock",
			"structural_difficulty": structuralDifficulty(docType),
		}
		if len(models) == 1 {
			metadata["model"] = models[0]
		} else if len(models) > 1 {
			metadata["models"] = models
		}
		documents = append(documents, service.IngestDocumentRequest{
			DocID:      docID,
			Title:      title,
			Content:    content,
			Category:   category,
			Brand:      "CleanCare",
			DocType:    docType,
			Version:    "kb-v1",
			Source:     "mock://clean-care/" + docID,
			IntentTags: tags,
			Metadata:   metadata,
		})
	}

	for _, product := range products {
		add(
			"kb_detail_"+slug(product.Model),
			product.Model+" "+product.Kind+"商品详情",
			renderProductDetail(product),
			product.Category,
			"product_detail",
			[]string{product.Model},
			"product_parameter", "purchase_recommendation",
		)
	}

	for _, product := range products {
		add(
			"kb_params_"+slug(product.Model),
			product.Model+" 结构化参数表",
			renderParameterTable(product),
			product.Category,
			"product_parameter",
			[]string{product.Model},
			"product_parameter",
		)
	}

	comparisons := []struct {
		id, title, content, category string
		models                       []string
	}{
		{"t20_x20pro", "T20 与 X20 Pro 对比", "T20 预算更低，X20 Pro 吸力和防缠绕能力更强；养猫且地毯多优先 X20 Pro。", "robot_vacuum", []string{"T20", "X20 Pro"}},
		{"r10_r20", "R10 与 R20 对比", "R10 适合小户型基础清扫，R20 更适合地毯和中等户型。", "robot_vacuum", []string{"R10", "R20"}},
		{"p400_p500", "P400 与 P500 对比", "P400 适合卧室和中等客厅，P500 CADR 更高，适合大客厅。", "air_purifier", []string{"P400", "P500"}},
		{"w300_w500", "W300 与 W500 对比", "W300 适合 3-4 人家庭，W500 通量更高，适合多人连续用水。", "water_purifier", []string{"W300", "W500"}},
		{"h100_h200", "H100 与 H200 对比", "H100 适合卧室，H200 水箱和加湿量更大，适合客厅。", "humidifier", []string{"H100", "H200"}},
	}
	for _, item := range comparisons {
		add(
			"kb_compare_"+item.id,
			item.title,
			renderProductComparison(
				findSeedProduct(products, item.models[0]),
				findSeedProduct(products, item.models[1]),
				item.content,
			),
			item.category,
			"product_comparison",
			item.models,
			"product_comparison",
		)
	}

	guides := []struct{ id, title, category, content string }{
		{"pet_home", "养宠家庭扫地机器人选购指南", "robot_vacuum", "优先考虑防缠绕滚刷、地毯增压、尘袋容量和边角清洁。两只猫且有地毯时建议 7000Pa 以上候选。"},
		{"large_home", "120 平米清洁方案指南", "robot_vacuum", "120 平米家庭应关注续航、自动回洗和地毯策略，并结合预算筛选 T20、X20 Pro 或 R20。"},
		{"air_area", "空气净化器面积与 CADR 选购指南", "air_purifier", "净化器应按房间面积和 CADR 选择，并考虑滤芯成本；卧室可选 P400，大客厅可选 P500。"},
		{"water_family", "家庭净水器通量选购指南", "water_purifier", "3-4 人家庭可选 400G，4-6 人或用水高峰明显时优先 600G。"},
		{"humidifier_room", "卧室和客厅加湿器选购指南", "humidifier", "卧室关注噪音和小面积控制，客厅关注水箱容量和持续加湿量。"},
	}
	for _, item := range guides {
		add(
			"kb_guide_"+item.id,
			item.title,
			renderPurchaseGuide(item.title, item.category, item.content),
			item.category,
			"purchase_guide",
			nil,
			"purchase_recommendation",
		)
	}

	compatibilities := []struct{ host, accessory, category, cycle string }{
		{"P400", "F400", "air_purifier", "6-12 个月"},
		{"P500", "F500", "air_purifier", "6-12 个月"},
		{"T20", "DB20", "robot_vacuum", "尘袋满后更换"},
		{"X20 Pro", "RB20", "robot_vacuum", "磨损后更换"},
		{"W300", "C300", "water_purifier", "12 个月"},
	}
	for _, item := range compatibilities {
		add(
			"kb_compat_"+slug(item.host)+"_"+slug(item.accessory),
			item.host+" 配件兼容表",
			renderCompatibility(item.host, item.accessory, item.cycle),
			item.category,
			"accessory_compatibility",
			[]string{item.host},
			"accessory_compatibility",
		)
	}

	manualModels := []seedProduct{products[0], products[1], products[2], products[4], products[5], products[6], products[8], products[9]}
	for _, product := range manualModels {
		add(
			"kb_manual_"+slug(product.Model),
			product.Model+" 使用说明书",
			renderUserManual(product),
			product.Category,
			"user_manual",
			[]string{product.Model},
			"usage_instruction",
		)
	}

	faults := []struct{ id, title, category, model, content string }{
		{"t20_charge", "T20 无法充电排查", "robot_vacuum", "T20", "先确认充电座通电，再清洁金属触点；仍失败时记录指示灯和错误码，不要拆机。"},
		{"x20_tangle", "X20 Pro 滚刷缠绕排查", "robot_vacuum", "X20 Pro", "关机后检查滚刷和轴端异物，清理后重新安装；电机异响时停止使用。"},
		{"p400_noise", "P400 异响排查", "air_purifier", "P400", "检查滤芯包装是否移除、进出风口是否堵塞；持续异响应停止使用并申请检测。"},
		{"p500_sensor", "P500 传感器异常排查", "air_purifier", "P500", "清洁传感器窗口并重启；数值持续异常时记录环境和错误码。"},
		{"w300_leak", "W300 漏水排查", "water_purifier", "W300", "立即关闭进水阀和电源，检查接头，不得带压拆卸。"},
		{"h100_mist", "H100 不出雾排查", "humidifier", "H100", "确认水位、浮子和雾化片清洁状态，禁止干烧。"},
	}
	for _, item := range faults {
		add(
			"kb_fault_"+item.id,
			item.title,
			renderTroubleshootingTree(item.id, item.model, item.content),
			item.category,
			"troubleshooting",
			[]string{item.model},
			"troubleshooting",
		)
	}

	policies := []struct{ id, title, content string }{
		{"return_7d", "七天无理由退货政策", "签收次日起 7 天内、商品及附件包装完好且不影响二次销售时可申请。超过期限不直接承诺退款。"},
		{"quality_exchange", "质量问题换货政策", "质量故障需结合订单、检测结论和政策判断；证据不足时转人工。"},
		{"warranty", "清洁电器保修政策", "保修期从签收时间起算，无签收时间时使用支付时间；具体月数以订单项为准。"},
		{"accessory_return", "耗材配件退换政策", "已拆封并使用的滤芯、尘袋等耗材通常不适用无理由退货，质量问题除外。"},
		{"ticket", "售后工单创建规则", "创建工单前必须确认订单、问题描述和用户明确授权；使用幂等键避免重复建单。"},
	}
	for _, item := range policies {
		add(
			"kb_policy_"+item.id,
			item.title,
			renderPolicy(item.title, item.content),
			"cleaning_appliance",
			"after_sales_policy",
			nil,
			"return_eligibility",
			"warranty_query",
		)
	}

	faqs := []struct{ id, title, category, model, content string }{
		{"t20_wifi", "T20 如何重新联网", "robot_vacuum", "T20", "长按联网键进入配网状态，并使用 2.4GHz Wi-Fi。"},
		{"x20_carpet", "X20 Pro 能否清洁地毯", "robot_vacuum", "X20 Pro", "支持地毯识别和自动增压，长毛地毯应先设置禁区验证。"},
		{"p400_filter", "P400 多久换滤芯", "air_purifier", "P400", "通常 6-12 个月，并结合滤芯寿命提示和环境污染程度。"},
		{"p500_sleep", "P500 睡眠模式说明", "air_purifier", "P500", "睡眠模式降低风量和灯光，适合夜间使用。"},
		{"w300_flush", "W300 首次使用如何冲洗", "water_purifier", "W300", "按说明完成首次冲洗，出水稳定后再饮用。"},
		{"w500_flow", "W500 出水变小怎么办", "water_purifier", "W500", "检查水压和滤芯寿命，持续异常时联系售后。"},
		{"h100_clean", "H100 如何清洁水箱", "humidifier", "H100", "断电后倒空水箱，使用软布清洁，不得让底座电气部件进水。"},
		{"h200_water", "H200 应使用什么水", "humidifier", "H200", "优先使用洁净软水，并按周期清洁水箱和雾化片。"},
	}
	for _, item := range faqs {
		add(
			"kb_faq_"+item.id,
			item.title,
			renderFAQ(item.title, item.content),
			item.category,
			"faq",
			[]string{item.model},
			"usage_instruction",
			"product_parameter",
		)
	}
	documents = append(documents, expandedKnowledgeDocuments(products)...)
	return documents
}

func structuralDifficulty(docType string) string {
	switch docType {
	case "product_detail", "product_parameter", "product_comparison":
		return "参数表完整性与多型号同口径对齐"
	case "purchase_guide":
		return "多条件约束、硬条件过滤与方案取舍"
	case "accessory_compatibility":
		return "主机型号、配件型号与兼容关系精确匹配"
	case "user_manual":
		return "操作步骤顺序、安全警告与前置条件不可拆散"
	case "troubleshooting":
		return "故障树父子节点、条件分支与安全停止"
	case "after_sales_policy":
		return "条款、适用条件、例外与时间边界联合判断"
	case "faq":
		return "问题、答案与型号范围必须保持在同一块"
	default:
		return "结构化字段与正文语义一致性"
	}
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return strings.ReplaceAll(value, "-", "_")
}
