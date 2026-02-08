package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/store"
)

func TestFormatEdgeTypeSummarySorted(t *testing.T) {
	edges := map[rpg.EdgeType]int{
		rpg.EdgeInvokes:     2,
		rpg.EdgeContains:    7,
		rpg.EdgeMapsToChunk: 3,
	}

	got := formatEdgeTypeSummary(edges)
	want := "contains: 7, invokes: 2, maps_to_chunk: 3"
	if got != want {
		t.Fatalf("formatEdgeTypeSummary() = %q, want %q", got, want)
	}
}

func TestRenderStatusSummaryNonInteractive(t *testing.T) {
	lastUpdated := time.Date(2026, time.February, 8, 12, 34, 56, 0, time.UTC)
	stats := &store.IndexStats{
		TotalFiles:  42,
		TotalChunks: 314,
		IndexSize:   2048,
		LastUpdated: lastUpdated,
	}
	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			Provider: "openai",
			Model:    "text-embedding-3-small",
		},
	}
	rpgStats := &rpg.GraphStats{
		TotalNodes: 10,
		TotalEdges: 20,
		EdgesByType: map[rpg.EdgeType]int{
			rpg.EdgeInvokes:  5,
			rpg.EdgeContains: 15,
		},
	}

	got := renderStatusSummary(stats, cfg, rpgStats)

	required := []string{
		"grepai index status",
		"Files indexed:    42",
		"Total chunks:     314",
		"Index size:       2.0 KB",
		"Last updated:     2026-02-08 12:34:56",
		"Provider:         openai (text-embedding-3-small)",
		"RPG Graph:        10 nodes, 20 edges",
		"  Edge types:     contains: 15, invokes: 5",
	}

	for _, line := range required {
		if !strings.Contains(got, line) {
			t.Fatalf("renderStatusSummary() missing line %q\nfull output:\n%s", line, got)
		}
	}
}
