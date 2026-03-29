# Design: doctor-ux-improvement

## Context

`uf doctor` uses a consistent output format across all its sections:

```
[PASS] check-name          description (path)
[WARN] check-name          not found
   Fix: command to fix
```

Key patterns:
- Check names are left-padded to ~20 chars
- Descriptions are human-readable, not raw values
- Paths shown in parens at the end of the description
- Single-line `Fix:` with 3-space indent
- Only three markers: `[PASS]`, `[WARN]`, `[FAIL]`
- Summary box at the bottom with emoji counters

`dewey doctor` currently deviates from all of these patterns.

## Goals / Non-Goals

### Goals
- Match `uf doctor` output format exactly
- Add summary box with pass/warn/fail counts
- Remove `[    ]` neutral marker — everything is PASS, WARN, or FAIL
- Hide implementation details (WAL/SHM files, raw byte counts)
- Standardize column alignment (20-char name field)
- Remove duplicate lock reporting (currently in both Database and MCP Server)
- Single-line `Fix:` pattern with 3-space indent

### Non-Goals
- Adding new checks
- Changing what is checked (same diagnostics, better formatting)
- Adding emoji to title (Dewey doesn't use charmbracelet/lipgloss for box drawing)

## Decisions

**D1: Output format per check**

```
  [PASS] check-name          description (path)
  [WARN] check-name          problem description
     Fix: command to run
  [FAIL] check-name          error description
     Fix: command to run
```

- 2-space indent before marker
- 20-char name field after marker
- Description is human-readable
- Path in parens if relevant
- Fix on next line, 5-space indent (aligns with description start)

**D2: Remove `[    ]` marker**

Currently used for informational items (endpoint, model name, WAL/SHM files). Replace with:
- Endpoint/model: show as sub-items under the Embedding Layer header (no marker)
- WAL/SHM: remove entirely (implementation details)
- "not present" items: `[PASS]` with "not present (no active lock)" style

**D3: Summary box**

```
╭──────────────────────────────────────────────╮
│   ✅ 12 passed  ⚠️  1 warning  ❌ 0 failed   │
╰──────────────────────────────────────────────╯
```

Count every `[PASS]`, `[WARN]`, `[FAIL]` marker throughout the output. Print at the end.

**D4: Human-readable descriptions**

| Current | New |
|---------|-----|
| `vault path       /Users/.../dewey` | `vault                /Users/.../dewey` |
| `graph.db         /path (54235136 bytes)` | `graph.db             52 MB, 1179 pages (/path)` |
| `dewey.log        /path (3380 bytes)` | `dewey.log            3.3 KB (/path)` |
| `sources.yaml     7 sources configured` | `sources.yaml         7 sources (/path)` |

**D5: Lock status — show once, in MCP Server section**

Remove lock check from Database section. The MCP Server section already checks it. Database section focuses on data health (page counts, embedding counts).

## Risks / Trade-offs

**Low risk**: Output formatting only. No behavioral changes to what is checked or how errors are detected. Tests need updating for new format strings.
