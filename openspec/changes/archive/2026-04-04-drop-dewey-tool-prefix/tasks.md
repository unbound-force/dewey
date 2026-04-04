## 1. Rename Tool Registrations

- [x] 1.1 In `server.go` `registerSemanticTools()`: rename `dewey_semantic_search` → `semantic_search`
- [x] 1.2 In `server.go` `registerSemanticTools()`: rename `dewey_similar` → `similar`
- [x] 1.3 In `server.go` `registerSemanticTools()`: rename `dewey_semantic_search_filtered` → `semantic_search_filtered`
- [x] 1.4 In `server.go` `registerLearningTools()`: rename `dewey_store_learning` → `store_learning`

## 2. Update Tool Description Cross-References

- [x] 2.1 In `server.go` `registerLearningTools()`: update `store_learning` description to reference `semantic_search` instead of `dewey_semantic_search`

## 3. Verification

- [x] 3.1 Run `go build ./...` and `go vet ./...`
- [x] 3.2 Run `go test -race -count=1 ./...` — all tests pass
- [x] 3.3 Verify constitution alignment: tool outputs unchanged (Observable Quality), no new dependencies (Composability), tool discovery still works (Autonomous Collaboration)
