package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// OpenGraph structures based on BloodHound OpenGraph schema

type NodeReference struct {
	MatchBy string `json:"match_by"`
	Value   string `json:"value"`
}

type Edge struct {
	Start      NodeReference          `json:"start"`
	End        NodeReference          `json:"end"`
	Kind       string                 `json:"kind"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

type Node struct {
	ID         string                 `json:"id"`
	Kinds      []string               `json:"kinds"`
	Properties map[string]interface{} `json:"properties"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Metadata struct {
	SourceKind string `json:"source_kind,omitempty"`
}

type OpenGraph struct {
	Graph    Graph    `json:"graph"`
	Metadata Metadata `json:"metadata"`
}

type CSVType int

const (
	CSVTypeUnknown CSVType = iota
	CSVTypeNodes
	CSVTypeEdges
)

type CSVFile struct {
	Type    CSVType
	Headers []string
	Rows    [][]string
}

func main() {
	// Define command-line flags
	var csvFiles []string
	var sourceKind string

	flag.StringVar(&sourceKind, "source_kind", "", "Source kind for the OpenGraph metadata")
	flag.StringVar(&sourceKind, "s", "", "Source kind for the OpenGraph metadata (shorthand)")

	// Custom flag to handle multiple -c/--csv arguments
	flag.Func("csv", "CSV file to process (can be specified multiple times)", func(s string) error {
		csvFiles = append(csvFiles, s)
		return nil
	})
	flag.Func("c", "CSV file to process (shorthand)", func(s string) error {
		csvFiles = append(csvFiles, s)
		return nil
	})

	flag.Parse()

	// Parse CSV data
	var parsedCSVs []*CSVFile

	// Check if data is being piped via stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Data is being piped
		csvData, err := parseCSVFromReader(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading from stdin: %v\n", err)
			os.Exit(1)
		}
		parsedCSVs = append(parsedCSVs, csvData...)
	}

	// Process files from command-line arguments
	for _, csvFile := range csvFiles {
		file, err := os.Open(csvFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", csvFile, err)
			os.Exit(1)
		}

		csvData, err := parseCSVFromReader(file)
		file.Close()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing file %s: %v\n", csvFile, err)
			os.Exit(1)
		}

		parsedCSVs = append(parsedCSVs, csvData...)
	}

	if len(parsedCSVs) == 0 {
		fmt.Fprintf(os.Stderr, "No CSV data provided. Use --csv/-c flags or pipe data via stdin.\n")
		os.Exit(1)
	}

	// Create OpenGraph instance
	graph := &OpenGraph{
		Graph: Graph{
			Nodes: []Node{},
			Edges: []Edge{},
		},
		Metadata: Metadata{
			SourceKind: sourceKind,
		},
	}

	// Separate nodes and edges
	var nodeCSVs []*CSVFile
	var edgeCSVs []*CSVFile

	for _, csv := range parsedCSVs {
		switch csv.Type {
		case CSVTypeNodes:
			nodeCSVs = append(nodeCSVs, csv)
		case CSVTypeEdges:
			edgeCSVs = append(edgeCSVs, csv)
		case CSVTypeUnknown:
			fmt.Fprintf(os.Stderr, "Warning: Could not determine CSV type (needs 'id' for nodes or 'start'/'end' for edges)\n")
		}
	}

	// Process all node CSVs first
	nodeIDs := make(map[string]bool)
	for _, csv := range nodeCSVs {
		processNodeCSV(graph, csv, nodeIDs)
	}

	// Process all edge CSVs
	for _, csv := range edgeCSVs {
		processEdgeCSV(graph, csv, nodeIDs)
	}

	// Export to JSON using standard json.Marshal
	jsonData, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling to JSON: %v\n", err)
		os.Exit(1)
	}

	// Output to stdout
	fmt.Println(string(jsonData))
}

// parseCSVFromReader reads CSV data from a reader and detects multiple CSV blocks
func parseCSVFromReader(reader io.Reader) ([]*CSVFile, error) {
	var result []*CSVFile
	scanner := bufio.NewScanner(reader)

	var currentLines []string
	var inFirstCSV = true

	for scanner.Scan() {
		line := scanner.Text()

		// Skip completely empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check if this line is a header by seeing if it's distinctly different from data rows
		// A header will have field names, not data values
		isLikelyNewHeader := false
		if len(currentLines) > 0 && !inFirstCSV {
			isLikelyNewHeader = looksLikeNewCSVHeader(line, currentLines[0])
		}

		// If we detect a new header, process the accumulated lines
		if isLikelyNewHeader {
			csvFile, err := parseCSVLines(currentLines)
			if err != nil {
				return nil, err
			}
			if csvFile != nil {
				result = append(result, csvFile)
			}

			// Start new batch with this header
			currentLines = []string{line}
			continue
		}

		currentLines = append(currentLines, line)

		// After first line, we're no longer in the first CSV header check
		if inFirstCSV && len(currentLines) > 1 {
			inFirstCSV = false
		}
	}

	// Process any remaining lines
	if len(currentLines) > 0 {
		csvFile, err := parseCSVLines(currentLines)
		if err != nil {
			return nil, err
		}
		if csvFile != nil {
			result = append(result, csvFile)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// looksLikeNewCSVHeader checks if a line is likely a new CSV header vs a data row
func looksLikeNewCSVHeader(line string, currentHeader string) bool {
	// Parse both as CSV to compare
	r1 := csv.NewReader(strings.NewReader(line))
	r2 := csv.NewReader(strings.NewReader(currentHeader))

	fields1, err1 := r1.Read()
	fields2, err2 := r2.Read()

	if err1 != nil || err2 != nil {
		return false
	}

	// If they have different field counts, might be a new CSV
	if len(fields1) != len(fields2) {
		return true
	}

	// Check if the fields in line look like headers (contain common header names)
	// and are different from the current header
	lineLower := strings.ToLower(line)
	headerLower := strings.ToLower(currentHeader)

	if lineLower == headerLower {
		return false // Same header, probably repeated in concatenation
	}

	// Check for distinctive header patterns that differ
	hasNodeHeaders := strings.Contains(lineLower, "id") && strings.Contains(lineLower, "kinds")
	hasEdgeHeaders := strings.Contains(lineLower, "start") && strings.Contains(lineLower, "end")

	currentHasNodeHeaders := strings.Contains(headerLower, "id") && strings.Contains(headerLower, "kinds")
	currentHasEdgeHeaders := strings.Contains(headerLower, "start") && strings.Contains(headerLower, "end")

	// If the pattern changed from node to edge or vice versa, it's a new CSV
	if (hasNodeHeaders && currentHasEdgeHeaders) || (hasEdgeHeaders && currentHasNodeHeaders) {
		return true
	}

	return false
}

// parseCSVLines parses a slice of lines as a single CSV
func parseCSVLines(lines []string) (*CSVFile, error) {
	if len(lines) == 0 {
		return nil, nil
	}

	// Join lines and parse as CSV
	csvData := strings.Join(lines, "\n")
	reader := csv.NewReader(strings.NewReader(csvData))

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 1 {
		return nil, nil
	}

	csvFile := &CSVFile{
		Headers: records[0],
		Rows:    records[1:],
	}

	// Determine CSV type based on headers
	csvFile.Type = detectCSVType(csvFile.Headers)

	return csvFile, nil
}

// detectCSVType determines if the CSV is for nodes or edges based on headers
func detectCSVType(headers []string) CSVType {
	hasID := false
	hasStart := false
	hasEnd := false

	for _, header := range headers {
		headerLower := strings.ToLower(strings.TrimSpace(header))
		if headerLower == "id" {
			hasID = true
		}
		if headerLower == "start" {
			hasStart = true
		}
		if headerLower == "end" {
			hasEnd = true
		}
	}

	if hasStart && hasEnd {
		return CSVTypeEdges
	}
	if hasID {
		return CSVTypeNodes
	}

	return CSVTypeUnknown
}

// processNodeCSV processes a node CSV and adds nodes to the graph
func processNodeCSV(graph *OpenGraph, csv *CSVFile, nodeIDs map[string]bool) {
	// Find column indices
	idIdx := findHeaderIndex(csv.Headers, "id")
	kindsIdx := findHeaderIndex(csv.Headers, "kinds")

	if idIdx == -1 {
		fmt.Fprintf(os.Stderr, "Warning: Node CSV missing 'id' column\n")
		return
	}

	if kindsIdx == -1 {
		fmt.Fprintf(os.Stderr, "Warning: Node CSV missing 'kinds' column\n")
		return
	}

	for rowNum, row := range csv.Rows {
		if len(row) <= idIdx || len(row) <= kindsIdx {
			fmt.Fprintf(os.Stderr, "Warning: Row %d has insufficient columns\n", rowNum+2)
			continue
		}

		id := strings.TrimSpace(row[idIdx])
		kindsStr := strings.TrimSpace(row[kindsIdx])

		if id == "" {
			fmt.Fprintf(os.Stderr, "Warning: Row %d has empty 'id'\n", rowNum+2)
			continue
		}

		if kindsStr == "" {
			fmt.Fprintf(os.Stderr, "Warning: Row %d has empty 'kinds'\n", rowNum+2)
			continue
		}

		// Parse kinds (comma-separated)
		kinds := parseKinds(kindsStr)

		// Create properties from remaining columns
		props := make(map[string]interface{})
		for i, header := range csv.Headers {
			if i == idIdx || i == kindsIdx {
				continue
			}

			if i < len(row) {
				value := strings.TrimSpace(row[i])
				if value != "" {
					props[strings.TrimSpace(header)] = value
				}
			}
		}

		// Create and add node
		node := Node{
			ID:         id,
			Kinds:      kinds,
			Properties: props,
		}

		graph.Graph.Nodes = append(graph.Graph.Nodes, node)
		nodeIDs[id] = true
	}
}

// processEdgeCSV processes an edge CSV and adds edges to the graph
func processEdgeCSV(graph *OpenGraph, csv *CSVFile, nodeIDs map[string]bool) {
	// Find column indices
	startIdx := findHeaderIndex(csv.Headers, "start")
	endIdx := findHeaderIndex(csv.Headers, "end")
	kindIdx := findHeaderIndex(csv.Headers, "kind")

	if startIdx == -1 {
		fmt.Fprintf(os.Stderr, "Warning: Edge CSV missing 'start' column\n")
		return
	}

	if endIdx == -1 {
		fmt.Fprintf(os.Stderr, "Warning: Edge CSV missing 'end' column\n")
		return
	}

	if kindIdx == -1 {
		fmt.Fprintf(os.Stderr, "Warning: Edge CSV missing 'kind' column\n")
		return
	}

	for rowNum, row := range csv.Rows {
		if len(row) <= startIdx || len(row) <= endIdx || len(row) <= kindIdx {
			fmt.Fprintf(os.Stderr, "Warning: Row %d has insufficient columns\n", rowNum+2)
			continue
		}

		start := strings.TrimSpace(row[startIdx])
		end := strings.TrimSpace(row[endIdx])
		kind := strings.TrimSpace(row[kindIdx])

		if start == "" || end == "" || kind == "" {
			fmt.Fprintf(os.Stderr, "Warning: Row %d has empty required field\n", rowNum+2)
			continue
		}

		// Check if referenced nodes exist
		if !nodeIDs[start] {
			fmt.Fprintf(os.Stderr, "Warning: Edge references non-existent start node '%s'\n", start)
		}
		if !nodeIDs[end] {
			fmt.Fprintf(os.Stderr, "Warning: Edge references non-existent end node '%s'\n", end)
		}

		// Create properties from remaining columns
		props := make(map[string]interface{})
		for i, header := range csv.Headers {
			if i == startIdx || i == endIdx || i == kindIdx {
				continue
			}

			if i < len(row) {
				value := strings.TrimSpace(row[i])
				if value != "" {
					props[strings.TrimSpace(header)] = value
				}
			}
		}

		// Create and add edge
		edge := Edge{
			Start: NodeReference{
				MatchBy: "id",
				Value:   start,
			},
			End: NodeReference{
				MatchBy: "id",
				Value:   end,
			},
			Kind: kind,
		}

		// Only set properties if there are any
		if len(props) > 0 {
			edge.Properties = props
		}

		graph.Graph.Edges = append(graph.Graph.Edges, edge)
	}
}

// parseKinds parses a comma-separated string of kinds
func parseKinds(kindsStr string) []string {
	parts := strings.Split(kindsStr, ",")
	var kinds []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			kinds = append(kinds, trimmed)
		}
	}
	return kinds
}

// findHeaderIndex finds the index of a header (case-insensitive)
func findHeaderIndex(headers []string, target string) int {
	targetLower := strings.ToLower(target)
	for i, header := range headers {
		if strings.ToLower(strings.TrimSpace(header)) == targetLower {
			return i
		}
	}
	return -1
}
