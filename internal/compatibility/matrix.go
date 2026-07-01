package compatibility

import "strings"

type Status string

const (
	Compatible   Status = "compatible"
	Incompatible Status = "incompatible"
	Unknown      Status = "unknown"
)

type Entry struct {
	HostModel      string
	AccessoryModel string
	AccessoryType  string
	Status         Status
	Reason         string
	EvidenceDocID  string
}

type Result struct {
	HostModel      string
	AccessoryModel string
	AccessoryType  string
	Status         Status
	Reason         string
	EvidenceDocID  string
}

type Matrix struct {
	entries map[string]Entry
}

func NewDefaultMatrix() *Matrix {
	entries := []Entry{
		{"P400", "F400", "滤芯", Compatible, "F400 是 P400 对应的复合滤芯型号。", "kb_compat_p400_f400"},
		{"P400", "F500", "滤芯", Incompatible, "F500 的结构和尺寸面向 P500，不能安装到 P400。", "kb_compat_p400_f400"},
		{"P500", "F500", "滤芯", Compatible, "F500 是 P500 对应的复合滤芯型号。", "kb_compat_p500_f500"},
		{"P500", "F400", "滤芯", Incompatible, "F400 的结构和尺寸面向 P400，不能安装到 P500。", "kb_compat_p500_f500"},
		{"T20", "DB20", "尘袋", Compatible, "DB20 是 T20 基站适配尘袋。", "kb_compat_t20_db20"},
		{"X20 Pro", "RB20", "滚刷", Compatible, "RB20 是 X20 Pro 适配滚刷。", "kb_compat_x20_pro_rb20"},
		{"W300", "C300", "滤芯", Compatible, "C300 是 W300 对应滤芯。", "kb_compat_w300_c300"},
	}
	return NewMatrix(entries)
}

func NewMatrix(entries []Entry) *Matrix {
	matrix := &Matrix{entries: make(map[string]Entry, len(entries))}
	matrix.Merge(entries)
	return matrix
}

func (m *Matrix) Merge(entries []Entry) {
	if m.entries == nil {
		m.entries = make(map[string]Entry, len(entries))
	}
	for _, entry := range entries {
		entry.HostModel = normalize(entry.HostModel)
		entry.AccessoryModel = normalize(entry.AccessoryModel)
		if entry.HostModel == "" || entry.AccessoryModel == "" {
			continue
		}
		if entry.Status == "" {
			entry.Status = Unknown
		}
		m.entries[key(entry.HostModel, entry.AccessoryModel)] = entry
	}
}

func (m *Matrix) Entries() []Entry {
	if m == nil || len(m.entries) == 0 {
		return nil
	}
	entries := make([]Entry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}
	return entries
}

func (m *Matrix) Check(hostModel, accessoryModel string) Result {
	hostModel = normalize(hostModel)
	accessoryModel = normalize(accessoryModel)
	entry, ok := m.entries[key(hostModel, accessoryModel)]
	if !ok {
		return Result{
			HostModel:      hostModel,
			AccessoryModel: accessoryModel,
			Status:         Unknown,
			Reason:         "当前兼容矩阵没有收录这组主机和配件关系，不能根据名称相似度推断兼容。",
		}
	}
	return Result{
		HostModel:      entry.HostModel,
		AccessoryModel: entry.AccessoryModel,
		AccessoryType:  entry.AccessoryType,
		Status:         entry.Status,
		Reason:         entry.Reason,
		EvidenceDocID:  entry.EvidenceDocID,
	}
}

func key(hostModel, accessoryModel string) string {
	return strings.ToUpper(strings.TrimSpace(hostModel)) + "|" +
		strings.ToUpper(strings.TrimSpace(accessoryModel))
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
