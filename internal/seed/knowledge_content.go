package seed

import (
	"fmt"
	"strings"
)

func findSeedProduct(products []seedProduct, model string) seedProduct {
	for _, product := range products {
		if product.Model == model {
			return product
		}
	}
	return seedProduct{Model: model}
}

func renderProductComparison(left, right seedProduct, conclusion string) string {
	leftValues := parameterValues(left)
	rightValues := parameterValues(right)
	names := make([]string, 0, len(left.Parameters)+len(right.Parameters))
	seen := map[string]struct{}{}
	for _, product := range []seedProduct{left, right} {
		for _, parameter := range product.Parameters {
			if _, ok := seen[parameter.Name]; ok {
				continue
			}
			seen[parameter.Name] = struct{}{}
			names = append(names, parameter.Name)
		}
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s 与 %s 结构化对比\n\n## 核心结论\n\n%s\n\n", left.Model, right.Model, conclusion)
	fmt.Fprintf(&builder, "## 参数对比\n\n| 对比维度 | %s | %s |\n|---|---|---|\n", left.Model, right.Model)
	for _, name := range names {
		fmt.Fprintf(&builder, "| %s | %s | %s |\n", name, leftValues[name], rightValues[name])
	}
	fmt.Fprintf(&builder, "\n## 场景取舍\n\n- 优先 %s：%s。\n", left.Model, strings.Join(left.SuitableFor, "；"))
	fmt.Fprintf(&builder, "- 优先 %s：%s。\n", right.Model, strings.Join(right.SuitableFor, "；"))
	builder.WriteString("- 硬约束先过滤适用面积、安装条件和安全限制，再比较体验项。\n")
	builder.WriteString("- 静态对比不包含实时价格、优惠和库存；这些信息必须通过动态工具查询。\n")
	builder.WriteString("\n> 数据来源：mock://clean-care/product-catalog-v2。")
	return builder.String()
}

func parameterValues(product seedProduct) map[string]string {
	values := make(map[string]string, len(product.Parameters))
	for _, parameter := range product.Parameters {
		value := parameter.Value
		if parameter.Name == "型号" {
			value = product.Model
		}
		values[parameter.Name] = value
	}
	return values
}

func renderPurchaseGuide(title, category, summary string) string {
	dimensions := map[string][]string{
		"robot_vacuum":   {"户型面积与单次续航", "地毯类型与拖布抬升", "宠物数量与滚刷防缠绕", "越障高度", "基站维护成本", "预算"},
		"air_purifier":   {"房间面积", "颗粒物 CADR", "睡眠噪声", "滤芯型号与周期", "传感器类型", "摆放空间"},
		"water_purifier": {"家庭人数", "早晚用水峰值", "额定通量与流速", "进水压力", "厨下空间", "滤芯与废水比"},
		"humidifier":     {"房间面积", "目标加湿量", "夜间噪声", "水箱容量", "水质与清洁频率", "缺水保护"},
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n## 场景结论\n\n%s\n\n## 必问约束\n\n", title, summary)
	for index, dimension := range dimensions[category] {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, dimension)
	}
	builder.WriteString("\n## 决策流程\n\n")
	builder.WriteString("1. 提取面积、人数、宠物、地毯、预算等硬条件。\n")
	builder.WriteString("2. 用结构化参数表排除不满足硬条件的型号。\n")
	builder.WriteString("3. 对剩余型号比较噪声、维护成本和自动化能力。\n")
	builder.WriteString("4. 调用实时价格与库存工具，不能使用静态文档中的旧价格。\n")
	builder.WriteString("5. 明确推荐理由、关键取舍和不适用情况；证据不足时先澄清。\n")
	builder.WriteString("\n## 风险提示\n\n- 宣传面积不能替代真实户型、门槛和摆放条件。\n- 耗材寿命受环境和使用强度影响，不承诺固定更换日期。\n- 涉及安装、电气或漏水风险时优先给出安全动作。\n")
	return builder.String()
}

func renderCompatibility(host, accessory, cycle string) string {
	return fmt.Sprintf(`# %s 配件兼容关系

| 主机型号 | 配件型号 | 兼容状态 | 建议更换周期 | 核验方式 |
|---|---|---|---|---|
| %s | %s | 明确兼容 | %s | 核对主机底部铭牌和配件包装型号 |

## 查询步骤

1. 无型号时先查用户购买记录或要求用户查看主机铭牌。
2. 以兼容表中的“主机型号 + 配件型号”精确关系为准，外观相似不能作为兼容证据。
3. 确认兼容后再调用价格和库存工具。
4. 已拆封耗材是否可退需单独依据售后政策判断。

## 禁止推断

- 不同型号接口相似不代表可混用。
- 本文档不包含实时价格和库存。
- 找不到明确兼容记录时，应回复“证据不足，需要人工核验”，不得猜测。`,
		host, host, accessory, cycle)
}

func renderUserManual(product seedProduct) string {
	return fmt.Sprintf(`# %s 使用说明书

## 任务1：开箱与安装

1. 核对主机、供电部件、耗材和说明书是否齐全。
2. 移除运输保护、滤芯塑封或固定件；未移除包装不得通电运行。
3. 按安装示意预留进出风、进出水或基站通道。

## 任务2：首次启动

1. 使用额定电源并完成必要的加水、冲洗、建图或配网。
2. 配网仅支持 2.4GHz Wi-Fi 时，不要使用纯 5GHz 网络。
3. 首次运行观察异响、漏水、错误码和异常气味，发现异常立即停机。

## 任务3：日常使用

- 适用场景：%s。
- 不适用或需额外注意：%s。
- 动态耗材余量、价格和库存以工具查询结果为准。

## 任务4：清洁维护

1. 维护前断电；涉水设备同时关闭进水阀。
2. 可拆件按说明清洁并完全晾干后复装。
3. 底座、电机和电气接口不得浸水，不得使用腐蚀性清洁剂。

## 任务5：停止使用并转售后

出现持续漏水、焦糊味、冒烟、明显电机异响、反复报错或清洁后仍无法恢复时停止使用。记录型号、错误码、照片和已完成步骤，不得自行拆解电气部件。`,
		product.Model,
		strings.Join(product.SuitableFor, "；"),
		strings.Join(product.Limits, "；"),
	)
}

func renderTroubleshootingTree(id, model, summary string) string {
	steps := strings.FieldsFunc(summary, func(r rune) bool {
		return r == '；' || r == '。'
	})
	for len(steps) < 2 {
		steps = append(steps, "记录现象、指示灯和错误码")
	}
	return fmt.Sprintf(`# %s 故障树

node_id: %s_root
parent_node_id:
symptom: 用户报告 %s 异常
question: 是否存在漏水、冒烟、焦糊味或触电风险？
yes_next: %s_safety_stop
no_next: %s_check_1

node_id: %s_safety_stop
parent_node_id: %s_root
action: 立即断电；涉水设备关闭进水阀；远离积水，不继续试机。
result: 转人工售后，不指导拆机。

node_id: %s_check_1
parent_node_id: %s_root
action: %s
yes_next: %s_verify
no_next: %s_check_2

node_id: %s_check_2
parent_node_id: %s_check_1
action: %s
yes_next: %s_verify
no_next: %s_escalate

node_id: %s_verify
parent_node_id: %s_check_2
action: 复装后进行一次短时验证，观察错误码、噪声、漏水或充电状态。
resolved_next: close
unresolved_next: %s_escalate

node_id: %s_escalate
parent_node_id: %s_verify
action: 停止继续排查，收集订单号、型号、错误码、照片和已完成步骤，经用户确认后创建售后工单。

## 已知排查摘要

%s`,
		model, id, model, id, id,
		id, id,
		id, id, strings.TrimSpace(steps[0]), id, id,
		id, id, strings.TrimSpace(steps[1]), id, id,
		id, id, id,
		id, id,
		summary,
	)
}

func renderPolicy(title, core string) string {
	return fmt.Sprintf(`# %s

第一条：核心规则
%s

第二条：适用条件
- 必须校验当前用户的订单归属、订单状态、签收或支付时间。
- 商品状态、附件、包装、耗材拆封情况和故障证据必须明确。
- 时间统一按 UTC 存储，判断时以订单记录中的有效时间为准。

第三条：例外与边界
- 质量问题、运输损坏和安全风险不能只按“七天无理由”处理。
- 动态政策版本、订单状态或证据缺失时，不给出绝对退款/换货承诺。
- 涉及支付、物流和 ERP 的状态均为 mock 数据，不声称已接入真实系统。

第四条：输出要求
- 使用“如果……则……”条件化表述。
- 列出已满足、未满足和待核验条件。
- 创建售后工单必须有用户明确确认和非空幂等键。`,
		title, core)
}

func renderFAQ(question, answer string) string {
	return fmt.Sprintf(`# FAQ

Q: %s
A: %s

补充说明：
- 回答只适用于题目中明确的型号。
- 涉及安全风险时先断电或关闭进水阀。
- 涉及价格、库存、订单、保修和政策有效期时，以动态工具或当前政策文档为准。`,
		question, answer)
}
