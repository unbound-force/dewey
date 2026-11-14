## 1. Store content-bound preservation

- [x] 1.1 Update `store/store.go:UpdatePage` tier CASE to `CASE WHEN tier = 'validated' AND ? = content_hash THEN 'validated' ELSE ? END` with hash-equality check, empty-hash equality, GoDoc, and non-validated passthrough (VTB-FR-001)
- [x] 1.2 [P] Add `store/store_test.go` coverage: changed-content demotes validated, identical content preserves validated, hash-less writers keep validated, promote race stays closed, non-validated tiers unaffected

## 2. Compile demote-on-regeneration and hashes

- [x] 2.1 Remove `synthesizeClusters` validated-tier lookup so every cluster is `tier: draft`; preservation is store-owned (VTB-FR-003, KD-FR-108)
- [x] 2.2 Add truncated SHA-256 helper `contentHash` (first 8 bytes hex) and wire into `persistCompiledArticle` and `StoreCompiled` page construction over persisted bytes (VTB-FR-002)
- [ ] 2.3 [P] Update `tools/compile_test.go`: repurpose `TestCompile_PreservesValidatedTierAndProvenance` to assert demotion on changed bytes plus byte-identical preservation and `compiled_by`; add hash-recording assertions; remove lookup-failure test with the deleted branch; cover failed-topic/total-failure/empty-collection retention carve-outs still retain validated
- [ ] 2.4 [P] Update `tools/curate_test.go` knowledge re-index cases: changed validated file lands `curated`, unchanged validated file skipped without write (VTB-FR-004)

## 3. Curate re-index tier on change

- [ ] 3.1 Update `tools/curate.go:IndexKnowledgeStore` to write `curated` on hash change and skip on hash equality, leaving validated untouched only when bytes unchanged (VTB-FR-004)

## 4. Docs and constitution alignment verification

- [ ] 4.1 [P] File website docs sync issue (unbound-force/website, `docs` label) for user-visible `compile` tier/frontmatter change and retain `verified` site state until re-promoted
- [ ] 4.2 Verify constitution alignment: I Autonomous Collaboration — artifact-level trust boundary, no coupling; II Composability First — no deps/config, internal to `store`/`tools`; III Observable Quality — tier+hash queryable provenance; IV Testability — :memory: SQLite and NoopSynthesizer seams exercised
