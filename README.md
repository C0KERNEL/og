# og - OpenGraph CSV to JSON Converter

`og` (short for OpenGraph) is a command-line tool that converts CSV files containing nodes and edges into BloodHound-compatible OpenGraph JSON format.

## Features

- **Multiple input methods**: Read from stdin (pipeline) or specify files via command-line arguments
- **Automatic CSV detection**: Automatically recognizes node CSVs (with `id` and `kinds` columns) and edge CSVs (with `start`, `end`, and `kind` columns)
- **Multiple file support**: Process multiple CSV files in a single run
- **Optional source_kind**: Add a `source_kind` to the metadata when needed
- **Built with gopengraph**: Leverages the official BloodHound gopengraph library

## Installation

```bash
go build -o og
```

## Usage

### Basic Usage

Process CSV files via pipeline:
```bash
cat nodes.csv edges.csv | og > output.json
```

Process CSV files via command-line arguments:
```bash
og -c nodes.csv -c edges.csv > output.json
og --csv nodes.csv --csv edges.csv > output.json
```

### With source_kind

Add a `source_kind` to the metadata:
```bash
cat nodes.csv edges.csv | og -s MyDataSource > output.json
cat nodes.csv edges.csv | og --source_kind MyDataSource > output.json
```

### Example Use Case

```bash
cat nodes1.csv nodes2.csv edges1.csv edges2.csv | og > opengraph.json
```

## CSV Format

### Node CSV Format

Node CSVs must have at minimum:
- `id`: Unique identifier for the node
- `kinds`: Node types (comma-separated for multiple kinds)

Additional columns become node properties.

Example:
```csv
id,kinds,displayname,email
user-001,User,Alice Smith,alice@example.com
dept-001,"Department,OrgUnit",Engineering,IT
```

### Edge CSV Format

Edge CSVs must have:
- `start`: ID of the start node
- `end`: ID of the end node
- `kind`: Type of the relationship

Additional columns become edge properties.

**Note:** All edges use `match_by: "id"` in the OpenGraph output, meaning they reference nodes by their ID field.

Example:
```csv
start,end,kind,since
user-001,dept-001,MemberOf,2020-01-15
```

This produces edges that reference nodes by ID:
```json
{
  "start": {
    "match_by": "id",
    "value": "user-001"
  },
  "end": {
    "match_by": "id",
    "value": "dept-001"
  },
  "kind": "MemberOf",
  "properties": {
    "since": "2020-01-15"
  }
}
```

## Output Format

The tool generates BloodHound-compatible OpenGraph JSON with the following structure:

```json
{
  "graph": {
    "nodes": [...],
    "edges": [...]
  },
  "metadata": {
    "source_kind": "..." // Only included if --source_kind/-s is provided
  }
}
```

## Testing

Run the test suite:
```bash
go test -v
```

Test with sample data:
```bash
cat testdata/nodes1.csv testdata/nodes2.csv testdata/edges1.csv testdata/edges2.csv | ./og -s ExampleSource
```

## Command-line Options

- `-c, --csv`: CSV file to process (can be specified multiple times)
- `-s, --source_kind`: Source kind for the OpenGraph metadata (optional)

## Examples

See the `testdata/` directory for example CSV files:
- `nodes1.csv` - User nodes
- `nodes2.csv` - Department nodes
- `edges1.csv` - MemberOf relationships
- `edges2.csv` - Permission relationships

## License

This tool uses the [gopengraph](https://github.com/TheManticoreProject/gopengraph) library
