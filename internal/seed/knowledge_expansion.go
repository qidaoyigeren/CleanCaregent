package seed

import (
	"fmt"
	"strings"

	"CleanCaregent/internal/service"
)

type knowledgeVariant struct {
	ID          string
	Title       string
	Region      string
	Config      string
	Audience    string
	Constraints string
}

func expandedKnowledgeDocuments(products []seedProduct) []service.IngestDocumentRequest {
	documents := make([]service.IngestDocumentRequest, 0, 81)
	add := func(docID, title, content, category, docType string, models []string, metadata map[string]any, tags ...string) {
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["data_scope"] = "mock"
		metadata["structural_difficulty"] = structuralDifficulty(docType)
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
			Version:    "kb-v2",
			Source:     "mock://clean-care/" + docID,
			IntentTags: tags,
			Metadata:   metadata,
		})
	}

	variants := []knowledgeVariant{
		{ID: "standard_cn", Title: "中国大陆标准版", Region: "CN", Config: "标准配置", Audience: "首次购买、需求均衡的家庭", Constraints: "不含延保、安装服务和实时优惠"},
		{ID: "pet_bundle", Title: "养宠增强套装", Region: "CN", Config: "主机+高频耗材套装", Audience: "一只以上猫狗、毛发清理频繁的家庭", Constraints: "套装耗材不改变主机额定参数"},
		{ID: "large_home", Title: "大户型配置说明", Region: "CN", Config: "大户型使用建议", Audience: "120 平米以上或多房间家庭", Constraints: "适用面积受门槛、地毯和分区数量影响"},
		{ID: "south_region", Title: "南方潮湿地区使用版", Region: "CN-SOUTH", Config: "防潮维护建议", Audience: "高湿、梅雨季或沿海地区家庭", Constraints: "地区建议不是额外硬件版本，参数仍以标准版为准"},
	}
	for _, product := range products {
		for _, variant := range variants {
			docID := "kb_detail_" + slug(product.Model) + "_" + variant.ID
			add(
				docID,
				product.Model+" "+variant.Title+"商品详情",
				renderProductVariant(product, variant),
				product.Category,
				"product_detail",
				[]string{product.Model},
				map[string]any{
					"region":   variant.Region,
					"config":   variant.Config,
					"audience": variant.Audience,
					"variant":  variant.ID,
				},
				"product_parameter",
				"purchase_recommendation",
			)
		}
	}

	guides := []struct {
		ID, Title, Category, Scenario, Candidates string
	}{
		{"pet_large_family", "养宠大家庭扫地机选购指南", "robot_vacuum", "120-200 平米、两只以上宠物、硬地板与地毯混合", "X20 Pro、R20、T20"},
		{"small_budget_robot", "小户型两千元内扫地机选购指南", "robot_vacuum", "40-80 平米、无长毛地毯、预算优先", "R10、T20"},
		{"elderly_robot", "老人家庭低维护扫地机选购指南", "robot_vacuum", "操作简单、低维护、避免频繁倒尘", "T20、X20 Pro"},
		{"allergy_air", "过敏人群空气净化器选购指南", "air_purifier", "花粉和颗粒物敏感、卧室夜间使用", "P400、P500"},
		{"large_living_air", "大客厅净化器选购指南", "air_purifier", "45-70 平米客厅、多人活动、持续净化", "P500"},
		{"new_home_air", "新装修家庭净化器选购指南", "air_purifier", "颗粒物与气态污染物并存、需要持续监测", "P400、P500"},
		{"rental_water", "租住家庭净水器选购指南", "water_purifier", "厨下空间有限、搬家可能性高、安装改造受限", "W300"},
		{"large_family_water", "五口以上家庭净水器选购指南", "water_purifier", "早晚高峰集中用水、可能连接管线机", "W500"},
		{"baby_humidifier", "婴儿房加湿器选购指南", "humidifier", "18-25 平米、夜间安静、易清洁", "H100、H200"},
		{"dry_living_humidifier", "北方供暖季客厅加湿指南", "humidifier", "30-45 平米、长时间运行、减少加水频率", "H200"},
	}
	for _, guide := range guides {
		add(
			"kb_guide_"+guide.ID,
			guide.Title,
			renderScenarioGuide(guide.Title, guide.Scenario, guide.Candidates),
			guide.Category,
			"purchase_guide",
			nil,
			map[string]any{"scenario": guide.Scenario, "candidate_models": guide.Candidates},
			"purchase_recommendation",
			"scenario_constraint",
		)
	}

	manuals := []struct {
		ID, Model, Category, Task, Prerequisite, Steps, Safety string
	}{
		{"t20_first_map", "T20", "robot_vacuum", "首次建图", "基站已通电且地面障碍物已整理", "打开房门；在 App 选择快速建图；建图完成后命名房间并设置禁区", "建图时不要搬动主机或基站"},
		{"t20_deep_clean", "T20", "robot_vacuum", "尘盒与滤网深度清洁", "主机已关机", "取出尘盒；拆下滤网；清除灰尘；完全晾干后复装", "滤网未干透不得装回"},
		{"x20_station", "X20 Pro", "robot_vacuum", "基站清洁", "主机回到安全位置且基站断电", "取出清洗盘；清理污水；擦拭传感器；复装后短时验证", "基站电气接口不得冲水"},
		{"x20_carpet", "X20 Pro", "robot_vacuum", "地毯策略设置", "App 已完成地图", "标记地毯区域；选择增压或规避；先在小区域验证", "长毛地毯先使用禁区模式"},
		{"r10_wifi", "R10", "robot_vacuum", "重新配网", "手机连接 2.4GHz Wi-Fi", "重置网络；在 App 添加设备；输入 Wi-Fi；等待绑定完成", "不要在配网中途关闭主机"},
		{"r10_brush", "R10", "robot_vacuum", "主刷更换", "主机关机并翻转到软垫", "打开主刷盖；取出旧主刷；清理轴端；安装新主刷并锁紧", "发现电机轴损坏时停止操作"},
		{"r20_schedule", "R20", "robot_vacuum", "定时清扫", "地图已保存且时区设置正确", "选择房间；设置日期与时间；选择吸力；保存并观察首次执行", "有宠物饮水区时设置禁区"},
		{"r20_mop", "R20", "robot_vacuum", "拖布维护", "主机关机且拖布组件已冷却", "拆下拖布；清洗污渍；自然晾干；检查魔术贴后复装", "禁止湿拖布长期留在主机上"},
		{"p400_filter", "P400", "air_purifier", "滤芯更换", "设备断电且新滤芯型号为 F400", "打开后盖；取出旧滤芯；拆除新滤芯塑封；按方向安装；重置寿命", "未拆塑封不得开机"},
		{"p400_sensor", "P400", "air_purifier", "传感器清洁", "设备断电 10 分钟", "打开传感器盖；用干燥软刷清洁；关闭盖板；重启观察数值", "不得向传感器喷液体"},
		{"p500_app", "P500", "air_purifier", "App 自动模式联动", "设备已联网", "添加房间；设置 PM2.5 阈值；配置自动模式；验证通知", "自动模式不能替代通风判断"},
		{"p500_prefilter", "P500", "air_purifier", "进风口预清洁", "设备断电", "吸除进风口浮尘；擦拭外壳；检查滤芯安装；恢复供电", "不得拆卸电机组件"},
		{"w300_flush", "W300", "water_purifier", "首次冲洗", "安装验收完成且无漏水", "打开进水阀；通电；按冲洗程序放水；确认无异味后使用", "漏水时立即关阀断电"},
		{"w300_filter", "W300", "water_purifier", "C300 滤芯更换", "关闭进水阀并释放管路压力", "旋出旧滤芯；清洁接口；装入 C300；开阀检漏；执行冲洗", "不得带压拆卸滤芯"},
		{"w500_flow", "W500", "water_purifier", "出水流量检查", "确认市政供水正常", "检查进水阀；查看滤芯寿命；排除管路折弯；记录一分钟出水量", "持续低流量时联系售后"},
		{"h100_scale", "H100", "humidifier", "水垢清洁", "设备断电且水箱已倒空", "用软布清洁水箱；按说明处理雾化片；清水冲净；完全擦干", "底座和电源接口不得进水"},
		{"h200_night", "H200", "humidifier", "夜间模式设置", "水箱水位正常", "设置目标湿度；开启睡眠模式；关闭提示灯；观察两小时湿度", "不得遮挡出雾口"},
	}
	for _, manual := range manuals {
		add(
			"kb_manual_"+manual.ID,
			manual.Model+" "+manual.Task+"操作手册",
			renderTaskManual(manual.Model, manual.Task, manual.Prerequisite, manual.Steps, manual.Safety),
			manual.Category,
			"user_manual",
			[]string{manual.Model},
			map[string]any{"task": manual.Task, "safety_level": "controlled"},
			"usage_instruction",
		)
	}

	policies := []struct {
		ID, Title, Core, Applies, Exception string
	}{
		{"unopened_7d", "未拆封商品七天无理由规则", "签收次日起七日内且商品、附件、赠品和包装完整，可发起申请。", "CleanCare 主机类商品", "定制安装、账号绑定未解除或影响二次销售时需人工核验"},
		{"opened_7d", "已拆封商品退货核验规则", "拆封不等于必然不能退，但需核验使用痕迹、耗材状态和二次销售影响。", "主机已拆封但未明显使用", "滤芯、尘袋等耗材拆封后通常不适用"},
		{"quality_15d", "十五日质量问题换货规则", "在政策窗口内有可验证质量故障时，可按检测结论申请换货。", "主机质量故障", "人为损坏、进液和非授权拆机不适用"},
		{"robot_warranty", "扫地机器人整机保修规则", "保修起算时间以订单有效签收时间为准，具体期限以订单项记录为准。", "T20、X20 Pro、R10、R20", "耗材和人为损坏不按整机保修"},
		{"air_warranty", "空气净化器整机保修规则", "主机与电气部件按订单保修月数判断。", "P400、P500", "滤芯属于耗材，寿命下降不等于整机故障"},
		{"water_install", "净水器安装与漏水服务规则", "首次安装和漏水问题必须核验安装记录、现场照片和阀门状态。", "W300、W500", "用户自行改管导致的问题需人工判责"},
		{"humidifier_warranty", "加湿器保修与水垢边界", "电气故障按保修处理，水质导致的正常水垢需先按说明维护。", "H100、H200", "干烧、进水或腐蚀性清洁剂造成的损坏不适用"},
		{"extended_warranty", "延保服务生效规则", "延保必须在订单中存在有效服务项，且从原保修结束后衔接。", "购买延保的主机", "口头承诺、截图或过期活动不能替代订单服务项"},
		{"accessory_warranty", "配件与耗材保修规则", "配件质量问题需核验 SKU、批次和使用状态。", "滤芯、尘袋、滚刷、拖布", "自然消耗和超过建议周期不视为质量故障"},
		{"onsite_service", "上门检测服务规则", "存在漏水、电气或无法远程判断的问题时，可建议上门检测。", "需要现场检测的安全或安装问题", "未确认地址、时间和费用前不得承诺已预约"},
	}
	for _, policy := range policies {
		add(
			"kb_policy_"+policy.ID,
			policy.Title,
			renderConditionalPolicy(policy.Title, policy.Core, policy.Applies, policy.Exception),
			"cleaning_appliance",
			"after_sales_policy",
			nil,
			map[string]any{"policy_scope": policy.Applies, "effective_region": "CN"},
			"return_eligibility",
			"warranty_query",
		)
	}

	faults := []struct {
		ID, Title, Category, Model, Summary string
	}{
		{"t20_wifi", "T20 Wi-Fi 连接异常排查", "robot_vacuum", "T20", "确认手机连接 2.4GHz Wi-Fi；重置主机网络；检查路由器隔离设置；仍失败时记录 App 错误码"},
		{"x20_app_pair", "X20 Pro App 无法配对排查", "robot_vacuum", "X20 Pro", "确认账号地区与设备地区一致；允许蓝牙和定位权限；重新进入配网模式；仍失败时停止重复绑定"},
		{"r20_noise", "R20 异响诊断", "robot_vacuum", "R20", "关机检查主刷、边刷和万向轮异物；清理轴端毛发；空载短时验证；持续金属摩擦声时停止使用"},
		{"h200_runtime", "H200 续航下降排查", "humidifier", "H200", "核对目标湿度和档位；检查水箱实际容量与漏水；清洁雾化组件；持续异常时记录运行时长"},
	}
	for _, fault := range faults {
		add(
			"kb_fault_"+fault.ID,
			fault.Title,
			renderTroubleshootingTree(fault.ID, fault.Model, fault.Summary),
			fault.Category,
			"troubleshooting",
			[]string{fault.Model},
			map[string]any{"fault_id": fault.ID, "risk_level": "medium"},
			"troubleshooting",
		)
	}
	return documents
}

func renderProductVariant(product seedProduct, variant knowledgeVariant) string {
	return fmt.Sprintf(`%s

## 版本信息

| 字段 | 值 |
|---|---|
| 型号 | %s |
| 版本 | %s |
| 地区 | %s |
| 配置 | %s |
| 目标用户 | %s |

## 配置差异矩阵

| 核验项 | 本版本结论 | 回答要求 |
|---|---|---|
| 主机核心参数 | 与标准参数表一致 | 数值必须引用参数表 |
| 随箱配件 | 按配置清单核验 | 不把赠品写成主机能力 |
| 地区适配 | %s | 不推断跨地区保修 |
| 实时价格库存 | 本文档不提供 | 必须调用动态工具 |

## 约束与例外

- %s。
- 套装、地区和使用建议不会改变主机额定吸力、CADR、通量或加湿量。
- 对比其他型号时必须使用同一参数口径，缺失字段标记为“未提供”。
`, renderProductDetail(product), product.Model, variant.Title, variant.Region, variant.Config, variant.Audience, variant.Region, variant.Constraints)
}

func renderScenarioGuide(title, scenario, candidates string) string {
	return fmt.Sprintf(`# %s

## 场景画像

%s。

## 候选范围

%s。

## 多条件决策表

| 决策维度 | 硬条件 | 软偏好 | 不满足时 |
|---|---|---|---|
| 空间与面积 | 不低于目标覆盖范围 | 留出 20%% 余量 | 排除候选 |
| 安全与安装 | 满足供电、供水、摆放条件 | 维护方便 | 先澄清 |
| 核心性能 | 满足场景最低参数 | 选择更低噪声或更高自动化 | 给出取舍 |
| 预算 | 到手价不超过预算 | 耗材成本可接受 | 查询优惠或降档 |

## 决策流程

1. 提取面积、人数、宠物、地毯、预算和安装条件。
2. 用结构化参数表做硬条件过滤。
3. 检索候选商品详情，核对限制项和维护成本。
4. 查询实时价格与库存，静态文档不得代替动态数据。
5. 输出推荐、备选、不适用条件和仍需确认的信息。

## 避坑

- 不只比较单一峰值参数。
- 不把宣传面积当作所有户型的绝对覆盖承诺。
- 证据无法覆盖用户硬条件时必须先澄清。`, title, scenario, candidates)
}

func renderTaskManual(model, task, prerequisite, stepText, safety string) string {
	steps := strings.Split(stepText, "；")
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s %s\n\n## 任务目标\n\n在不拆解电气部件的前提下完成%s。\n\n", model, task, task)
	fmt.Fprintf(&builder, "## 任务前置条件\n\n%s。\n\n## 任务操作步骤\n\n", prerequisite)
	for index, step := range steps {
		fmt.Fprintf(&builder, "%d. %s。\n", index+1, strings.TrimSpace(step))
	}
	fmt.Fprintf(&builder, "\n## 任务完成标准\n\n- 设备无漏水、异响、焦味和错误码。\n- App 或设备状态与任务目标一致。\n\n## 任务安全警告\n\n- %s。\n- 发现冒烟、漏电、焦糊味或持续漏水时立即停止并转售后。\n", safety)
	return builder.String()
}

func renderConditionalPolicy(title, core, applies, exception string) string {
	return fmt.Sprintf(`# %s

## 核心条款

第一条：核心规则
%s

## 条件分支

第二条：适用条件
| 条件 | 满足时 | 不满足时 |
|---|---|---|
| 订单归属当前用户 | 继续核验 | 拒绝披露并提示重新登录 |
| 时间在政策窗口内 | 核验商品状态 | 不直接承诺，检查质量例外 |
| 商品状态符合要求 | 可进入申请流程 | 说明缺失条件 |
| 证据与字段完整 | 给出条件化结论 | 转人工核验 |

## 适用范围

第三条：范围与例外
%s。

## 例外

%s。

## 执行约束

第四条：执行边界
- 所有时间以 UTC 订单记录为准，API 展示时转换时区。
- 创建工单前必须有明确确认和非空幂等键。
- 本文档不代表已接入真实支付、物流或 ERP。`, title, core, applies, exception)
}
