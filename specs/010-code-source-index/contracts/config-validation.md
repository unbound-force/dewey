# Contract: Code Source Configuration Validation

**Package**: `source`  
**File**: `source/config.go`

## Configuration Schema

```yaml
sources:
  - id: code-replicator
    type: code
    name: replicator
    refresh_interval: daily    # optional
    config:
      path: "../replicator"    # required: path to source code directory
      languages:               # required: list of language identifiers
        - go
      include:                 # optional: glob patterns for paths to include
        - "cmd/"
        - "internal/"
      exclude:                 # optional: glob patterns for paths to exclude
        - "vendor/"
        - "testdata/"
      ignore:                  # optional: extra gitignore-compatible patterns
        - "generated_*.go"
      recursive: true          # optional: traverse subdirectories (default: true)
```

## Validation Rules (in `validateSourceConfig`)

When `src.Type == "code"`:

1. `config` map MUST NOT be nil → error: `"code source requires config with 'path' and 'languages'"`
2. `config["path"]` MUST be present and non-empty → error: `"code source requires 'path' in config"`
3. `config["languages"]` MUST be present and non-empty → error: `"code source requires 'languages' in config"`

## Manager Integration (in `createSource`)

New case in `createSource()` switch:

```go
case "code":
    return createCodeSource(cfg, basePath)
```

The `createCodeSource` function:
1. Extracts `path` from config (resolves relative to `basePath` if `"."`)
2. Extracts `languages` as `[]string`
3. Extracts optional `include`, `exclude`, `ignore` as `[]string`
4. Extracts optional `recursive` as `bool` (default: `true`)
5. Returns `NewCodeSource(cfg.ID, cfg.Name, path, languages, opts...)`

## Invariants

1. `type: code` MUST be accepted by `validateSourceConfig` without error when all required fields are present
2. Missing `path` MUST produce a validation error
3. Missing `languages` MUST produce a validation error
4. Empty `languages` list MUST produce a validation error
5. Unknown languages in the list MUST NOT cause validation errors (handled at index time by FR-009)
