package main

import (
	"encoding/json"
	"os"
	"testing"

	gopengraph "github.com/TheManticoreProject/gopengraph"
)

const linksSchema = `{
  "directed": false, "multigraph": false,
  "nodes": [
    {"id": "a", "label": "main()", "source_file": "app/cli.py", "community": 0},
    {"id": "b", "label": "GraphSchema", "source_file": "app/build.py", "community": 1, "node_type": "class"}
  ],
  "links": [
    {"source": "a", "target": "b", "relation": "calls", "confidence": "EXTRACTED"}
  ]
}`

func mustParse(t *testing.T, s string) *graphifyData {
	t.Helper()
	gd, err := parseGraphifyJSON([]byte(s))
	if err != nil {
		t.Fatalf("parseGraphifyJSON error: %v", err)
	}
	return gd
}

func TestParseGraphifyLinksSchema(t *testing.T) {
	gd := mustParse(t, linksSchema)
	if len(gd.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(gd.Nodes))
	}
	if len(gd.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(gd.Edges))
	}
	if gd.Edges[0].Source != "a" || gd.Edges[0].Target != "b" {
		t.Errorf("edge direction wrong: %+v", gd.Edges[0])
	}
	if gd.Edges[0].Relation != "calls" || gd.Edges[0].Confidence != "EXTRACTED" {
		t.Errorf("edge relation/confidence wrong: %+v", gd.Edges[0])
	}
}

func TestGraphifyColonIDsSanitized(t *testing.T) {
	// BloodHound does not support colons in OpenGraph object ids, and real
	// graphify output contains them (e.g. C# "csharp_namespace:<hash>").
	s := `{"nodes":[{"id":"csharp_namespace:abc123","label":"Ns"},{"id":"plain","label":"P"}],
	       "links":[{"source":"csharp_namespace:abc123","target":"plain","relation":"contains","confidence":"EXTRACTED"}]}`
	gd := mustParse(t, s)
	if gd.Nodes[0].ID != "csharp_namespace_abc123" {
		t.Errorf("node id = %q, want colon replaced with underscore", gd.Nodes[0].ID)
	}
	if gd.Edges[0].Source != "csharp_namespace_abc123" {
		t.Errorf("edge start = %q, want sanitized consistently with the node id", gd.Edges[0].Source)
	}
}

func TestGraphifyReservedPropertyNamesExcluded(t *testing.T) {
	// "objectid" and "ref" are reserved property names in BloodHound node
	// definitions; a payload defining them in properties fails ingest.
	s := `{"nodes":[{"id":"a","label":"A","objectid":"stale","ref":"nope","pagerank":0.5}],"links":[]}`
	gd := mustParse(t, s)
	n := gd.Nodes[0]
	if _, ok := n.Extra["objectid"]; ok {
		t.Error("objectid must not pass through to properties")
	}
	if _, ok := n.Extra["ref"]; ok {
		t.Error("ref must not pass through to properties")
	}
	if _, ok := n.Extra["pagerank"]; !ok {
		t.Error("non-reserved extra scalars should still pass through")
	}
}

func TestParseGraphifyEdgesSchema(t *testing.T) {
	// raw writer stores edges under "edges" instead of "links"
	s := `{"nodes":[{"id":"a"},{"id":"b"}],"edges":[{"source":"a","target":"b","relation":"uses","confidence":"EXTRACTED"}]}`
	gd := mustParse(t, s)
	if len(gd.Nodes) != 2 || len(gd.Edges) != 1 {
		t.Fatalf("nodes=%d edges=%d, want 2/1", len(gd.Nodes), len(gd.Edges))
	}
}

func TestParseGraphifyAlternateFields(t *testing.T) {
	// src/dst, name-as-id, cluster, weight; vertices instead of nodes
	s := `{
      "vertices": [
        {"name":"a","file":"x.py","cluster":"7"},
        {"name":"b","file":"y.py","cluster":"7"}
      ],
      "edges": [
        {"src":"a","dst":"b","type":"imports","evidence":"INFERRED","weight":0.95}
      ]
    }`
	gd := mustParse(t, s)
	if len(gd.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(gd.Nodes))
	}
	if gd.Nodes[0].ID != "a" || gd.Nodes[0].Community != "7" || gd.Nodes[0].SourceFile != "x.py" {
		t.Errorf("node normalization wrong: %+v", gd.Nodes[0])
	}
	if len(gd.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(gd.Edges))
	}
	e := gd.Edges[0]
	if e.Source != "a" || e.Target != "b" || e.Relation != "imports" || e.Confidence != "INFERRED" {
		t.Errorf("edge normalization wrong: %+v", e)
	}
	if !e.HasScore || e.ConfidenceScore != 0.95 {
		t.Errorf("edge score wrong: %+v", e)
	}
}

func TestParseGraphifyObjectEndpoints(t *testing.T) {
	// endpoints given as node-like objects rather than bare ids
	s := `{"nodes":[{"id":"a"},{"id":"b"}],
           "links":[{"source":{"id":"a"},"target":{"node_id":"b"},"relation":"calls"}]}`
	gd := mustParse(t, s)
	if len(gd.Edges) != 1 || gd.Edges[0].Source != "a" || gd.Edges[0].Target != "b" {
		t.Fatalf("object-endpoint edge not normalized: %+v", gd.Edges)
	}
}

func TestParseGraphifyGraphWrapper(t *testing.T) {
	s := `{"graph":{"nodes":[{"id":"a"},{"id":"b"}],"links":[{"source":"a","target":"b"}]}}`
	gd := mustParse(t, s)
	if len(gd.Nodes) != 2 || len(gd.Edges) != 1 {
		t.Fatalf("graph-wrapper not handled: nodes=%d edges=%d", len(gd.Nodes), len(gd.Edges))
	}
	// default relation/confidence applied
	if gd.Edges[0].Relation != "relates" || gd.Edges[0].Confidence != "EXTRACTED" {
		t.Errorf("defaults wrong: %+v", gd.Edges[0])
	}
}

func TestParseGraphifyDropsBadRows(t *testing.T) {
	// non-object nodes, duplicate ids, and edges missing an endpoint are dropped
	s := `{"nodes":[{"id":"a"},{"id":"a"},"garbage",{"id":"b"}],
           "links":[{"source":"a"},{"source":"a","target":"b"}]}`
	gd := mustParse(t, s)
	if len(gd.Nodes) != 2 {
		t.Errorf("dedupe/skip failed: nodes=%d want 2", len(gd.Nodes))
	}
	if len(gd.Edges) != 1 {
		t.Errorf("bad edge not dropped: edges=%d want 1", len(gd.Edges))
	}
}

func TestGraphifyKind(t *testing.T) {
	cases := []struct {
		n    graphifyNode
		want string
	}{
		{graphifyNode{Label: "GraphSchema", NodeType: "class"}, "klass"},
		{graphifyNode{Label: "ApiRouter", NodeType: "endpoint"}, "api"},
		{graphifyNode{Label: "README.md", FileType: "document"}, "concept"},
		{graphifyNode{Label: "run_cli_command()"}, "entry"},
		{graphifyNode{Label: "stream_results()"}, "async"},
		{graphifyNode{Label: "GraphWriter"}, "klass"},     // capitalized => class
		{graphifyNode{Label: "parse_file()"}, "function"}, // default
		{graphifyNode{Label: "x", SourceFile: "tests/t.py"}, "test"},
	}
	for _, c := range cases {
		if got := graphifyKind(c.n); got != c.want {
			t.Errorf("graphifyKind(%q) = %q, want %q", c.n.Label, got, c.want)
		}
	}
}

func TestGraphifyNodeKinds(t *testing.T) {
	// Exactly one classified kind; gopengraph appends the import-wide source_kind
	// on export, so a single kind here keeps nodes within cogs's 2-kind cap.
	if got := graphifyNodeKinds(graphifyNode{Label: "GraphSchema", NodeType: "class"}); len(got) != 1 || got[0] != "Class" {
		t.Errorf("kinds = %v, want [Class]", got)
	}
	if got := graphifyNodeKinds(graphifyNode{Label: "parse_file()"}); len(got) != 1 || got[0] != "Function" {
		t.Errorf("kinds = %v, want [Function]", got)
	}
}

func TestPascalCase(t *testing.T) {
	cases := map[string]string{
		"calls": "Calls", "imports_from": "ImportsFrom",
		"conceptually_related_to": "ConceptuallyRelatedTo", "": "Relates",
	}
	for in, want := range cases {
		if got := pascalCase(in, "Relates"); got != want {
			t.Errorf("pascalCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEdgeIncluded(t *testing.T) {
	cases := []struct {
		e    graphifyEdge
		want bool
	}{
		{graphifyEdge{Confidence: "EXTRACTED"}, true},
		{graphifyEdge{Confidence: "INFERRED", ConfidenceScore: 0.9, HasScore: true}, true},
		{graphifyEdge{Confidence: "INFERRED", ConfidenceScore: 0.3, HasScore: true}, false},
		{graphifyEdge{Confidence: "AMBIGUOUS", ConfidenceScore: 0.99, HasScore: true}, false},
	}
	for _, c := range cases {
		if got := edgeIncluded(c.e); got != c.want {
			t.Errorf("edgeIncluded(%+v) = %v, want %v", c.e, got, c.want)
		}
	}
}

func TestPassthroughScalarsKeepsExtras(t *testing.T) {
	gd := mustParse(t, `{"nodes":[{"id":"a","pagerank":0.42,"owner":"team-x","nested":{"z":1}}],"links":[]}`)
	if len(gd.Nodes) != 1 {
		t.Fatalf("nodes=%d", len(gd.Nodes))
	}
	ex := gd.Nodes[0].Extra
	if ex["owner"] != "team-x" {
		t.Errorf("string extra lost: %+v", ex)
	}
	if _, ok := ex["nested"]; ok {
		t.Errorf("nested object should not be a passthrough scalar: %+v", ex)
	}
	if pr, ok := ex["pagerank"].(float64); !ok || pr != 0.42 {
		t.Errorf("numeric extra lost/retyped: %+v", ex)
	}
}

// End-to-end: build via gopengraph and inspect the exported OpenGraph JSON.
func TestBuildGraphifyEndToEnd(t *testing.T) {
	gd := mustParse(t, linksSchema)
	graph := gopengraph.NewOpenGraph("Graphify")
	nodeIDs := map[string]bool{}
	buildGraphify(graph, gd, nodeIDs)

	jsonStr, err := graph.ExportJSON(true)
	if err != nil {
		t.Fatalf("ExportJSON error: %v", err)
	}

	var payload struct {
		Graph struct {
			Nodes []struct {
				ID         string                 `json:"id"`
				Kinds      []string               `json:"kinds"`
				Properties map[string]interface{} `json:"properties"`
			} `json:"nodes"`
			Edges []struct {
				Kind  string `json:"kind"`
				Start struct {
					Value   string `json:"value"`
					MatchBy string `json:"match_by"`
				} `json:"start"`
				End struct {
					Value   string `json:"value"`
					MatchBy string `json:"match_by"`
				} `json:"end"`
			} `json:"edges"`
		} `json:"graph"`
		Metadata struct {
			SourceKind string `json:"source_kind"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		t.Fatalf("exported JSON did not unmarshal: %v\n%s", err, jsonStr)
	}

	if len(payload.Graph.Nodes) != 2 {
		t.Fatalf("exported nodes = %d, want 2", len(payload.Graph.Nodes))
	}
	if len(payload.Graph.Edges) != 1 {
		t.Fatalf("exported edges = %d, want 1", len(payload.Graph.Edges))
	}
	if payload.Metadata.SourceKind != "Graphify" {
		t.Errorf("metadata.source_kind = %q, want Graphify", payload.Metadata.SourceKind)
	}

	byID := map[string][]string{}
	for _, n := range payload.Graph.Nodes {
		byID[n.ID] = n.Kinds
		if _, ok := n.Properties["objectid"]; ok {
			t.Errorf("node %s carries reserved property objectid — BloodHound fails ingest on it", n.ID)
		}
	}
	// class node "b" classifies as Class; gopengraph appends the source_kind
	// ("Graphify") on export -> exactly [Class, Graphify].
	if len(byID["b"]) != 2 || !contains(byID["b"], "Class") || !contains(byID["b"], "Graphify") {
		t.Errorf("node b kinds = %v, want [Class Graphify]", byID["b"])
	}
	// every node must stay within cogs's 2-kind cap so `og -g ... | cogs` works
	for id, kinds := range byID {
		if len(kinds) > 2 {
			t.Errorf("node %s has %d kinds (%v), exceeds cogs's 2-kind cap", id, len(kinds), kinds)
		}
	}

	e := payload.Graph.Edges[0]
	if e.Kind != "Calls" {
		t.Errorf("edge kind = %q, want Calls", e.Kind)
	}
	if e.Start.MatchBy != "id" || e.End.MatchBy != "id" {
		t.Errorf("edge match_by = %q/%q, want id/id", e.Start.MatchBy, e.End.MatchBy)
	}
	if e.Start.Value != "a" || e.End.Value != "b" {
		t.Errorf("edge endpoints = %q->%q, want a->b", e.Start.Value, e.End.Value)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestParseGraphifyInvalidJSON(t *testing.T) {
	if _, err := parseGraphifyJSON([]byte("{not json")); err == nil {
		t.Error("expected error on invalid JSON")
	}
}

// sanity: the bundled sample file parses and json round-trips
func TestBundledSampleParses(t *testing.T) {
	data, err := os.ReadFile("testdata/graphify_sample.json")
	if err != nil {
		t.Skipf("sample not present: %v", err)
	}
	gd, err := parseGraphifyJSON(data)
	if err != nil {
		t.Fatalf("sample parse error: %v", err)
	}
	if len(gd.Nodes) != 6 || len(gd.Edges) != 5 {
		t.Fatalf("sample nodes/edges = %d/%d, want 6/5", len(gd.Nodes), len(gd.Edges))
	}
	// spot-check that a normalized node marshals cleanly
	if _, err := json.Marshal(gd.Nodes[0]); err != nil {
		t.Errorf("node marshal error: %v", err)
	}
}
