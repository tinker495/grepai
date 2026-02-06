package rpg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoanbernabeu/grepai/store"
	"github.com/yoanbernabeu/grepai/trace"
)

// RPGIndexer orchestrates building and maintaining the RPG graph.
// It connects the trace symbol store, vector store, and RPG graph.
type RPGIndexer struct {
	store       RPGStore
	extractor   FeatureExtractor
	hierarchy   *HierarchyBuilder
	evolver     *Evolver
	projectRoot string
	cfg         RPGIndexerConfig
}

// RPGIndexerConfig configures the RPG indexer behavior.
type RPGIndexerConfig struct {
	DriftThreshold    float64
	MaxTraversalDepth int
}

// NewRPGIndexer creates a new RPG indexer instance.
func NewRPGIndexer(rpgStore RPGStore, extractor FeatureExtractor, projectRoot string, cfg RPGIndexerConfig) *RPGIndexer {
	graph := rpgStore.GetGraph()
	hierarchy := NewHierarchyBuilder(graph, extractor)
	evolver := NewEvolver(graph, extractor, hierarchy, cfg.DriftThreshold)

	return &RPGIndexer{
		store:       rpgStore,
		extractor:   extractor,
		hierarchy:   hierarchy,
		evolver:     evolver,
		projectRoot: projectRoot,
		cfg:         cfg,
	}
}

// BuildFull performs a complete rebuild of the RPG graph from scratch.
func (idx *RPGIndexer) BuildFull(ctx context.Context, symbolStore trace.SymbolStore, vectorStore store.VectorStore) error {
	graph := idx.store.GetGraph()

	// Clear existing graph
	graph = NewGraph()

	// Get all documents from vector store to know which files to process
	docs, err := vectorStore.ListDocuments(ctx)
	if err != nil {
		return fmt.Errorf("failed to list documents: %w", err)
	}

	// Track which files we've seen
	filesProcessed := make(map[string]bool)

	// Step 1: Create file and symbol nodes from trace store
	// First, we need to get all symbols - we'll iterate through all files
	for _, filePath := range docs {
		if filesProcessed[filePath] {
			continue
		}
		filesProcessed[filePath] = true

		// Create file node
		fileNodeID := MakeNodeID(KindFile, filePath)
		fileNode := &Node{
			ID:        fileNodeID,
			Kind:      KindFile,
			Path:      filePath,
			UpdatedAt: time.Now(),
		}
		graph.AddNode(fileNode)

		// Get all symbols from this file
		// We'll need to query the symbol store - since SymbolStore doesn't have a ListSymbolsForFile,
		// we'll get all chunks for the file and extract symbols that overlap
		chunks, err := vectorStore.GetChunksForFile(ctx, filePath)
		if err != nil {
			return fmt.Errorf("failed to get chunks for file %s: %w", filePath, err)
		}

		// Since SymbolStore doesn't provide a ListSymbolsForFile method,
		// we'll skip symbol extraction during BuildFull
		// Symbols will be populated incrementally through HandleFileEvent
		// when the watcher processes files with actual trace data

		// Store chunks for later linking (after symbols are added)
		_ = chunks // Used in Step 4 below
	}

	// Step 2: Wire invocation edges from call graph
	// Note: Since SymbolStore doesn't provide a way to list all symbols,
	// we rely on incremental updates through HandleFileEvent to populate the graph
	// The evolver will wire up call edges when symbols are added

	// Step 3: Wire import edges from references
	// We'll analyze references to detect imports
	// For now, we'll skip this as the SymbolStore interface doesn't expose references directly
	// This can be enhanced later

	// Step 4: Link vector chunks to symbols
	for _, filePath := range docs {
		chunks, err := vectorStore.GetChunksForFile(ctx, filePath)
		if err != nil {
			continue
		}

		if err := idx.linkChunksToSymbols(graph, filePath, chunks); err != nil {
			return fmt.Errorf("failed to link chunks for file %s: %w", filePath, err)
		}
	}

	// Step 5: Build hierarchy
	idx.hierarchy.BuildHierarchy()

	// Step 6: Persist
	if err := idx.store.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist RPG store: %w", err)
	}

	return nil
}

// HandleFileEvent handles incremental updates for file events.
func (idx *RPGIndexer) HandleFileEvent(ctx context.Context, eventType string, filePath string, symbols []trace.Symbol) error {
	switch strings.ToLower(eventType) {
	case "create", "modify":
		idx.evolver.HandleModify(filePath, symbols)
	case "delete":
		idx.evolver.HandleDelete(filePath)
	default:
		return fmt.Errorf("unknown event type: %s", eventType)
	}

	// Persist after update
	if err := idx.store.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist RPG store: %w", err)
	}

	return nil
}

// LinkChunksForFile links vector chunks to overlapping symbols in the graph.
func (idx *RPGIndexer) LinkChunksForFile(ctx context.Context, filePath string, chunks []store.Chunk) error {
	graph := idx.store.GetGraph()

	if err := idx.linkChunksToSymbols(graph, filePath, chunks); err != nil {
		return fmt.Errorf("failed to link chunks: %w", err)
	}

	// Persist after linking
	if err := idx.store.Persist(ctx); err != nil {
		return fmt.Errorf("failed to persist RPG store: %w", err)
	}

	return nil
}

// GetGraph returns the underlying graph.
func (idx *RPGIndexer) GetGraph() *Graph {
	return idx.store.GetGraph()
}

// GetEvolver returns the evolver for direct use.
func (idx *RPGIndexer) GetEvolver() *Evolver {
	return idx.evolver
}

// linkChunksToSymbols creates EdgeMapsToChunk edges for overlapping chunks and symbols.
func (idx *RPGIndexer) linkChunksToSymbols(graph *Graph, filePath string, chunks []store.Chunk) error {
	// Get all symbol nodes for this file
	fileNodes := graph.GetNodesByFile(filePath)
	var symbolNodes []*Node
	for _, node := range fileNodes {
		if node.Kind == KindSymbol {
			symbolNodes = append(symbolNodes, node)
		}
	}

	// For each chunk, find overlapping symbols
	for _, chunk := range chunks {
		// Create chunk node if it doesn't exist
		chunkNodeID := MakeNodeID(KindChunk, chunk.ID)
		chunkNode := graph.GetNode(chunkNodeID)
		if chunkNode == nil {
			chunkNode = &Node{
				ID:        chunkNodeID,
				Kind:      KindChunk,
				Path:      chunk.FilePath,
				StartLine: chunk.StartLine,
				EndLine:   chunk.EndLine,
				ChunkID:   chunk.ID,
				UpdatedAt: time.Now(),
			}
			graph.AddNode(chunkNode)
		}

		// Find symbols that overlap with this chunk
		for _, symbolNode := range symbolNodes {
			// Check if chunk and symbol line ranges overlap
			if overlaps(chunk.StartLine, chunk.EndLine, symbolNode.StartLine, symbolNode.EndLine) {
				// Create EdgeMapsToChunk from symbol to chunk
				edge := &Edge{
					From:      symbolNode.ID,
					To:        chunkNodeID,
					Type:      EdgeMapsToChunk,
					Weight:    1.0,
					UpdatedAt: time.Now(),
				}
				graph.AddEdge(edge)
			}
		}
	}

	return nil
}

// findSymbolNodeID finds the node ID for a symbol by name, file, and line.
func findSymbolNodeID(graph *Graph, symbolName, filePath string, line int) string {
	// Get all nodes for this file
	nodes := graph.GetNodesByFile(filePath)

	// Find symbol node with matching name and line
	for _, node := range nodes {
		if node.Kind == KindSymbol && node.SymbolName == symbolName {
			// Check if line is within symbol's range
			if line >= node.StartLine && line <= node.EndLine {
				return node.ID
			}
		}
	}

	return ""
}

// overlaps checks if two line ranges overlap.
func overlaps(start1, end1, start2, end2 int) bool {
	return start1 <= end2 && start2 <= end1
}
