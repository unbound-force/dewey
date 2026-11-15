# Data Model: Weekly Unattended Distill

**Feature**: 018-weekly-distill-automation | **Date**: 2026-11-15

## Entities

### DistillIssue
| Field | Type | Constraints |
|-------|------|-------------|
| number | int | GitHub issue number |
| label | string | always `dewey-distill` |
| assignee | string | repo maintainer (workflow var) |
| state | enum | `open` \| `closed` |
| dispatch_marker | string | comment `<!-- dewey-distill-trigger -->` |

Invariants: ≤1 open at any time (FR-003). Lifecycle: created by workflow, closed manually; re-trigger via comment.

### ScopeSet
| Field | Type | Constraints |
|-------|------|-------------|
| all_learnings | set<identity> | from git ls-tree `learning/<id>.md` |
| cited | set<identity> | union of all compiled `sources` |
| in_scope | set<identity> | `all − cited − policy_excluded` |
| empty | bool | `in_scope == ∅` → no-op |
| provenance | string | `git:<ref>:<sha>` or `filesystem:degraded` |

Computed from git object DB; no server required.

### DistillPolicy (optional, `.uf/dewey/distill-policy.yaml`)
```yaml
exclude_tags: ["scratch-*", "tmp/*"]
exclude_stores: ["team-knowledge-draft"]
```
Missing → no exclusions. Malformed entry → warn + skip.

### Theme (grouped cluster)
| Field | Type | Constraints |
|-------|------|-------------|
| name | string | sanitized tag/topic, 1:1 to PR |
| existing_article | string? | path `.uf/dewey/compiled/<name>.md` if exists |
| members | set<identity> | assigned learnings |
| supersessions | list<link> | explicit `supersedes` pairs, must colocate |
| decisions | list<AssignmentDecision> | provenance + confidence per member |

Seeded from 2026-10-25 index on backlog bootstrap.

### AssignmentDecision
| Field | Type |
|-------|------|
| identity | string |
| target_theme | string |
| source | enum `embedding_distance` \| `tag_fallback` \| `supersession_coercion` \| `tie_breaker` \| `llm_judge` |
| confidence | float 0..1 |
| details | string (distance value, rule name, judge id) |

### RollingPR
| Field | Type | Constraints |
|-------|------|-------------|
| branch | string | `dewey-distill/<theme>` |
| pr_number | int? | GitHub PR |
| state | enum | `open` \| `closed` \| `merged` |
| labels | set<string> | always includes `human-gate`, possibly `needs-human-resolution` |
| head_sha | string | current branch tip |
| human_commits_preserved | bool | post-rebase invariant |
| body | string | ReviewQueue rendering |
| tier_written | enum | always `draft` in distiller mode |

Transitions: `∄open → create`, `open → rebase/update`, `closed → create fresh`, `empty scope → no-op`.

### ReviewQueue (PR body)
Per-article section: `## <article>` + `### What changed` + `Newly cited: [ids]` + `Supersessions: [links]` + `Lint: [drift findings]` + `Confidence: [per-decision]`.

## Relationships

- DistillIssue 1—1 DistillRun (per weekly fire)
- DistillRun 1—N Theme (via ScopeSet)
- Theme 1—1 RollingPR
- Theme 1—N AssignmentDecision
- ScopeSet derived from Learning + CompiledArticle citations

## Validation

- Branch name sanitized: `[a-z0-9-_/]+`, max 100 chars
- Identity exists in git ls-tree output
- Supersession link both ends in scope or already cited
- No `tier: validated` file in any distiller write (server guard)
