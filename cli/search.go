package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/alpkeskin/gotoon"
	"github.com/spf13/cobra"
	"github.com/yoanbernabeu/grepai/config"
	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/rpg"
	"github.com/yoanbernabeu/grepai/search"
	"github.com/yoanbernabeu/grepai/store"
)

var (
	searchLimit              int
	searchJSON               bool
	searchTOON               bool
	searchCompact            bool
	searchWorkspace          string
	searchProjects           []string
	searchDistanceFrom       string
	searchDistanceFromSymbol string
	searchDistanceFromFile   string
	searchDistanceMax        float64
	searchDistanceMetric     string
	searchDistanceEdgeTypes  string
	searchDistanceDirection  string
	searchSort               string
	searchDistanceWeight     float64
)

// SearchResultJSON is a lightweight struct for JSON output (excludes vector, hash, updated_at)
type SearchResultJSON struct {
	FilePath    string   `json:"file_path"`
	StartLine   int      `json:"start_line"`
	EndLine     int      `json:"end_line"`
	Score       float32  `json:"score"`
	Content     string   `json:"content"`
	FeaturePath string   `json:"feature_path,omitempty"`
	SymbolName  string   `json:"symbol_name,omitempty"`
	Distance    *float64 `json:"distance,omitempty"`
}

// SearchResultCompactJSON is a minimal struct for compact JSON output (no content field)
type SearchResultCompactJSON struct {
	FilePath    string   `json:"file_path"`
	StartLine   int      `json:"start_line"`
	EndLine     int      `json:"end_line"`
	Score       float32  `json:"score"`
	FeaturePath string   `json:"feature_path,omitempty"`
	SymbolName  string   `json:"symbol_name,omitempty"`
	Distance    *float64 `json:"distance,omitempty"`
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search codebase with natural language",
	Long: `Search your codebase using natural language queries.

The search will:
- Vectorize your query using the configured embedding provider
- Calculate cosine similarity against indexed code chunks
- Return the most relevant results with file path, line numbers, and score`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 10, "Maximum number of results to return")
	searchCmd.Flags().BoolVarP(&searchJSON, "json", "j", false, "Output results in JSON format (for AI agents)")
	searchCmd.Flags().BoolVarP(&searchTOON, "toon", "t", false, "Output results in TOON format (token-efficient for AI agents)")
	searchCmd.Flags().BoolVarP(&searchCompact, "compact", "c", false, "Output minimal format without content (requires --json or --toon)")
	searchCmd.Flags().StringVar(&searchWorkspace, "workspace", "", "Workspace name for cross-project search")
	searchCmd.Flags().StringArrayVar(&searchProjects, "project", nil, "Project name(s) to search (requires --workspace, can be repeated)")
	searchCmd.Flags().StringVar(&searchDistanceFrom, "distance-from", "", "RPG node ID to compute distance from each result (requires RPG enabled)")
	searchCmd.Flags().StringVar(&searchDistanceFromSymbol, "distance-from-symbol", "", "Symbol name to resolve to RPG node ID for distance computation")
	searchCmd.Flags().StringVar(&searchDistanceFromFile, "distance-from-file", "", "File path to use as RPG node ID for distance computation")
	searchCmd.Flags().Float64Var(&searchDistanceMax, "distance-max", 0, "Maximum distance to include (0 = no limit)")
	searchCmd.Flags().StringVar(&searchDistanceMetric, "distance-metric", "cost", "Distance metric: cost (1/weight) or hops (uniform 1.0)")
	searchCmd.Flags().StringVar(&searchDistanceEdgeTypes, "distance-edge-types", "", "Comma-separated edge types for distance: feature_parent,contains,invokes,imports,maps_to_chunk,semantic_sim")
	searchCmd.Flags().StringVar(&searchDistanceDirection, "distance-direction", "both", "Distance traversal direction: forward, reverse, or both")
	searchCmd.Flags().StringVar(&searchSort, "sort", "score", "Sort results by: score, distance, or combined")
	searchCmd.Flags().Float64Var(&searchDistanceWeight, "distance-weight", 0.0, "Alpha for combined sort: combined = score / (1 + alpha * distance)")
	searchCmd.MarkFlagsMutuallyExclusive("json", "toon")
	searchCmd.MarkFlagsMutuallyExclusive("distance-from", "distance-from-symbol", "distance-from-file")
}

// rpgEnrichment holds RPG context for a search result
type rpgEnrichment struct {
	FeaturePath string
	SymbolName  string
	Distance    *float64 // nil = not measured, negative = unreachable
}

// distanceOpts bundles all distance-related parameters for enrichWithRPG.
type distanceOpts struct {
	From       string
	FromSymbol string
	FromFile   string
	Max        float64
	Metric     string // "cost" or "hops"
	EdgeTypes  string // comma-separated edge types
	Direction  string // "forward", "reverse", or "both"
}

// resolveDistanceFrom resolves --distance-from, --distance-from-symbol, or --distance-from-file
// to a single RPG node ID. Returns "" if no distance flag is set.
func resolveDistanceFrom(qe *rpg.QueryEngine, distanceFrom, distanceFromSymbol, distanceFromFile string) (string, error) {
	if distanceFrom != "" {
		return distanceFrom, nil
	}
	if distanceFromSymbol != "" {
		matches, err := qe.ResolveSymbol(distanceFromSymbol)
		if err != nil {
			return "", err
		}
		if len(matches) > 1 {
			var candidates []string
			for _, m := range matches {
				candidates = append(candidates, fmt.Sprintf("  %s (%s)", m.ID, m.Path))
			}
			return "", fmt.Errorf("ambiguous symbol %q, %d candidates:\n%s", distanceFromSymbol, len(matches), strings.Join(candidates, "\n"))
		}
		return matches[0].ID, nil
	}
	if distanceFromFile != "" {
		return "file:" + distanceFromFile, nil
	}
	return "", nil
}

// enrichWithRPG enriches search results with RPG feature paths, symbol names, and optional distance.
// When distanceFrom is set, RPG load failures become hard errors instead of best-effort.
func enrichWithRPG(projectRoot string, cfg *config.Config, results []store.SearchResult, distanceFrom, distanceFromSymbol, distanceFromFile string, distanceMax float64, opts ...distanceOpts) ([]rpgEnrichment, error) {
	enrichments := make([]rpgEnrichment, len(results))
	wantDistance := distanceFrom != "" || distanceFromSymbol != "" || distanceFromFile != ""

	// Extract optional distance opts
	var dOpts distanceOpts
	if len(opts) > 0 {
		dOpts = opts[0]
	}

	if !cfg.RPG.Enabled {
		if wantDistance {
			return nil, fmt.Errorf("distance flags require RPG to be enabled")
		}
		return enrichments, nil
	}

	ctx := context.Background()
	rpgStore := rpg.NewGOBRPGStore(config.GetRPGIndexPath(projectRoot))
	if err := rpgStore.Load(ctx); err != nil {
		if wantDistance {
			return nil, fmt.Errorf("failed to load RPG index for distance computation: %w", err)
		}
		return enrichments, nil
	}
	defer rpgStore.Close()

	graph := rpgStore.GetGraph()
	if wantDistance && graph.Stats().TotalNodes == 0 {
		return nil, fmt.Errorf("RPG index is empty; cannot compute distances. Run 'grepai watch' first")
	}

	qe := rpg.NewQueryEngine(graph)

	// Resolve the source node ID
	resolvedSource, err := resolveDistanceFrom(qe, distanceFrom, distanceFromSymbol, distanceFromFile)
	if err != nil {
		return nil, err
	}

	// Validate source node exists when distance is requested
	if resolvedSource != "" && graph.GetNode(resolvedSource) == nil {
		return nil, fmt.Errorf("distance-from node not found in RPG graph: %s", resolvedSource)
	}

	// Pre-fill distance to -1 when distance is requested
	if resolvedSource != "" {
		for i := range enrichments {
			d := -1.0
			enrichments[i].Distance = &d
		}
	}

	// First pass: find target node IDs for each result
	type resultTarget struct {
		nodeID string
		node   *rpg.Node
	}
	targets := make([]resultTarget, len(results))

	for i, r := range results {
		nodes := graph.GetNodesByFile(r.Chunk.FilePath)
		for _, n := range nodes {
			if n.Kind == rpg.KindSymbol && n.StartLine <= r.Chunk.EndLine && r.Chunk.StartLine <= n.EndLine {
				fetchResult, fetchErr := qe.FetchNode(ctx, rpg.FetchNodeRequest{NodeID: n.ID})
				if fetchErr == nil && fetchResult != nil {
					enrichments[i].FeaturePath = fetchResult.FeaturePath
					enrichments[i].SymbolName = n.SymbolName
				}
				targets[i] = resultTarget{nodeID: n.ID, node: n}
				break
			}
		}
	}

	// Second pass: single DistancesFrom call for all targets
	if resolvedSource != "" {
		distReq := rpg.DistancesFromRequest{
			SourceID:  resolvedSource,
			MaxCost:   distanceMax,
			Direction: rpg.DijkstraDirection(dOpts.Direction),
			Metric:    rpg.DistanceMetric(dOpts.Metric),
		}
		if dOpts.EdgeTypes != "" {
			for _, et := range strings.Split(dOpts.EdgeTypes, ",") {
				et = strings.TrimSpace(et)
				if et != "" {
					distReq.EdgeTypes = append(distReq.EdgeTypes, rpg.EdgeType(et))
				}
			}
		}
		distResult, distErr := qe.DistancesFrom(ctx, distReq)
		if distErr == nil && distResult != nil {
			for i := range results {
				if targets[i].nodeID == "" {
					continue
				}
				if d, ok := distResult.Distances[targets[i].nodeID]; ok {
					enrichments[i].Distance = &d
				}
			}
		}
	}

	return enrichments, nil
}

// combinedScore computes score / (1 + alpha * distance).
// Returns 0 for unreachable results (distance < 0 or nil).
func combinedScore(score float32, distance *float64, alpha float64) float64 {
	if distance == nil || *distance < 0 {
		return 0
	}
	return float64(score) / (1.0 + alpha*(*distance))
}

// sortResults re-orders results and enrichments in place by the given sort mode.
func sortResults(results []store.SearchResult, enrichments []rpgEnrichment, sortMode string, alpha float64) {
	// Build index slice for co-sorting
	indices := make([]int, len(results))
	for i := range indices {
		indices[i] = i
	}

	sort.SliceStable(indices, func(a, b int) bool {
		ia, ib := indices[a], indices[b]
		switch sortMode {
		case "distance":
			da := enrichments[ia].Distance
			db := enrichments[ib].Distance
			// nil/unreachable goes last
			if da == nil || *da < 0 {
				return false
			}
			if db == nil || *db < 0 {
				return true
			}
			return *da < *db
		case "combined":
			ca := combinedScore(results[ia].Score, enrichments[ia].Distance, alpha)
			cb := combinedScore(results[ib].Score, enrichments[ib].Distance, alpha)
			return ca > cb // descending
		default: // "score" - already sorted by score
			return results[ia].Score > results[ib].Score
		}
	})

	// Apply permutation
	sortedResults := make([]store.SearchResult, len(results))
	sortedEnrichments := make([]rpgEnrichment, len(enrichments))
	for i, idx := range indices {
		sortedResults[i] = results[idx]
		sortedEnrichments[i] = enrichments[idx]
	}
	copy(results, sortedResults)
	copy(enrichments, sortedEnrichments)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	// Validate flag combination
	if searchCompact && !searchJSON && !searchTOON {
		return fmt.Errorf("--compact flag requires --json or --toon flag")
	}

	// Validate workspace-related flags
	if len(searchProjects) > 0 && searchWorkspace == "" {
		return fmt.Errorf("--project flag requires --workspace flag")
	}

	// Validate distance-from flags
	wantDistance := searchDistanceFrom != "" || searchDistanceFromSymbol != "" || searchDistanceFromFile != ""
	if wantDistance && searchWorkspace != "" {
		return fmt.Errorf("distance flags are not supported with --workspace (RPG is per-project)")
	}

	// Workspace mode
	if searchWorkspace != "" {
		return runWorkspaceSearch(ctx, query)
	}

	// Find project root
	projectRoot, err := config.FindProjectRoot()
	if err != nil {
		return err
	}

	// Load configuration
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate distance flags require RPG
	if wantDistance && !cfg.RPG.Enabled {
		return fmt.Errorf("distance flags require RPG to be enabled (set rpg.enabled: true in .grepai/config.yaml)")
	}

	// Initialize embedder
	var emb embedder.Embedder
	switch cfg.Embedder.Provider {
	case "ollama":
		opts := []embedder.OllamaOption{
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOllamaDimensions(*cfg.Embedder.Dimensions))
		}
		emb = embedder.NewOllamaEmbedder(opts...)
	case "openai":
		opts := []embedder.OpenAIOption{
			embedder.WithOpenAIModel(cfg.Embedder.Model),
			embedder.WithOpenAIKey(cfg.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(cfg.Embedder.Endpoint),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOpenAIDimensions(*cfg.Embedder.Dimensions))
		}
		var err error
		emb, err = embedder.NewOpenAIEmbedder(opts...)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
	case "lmstudio":
		opts := []embedder.LMStudioOption{
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
		}
		if cfg.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithLMStudioDimensions(*cfg.Embedder.Dimensions))
		}
		emb = embedder.NewLMStudioEmbedder(opts...)
	default:
		return fmt.Errorf("unknown embedding provider: %s", cfg.Embedder.Provider)
	}
	defer emb.Close()

	// Initialize store
	var st store.VectorStore
	switch cfg.Store.Backend {
	case "gob":
		indexPath := config.GetIndexPath(projectRoot)
		gobStore := store.NewGOBStore(indexPath)
		if err := gobStore.Load(ctx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}
		st = gobStore
	case "postgres":
		var err error
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
	case "qdrant":
		collectionName := cfg.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = store.SanitizeCollectionName(projectRoot)
		}
		var err error
		st, err = store.NewQdrantStore(ctx, cfg.Store.Qdrant.Endpoint, cfg.Store.Qdrant.Port, cfg.Store.Qdrant.UseTLS, collectionName, cfg.Store.Qdrant.APIKey, cfg.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to qdrant: %w", err)
		}
	default:
		return fmt.Errorf("unknown storage backend: %s", cfg.Store.Backend)
	}
	defer st.Close()

	// Create searcher with boost config
	searcher := search.NewSearcher(st, emb, cfg.Search)

	// Search with boosting
	results, err := searcher.Search(ctx, query, searchLimit)
	if err != nil {
		if searchJSON {
			return outputSearchErrorJSON(err)
		}
		if searchTOON {
			return outputSearchErrorTOON(err)
		}
		return fmt.Errorf("search failed: %w", err)
	}

	// Enrich results with RPG context
	enrichments, enrichErr := enrichWithRPG(projectRoot, cfg, results, searchDistanceFrom, searchDistanceFromSymbol, searchDistanceFromFile, searchDistanceMax, distanceOpts{
		Metric:    searchDistanceMetric,
		EdgeTypes: searchDistanceEdgeTypes,
		Direction: searchDistanceDirection,
	})
	if enrichErr != nil {
		return enrichErr
	}

	// Sort results if requested
	if searchSort != "" && searchSort != "score" {
		sortResults(results, enrichments, searchSort, searchDistanceWeight)
	}

	// JSON output mode
	if searchJSON {
		if searchCompact {
			return outputSearchCompactJSON(results, enrichments)
		}
		return outputSearchJSON(results, enrichments)
	}

	// TOON output mode
	if searchTOON {
		if searchCompact {
			return outputSearchCompactTOON(results, enrichments)
		}
		return outputSearchTOON(results, enrichments)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	// Display results
	fmt.Printf("Found %d results for: %q\n\n", len(results), query)

	for i, result := range results {
		fmt.Printf("─── Result %d (score: %.4f) ───\n", i+1, result.Score)
		fmt.Printf("File: %s:%d-%d\n", result.Chunk.FilePath, result.Chunk.StartLine, result.Chunk.EndLine)
		if enrichments[i].Distance != nil {
			d := *enrichments[i].Distance
			distLabel := searchDistanceFrom
			if distLabel == "" {
				distLabel = searchDistanceFromSymbol
			}
			if distLabel == "" {
				distLabel = searchDistanceFromFile
			}
			if d < 0 {
				fmt.Printf("Distance from %s: unreachable\n", distLabel)
			} else {
				fmt.Printf("Distance from %s: %.4f\n", distLabel, d)
			}
		}
		fmt.Println()

		// Display content with line numbers
		lines := strings.Split(result.Chunk.Content, "\n")
		// Skip the "File: xxx" prefix line if present
		startIdx := 0
		if len(lines) > 0 && strings.HasPrefix(lines[0], "File: ") {
			startIdx = 2 // Skip "File: xxx" and empty line
		}

		lineNum := result.Chunk.StartLine
		for j := startIdx; j < len(lines) && j < startIdx+15; j++ {
			fmt.Printf("%4d │ %s\n", lineNum, lines[j])
			lineNum++
		}
		if len(lines)-startIdx > 15 {
			fmt.Printf("     │ ... (%d more lines)\n", len(lines)-startIdx-15)
		}
		fmt.Println()
	}

	return nil
}

// outputSearchJSON outputs results in JSON format for AI agents
func outputSearchJSON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	jsonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			Content:     r.Chunk.Content,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
			Distance:    enrichments[i].Distance,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// outputSearchCompactJSON outputs results in minimal JSON format (without content)
func outputSearchCompactJSON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	jsonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		jsonResults[i] = SearchResultCompactJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
			Distance:    enrichments[i].Distance,
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// outputSearchErrorJSON outputs an error in JSON format
func outputSearchErrorJSON(err error) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(map[string]string{"error": err.Error()})
	return nil
}

// outputSearchTOON outputs results in TOON format for AI agents
func outputSearchTOON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	toonResults := make([]SearchResultJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			Content:     r.Chunk.Content,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
			Distance:    enrichments[i].Distance,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		return fmt.Errorf("failed to encode TOON: %w", err)
	}
	fmt.Println(output)
	return nil
}

// outputSearchCompactTOON outputs results in minimal TOON format (without content)
func outputSearchCompactTOON(results []store.SearchResult, enrichments []rpgEnrichment) error {
	toonResults := make([]SearchResultCompactJSON, len(results))
	for i, r := range results {
		toonResults[i] = SearchResultCompactJSON{
			FilePath:    r.Chunk.FilePath,
			StartLine:   r.Chunk.StartLine,
			EndLine:     r.Chunk.EndLine,
			Score:       r.Score,
			FeaturePath: enrichments[i].FeaturePath,
			SymbolName:  enrichments[i].SymbolName,
			Distance:    enrichments[i].Distance,
		}
	}

	output, err := gotoon.Encode(toonResults)
	if err != nil {
		return fmt.Errorf("failed to encode TOON: %w", err)
	}
	fmt.Println(output)
	return nil
}

// outputSearchErrorTOON outputs an error in TOON format
func outputSearchErrorTOON(err error) error {
	output, encErr := gotoon.Encode(map[string]string{"error": err.Error()})
	if encErr != nil {
		return fmt.Errorf("failed to encode TOON error: %w", encErr)
	}
	fmt.Println(output)
	return nil
}

// SearchJSON returns results in JSON format for AI agents
func SearchJSON(projectRoot string, query string, limit int) ([]store.SearchResult, error) {
	ctx := context.Background()

	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil, err
	}

	var emb embedder.Embedder
	switch cfg.Embedder.Provider {
	case "ollama":
		emb = embedder.NewOllamaEmbedder(
			embedder.WithOllamaEndpoint(cfg.Embedder.Endpoint),
			embedder.WithOllamaModel(cfg.Embedder.Model),
		)
	case "openai":
		var err error
		emb, err = embedder.NewOpenAIEmbedder(
			embedder.WithOpenAIModel(cfg.Embedder.Model),
		)
		if err != nil {
			return nil, err
		}
	case "lmstudio":
		emb = embedder.NewLMStudioEmbedder(
			embedder.WithLMStudioEndpoint(cfg.Embedder.Endpoint),
			embedder.WithLMStudioModel(cfg.Embedder.Model),
		)
	default:
		return nil, fmt.Errorf("unknown provider: %s", cfg.Embedder.Provider)
	}
	defer emb.Close()

	var st store.VectorStore
	switch cfg.Store.Backend {
	case "gob":
		gobStore := store.NewGOBStore(config.GetIndexPath(projectRoot))
		if err := gobStore.Load(ctx); err != nil {
			return nil, err
		}
		st = gobStore
	case "postgres":
		var err error
		st, err = store.NewPostgresStore(ctx, cfg.Store.Postgres.DSN, projectRoot, cfg.Embedder.GetDimensions())
		if err != nil {
			return nil, err
		}
	}
	defer st.Close()

	// Create searcher with boost config
	searcher := search.NewSearcher(st, emb, cfg.Search)

	return searcher.Search(ctx, query, limit)
}

func init() {
	// Ensure the search command is registered
	_ = os.Getenv("GREPAI_DEBUG")
}

// runWorkspaceSearch handles workspace-level search operations
func runWorkspaceSearch(ctx context.Context, query string) error {
	// Load workspace config
	wsCfg, err := config.LoadWorkspaceConfig()
	if err != nil {
		return fmt.Errorf("failed to load workspace config: %w", err)
	}
	if wsCfg == nil {
		return fmt.Errorf("no workspaces configured; create one with: grepai workspace create <name>")
	}

	ws, err := wsCfg.GetWorkspace(searchWorkspace)
	if err != nil {
		return err
	}

	// Validate backend
	if err := config.ValidateWorkspaceBackend(ws); err != nil {
		return err
	}

	// Initialize embedder
	var emb embedder.Embedder
	switch ws.Embedder.Provider {
	case "ollama":
		opts := []embedder.OllamaOption{
			embedder.WithOllamaEndpoint(ws.Embedder.Endpoint),
			embedder.WithOllamaModel(ws.Embedder.Model),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOllamaDimensions(*ws.Embedder.Dimensions))
		}
		emb = embedder.NewOllamaEmbedder(opts...)
	case "openai":
		opts := []embedder.OpenAIOption{
			embedder.WithOpenAIModel(ws.Embedder.Model),
			embedder.WithOpenAIKey(ws.Embedder.APIKey),
			embedder.WithOpenAIEndpoint(ws.Embedder.Endpoint),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithOpenAIDimensions(*ws.Embedder.Dimensions))
		}
		emb, err = embedder.NewOpenAIEmbedder(opts...)
		if err != nil {
			return fmt.Errorf("failed to initialize OpenAI embedder: %w", err)
		}
	case "lmstudio":
		opts := []embedder.LMStudioOption{
			embedder.WithLMStudioEndpoint(ws.Embedder.Endpoint),
			embedder.WithLMStudioModel(ws.Embedder.Model),
		}
		if ws.Embedder.Dimensions != nil {
			opts = append(opts, embedder.WithLMStudioDimensions(*ws.Embedder.Dimensions))
		}
		emb = embedder.NewLMStudioEmbedder(opts...)
	default:
		return fmt.Errorf("unknown embedding provider: %s", ws.Embedder.Provider)
	}
	defer emb.Close()

	// Initialize store
	var st store.VectorStore
	projectID := "workspace:" + ws.Name

	switch ws.Store.Backend {
	case "postgres":
		st, err = store.NewPostgresStore(ctx, ws.Store.Postgres.DSN, projectID, ws.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
	case "qdrant":
		collectionName := ws.Store.Qdrant.Collection
		if collectionName == "" {
			collectionName = "workspace_" + ws.Name
		}
		st, err = store.NewQdrantStore(ctx, ws.Store.Qdrant.Endpoint, ws.Store.Qdrant.Port, ws.Store.Qdrant.UseTLS, collectionName, ws.Store.Qdrant.APIKey, ws.Embedder.GetDimensions())
		if err != nil {
			return fmt.Errorf("failed to connect to qdrant: %w", err)
		}
	default:
		return fmt.Errorf("unsupported backend for workspace: %s", ws.Store.Backend)
	}
	defer st.Close()

	// Create searcher with default search config
	searchCfg := config.SearchConfig{
		Hybrid: config.HybridConfig{Enabled: false, K: 60},
		Boost:  config.DefaultConfig().Search.Boost,
	}
	searcher := search.NewSearcher(st, emb, searchCfg)

	// Search
	results, err := searcher.Search(ctx, query, searchLimit)
	if err != nil {
		if searchJSON {
			return outputSearchErrorJSON(err)
		}
		if searchTOON {
			return outputSearchErrorTOON(err)
		}
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by projects if specified
	// File paths are stored as: workspaceName/projectName/relativePath
	if len(searchProjects) > 0 {
		filteredResults := make([]store.SearchResult, 0)
		for _, r := range results {
			for _, projectName := range searchProjects {
				// Match workspace/project/ prefix
				expectedPrefix := ws.Name + "/" + projectName + "/"
				if strings.HasPrefix(r.Chunk.FilePath, expectedPrefix) {
					filteredResults = append(filteredResults, r)
					break
				}
			}
		}
		results = filteredResults
	}

	// Workspace mode doesn't have RPG enrichment (no single projectRoot)
	enrichments := make([]rpgEnrichment, len(results))

	// JSON output mode
	if searchJSON {
		if searchCompact {
			return outputSearchCompactJSON(results, enrichments)
		}
		return outputSearchJSON(results, enrichments)
	}

	// TOON output mode
	if searchTOON {
		if searchCompact {
			return outputSearchCompactTOON(results, enrichments)
		}
		return outputSearchTOON(results, enrichments)
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	// Display results
	fmt.Printf("Found %d results for: %q in workspace %q\n\n", len(results), query, searchWorkspace)

	for i, result := range results {
		fmt.Printf("─── Result %d (score: %.4f) ───\n", i+1, result.Score)
		fmt.Printf("File: %s:%d-%d\n", result.Chunk.FilePath, result.Chunk.StartLine, result.Chunk.EndLine)
		fmt.Println()

		// Display content with line numbers
		lines := strings.Split(result.Chunk.Content, "\n")
		startIdx := 0
		if len(lines) > 0 && strings.HasPrefix(lines[0], "File: ") {
			startIdx = 2
		}

		lineNum := result.Chunk.StartLine
		for j := startIdx; j < len(lines) && j < startIdx+15; j++ {
			fmt.Printf("%4d │ %s\n", lineNum, lines[j])
			lineNum++
		}
		if len(lines)-startIdx > 15 {
			fmt.Printf("     │ ... (%d more lines)\n", len(lines)-startIdx-15)
		}
		fmt.Println()
	}

	return nil
}
