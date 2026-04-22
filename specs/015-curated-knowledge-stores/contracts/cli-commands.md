# Contract: CLI Commands

**Branch**: `015-curated-knowledge-stores` | **Date**: 2026-04-21

## New Command: `dewey curate`

### Synopsis

```
dewey curate [flags]
```

### Description

Run the curation pipeline to extract structured knowledge from indexed sources. Reads `knowledge-stores.yaml` for store definitions, queries the index for source content, uses an LLM to extract decisions/facts/patterns, and writes curated markdown files to the store's output directory.

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--store` | `-s` | string | `""` | Curate only the named store (default: all stores) |
| `--force` | `-f` | bool | `false` | Re-curate all content, ignoring checkpoints |
| `--no-embeddings` | | bool | `false` | Skip embedding generation for curated files |
| `--vault` | | string | `""` | Path to vault (default: OBSIDIAN_VAULT_PATH or CWD) |

### Output

```
dewey curate
  Curating store "team-decisions" (2 sources, 12 documents)...
  ✅ team-decisions: 5 knowledge files created (12 docs processed, 3 skipped)

  Curating store "architecture" (1 source, 8 documents)...
  ✅ architecture: 3 knowledge files created (8 docs processed, 0 skipped)

  Curation complete: 8 files created across 2 stores
```

### Error Output

```
dewey curate
  Error: No knowledge stores configured. Create .uf/dewey/knowledge-stores.yaml or run 'dewey init'.

dewey curate --store nonexistent
  Error: Knowledge store "nonexistent" not found in configuration.

dewey curate
  Error: LLM unavailable. Ensure Ollama is running with a generation model.
  To fix: ollama serve && ollama pull llama3.2:3b
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (all stores curated) |
| 1 | Error (config missing, LLM unavailable, store not found) |

### Implementation

```go
func newCurateCmd() *cobra.Command {
    var storeName string
    var force bool
    var noEmbeddings bool
    var vaultPath string

    cmd := &cobra.Command{
        Use:   "curate",
        Short: "Extract knowledge from indexed sources",
        Long:  "Run the curation pipeline to extract structured knowledge from indexed sources into knowledge store directories.",
        SilenceUsage: true,
        RunE: func(cmd *cobra.Command, args []string) error {
            // 1. Resolve vault path
            // 2. Load knowledge-stores.yaml
            // 3. Open store
            // 4. Create embedder (optional)
            // 5. Create LLM synthesizer (Ollama)
            // 6. Create pipeline
            // 7. Run curation for each store (or named store)
            // 8. Auto-index knowledge store directories
            // 9. Report results
        },
    }
    // ... flag registration
    return cmd
}
```

## Modified Command: `dewey init`

### New Scaffold

After creating `sources.yaml`, `dewey init` also creates `knowledge-stores.yaml`:

```yaml
# Knowledge store configuration
# Each store curates knowledge from indexed sources.
# Uncomment and customize the example below.

# stores:
#   - name: team-decisions
#     sources: [disk-local]
#     # path: .uf/dewey/knowledge/team-decisions  # default
#     # curate_on_index: false                     # default
#     # curation_interval: 10m                     # default
```

### Idempotency

If `knowledge-stores.yaml` already exists, it is not overwritten (same pattern as existing `config.yaml` and `sources.yaml` handling).

## Modified Command: `dewey lint`

### New Output Section

When knowledge stores are configured, `dewey lint` adds a "Knowledge Stores" section:

```
dewey lint
  ...existing output...

  Knowledge Stores
    team-decisions: 5 high, 3 medium, 1 low, 0 flagged confidence
    team-decisions: 1 incongruent flag, 0 missing_rationale
    team-decisions: 2 unprocessed documents (stale)

  Found 7 issues.
```

## Modified Command: `dewey doctor`

### New Check

The doctor command adds a knowledge stores check:

```
Knowledge Stores
  ✅ knowledge-stores.yaml  2 stores configured
  ✅ team-decisions          45 curated files (.uf/dewey/knowledge/team-decisions/)
  ⚠️ architecture            0 curated files (run 'dewey curate')
```

## Slash Command: `dewey-curate.md`

Added to `slash_commands.go` for scaffolding by `dewey init`:

```markdown
---
description: Curate knowledge from indexed sources into structured knowledge stores.
---

# Command: /dewey-curate

## Description

Run the Dewey curation pipeline to extract decisions, facts, patterns,
and context from indexed sources. Uses LLM analysis to produce structured
knowledge files with quality flags and confidence scores.

## Usage

/dewey-curate
/dewey-curate --store team-decisions
/dewey-curate --force

## Instructions

1. Call the `curate` MCP tool
2. If the tool returns extraction prompts (no local LLM), perform synthesis
3. Report the results: files created, quality flags, confidence distribution
```
