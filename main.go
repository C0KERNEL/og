package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	gopengraph "github.com/TheManticoreProject/gopengraph"
	"github.com/TheManticoreProject/gopengraph/edge"
	"github.com/TheManticoreProject/gopengraph/node"
	"github.com/TheManticoreProject/gopengraph/properties"
)

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
	var graphifyFiles []string
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

	// graphify graph.json input ("-" reads JSON from stdin)
	flag.Func("graphify", "graphify graph.json to convert (can be specified multiple times; '-' for stdin)", func(s string) error {
		graphifyFiles = append(graphifyFiles, s)
		return nil
	})
	flag.Func("g", "graphify graph.json to convert (shorthand)", func(s string) error {
		graphifyFiles = append(graphifyFiles, s)
		return nil
	})

	flag.Parse()

	// Parse CSV data
	var parsedCSVs []*CSVFile

	// If the user routed stdin to the graphify parser ("-g -"), don't also
	// consume stdin as CSV.
	stdinForGraphify := false
	for _, gf := range graphifyFiles {
		if gf == "-" {
			stdinForGraphify = true
		}
	}

	// Check if data is being piped via stdin
	stat, _ := os.Stdin.Stat()
	if !stdinForGraphify && (stat.Mode()&os.ModeCharDevice) == 0 {
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

	if len(parsedCSVs) == 0 && len(graphifyFiles) == 0 {
		fmt.Fprintf(os.Stderr, "No input provided. Use --csv/-c or --graphify/-g flags, or pipe CSV data via stdin.\n")
		os.Exit(1)
	}

	// Create OpenGraph instance using gopengraph library
	// If sourceKind is empty, we'll use empty string which the library handles
	graph := gopengraph.NewOpenGraph(sourceKind)

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

	// Process graphify graph.json inputs (each adds its own nodes then edges)
	for _, gf := range graphifyFiles {
		if err := processGraphifyFile(graph, gf, nodeIDs); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing graphify file %s: %v\n", gf, err)
			os.Exit(1)
		}
	}

	// Process all edge CSVs
	for _, csv := range edgeCSVs {
		processEdgeCSV(graph, csv, nodeIDs)
	}

	// Export to JSON using gopengraph library's built-in method
	// includeMetadata is true only if sourceKind is provided
	jsonData, err := graph.ExportJSON(sourceKind != "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling to JSON: %v\n", err)
		os.Exit(1)
	}

	// Output to stdout
	fmt.Println(jsonData)
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

	lineLower := strings.ToLower(line)
	headerLower := strings.ToLower(currentHeader)

	// If it's exactly the same header, skip it (duplicate header from concatenation)
	if lineLower == headerLower {
		return false // Same header, skip it
	}

	// Check for distinctive header patterns
	hasNodeHeaders := strings.Contains(lineLower, "id") && strings.Contains(lineLower, "kinds")
	hasEdgeHeaders := strings.Contains(lineLower, "start") && strings.Contains(lineLower, "end")

	currentHasNodeHeaders := strings.Contains(headerLower, "id") && strings.Contains(headerLower, "kinds")
	currentHasEdgeHeaders := strings.Contains(headerLower, "start") && strings.Contains(headerLower, "end")

	// If the pattern changed from node to edge or vice versa, it's a new CSV
	if (hasNodeHeaders && currentHasEdgeHeaders) || (hasEdgeHeaders && currentHasNodeHeaders) {
		return true
	}

	// If both look like node headers or both look like edge headers,
	// but they have different fields, it's likely a new CSV with different columns
	if (hasNodeHeaders && currentHasNodeHeaders) || (hasEdgeHeaders && currentHasEdgeHeaders) {
		// Compare the actual field names
		fields1Lower := make([]string, len(fields1))
		fields2Lower := make([]string, len(fields2))
		for i, f := range fields1 {
			fields1Lower[i] = strings.ToLower(strings.TrimSpace(f))
		}
		for i, f := range fields2 {
			fields2Lower[i] = strings.ToLower(strings.TrimSpace(f))
		}

		// If they have different numbers of fields, it's a new CSV
		if len(fields1Lower) != len(fields2Lower) {
			return true
		}

		// If any field name differs, it's a new CSV
		for i := range fields1Lower {
			if fields1Lower[i] != fields2Lower[i] {
				return true
			}
		}
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
func processNodeCSV(graph *gopengraph.OpenGraph, csv *CSVFile, nodeIDs map[string]bool) {
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

		// Create properties from remaining columns using gopengraph library
		props := properties.NewProperties()
		for i, header := range csv.Headers {
			if i == idIdx || i == kindsIdx {
				continue
			}

			if i < len(row) {
				value := strings.TrimSpace(row[i])
				if value != "" {
					props.SetProperty(strings.TrimSpace(header), value)
				}
			}
		}

		// Create node using gopengraph library
		nodeObj, err := node.NewNode(id, kinds, props)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error creating node %s: %v\n", id, err)
			continue
		}

		graph.AddNode(nodeObj)
		nodeIDs[id] = true
	}
}

// processEdgeCSV processes an edge CSV and adds edges to the graph
func processEdgeCSV(graph *gopengraph.OpenGraph, csv *CSVFile, nodeIDs map[string]bool) {
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

		// Create properties from remaining columns using gopengraph library
		props := properties.NewProperties()
		for i, header := range csv.Headers {
			if i == startIdx || i == endIdx || i == kindIdx {
				continue
			}

			if i < len(row) {
				value := strings.TrimSpace(row[i])
				if value != "" {
					props.SetProperty(strings.TrimSpace(header), value)
				}
			}
		}

		// Create edge using gopengraph library
		// Note: match_by defaults to "id" in the library
		edgeObj, err := edge.NewEdge(start, end, kind, props)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error creating edge: %v\n", err)
			continue
		}

		graph.AddEdge(edgeObj)
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
