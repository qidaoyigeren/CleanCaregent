package agent

import (
	"strings"
	"testing"

	"CleanCaregent/internal/rag"
)

func TestExpandHopQueryUsesPreviousEntities(t *testing.T) {
	previous := []rag.SearchResult{{
		Title:   "P400 配件兼容",
		Content: "P400 使用 F400 滤芯。",
	}}
	query := expandHopQuery("查询 {previous_entities} 与 P500 的兼容关系", previous)
	if !strings.Contains(query, "P400") || !strings.Contains(query, "F400") {
		t.Fatalf("expanded query = %q", query)
	}
}

func TestTagRetrievalResultsRecordsRouteAndHop(t *testing.T) {
	results := []rag.SearchResult{{ChunkID: "chunk-1"}}
	tagRetrievalResults(results, "multi_hop", 2)
	if results[0].Metadata["retrieval_route"] != "multi_hop" ||
		results[0].Metadata["hop_index"] != 2 {
		t.Fatalf("metadata = %#v", results[0].Metadata)
	}
}
