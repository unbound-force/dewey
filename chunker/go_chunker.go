package chunker

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

func init() {
	Register(&GoChunker{})
}

// GoChunker implements the Chunker interface for Go source files.
// It uses go/parser and go/ast from the standard library to extract
// exported declarations, doc comments, Cobra commands, and MCP tools.
//
// Design decision: Uses go/parser (stdlib) instead of regex for correct
// AST walking, multi-line string handling, and doc comment association.
// No CGO dependency — satisfies constitution's dependency minimization.
type GoChunker struct{}

// ChunkFile parses a Go source file and returns extracted blocks for
// exported declarations, Cobra commands, and MCP tool registrations.
// Returns an error if the file has syntax errors. Returns an empty
// (non-nil) slice when no declarations are found.
func (g *GoChunker) ChunkFile(filename string, content []byte) ([]Block, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}

	var blocks []Block

	// Extract package doc comment.
	if b, ok := extractPackageDoc(file); ok {
		blocks = append(blocks, b)
	}

	// Walk top-level declarations for exported symbols.
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if b, ok := extractFuncDecl(fset, d); ok {
				blocks = append(blocks, b)
			}
		case *ast.GenDecl:
			blocks = append(blocks, extractGenDecl(fset, d)...)
		}
	}

	// Walk entire AST for Cobra commands and MCP tools.
	blocks = append(blocks, extractCobraCommands(file)...)
	blocks = append(blocks, extractMCPTools(file)...)

	// Invariant: return empty slice, not nil, when no blocks found.
	if blocks == nil {
		blocks = []Block{}
	}

	return blocks, nil
}

// IsTestFile reports whether the given filename is a Go test file.
// Uses the Go convention: files ending in _test.go are test files.
func (g *GoChunker) IsTestFile(filename string) bool {
	return strings.HasSuffix(filepath.Base(filename), "_test.go")
}

// Extensions returns the file extensions handled by the Go chunker.
func (g *GoChunker) Extensions() []string {
	return []string{".go"}
}

// Language returns the language identifier for the Go chunker.
func (g *GoChunker) Language() string {
	return "go"
}

// extractPackageDoc extracts the package-level doc comment as a Block.
// Returns (Block, false) if there is no package doc comment.
func extractPackageDoc(file *ast.File) (Block, bool) {
	if file.Doc == nil || file.Doc.Text() == "" {
		return Block{}, false
	}

	heading := "package " + file.Name.Name
	docText := formatDocComment(file.Doc.Text())
	content := docText + "\n" + heading

	return Block{
		Heading: heading,
		Content: content,
		Kind:    "package",
	}, true
}

// extractFuncDecl extracts an exported function or method declaration.
// Unexported functions are skipped. The function body is removed before
// formatting to produce a clean signature.
func extractFuncDecl(fset *token.FileSet, decl *ast.FuncDecl) (Block, bool) {
	if !decl.Name.IsExported() {
		return Block{}, false
	}

	heading := buildFuncHeading(decl)
	sig := formatFuncSignature(fset, decl)
	content := prependDocComment(decl.Doc, sig)

	return Block{
		Heading: heading,
		Content: content,
		Kind:    "func",
	}, true
}

// buildFuncHeading creates the heading for a function or method.
// Functions: "func Name". Methods: "func (*RecvType) Name".
func buildFuncHeading(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return "func " + decl.Name.Name
	}

	recvType := formatReceiverType(decl.Recv.List[0].Type)
	return "func (" + recvType + ") " + decl.Name.Name
}

// formatReceiverType formats a receiver type expression as a string.
// Handles both pointer (*T) and value (T) receivers.
func formatReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return "*" + formatReceiverType(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		// Generic type: T[P]
		return formatReceiverType(t.X) + "[" + formatReceiverType(t.Index) + "]"
	default:
		return "?"
	}
}

// formatFuncSignature formats a function declaration as its signature
// (without body). Sets Body to nil before formatting with go/format.Node.
func formatFuncSignature(fset *token.FileSet, decl *ast.FuncDecl) string {
	// Clone the declaration to avoid mutating the original AST.
	// We only need to nil out the body — shallow copy is sufficient.
	clone := *decl
	clone.Body = nil
	clone.Doc = nil // doc is handled separately

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, &clone); err != nil {
		// Fallback: use the heading as content if formatting fails.
		return buildFuncHeading(decl)
	}
	return buf.String()
}

// extractGenDecl extracts exported type, const, and var declarations.
// Each exported spec within a GenDecl produces a separate Block.
func extractGenDecl(fset *token.FileSet, decl *ast.GenDecl) []Block {
	switch decl.Tok {
	case token.TYPE:
		return extractTypeDecls(fset, decl)
	case token.CONST:
		return extractValueDecls(fset, decl, "const")
	case token.VAR:
		return extractValueDecls(fset, decl, "var")
	default:
		return nil
	}
}

// extractTypeDecls extracts exported type declarations from a GenDecl.
// Each exported TypeSpec produces a Block with Kind "type".
func extractTypeDecls(fset *token.FileSet, decl *ast.GenDecl) []Block {
	var blocks []Block
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok || !ts.Name.IsExported() {
			continue
		}

		heading := "type " + ts.Name.Name
		formatted := formatTypeSpec(fset, decl, ts)
		// Use the spec's own doc comment if available, otherwise fall
		// back to the GenDecl's doc comment (for ungrouped declarations).
		doc := ts.Doc
		if doc == nil {
			doc = decl.Doc
		}
		content := prependDocComment(doc, formatted)

		blocks = append(blocks, Block{
			Heading: heading,
			Content: content,
			Kind:    "type",
		})
	}
	return blocks
}

// formatTypeSpec formats a single type declaration. For grouped
// declarations (with parentheses), it formats just the individual spec.
// For ungrouped declarations, it formats the entire GenDecl.
func formatTypeSpec(fset *token.FileSet, decl *ast.GenDecl, ts *ast.TypeSpec) string {
	// For ungrouped declarations (no parens), format the whole GenDecl
	// to get "type Name struct { ... }" with proper formatting.
	if !decl.Lparen.IsValid() {
		clone := *decl
		clone.Doc = nil
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, &clone); err != nil {
			return "type " + ts.Name.Name
		}
		return buf.String()
	}

	// For grouped declarations, format just "type Name <underlying>".
	var buf bytes.Buffer
	buf.WriteString("type ")
	if err := format.Node(&buf, fset, ts); err != nil {
		return "type " + ts.Name.Name
	}
	return buf.String()
}

// extractValueDecls extracts exported const or var declarations.
// Each exported ValueSpec produces a Block with the given kind.
func extractValueDecls(fset *token.FileSet, decl *ast.GenDecl, kind string) []Block {
	var blocks []Block
	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			if !name.IsExported() {
				continue
			}

			heading := kind + " " + name.Name
			formatted := formatValueSpec(fset, decl, kind)
			doc := vs.Doc
			if doc == nil {
				doc = decl.Doc
			}
			content := prependDocComment(doc, formatted)

			blocks = append(blocks, Block{
				Heading: heading,
				Content: content,
				Kind:    kind,
			})
		}
	}
	return blocks
}

// formatValueSpec formats a const or var declaration. For ungrouped
// declarations, formats the entire GenDecl. For grouped declarations,
// formats just "const/var Name = value".
func formatValueSpec(fset *token.FileSet, decl *ast.GenDecl, kind string) string {
	if !decl.Lparen.IsValid() {
		clone := *decl
		clone.Doc = nil
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, &clone); err != nil {
			return kind
		}
		return buf.String()
	}

	// For grouped declarations, format the whole group.
	clone := *decl
	clone.Doc = nil
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, &clone); err != nil {
		return kind
	}
	return buf.String()
}

// extractCobraCommands walks the AST looking for Cobra command composite
// literals (&cobra.Command{...}) and extracts Use, Short, and Long fields.
func extractCobraCommands(file *ast.File) []Block {
	var blocks []Block

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		if !isCobraCommandLit(lit) {
			return true
		}

		fields := extractKeyValueStrings(lit, "Use", "Short", "Long")
		use := fields["Use"]
		if use == "" {
			return true // no Use field — skip
		}

		// Build content in the contract-specified format.
		var content strings.Builder
		content.WriteString("CLI Command: ")
		content.WriteString(use)
		if short := fields["Short"]; short != "" {
			content.WriteString("\nShort: ")
			content.WriteString(short)
		}
		if long := fields["Long"]; long != "" {
			content.WriteString("\nLong: ")
			content.WriteString(long)
		}

		blocks = append(blocks, Block{
			Heading: "command: " + use,
			Content: content.String(),
			Kind:    "command",
		})

		return true
	})

	return blocks
}

// isCobraCommandLit checks if a composite literal is a cobra.Command type.
// Handles both "cobra.Command" (selector) and "&cobra.Command" (unary expr
// parent handles the pointer — the composite literal type is the same).
func isCobraCommandLit(lit *ast.CompositeLit) bool {
	switch t := lit.Type.(type) {
	case *ast.SelectorExpr:
		// cobra.Command
		return t.Sel.Name == "Command"
	case *ast.Ident:
		// Imported as just "Command" (dot import or alias)
		return t.Name == "Command"
	default:
		return false
	}
}

// extractMCPTools walks the AST looking for mcp.AddTool() call expressions
// and extracts Name and Description from the mcp.Tool composite literal.
func extractMCPTools(file *ast.File) []Block {
	var blocks []Block

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if !isMCPAddToolCall(call) {
			return true
		}

		// The mcp.Tool literal is typically the second argument:
		// mcp.AddTool(srv, &mcp.Tool{...}, handler)
		toolLit := findMCPToolLiteral(call)
		if toolLit == nil {
			return true
		}

		fields := extractKeyValueStrings(toolLit, "Name", "Description")
		name := fields["Name"]
		if name == "" {
			return true
		}

		var content strings.Builder
		content.WriteString("MCP Tool: ")
		content.WriteString(name)
		if desc := fields["Description"]; desc != "" {
			content.WriteString("\nDescription: ")
			content.WriteString(desc)
		}

		blocks = append(blocks, Block{
			Heading: "tool: " + name,
			Content: content.String(),
			Kind:    "tool",
		})

		return true
	})

	return blocks
}

// isMCPAddToolCall checks if a call expression is mcp.AddTool(...).
func isMCPAddToolCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "AddTool"
}

// findMCPToolLiteral finds the mcp.Tool composite literal in an
// mcp.AddTool() call's arguments. It looks for &mcp.Tool{...} or
// mcp.Tool{...} in any argument position.
func findMCPToolLiteral(call *ast.CallExpr) *ast.CompositeLit {
	for _, arg := range call.Args {
		// Handle &mcp.Tool{...}
		if unary, ok := arg.(*ast.UnaryExpr); ok {
			if lit, ok := unary.X.(*ast.CompositeLit); ok {
				if isMCPToolType(lit.Type) {
					return lit
				}
			}
		}
		// Handle mcp.Tool{...} (without &)
		if lit, ok := arg.(*ast.CompositeLit); ok {
			if isMCPToolType(lit.Type) {
				return lit
			}
		}
	}
	return nil
}

// isMCPToolType checks if an expression refers to the mcp.Tool type.
func isMCPToolType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "Tool"
}

// extractKeyValueStrings extracts string literal values from a composite
// literal's key-value expressions. Only returns values for the specified
// keys, and only when the value is a string literal (ast.BasicLit with
// token.STRING). Non-string values (variables, function calls) are skipped.
func extractKeyValueStrings(lit *ast.CompositeLit, keys ...string) map[string]string {
	wanted := make(map[string]bool, len(keys))
	for _, k := range keys {
		wanted[k] = true
	}

	result := make(map[string]string, len(keys))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok || !wanted[ident.Name] {
			continue
		}
		bl, ok := kv.Value.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			continue
		}
		// Remove surrounding quotes from the string literal.
		val := bl.Value
		if len(val) >= 2 {
			if val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			} else if val[0] == '`' && val[len(val)-1] == '`' {
				val = val[1 : len(val)-1]
			}
		}
		result[ident.Name] = val
	}
	return result
}

// prependDocComment prepends a doc comment to content. If the doc comment
// is nil or empty, returns content unchanged.
func prependDocComment(doc *ast.CommentGroup, content string) string {
	if doc == nil {
		return content
	}
	text := doc.Text()
	if text == "" {
		return content
	}
	return formatDocComment(text) + "\n" + content
}

// formatDocComment converts go/ast doc comment text (which strips the //
// prefix) back into Go-style line comments. Each line is prefixed with
// "// " and trailing whitespace is trimmed.
func formatDocComment(text string) string {
	text = strings.TrimRight(text, "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = "//"
		} else {
			lines[i] = "// " + line
		}
	}
	return strings.Join(lines, "\n")
}
