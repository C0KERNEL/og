package main

import (
	"strings"
	"testing"
)

func TestDetectCSVType(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		expected CSVType
	}{
		{
			name:     "Node CSV with id and kinds",
			headers:  []string{"id", "kinds", "displayname"},
			expected: CSVTypeNodes,
		},
		{
			name:     "Node CSV case insensitive",
			headers:  []string{"ID", "KINDS", "DisplayName"},
			expected: CSVTypeNodes,
		},
		{
			name:     "Edge CSV with start, end, kind",
			headers:  []string{"start", "end", "kind"},
			expected: CSVTypeEdges,
		},
		{
			name:     "Edge CSV case insensitive",
			headers:  []string{"START", "END", "KIND"},
			expected: CSVTypeEdges,
		},
		{
			name:     "Unknown CSV",
			headers:  []string{"foo", "bar", "baz"},
			expected: CSVTypeUnknown,
		},
		{
			name:     "Empty headers",
			headers:  []string{},
			expected: CSVTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectCSVType(tt.headers)
			if result != tt.expected {
				t.Errorf("detectCSVType(%v) = %v, want %v", tt.headers, result, tt.expected)
			}
		})
	}
}

func TestFindHeaderIndex(t *testing.T) {
	tests := []struct {
		name     string
		headers  []string
		target   string
		expected int
	}{
		{
			name:     "Found at index 0",
			headers:  []string{"id", "kinds", "displayname"},
			target:   "id",
			expected: 0,
		},
		{
			name:     "Found at index 2",
			headers:  []string{"id", "kinds", "displayname"},
			target:   "displayname",
			expected: 2,
		},
		{
			name:     "Case insensitive match",
			headers:  []string{"ID", "KINDS", "DisplayName"},
			target:   "id",
			expected: 0,
		},
		{
			name:     "Not found",
			headers:  []string{"id", "kinds", "displayname"},
			target:   "email",
			expected: -1,
		},
		{
			name:     "With whitespace",
			headers:  []string{" id ", "kinds", "displayname"},
			target:   "id",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findHeaderIndex(tt.headers, tt.target)
			if result != tt.expected {
				t.Errorf("findHeaderIndex(%v, %q) = %v, want %v", tt.headers, tt.target, result, tt.expected)
			}
		})
	}
}

func TestParseKinds(t *testing.T) {
	tests := []struct {
		name     string
		kindsStr string
		expected []string
	}{
		{
			name:     "Single kind",
			kindsStr: "User",
			expected: []string{"User"},
		},
		{
			name:     "Multiple kinds",
			kindsStr: "User,Person",
			expected: []string{"User", "Person"},
		},
		{
			name:     "Multiple kinds with spaces",
			kindsStr: "User, Person, Entity",
			expected: []string{"User", "Person", "Entity"},
		},
		{
			name:     "Empty string",
			kindsStr: "",
			expected: []string{},
		},
		{
			name:     "With trailing comma",
			kindsStr: "User,Person,",
			expected: []string{"User", "Person"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKinds(tt.kindsStr)
			if len(result) != len(tt.expected) {
				t.Errorf("parseKinds(%q) returned %d kinds, want %d", tt.kindsStr, len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("parseKinds(%q)[%d] = %q, want %q", tt.kindsStr, i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParseCSVLines(t *testing.T) {
	tests := []struct {
		name        string
		lines       []string
		expectType  CSVType
		expectRows  int
		expectError bool
	}{
		{
			name: "Valid node CSV",
			lines: []string{
				"id,kinds,displayname",
				"user-001,User,Alice",
				"user-002,User,Bob",
			},
			expectType: CSVTypeNodes,
			expectRows: 2,
		},
		{
			name: "Valid edge CSV",
			lines: []string{
				"start,end,kind",
				"user-001,user-002,MemberOf",
			},
			expectType: CSVTypeEdges,
			expectRows: 1,
		},
		{
			name:        "Empty lines",
			lines:       []string{},
			expectType:  CSVTypeUnknown,
			expectRows:  0,
			expectError: false,
		},
		{
			name: "Only header",
			lines: []string{
				"id,kinds,displayname",
			},
			expectType: CSVTypeNodes,
			expectRows: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseCSVLines(tt.lines)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
				return
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result == nil && len(tt.lines) > 0 {
				t.Error("Expected result but got nil")
				return
			}
			if result != nil {
				if result.Type != tt.expectType {
					t.Errorf("CSV type = %v, want %v", result.Type, tt.expectType)
				}
				if len(result.Rows) != tt.expectRows {
					t.Errorf("Number of rows = %d, want %d", len(result.Rows), tt.expectRows)
				}
			}
		})
	}
}

func TestLooksLikeNewCSVHeader(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		currentHeader string
		expected      bool
	}{
		{
			name:          "Same header",
			line:          "id,kinds,displayname",
			currentHeader: "id,kinds,displayname",
			expected:      false,
		},
		{
			name:          "Different column count",
			line:          "start,end,kind",
			currentHeader: "id,kinds,displayname,email",
			expected:      true,
		},
		{
			name:          "Node to Edge transition",
			line:          "start,end,kind",
			currentHeader: "id,kinds,displayname",
			expected:      true,
		},
		{
			name:          "Edge to Node transition",
			line:          "id,kinds,displayname",
			currentHeader: "start,end,kind",
			expected:      true,
		},
		{
			name:          "Same case insensitive",
			line:          "ID,KINDS,DisplayName",
			currentHeader: "id,kinds,displayname",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := looksLikeNewCSVHeader(tt.line, tt.currentHeader)
			if result != tt.expected {
				t.Errorf("looksLikeNewCSVHeader(%q, %q) = %v, want %v", tt.line, tt.currentHeader, result, tt.expected)
			}
		})
	}
}

func TestParseCSVFromReader(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectCSVs  int
		expectError bool
	}{
		{
			name: "Single node CSV",
			input: `id,kinds,displayname
user-001,User,Alice
user-002,User,Bob`,
			expectCSVs: 1,
		},
		{
			name: "Single edge CSV",
			input: `start,end,kind
user-001,user-002,MemberOf`,
			expectCSVs: 1,
		},
		{
			name: "Multiple CSVs",
			input: `id,kinds,displayname
user-001,User,Alice
start,end,kind
user-001,user-002,MemberOf`,
			expectCSVs: 2,
		},
		{
			name: "With empty lines",
			input: `id,kinds,displayname
user-001,User,Alice

user-002,User,Bob`,
			expectCSVs: 1,
		},
		{
			name: "Multiple CSVs with gap",
			input: `id,kinds,displayname
user-001,User,Alice

start,end,kind
user-001,user-002,MemberOf`,
			expectCSVs: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result, err := parseCSVFromReader(reader)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
				return
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if len(result) != tt.expectCSVs {
				t.Errorf("Number of CSVs = %d, want %d", len(result), tt.expectCSVs)
			}
		})
	}
}
