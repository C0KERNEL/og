# Graph Report - .  (2026-08-13)

## Corpus Check
- Corpus is ~7,415 words - fits in a single context window. You may not need a graph.

## Summary
- 85 nodes · 190 edges · 11 communities (10 shown, 1 thin omitted)
- Extraction: 92% EXTRACTED · 8% INFERRED · 0% AMBIGUOUS · INFERRED: 16 edges (avg confidence: 0.8)
- Token cost: 42,337 input · 0 output

## Community Hubs (Navigation)
- graphify Converter Tests
- README Concepts & Ecosystem
- graphify Field Normalization
- Kind Classification Heuristics
- CSV Parser Tests
- graphify-to-OpenGraph Build
- CSV Processing Pipeline
- CSV Type Detection
- graphify JSON Parsing
- CSV Stream Reader
- Go Module

## God Nodes (most connected - your core abstractions)
1. `mustParse()` - 14 edges
2. `parseGraphifyJSON()` - 10 edges
3. `buildGraphify()` - 10 edges
4. `normalizeGraphifyEdge()` - 9 edges
5. `og OpenGraph JSON Builder` - 8 edges
6. `graphifyKind()` - 7 edges
7. `parseCSVFromReader()` - 7 edges
8. `graphify-to-OpenGraph Converter` - 7 edges
9. `graphifyData` - 6 edges
10. `normalizeGraphifyNode()` - 6 edges

## Surprising Connections (you probably didn't know these)
- `mustParse()` --calls--> `parseGraphifyJSON()`  [INFERRED]
  graphify_test.go → graphify.go
- `TestBundledSampleParses()` --calls--> `parseGraphifyJSON()`  [INFERRED]
  graphify_test.go → graphify.go
- `TestParseGraphifyInvalidJSON()` --calls--> `parseGraphifyJSON()`  [INFERRED]
  graphify_test.go → graphify.go
- `TestBuildGraphifyEndToEnd()` --calls--> `buildGraphify()`  [INFERRED]
  graphify_test.go → graphify.go
- `main()` --calls--> `processGraphifyFile()`  [INFERRED]
  main.go → graphify.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **graphify-to-BloodHound Conversion Pipeline** — readme_graphify, readme_og, readme_gopengraph, readme_cogs [EXTRACTED 1.00]
- **Two-Kind Cap Merge Compatibility** — readme_single_kind_classification, readme_source_kind, readme_cogs, readme_gopengraph [EXTRACTED 1.00]

## Communities (11 total, 1 thin omitted)

### Community 0 - "graphify Converter Tests"
Cohesion: 0.24
Nodes (19): contains(), T, mustParse(), TestBuildGraphifyEndToEnd(), TestBundledSampleParses(), TestEdgeIncluded(), TestGraphifyColonIDsSanitized(), TestGraphifyKind() (+11 more)

### Community 1 - "README Concepts & Ecosystem"
Cohesion: 0.23
Nodes (14): BloodHound OpenGraph JSON Format, cogs OpenGraph Merger, Colon-to-Underscore ID Sanitization, include Confidence Filter, Edge CSV Format, Node CSV Format, gopengraph Library, graphify Knowledge Graph Tool (+6 more)

### Community 2 - "graphify Field Normalization"
Cohesion: 0.29
Nodes (10): endpointID(), firstFloat(), firstString(), firstValue(), formatFloat(), normalizeGraphifyEdge(), normalizeGraphifyNode(), passthroughScalars() (+2 more)

### Community 3 - "Kind Classification Heuristics"
Cohesion: 0.50
Nodes (7): baseName(), containsAny(), graphifyKind(), graphifyNodeKinds(), hasSuffixAny(), humanizeLabel(), graphifyNode

### Community 4 - "CSV Parser Tests"
Cohesion: 0.43
Nodes (7): T, TestDetectCSVType(), TestFindHeaderIndex(), TestLooksLikeNewCSVHeader(), TestParseCSVFromReader(), TestParseCSVLines(), TestParseKinds()

### Community 5 - "graphify-to-OpenGraph Build"
Cohesion: 0.38
Nodes (7): buildGraphify(), edgeIncluded(), OpenGraph, pascalCase(), processGraphifyFile(), graphifyData, graphifyEdge

### Community 6 - "CSV Processing Pipeline"
Cohesion: 0.57
Nodes (6): findHeaderIndex(), OpenGraph, main(), parseKinds(), processEdgeCSV(), processNodeCSV()

### Community 7 - "CSV Type Detection"
Cohesion: 0.67
Nodes (4): detectCSVType(), parseCSVLines(), CSVFile, CSVType

### Community 8 - "graphify JSON Parsing"
Cohesion: 0.67
Nodes (3): firstList(), mget(), parseGraphifyJSON()

### Community 9 - "CSV Stream Reader"
Cohesion: 0.67
Nodes (3): looksLikeNewCSVHeader(), parseCSVFromReader(), Reader

## Knowledge Gaps
- **2 isolated node(s):** `github.com/C0KERNEL/og`, `Node CSV Format`
  These have ≤1 connection - possible missing edges or undocumented components.
- **1 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `processGraphifyFile()` connect `graphify-to-OpenGraph Build` to `graphify JSON Parsing`, `Kind Classification Heuristics`, `CSV Processing Pipeline`?**
  _High betweenness centrality (0.301) - this node is a cross-community bridge._
- **Why does `main()` connect `CSV Processing Pipeline` to `CSV Stream Reader`, `graphify-to-OpenGraph Build`?**
  _High betweenness centrality (0.290) - this node is a cross-community bridge._
- **Why does `parseGraphifyJSON()` connect `graphify JSON Parsing` to `graphify Converter Tests`, `graphify Field Normalization`, `Kind Classification Heuristics`, `graphify-to-OpenGraph Build`?**
  _High betweenness centrality (0.158) - this node is a cross-community bridge._
- **Are the 3 inferred relationships involving `parseGraphifyJSON()` (e.g. with `mustParse()` and `TestBundledSampleParses()`) actually correct?**
  _`parseGraphifyJSON()` has 3 INFERRED edges - model-reasoned connections that need verification._
- **What connects `github.com/C0KERNEL/og`, `Node CSV Format` to the rest of the system?**
  _2 weakly-connected nodes found - possible documentation gaps or missing edges._