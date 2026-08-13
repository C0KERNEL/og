package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	gopengraph "github.com/TheManticoreProject/gopengraph"
	"github.com/TheManticoreProject/gopengraph/edge"
	"github.com/TheManticoreProject/gopengraph/node"
	"github.com/TheManticoreProject/gopengraph/properties"
)

// graphify.go adds a graphify (github.com/Graphify-Labs/graphify) input mode to
// og: it reads a graphify knowledge-graph `graph.json` and maps its nodes and
// edges into the same BloodHound OpenGraph structure the CSV path produces.
//
// graphify writes two graph.json variants (a clustered node-link writer that
// stores edges under "links", and a raw writer that uses "edges") and tolerates
// several field-name spellings; the parser here normalizes across them so either
// export works without flags.

// graphifyNode is a schema-normalized graphify node.
type graphifyNode struct {
	ID         string
	Label      string
	SourceFile string
	Community  string
	NodeType   string
	FileType   string
	Extra      map[string]interface{} // passthrough scalar fields
}

// graphifyEdge is a schema-normalized graphify edge.
type graphifyEdge struct {
	Source          string
	Target          string
	Relation        string
	Confidence      string
	ConfidenceScore float64
	HasScore        bool
	Extra           map[string]interface{} // passthrough scalar fields
}

// graphifyData is the normalized result of parsing a graph.json.
type graphifyData struct {
	Nodes []graphifyNode
	Edges []graphifyEdge
}

// nodeReserved / edgeReserved are keys consumed into typed fields; everything
// else scalar becomes a passthrough property.
var nodeReserved = map[string]bool{
	"id": true, "node_id": true, "key": true, "uid": true, "name": true,
	"qualified_name": true, "fqname": true, "symbol": true,
	"label": true, "display_name": true, "title": true,
	"source_file": true, "file": true, "file_path": true, "filepath": true,
	"path": true, "module_path": true, "defined_in": true,
	"community": true, "community_id": true, "cluster": true, "cluster_id": true,
	"group": true, "group_id": true, "modularity_class": true,
	"node_type": true, "kind": true, "type": true, "category": true,
	"file_type": true, "content_type": true, "artifact_type": true,
}

var edgeReserved = map[string]bool{
	"source": true, "src": true, "from": true, "from_id": true, "start": true, "u": true,
	"target": true, "dst": true, "to": true, "to_id": true, "end": true, "v": true,
	"relation": true, "type": true, "kind": true, "label": true, "predicate": true,
	"confidence": true, "evidence": true, "provenance": true,
	"confidence_score": true, "score": true, "weight": true, "probability": true,
	"id": true, "edge_id": true,
}

// parseGraphifyJSON normalizes a graphify graph.json (bytes) into typed nodes
// and edges. It is deliberately free of any gopengraph dependency so the
// normalization can be unit-tested on its own.
func parseGraphifyJSON(data []byte) (*graphifyData, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var wrapper map[string]interface{}
	if g, ok := root["graph"].(map[string]interface{}); ok {
		wrapper = g
	}

	rawNodes := firstList(root["nodes"], root["vertices"], mget(wrapper, "nodes"), mget(wrapper, "vertices"))
	rawEdges := firstList(root["links"], root["edges"], mget(wrapper, "links"), mget(wrapper, "edges"))

	out := &graphifyData{}
	seen := map[string]bool{}

	for i, raw := range rawNodes {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		n := normalizeGraphifyNode(m, i)
		if n.ID == "" || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		out.Nodes = append(out.Nodes, n)
	}

	for i, raw := range rawEdges {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		e, ok := normalizeGraphifyEdge(m, i)
		if !ok {
			continue
		}
		out.Edges = append(out.Edges, e)
	}

	return out, nil
}

func normalizeGraphifyNode(m map[string]interface{}, index int) graphifyNode {
	id := firstString(m, "id", "node_id", "key", "uid", "name", "qualified_name", "fqname", "symbol")
	if id == "" {
		id = fmt.Sprintf("node_%d", index+1)
	}
	source := firstString(m, "source_file", "file", "file_path", "filepath", "path", "module_path", "defined_in")
	label := firstString(m, "label", "display_name", "title", "name", "qualified_name", "fqname", "symbol")
	if label == "" {
		label = id
	}
	community := firstString(m, "community", "community_id", "cluster", "cluster_id", "group", "group_id", "modularity_class")
	if community == "" {
		community = "unknown"
	}
	nodeType := firstString(m, "node_type", "kind", "type", "category")
	fileType := firstString(m, "file_type", "content_type", "artifact_type")
	if fileType == "" {
		fileType = "code"
		lower := strings.ToLower(source)
		for _, ext := range []string{".md", ".mdx", ".rst", ".txt"} {
			if strings.HasSuffix(lower, ext) {
				fileType = "document"
				break
			}
		}
	}

	return graphifyNode{
		ID:         id,
		Label:      label,
		SourceFile: source,
		Community:  community,
		NodeType:   nodeType,
		FileType:   fileType,
		Extra:      passthroughScalars(m, nodeReserved),
	}
}

func normalizeGraphifyEdge(m map[string]interface{}, index int) (graphifyEdge, bool) {
	source := endpointID(firstValue(m, "source", "src", "from", "from_id", "start", "u"))
	target := endpointID(firstValue(m, "target", "dst", "to", "to_id", "end", "v"))
	if source == "" || target == "" {
		return graphifyEdge{}, false
	}
	relation := firstString(m, "relation", "type", "kind", "label", "predicate")
	if relation == "" {
		relation = "relates"
	}
	confidence := firstString(m, "confidence", "evidence", "provenance")
	if confidence == "" {
		confidence = "EXTRACTED"
	}
	score, hasScore := firstFloat(m, "confidence_score", "score", "weight", "probability")

	return graphifyEdge{
		Source:          source,
		Target:          target,
		Relation:        strings.ToLower(relation),
		Confidence:      strings.ToUpper(confidence),
		ConfidenceScore: score,
		HasScore:        hasScore,
		Extra:           passthroughScalars(m, edgeReserved),
	}, true
}

// buildGraphify adds a parsed graphify graph to the OpenGraph, mirroring how the
// CSV path adds nodes then edges. The import-wide source_kind (from -s) is set on
// metadata by main (matching og's CSV path), not injected into node kinds — that
// keeps nodes at <=2 kinds so `og -g ... | cogs` round-trips cleanly. nodeIDs is
// shared with the CSV path for cross-reference warnings.
func buildGraphify(graph *gopengraph.OpenGraph, gd *graphifyData, nodeIDs map[string]bool) {
	for _, n := range gd.Nodes {
		kinds := graphifyNodeKinds(n)

		props := properties.NewProperties()
		props.SetProperty("name", strings.ToUpper(n.Label))
		props.SetProperty("displayname", humanizeLabel(n.Label, n.SourceFile))
		props.SetProperty("objectid", n.ID) // BloodHound dedup key
		if n.SourceFile != "" {
			props.SetProperty("source_file", n.SourceFile)
		}
		props.SetProperty("community", n.Community)
		if n.NodeType != "" {
			props.SetProperty("node_type", n.NodeType)
		}
		props.SetProperty("file_type", n.FileType)
		props.SetProperty("kind_heuristic", graphifyKind(n))
		for k, v := range n.Extra {
			props.SetProperty(k, v)
		}

		nodeObj, err := node.NewNode(n.ID, kinds, props)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error creating graphify node %s: %v\n", n.ID, err)
			continue
		}
		graph.AddNode(nodeObj)
		nodeIDs[n.ID] = true
	}

	for _, e := range gd.Edges {
		if !nodeIDs[e.Source] {
			fmt.Fprintf(os.Stderr, "Warning: graphify edge references non-existent start node '%s'\n", e.Source)
		}
		if !nodeIDs[e.Target] {
			fmt.Fprintf(os.Stderr, "Warning: graphify edge references non-existent end node '%s'\n", e.Target)
		}

		props := properties.NewProperties()
		props.SetProperty("relation", e.Relation)
		props.SetProperty("confidence", e.Confidence)
		if e.HasScore {
			props.SetProperty("confidence_score", e.ConfidenceScore)
		}
		props.SetProperty("include", edgeIncluded(e))
		for k, v := range e.Extra {
			props.SetProperty(k, v)
		}

		edgeObj, err := edge.NewEdge(e.Source, e.Target, pascalCase(e.Relation, "Relates"), props)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error creating graphify edge %s->%s: %v\n", e.Source, e.Target, err)
			continue
		}
		graph.AddEdge(edgeObj)
	}
}

// processGraphifyFile reads one graphify graph.json (path, or "-" for stdin) and
// adds it to the graph.
func processGraphifyFile(graph *gopengraph.OpenGraph, path string, nodeIDs map[string]bool) error {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return err
	}
	gd, err := parseGraphifyJSON(data)
	if err != nil {
		return err
	}
	if len(gd.Nodes) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: graphify file %s contained 0 nodes\n", path)
	}
	buildGraphify(graph, gd, nodeIDs)
	return nil
}

// ── classification helpers ─────────────────────────────────────────────

var bhKind = map[string]string{
	"entry": "Entrypoint", "api": "Endpoint", "async": "AsyncFunction",
	"klass": "Class", "ui": "Component", "module": "Module",
	"test": "Test", "concept": "Concept", "function": "Function",
}

// graphifyNodeKinds is the OpenGraph "kinds" list: a single classified kind
// (Function, Class, Endpoint, ...). gopengraph appends the import-wide
// source_kind (from -s) to every node on export, so emitting exactly one kind
// here keeps nodes at <=2 kinds — within the cap that `cogs` enforces — letting
// `og -g ... -s X | cogs` round-trip. It also mirrors og's CSV path, where
// source_kind occupies the extra slot rather than being authored into kinds.
func graphifyNodeKinds(n graphifyNode) []string {
	return []string{bhKind[graphifyKind(n)]}
}

// graphifyKind ports graphify's node classification heuristic (node_type first,
// then label/source/file-type signals) to a compact kind string.
func graphifyKind(n graphifyNode) string {
	label := strings.ToLower(n.Label)
	source := strings.ToLower(n.SourceFile)
	ft := strings.ToLower(n.FileType)
	nt := strings.ToLower(n.NodeType)

	switch nt {
	case "class", "klass", "struct", "interface", "enum", "trait", "model":
		return "klass"
	case "module", "file", "package", "namespace":
		return "module"
	case "endpoint", "route", "api", "handler", "controller":
		return "api"
	case "test", "spec":
		return "test"
	case "component", "hook", "view", "page":
		return "ui"
	}
	if ft == "rationale" || ft == "document" {
		return "concept"
	}
	if strings.Contains(source, "test") || strings.HasPrefix(label, "test_") || strings.Contains(source, "spec") {
		return "test"
	}
	if containsAny(label, "endpoint", "router", "api", "route") {
		return "api"
	}
	if containsAny(label, "cli", "command", "click", "typer") {
		return "entry"
	}
	if containsAny(label, "async", "await", "stream", "sse") {
		return "async"
	}
	hookLike := strings.HasPrefix(n.Label, "use") && len(n.Label) > 3 &&
		(n.Label[3] >= 'A' && n.Label[3] <= 'Z' || n.Label[3] == '_' || n.Label[3] == '-')
	if containsAny(label, "component", "props", "hook", "store") || hookLike ||
		hasSuffixAny(source, ".tsx", ".jsx", ".vue", ".svelte") {
		return "ui"
	}
	if len(n.Label) > 0 && n.Label[0] >= 'A' && n.Label[0] <= 'Z' && !strings.HasSuffix(n.Label, "()") {
		return "klass"
	}
	if hasSuffixAny(n.Label, ".py", ".ts", ".tsx", ".js", ".jsx", ".go", ".rs",
		".java", ".kt", ".rb", ".php", ".cs", ".swift", ".vue", ".svelte") {
		return "module"
	}
	return "function"
}

// edgeIncluded mirrors graphify's HTML-export confidence filter: EXTRACTED, or
// INFERRED with score >= 0.85.
func edgeIncluded(e graphifyEdge) bool {
	if e.Confidence == "EXTRACTED" {
		return true
	}
	if e.Confidence == "INFERRED" {
		score := e.ConfidenceScore
		if !e.HasScore {
			score = 1.0
		}
		return score >= 0.85
	}
	return false
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]+`)

// pascalCase turns a relation like "imports_from" into "ImportsFrom" for use as
// a BloodHound edge kind. Falls back to fallback when empty.
func pascalCase(s, fallback string) string {
	parts := nonAlnum.Split(s, -1)
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	if b.Len() == 0 {
		return fallback
	}
	return b.String()
}

// humanizeLabel produces a short, human-scannable display label.
func humanizeLabel(label, sourceFile string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		if sourceFile != "" {
			return baseName(sourceFile)
		}
		return "Unknown"
	}
	if strings.HasPrefix(label, ".") && strings.HasSuffix(label, "()") {
		return label[1:]
	}
	if hasSuffixAny(label, ".py", ".ts", ".tsx", ".js", ".jsx", ".go", ".rs", ".java", ".rb") {
		return baseName(label)
	}
	return label
}

// ── generic map/scalar helpers ─────────────────────────────────────────

func mget(m map[string]interface{}, key string) interface{} {
	if m == nil {
		return nil
	}
	return m[key]
}

// firstList returns the first value that is a JSON array.
func firstList(values ...interface{}) []interface{} {
	for _, v := range values {
		if arr, ok := v.([]interface{}); ok {
			return arr
		}
	}
	return nil
}

// firstValue returns the first present, non-empty raw value for any key.
func firstValue(m map[string]interface{}, keys ...string) interface{} {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && v != "" {
			return v
		}
	}
	return nil
}

// firstString returns the first key's value rendered as a non-empty string.
func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := scalarString(v); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// firstFloat returns the first key parseable as a float (numbers and numeric
// strings), plus whether one was found.
func firstFloat(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t, true
		case json.Number:
			if f, err := t.Float64(); err == nil {
				return f, true
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

// endpointID normalizes an edge endpoint that may be a string or a node object.
func endpointID(v interface{}) string {
	if m, ok := v.(map[string]interface{}); ok {
		return firstString(m, "id", "node_id", "key", "name", "qualified_name")
	}
	s, _ := scalarString(v)
	return s
}

// scalarString renders a scalar JSON value as a string; returns ok=false for
// objects, arrays, and nil (so they are skipped rather than stringified).
func scalarString(v interface{}) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case float64:
		return formatFloat(t), true
	case json.Number:
		return t.String(), true
	case nil:
		return "", false
	default:
		return "", false
	}
}

// passthroughScalars collects scalar fields not already consumed into typed
// fields, preserving their native type (numbers/bools stay numbers/bools).
func passthroughScalars(m map[string]interface{}, reserved map[string]bool) map[string]interface{} {
	var out map[string]interface{}
	for k, v := range m {
		if reserved[k] {
			continue
		}
		switch v.(type) {
		case string, bool, float64, json.Number:
			if out == nil {
				out = map[string]interface{}{}
			}
			out[k] = v
		}
	}
	return out
}

func formatFloat(f float64) string {
	if f == math.Trunc(f) && !math.IsInf(f, 0) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func hasSuffixAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.HasSuffix(s, sub) {
			return true
		}
	}
	return false
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
