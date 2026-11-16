## Example Output

The fenced block below is the report `gaze report --format=markdown`
renders for the analysis payload used to generate it: byte-identical to
the renderer golden output. Show that output unchanged, then add an
`## Assessment` section shaped like the example that follows the block.

```markdown
# 🔍 Gaze Full Quality Report

Project: github.com/example/project · Branch: main
Gaze Version: v2.4.0 · Date: 2026-11-01

---

## 📊 CRAP Summary

| Metric | Value |
|--------|------:|
| Total functions analyzed | 12 |
| Average complexity | 7.3 |
| Average line coverage | 47.1% |
| Average CRAP score | 52.6 |
| CRAPload | 7 (functions ≥ threshold 15) |
| GazeCRAPload | 6 (functions ≥ threshold 15) |

### Top 5 Worst CRAP Scores

| Function | CRAP | Complexity | Coverage | Fix Strategy | Location |
|----------|-----:|-----------:|---------:|--------------|----------|
| `(*队列处理器).处理消息` | 342.0 | 18 | 0.0% | decompose_and_test | internal/队列/处理器.go:42 |
| `processGeneratedProtocolBufferMessageForTheExtremelyLongServiceNameprocessGeneratedProtocolBufferMessageForTheExtremelyLongServiceNameprocessGeneratedProtocolBufferMessageForTheExtremelyLongServiceNam` | 90.0 | 9 | 0.0% | add_tests | internal/codegen/generated.go:310 |
| ``parse`raw`Literal`` | 72.0 | 8 | 0.0% | add_tests | internal/lexer/literal.go:77 |
| `(*Parser).parse\|Pipe` | 48.0 | 16 | 50.0% | decompose | internal/parser/pipe.go:55 |
| `validateInput` | 22.5 | 10 | 50.0% | add_assertions | internal/api/validate.go:21 |

### GazeCRAP Quadrant Distribution

| Quadrant | Count | Meaning |
|----------|------:|---------|
| 🟢 Q1 — Safe | 4 | Low complexity, high contract coverage |
| 🟡 Q2 — Complex But Tested | 2 | High complexity, contracts verified |
| ⚪ Q3 — Needs Tests | 2 | Simple but underspecified |
| 🔴 Q4 — Dangerous | 4 | Complex and contracts not adequately verified |

### Remediation Breakdown

| Fix Strategy | Functions |
|--------------|----------:|
| decompose | 1 |
| add_tests | 4 |
| add_assertions | 1 |
| decompose_and_test | 1 |

---

## 🧪 Quality Summary

| Metric | Value |
|--------|------:|
| Tests analyzed | 8 |
| Average contract coverage | 43.1% |
| Over-specifications | 1 |
| Assertion detection confidence | 92% |
| Skipped tests | 2 |

### Worst Tests by Contract Coverage

| Test | Target | Contract Coverage | Gaps |
|------|--------|------------------:|-----:|
| `TestProcessMessage` | `(*队列处理器).处理消息` | 0.0% | 12 |
| `TestLoadConfig` | `loadConfig` | 0.0% | 2 |
| `TestValidateInput` | `validateInput` | 0.0% | 3 |
| `TestParser_parse\|Pipe` | `(*Parser).parse\|Pipe` | 40.0% | 3 |
| `TestServe` | `(*Handler).ServeHTTP` | 50.0% | 1 |

### Coverage Gaps in Worst Tests

| Target | Effect | Description | Location |
|--------|--------|-------------|----------|
| `(*队列处理器).处理消息` | ReturnValue | returns int at position 0 | internal/队列/处理器.go:50:2 |
| `(*队列处理器).处理消息` | ErrorReturn | returns error at position 1 | internal/队列/处理器.go:55:2 |
| `(*队列处理器).处理消息` | ReceiverMutation | mutates receiver field 已处理 | internal/队列/处理器.go:60:2 |
| `(*队列处理器).处理消息` | ReceiverMutation | mutates receiver field 失败 | internal/队列/处理器.go:65:2 |
| `(*队列处理器).处理消息` | ReceiverMutation | mutates receiver field 重试 | internal/队列/处理器.go:70:2 |
| `(*队列处理器).处理消息` | PointerArgMutation | mutates pointer argument 消息 | internal/队列/处理器.go:75:2 |
| `(*队列处理器).处理消息` | SliceMutation | appends to slice 队列 | internal/队列/处理器.go:80:2 |
| `(*队列处理器).处理消息` | MapMutation | writes map 索引 | internal/队列/处理器.go:85:2 |
| `(*队列处理器).处理消息` | ChannelSend | sends on channel 完成 | internal/队列/处理器.go:90:2 |
| `(*队列处理器).处理消息` | ChannelClose | closes channel 停止 | internal/队列/处理器.go:95:2 |

> ⚠️ SSA analysis degraded for 1 package(s): github.com/example/project/internal/legacy

---

## 🏷️ Classification Summary

| Classification | Count | % |
|----------------|------:|--:|
| Contractual | 4 | 26.7% |
| Ambiguous | 9 | 60.0% |
| Incidental | 2 | 13.3% |

---

## 🏥 Overall Health Assessment

### Summary Scorecard

| Dimension | Grade | Details |
|-----------|-------|---------|
| CRAPload | 🔴 F | 7/12 functions (58.3%) at or above threshold 15 |
| GazeCRAPload | 🔴 F | 6/12 functions (50.0%) at or above threshold 15 |
| Avg Line Coverage | 🔴 D | 47.1% average line coverage |
| Contract Coverage | 🟡 B- | 43% average contract coverage across 8 tests |
| Complexity | 🟡 C+ | 7.3 average cyclomatic complexity |

Documentation: 2 files scanned.

### Recommendations

1. 🔴 Add tests for `processGeneratedProtocolBufferMessageForTheExtremelyLongServiceNameprocessGeneratedProtocolBufferMessageForTheExtremelyLongServiceNameprocessGeneratedProtocolBufferMessageForTheExtremelyLongServiceNam` — 0.0% line coverage, complexity 9, CRAP 90.0 (internal/codegen/generated.go:310).
2. 🔴 Add tests for ``parse`raw`Literal`` — 0.0% line coverage, complexity 8, CRAP 72.0 (internal/lexer/literal.go:77).
3. 🔴 Add tests for `loadConfig` — 0.0% line coverage, complexity 4, CRAP 20.0 (internal/config/load.go:30).
4. 🟡 Add tests for `(*Handler).ServeHTTP` — 20.0% line coverage, complexity 5, CRAP 17.8 (internal/api/handler.go:163).
5. 🟡 Strengthen assertions for `validateInput` — tests execute it without verifying its contracts; GazeCRAP 110.0 (internal/api/validate.go:21).
6. 🟡 Resolve SSA degradation in 1 package(s) — quality and contract data are partial there.
7. 🟢 Map 2 skipped test(s) to targets with `gaze quality --target` — no target function could be inferred.
8. 🟢 Document contracts for ambiguous side effects — 9 of 15 effects (60.0%) are ambiguous.
```

## Assessment

The project is in fair shape overall: contract coverage (43%) carries the
score while line coverage (47.1%) and complexity (7.3 average) hold it back.
The queue message handler dominates the risk: complexity 18 with no line
coverage produces a CRAP score of 342 and lands it in Q4 Dangerous.

1. Decompose `(*队列处理器).处理消息` first; at complexity 18 no amount of
   testing brings its CRAP score below the threshold.
2. Add tests for `parse`raw`Literal`` and `loadConfig`, which have zero line
   coverage despite low complexity, so tests alone close their gap.
3. Treat the 9 ambiguous effects as documentation work: Gaze cannot credit
   tests for contracts it cannot classify, so the 43% contract coverage
   understates what the suite actually verifies.
<!-- scaffolded by gaze dev -->
