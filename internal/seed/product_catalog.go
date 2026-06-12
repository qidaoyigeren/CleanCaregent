package seed

import (
	"fmt"
	"strings"
)

type productParameter struct {
	Name  string
	Value string
}

type seedProduct struct {
	Model       string
	Category    string
	Kind        string
	Summary     string
	Parameters  []productParameter
	SuitableFor []string
	Limits      []string
}

func defaultProducts() []seedProduct {
	return []seedProduct{
		{
			Model:    "T20",
			Category: "robot_vacuum",
			Kind:     "扫拖一体机器人",
			Summary:  "面向中小户型的均衡型扫拖机器人，侧重硬质地板清洁和基础宠物毛发处理。",
			Parameters: robotParameters(
				"6000Pa", "LDS 激光导航 + 结构光避障", "180 分钟", "80-120㎡",
				"400mL", "200mL 电控水箱", "≤65dB(A)", "≤20mm",
				"地毯识别 + 自动增压", "胶刷+毛刷组合", "手动清理尘盒",
			),
			SuitableFor: []string{"80-120㎡ 中小户型", "以瓷砖或木地板为主的家庭", "单只短毛宠物家庭"},
			Limits:      []string{"多只长毛宠物家庭需要更频繁清理滚刷", "大面积长毛地毯场景建议选择更强防缠绕型号"},
		},
		{
			Model:    "X20 Pro",
			Category: "robot_vacuum",
			Kind:     "全能基站扫拖机器人",
			Summary:  "面向大户型和养宠家庭，提供更高吸力、双胶刷防缠绕和基站自清洁能力。",
			Parameters: robotParameters(
				"8000Pa", "LDS 激光导航 + AI 结构光避障", "210 分钟", "100-150㎡",
				"350mL", "基站自动补水", "≤64dB(A)", "≤22mm",
				"地毯识别 + 拖布抬升 + 自动增压", "双胶刷零缠绕结构", "自动集尘",
			),
			SuitableFor: []string{"100-150㎡ 大户型", "多宠物或重度掉毛家庭", "地毯与硬质地板混合家庭"},
			Limits:      []string{"基站占地和维护成本高于基础型号", "首次安装需要预留基站进出空间"},
		},
		{
			Model:    "R10",
			Category: "robot_vacuum",
			Kind:     "入门扫地机器人",
			Summary:  "面向小户型和基础清扫需求，结构简单、维护成本较低。",
			Parameters: robotParameters(
				"5000Pa", "LDS 激光导航 + 红外避障", "150 分钟", "60-100㎡",
				"450mL", "180mL 电控水箱", "≤66dB(A)", "≤18mm",
				"地毯识别 + 增压", "单胶刷", "手动清理尘盒",
			),
			SuitableFor: []string{"60-100㎡ 小户型", "硬质地板为主的日常清扫", "预算敏感用户"},
			Limits:      []string{"不支持自动集尘", "复杂障碍物和重度毛发场景需要人工辅助"},
		},
		{
			Model:    "R20",
			Category: "robot_vacuum",
			Kind:     "增强型扫拖机器人",
			Summary:  "适合中等户型和地毯场景，在吸力、续航和越障能力之间保持平衡。",
			Parameters: robotParameters(
				"7000Pa", "LDS 激光导航 + 双线激光避障", "190 分钟", "90-130㎡",
				"400mL", "220mL 电控水箱", "≤65dB(A)", "≤20mm",
				"地毯识别 + 自动增压", "防缠绕胶刷", "可选自动集尘基站",
			),
			SuitableFor: []string{"90-130㎡ 中等户型", "有短毛地毯的家庭", "需要更强吸力但不需要全能基站的用户"},
			Limits:      []string{"自动洗拖布不是标准配置", "重度长毛宠物场景仍需定期清理轴端"},
		},
		{
			Model:    "P400",
			Category: "air_purifier",
			Kind:     "空气净化器",
			Summary:  "适合卧室和中等客厅，兼顾颗粒物净化、睡眠噪声和滤芯成本。",
			Parameters: airParameters(
				"450m³/h", "35-55㎡", "F400 复合滤芯", "6-12 个月",
				"≤32dB(A)", "45W", "5 档", "PM2.5 激光传感器",
			),
			SuitableFor: []string{"卧室和中等面积客厅", "关注花粉、灰尘和宠物浮毛的家庭", "夜间需要低噪运行的用户"},
			Limits:      []string{"不能替代新风系统提供室外换气", "滤芯寿命取决于污染程度和运行时长"},
		},
		{
			Model:    "P500",
			Category: "air_purifier",
			Kind:     "大空间空气净化器",
			Summary:  "面向大客厅和开放空间，提供更高 CADR、双传感器和更大的循环风量。",
			Parameters: airParameters(
				"600m³/h", "50-75㎡", "F500 复合滤芯", "6-12 个月",
				"≤34dB(A)", "65W", "6 档", "PM2.5 + VOC 双传感器",
			),
			SuitableFor: []string{"大客厅和开放式空间", "多人或多宠物家庭", "对颗粒物和异味都较敏感的用户"},
			Limits:      []string{"机身体积和最大档噪声高于 P400", "不能处理一氧化碳等需要专业报警器监测的气体"},
		},
		{
			Model:    "W300",
			Category: "water_purifier",
			Kind:     "RO 反渗透净水器",
			Summary:  "面向 3-4 人家庭的厨下式净水器，强调稳定出水和滤芯状态提示。",
			Parameters: waterParameters(
				"400G", "1.05L/min", "3-4 人", "C300 复合滤芯 + RO 膜",
				"2:1", "60W", "0.1-0.4MPa", "≤55dB(A)",
			),
			SuitableFor: []string{"3-4 人家庭日常饮水和烹饪", "厨下空间有限的家庭", "需要 TDS 显示和滤芯提醒的用户"},
			Limits:      []string{"安装位置需要进水、排水和电源", "滤芯属于耗材，实际周期受原水水质影响"},
		},
		{
			Model:    "W500",
			Category: "water_purifier",
			Kind:     "大通量 RO 净水器",
			Summary:  "面向多人家庭和用水高峰场景，提供更高通量和更快的连续出水速度。",
			Parameters: waterParameters(
				"600G", "1.58L/min", "4-6 人", "C500 复合滤芯 + RO 膜",
				"2.5:1", "75W", "0.1-0.4MPa", "≤56dB(A)",
			),
			SuitableFor: []string{"4-6 人家庭", "早晚用水高峰明显的家庭", "经常需要连续接水的厨房"},
			Limits:      []string{"机身体积和额定功率高于 W300", "低水压环境可能需要安装增压设备"},
		},
		{
			Model:    "H100",
			Category: "humidifier",
			Kind:     "卧室加湿器",
			Summary:  "适合卧室和书房，重点控制夜间噪声、缺水保护和清洁便利性。",
			Parameters: humidifierParameters(
				"4L", "300mL/h", "20-35㎡", "≤30dB(A)", "13 小时",
				"25W", "3 档", "湿度传感器",
			),
			SuitableFor: []string{"卧室、书房和儿童房", "关注夜间低噪的用户", "中小面积持续加湿"},
			Limits:      []string{"不建议加入精油或消毒液", "需要定期清洁水箱和雾化片以避免水垢"},
		},
		{
			Model:    "H200",
			Category: "humidifier",
			Kind:     "大容量智能加湿器",
			Summary:  "面向客厅和较大房间，提供更大水箱、更高加湿量和自动恒湿能力。",
			Parameters: humidifierParameters(
				"6L", "450mL/h", "30-50㎡", "≤32dB(A)", "13 小时",
				"35W", "4 档", "双湿度传感器",
			),
			SuitableFor: []string{"客厅和大卧室", "需要长时间连续加湿的家庭", "希望自动恒湿和远程控制的用户"},
			Limits:      []string{"满水后整机重量较高", "高矿物质硬水可能产生水垢，需要更频繁清洁"},
		},
	}
}

func robotParameters(
	suction, navigation, runtime, area, dustbin, waterTank, noise, obstacle,
	carpet, brush, dustCollection string,
) []productParameter {
	return []productParameter{
		{"型号", ""},
		{"品类", "扫地机器人（扫拖一体）"},
		{"额定吸力", suction},
		{"导航与避障", navigation},
		{"标准模式续航", runtime},
		{"电池容量", "5200mAh"},
		{"适用面积", area},
		{"尘盒容量", dustbin},
		{"水箱/补水方式", waterTank},
		{"标准模式噪声", noise},
		{"越障能力", obstacle},
		{"地毯清洁", carpet},
		{"主刷类型", brush},
		{"集尘方式", dustCollection},
		{"联网方式", "2.4GHz Wi-Fi"},
		{"支持应用", "CleanCare Home"},
	}
}

func airParameters(cadr, area, filter, life, noise, power, levels, sensor string) []productParameter {
	return []productParameter{
		{"型号", ""},
		{"品类", "空气净化器"},
		{"颗粒物 CADR", cadr},
		{"建议适用面积", area},
		{"滤芯型号", filter},
		{"滤芯结构", "初效滤网 + HEPA + 活性炭"},
		{"建议更换周期", life},
		{"睡眠模式噪声", noise},
		{"额定功率", power},
		{"风量档位", levels},
		{"空气质量传感器", sensor},
		{"显示方式", "实时空气质量数值 + 颜色指示"},
		{"定时功能", "1-12 小时"},
		{"联网方式", "2.4GHz Wi-Fi"},
		{"支持应用", "CleanCare Home"},
	}
}

func waterParameters(flow, speed, people, filter, wastewater, power, pressure, noise string) []productParameter {
	return []productParameter{
		{"型号", ""},
		{"品类", "厨下式 RO 反渗透净水器"},
		{"额定通量", flow},
		{"纯水流速", speed},
		{"建议家庭人数", people},
		{"滤芯配置", filter},
		{"储水方式", "无桶即滤式"},
		{"净废水比", wastewater},
		{"额定功率", power},
		{"适用进水压力", pressure},
		{"运行噪声", noise},
		{"水质显示", "龙头 TDS 指示"},
		{"安全保护", "缺水保护 + 儿童锁"},
		{"滤芯提醒", "CleanCare Home 应用提醒"},
		{"安装方式", "厨下安装，需进水/排水/电源"},
	}
}

func humidifierParameters(tank, output, area, noise, runtime, power, levels, sensor string) []productParameter {
	return []productParameter{
		{"型号", ""},
		{"品类", "智能加湿器"},
		{"水箱容量", tank},
		{"额定加湿量", output},
		{"建议适用面积", area},
		{"睡眠模式噪声", noise},
		{"满水最长运行", runtime},
		{"额定功率", power},
		{"加湿档位", levels},
		{"湿度检测", sensor},
		{"自动恒湿", "支持 40%-70% 目标湿度"},
		{"缺水保护", "缺水自动停机"},
		{"清洁方式", "可拆水箱，底座禁止浸水"},
		{"联网方式", "2.4GHz Wi-Fi"},
		{"建议用水", "洁净软水或纯净水"},
	}
}

func renderProductDetail(product seedProduct) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s %s\n\n%s\n\n## 结构化参数\n\n", product.Model, product.Kind, product.Summary)
	builder.WriteString("| 参数 | 模拟值 |\n|---|---|\n")
	for _, parameter := range product.Parameters {
		value := parameter.Value
		if parameter.Name == "型号" {
			value = product.Model
		}
		fmt.Fprintf(&builder, "| %s | %s |\n", parameter.Name, value)
	}
	builder.WriteString("\n## 适用场景\n")
	for _, item := range product.SuitableFor {
		fmt.Fprintf(&builder, "- %s\n", item)
	}
	builder.WriteString("\n## 限制与注意事项\n")
	for _, item := range product.Limits {
		fmt.Fprintf(&builder, "- %s\n", item)
	}
	builder.WriteString("\n> 数据来源：mock://clean-care/product-catalog-v2。以上为面试演示用模拟参数；价格、库存、订单和保修状态必须通过动态工具查询。")
	return builder.String()
}

func renderParameterTable(product seedProduct) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s 结构化参数表\n\n", product.Model)
	builder.WriteString("| 字段 | 模拟值 |\n|---|---|\n")
	for _, parameter := range product.Parameters {
		value := parameter.Value
		if parameter.Name == "型号" {
			value = product.Model
		}
		fmt.Fprintf(&builder, "| %s | %s |\n", parameter.Name, value)
	}
	builder.WriteString("\n数据来源：mock://clean-care/product-catalog-v2。动态价格和库存不写入静态参数表。")
	return builder.String()
}
