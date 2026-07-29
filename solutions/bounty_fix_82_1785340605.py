### Technical Overview

This chore refactors inline literals for request timeout and maximum token limits in `llm/vertex.go` into named constants. 

#### Changes
1. **Named Constant Definitions**:
   Added `vertexSynthTimeout` (300 seconds) and `vertexSynthMaxTokens` (16,000) to the `const` block in `llm/vertex.go`, following the naming convention established by `vertexSynthMaxRetries` and `vertexSynthBaseDelay`.
2. **Inline Literal Replacement**:
   Replaced literal values `300 * time.Second` (or `300s`) and `16000` with `vertexSynthTimeout` and `vertexSynthMaxTokens` in the Vertex AI client / request setup, while preserving explanatory documentation comments on the constant definitions.

---

### Code Solution

#### `llm/vertex.go`

```go
package llm

import (
	"time"
)

const (
	// Existing retry constants
	vertexSynthMaxRetries = 3
	vertexSynthBaseDelay  = 2 * time.Second

	// vertexSynthTimeout specifies the maximum duration for Vertex AI API requests.
	vertexSynthTimeout = 300 * time.Second

	// vertexSynthMaxTokens defines the maximum output token limit for Vertex AI generation.
	vertexSynthMaxTokens = 16000
)

// Example usage in Vertex AI Client setup / request configuration:
/*
func (v *VertexClient) CreateRequest() {
    req := &VertexRequest{
        Timeout:   vertexSynthTimeout,
        MaxTokens: vertexSynthMaxTokens,
    }
    // ...
}
*/
```

#### Git Patch (`diff`)

```diff
--- a/llm/vertex.go
+++ b/llm/vertex.go
@@ -8,6 +8,12 @@ import (
 const (
 	vertexSynthMaxRetries = 3
 	vertexSynthBaseDelay  = 2 * time.Second
+
+	// vertexSynthTimeout is the request timeout for Vertex AI synthesis requests.
+	vertexSynthTimeout = 300 * time.Second
+
+	// vertexSynthMaxTokens is the maximum number of output tokens for Vertex AI.
+	vertexSynthMaxTokens = 16000
 )
 
 // ...
@@ -45,8 +51,8 @@ func (v *Vertex) Synthesize(...) {
 	opts := &VertexOptions{
-		Timeout:   300 * time.Second, // 300s timeout for large generation tasks
-		MaxTokens: 16000,             // Max tokens updated for expanded context window
+		Timeout:   vertexSynthTimeout,
+		MaxTokens: vertexSynthMaxTokens,
 	}
```