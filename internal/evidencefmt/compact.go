package evidencefmt

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const compressionMarker = "...[中间内容已压缩]..."

var (
	asciiTermPattern = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9+._-]*`)
	numberPattern    = regexp.MustCompile(`\d`)
)

type segment struct {
	index int
	text  string
	score int
}

// Compact reduces evidence text without blindly dropping the tail. It keeps
// document structure, query-related lines, numeric facts and condition or
// exception clauses, then restores the selected lines in source order.
func Compact(value string, limit int, focus ...string) string {
	value = normalize(value)
	if limit <= 0 || runeLen(value) <= limit {
		return value
	}

	segments := splitSegments(value)
	if len(segments) == 0 {
		return ""
	}
	if len(segments) == 1 {
		return headTail(segments[0], limit)
	}

	terms := focusTerms(strings.Join(focus, " "))
	candidates := make([]segment, 0, len(segments))
	for index, text := range segments {
		candidates = append(candidates, segment{
			index: index,
			text:  text,
			score: segmentScore(text, index, len(segments), terms),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].index < candidates[j].index
		}
		return candidates[i].score > candidates[j].score
	})

	selected := map[int]struct{}{0: {}, len(segments) - 1: {}}
	for index, text := range segments {
		if isTableHeader(text, index, segments) {
			selected[index] = struct{}{}
		}
	}
	for _, candidate := range candidates {
		if _, exists := selected[candidate.index]; exists {
			continue
		}
		selected[candidate.index] = struct{}{}
		rendered := renderSelected(segments, selected)
		if runeLen(rendered) > limit {
			delete(selected, candidate.index)
		}
	}

	rendered := renderSelected(segments, selected)
	if runeLen(rendered) <= limit {
		return rendered
	}
	return headTail(rendered, limit)
}

func normalize(value string) string {
	value = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(value)
	return strings.TrimSpace(value)
}

func splitSegments(value string) []string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if runeLen(line) <= 320 || strings.Contains(line, "|") {
			result = append(result, line)
			continue
		}
		result = append(result, splitLongLine(line)...)
	}
	return result
}

func splitLongLine(value string) []string {
	var result []string
	var current []rune
	flush := func() {
		text := strings.TrimSpace(string(current))
		if text != "" {
			result = append(result, text)
		}
		current = current[:0]
	}
	for _, char := range []rune(value) {
		current = append(current, char)
		if char == '。' || char == '！' || char == '？' || char == '；' || len(current) >= 240 {
			flush()
		}
	}
	flush()
	return result
}

func segmentScore(text string, index, total int, terms []string) int {
	lower := strings.ToLower(text)
	score := 0
	if index == 0 {
		score += 1000
	}
	if index == total-1 {
		score += 250
	}
	if strings.HasPrefix(text, "#") {
		score += 80
	}
	if strings.Contains(text, "|") {
		score += 40
	}
	if numberPattern.MatchString(text) {
		score += 25
	}
	for _, keyword := range []string{
		"例外", "除外", "不适用", "条件", "必须", "不得", "禁止",
		"如果", "当", "否则", "步骤", "故障", "安全", "型号", "参数", "结论",
	} {
		if strings.Contains(text, keyword) {
			score += 35
		}
	}
	for _, term := range terms {
		if strings.Contains(lower, term) {
			score += 120
		}
	}
	return score
}

func focusTerms(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	seen := make(map[string]struct{})
	var result []string
	add := func(term string) {
		term = strings.TrimSpace(term)
		if runeLen(term) < 2 {
			return
		}
		if _, exists := seen[term]; exists {
			return
		}
		seen[term] = struct{}{}
		result = append(result, term)
	}
	for _, term := range asciiTermPattern.FindAllString(value, -1) {
		add(term)
	}
	var han []rune
	flushHan := func() {
		if len(han) == 0 {
			return
		}
		if len(han) <= 8 {
			add(string(han))
		}
		for index := 0; index+2 <= len(han); index++ {
			add(string(han[index : index+2]))
		}
		han = han[:0]
	}
	for _, char := range []rune(value) {
		if unicode.Is(unicode.Han, char) {
			han = append(han, char)
			continue
		}
		flushHan()
	}
	flushHan()
	return result
}

func isTableHeader(text string, index int, segments []string) bool {
	if !strings.Contains(text, "|") {
		return false
	}
	if index == 0 {
		return true
	}
	if index == 1 && strings.Contains(segments[0], "|") {
		return true
	}
	trimmed := strings.NewReplacer("|", "", "-", "", ":", "", " ", "").Replace(text)
	return trimmed == ""
}

func renderSelected(segments []string, selected map[int]struct{}) string {
	indices := make([]int, 0, len(selected))
	for index := range selected {
		indices = append(indices, index)
	}
	sort.Ints(indices)

	var builder strings.Builder
	previous := -1
	for _, index := range indices {
		if index < 0 || index >= len(segments) {
			continue
		}
		if builder.Len() > 0 {
			if previous+1 != index {
				builder.WriteString("\n")
				builder.WriteString(compressionMarker)
			}
			builder.WriteString("\n")
		}
		builder.WriteString(segments[index])
		previous = index
	}
	return builder.String()
}

func headTail(value string, limit int) string {
	if limit <= 0 || runeLen(value) <= limit {
		return value
	}
	marker := []rune("\n" + compressionMarker + "\n")
	if limit <= len(marker)+2 {
		return string([]rune(value)[:limit])
	}
	remaining := limit - len(marker)
	headSize := remaining * 2 / 3
	tailSize := remaining - headSize
	runes := []rune(value)
	return string(runes[:headSize]) + string(marker) + string(runes[len(runes)-tailSize:])
}

func runeLen(value string) int {
	return len([]rune(value))
}
