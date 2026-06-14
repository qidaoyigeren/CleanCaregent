package evidencefmt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCompactPreservesFocusedTableRowAndHeader(t *testing.T) {
	content := `| 型号 | 吸力 | 适用场景 |
|---|---:|---|
| T20 | 6000Pa | 硬质地面 |
| R10 | 5000Pa | 小户型 |
| R20 | 7000Pa | 地毯 |
| P400 | 450m3/h | 卧室 |
| P500 | 600m3/h | 客厅 |
| W300 | 400G | 三口之家 |
| W500 | 600G | 多人家庭 |
| H100 | 300mL/h | 卧室 |
| H200 | 450mL/h | 客厅 |
| X20 Pro | 8000Pa | 养宠和地毯 |`

	got := Compact(content, 150, "X20 Pro 吸力")
	for _, expected := range []string{"| 型号 | 吸力", "X20 Pro", "8000Pa"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("compressed evidence missing %q:\n%s", expected, got)
		}
	}
	if utf8.RuneCountInString(got) > 150 {
		t.Fatalf("compressed evidence has %d runes", utf8.RuneCountInString(got))
	}
}

func TestCompactPreservesPolicyExceptionAtTail(t *testing.T) {
	content := "# 退货政策\n" +
		strings.Repeat("商品签收后应保持包装和配件完整。\n", 20) +
		"例外：滤芯拆封并投入使用后，不适用七天无理由退货。"

	got := Compact(content, 180, "滤芯拆封能否退货")
	if !strings.Contains(got, "例外") || !strings.Contains(got, "不适用七天无理由退货") {
		t.Fatalf("policy exception was lost:\n%s", got)
	}
	if utf8.RuneCountInString(got) > 180 {
		t.Fatalf("compressed evidence has %d runes", utf8.RuneCountInString(got))
	}
}
