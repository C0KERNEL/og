# og - OpenGraph CSV to JSON Converter

`og` (short for OpenGraph) is a command-line tool that converts CSV files containing nodes and edges into BloodHound-compatible OpenGraph JSON format.

Built using the [gopengraph](https://github.com/TheManticoreProject/gopengraph) library for BloodHound OpenGraph compatibility.

## Features

- **Multiple input methods**: Read from stdin (pipeline) or specify files via command-line arguments
- **Automatic CSV detection**: Automatically recognizes node CSVs (with `id` and `kinds` columns) and edge CSVs (with `start`, `end`, and `kind` columns)
- **graphify import**: Convert a [graphify](https://github.com/Graphify-Labs/graphify) knowledge-graph `graph.json` straight to OpenGraph (`-g/--graphify`)
- **Multiple file support**: Process multiple CSV and/or graphify files in a single run
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

## graphify Input

[graphify](https://github.com/Graphify-Labs/graphify) builds a code/knowledge
graph and writes a `graph.json`. `og` can convert that file directly to
OpenGraph — no intermediate CSV needed:

```bash
og -g graphify-out/graph.json -s Graphify > opengraph.json
og --graphify graph.json --source_kind Graphify > opengraph.json
cat graph.json | og -g - -s Graphify > opengraph.json      # '-' reads JSON from stdin
```

You can mix graphify and CSV inputs in one run (IDs are shared across both):

```bash
cat extra_nodes.csv | og -g graph.json -c more_edges.csv -s Combined > opengraph.json
```

Generate a `graph.json` with graphify first (code-only extraction needs no API key):

```bash
graphify extract ./your-repo --code-only     # local AST, deterministic, no LLM
```

### Mapping

The converter is schema-tolerant: it accepts both graphify writers (edges under
`links` *or* `edges`), the `{"graph": {...}}` wrapper, and the common field-name
variants (`src`/`dst`, `name`-as-id, `cluster`, `weight`, object-valued
endpoints, …).

**Nodes** → OpenGraph nodes:

| OpenGraph | Source |
|-----------|--------|
| `id` | graphify node id (`id`/`node_id`/`name`/…) |
| `kinds` | a single classified kind, inferred from `node_type` and label/file heuristics (e.g. `Class`, `Endpoint`, `Entrypoint`, `Concept`, `Function`) |
| `properties` | `name`, `displayname`, `objectid` (BloodHound dedup key), `source_file`, `community`, `node_type`, `file_type`, `kind_heuristic`, plus any extra scalar fields (numbers/bools preserved) |

Emitting a single classified kind is deliberate: gopengraph appends the
import-wide `source_kind` (`-s`) to every node on export, so each node ends up
with at most two kinds — within the cap that
[`cogs`](https://github.com/C0KERNEL/cogs) enforces. That means graphify output
merges cleanly:

```bash
og -g graph.json -s Graphify | cogs -j other-source.json -s Combined > merged.json
```

**Edges** → OpenGraph edges:

| OpenGraph | Source |
|-----------|--------|
| `kind` | PascalCased relation (`calls` → `Calls`, `imports_from` → `ImportsFrom`) |
| `start` / `end` | node ids, `match_by: "id"` |
| `properties` | `relation`, `confidence`, `confidence_score`, `include` (graphify's confidence filter: `EXTRACTED`, or `INFERRED` ≥ 0.85), plus extra scalar fields |

No edges are dropped — low-confidence ones are kept with `include: false`. A
`source_kind` (`-s`) is folded into every node's `kinds` so the whole import can
be filtered or deleted as a unit in BloodHound.

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
- `-g, --graphify`: graphify `graph.json` to convert (can be specified multiple times; `-` reads JSON from stdin)
- `-s, --source_kind`: Source kind for the OpenGraph metadata (optional)

## Examples

See the `testdata/` directory for example CSV files:
- `nodes1.csv` - User nodes
- `nodes2.csv` - Department nodes
- `edges1.csv` - MemberOf relationships
- `edges2.csv` - Permission relationships

