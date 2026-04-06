package chunker

import (
	"strings"
	"testing"
)

func TestGoChunker_PackageDoc(t *testing.T) {
	src := `// Package server provides HTTP server functionality.
package server
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlock(blocks, "package")
	if found == nil {
		t.Fatal("expected a block with Kind 'package', got none")
	}
	if found.Heading != "package server" {
		t.Errorf("heading = %q, want %q", found.Heading, "package server")
	}
	if !strings.Contains(found.Content, "Package server provides HTTP server functionality") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "package server") {
		t.Errorf("content should contain package declaration, got %q", found.Content)
	}
}

func TestGoChunker_ExportedFunc(t *testing.T) {
	src := `package server

// NewServer creates a new HTTP server with the given config.
func NewServer(cfg Config) *Server {
	return &Server{cfg: cfg}
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "func NewServer")
	if found == nil {
		t.Fatal("expected block with heading 'func NewServer'")
	}
	if found.Kind != "func" {
		t.Errorf("kind = %q, want %q", found.Kind, "func")
	}
	if !strings.Contains(found.Content, "NewServer creates a new HTTP server") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "func NewServer(cfg Config) *Server") {
		t.Errorf("content should contain signature, got %q", found.Content)
	}
	// Invariant: function body must NOT appear in content.
	if strings.Contains(found.Content, "return &Server") {
		t.Errorf("content must not contain function body, got %q", found.Content)
	}
}

func TestGoChunker_UnexportedFunc(t *testing.T) {
	src := `package server

// helper does internal work.
func helper() {}

// Exported is public.
func Exported() {}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, b := range blocks {
		if b.Kind == "func" && strings.Contains(b.Heading, "helper") {
			t.Errorf("unexported function 'helper' should not appear in blocks")
		}
	}

	found := findBlockByHeading(blocks, "func Exported")
	if found == nil {
		t.Fatal("expected block for exported function 'Exported'")
	}
}

func TestGoChunker_ExportedMethod(t *testing.T) {
	src := `package server

// Handle processes an incoming request.
func (s *Server) Handle(r Request) Response {
	return Response{}
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "func (*Server) Handle")
	if found == nil {
		t.Fatal("expected block with heading 'func (*Server) Handle'")
	}
	if found.Kind != "func" {
		t.Errorf("kind = %q, want %q", found.Kind, "func")
	}
	if !strings.Contains(found.Content, "Handle processes an incoming request") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "func (s *Server) Handle(r Request) Response") {
		t.Errorf("content should contain method signature with receiver, got %q", found.Content)
	}
}

func TestGoChunker_ExportedType(t *testing.T) {
	src := `package server

// Config holds server configuration.
type Config struct {
	Port int
	Host string
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "type Config")
	if found == nil {
		t.Fatal("expected block with heading 'type Config'")
	}
	if found.Kind != "type" {
		t.Errorf("kind = %q, want %q", found.Kind, "type")
	}
	if !strings.Contains(found.Content, "Config holds server configuration") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "Port int") {
		t.Errorf("content should contain struct fields, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "Host string") {
		t.Errorf("content should contain struct fields, got %q", found.Content)
	}
}

func TestGoChunker_InterfaceType(t *testing.T) {
	src := `package server

// Handler processes requests.
type Handler interface {
	Handle(r Request) Response
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "type Handler")
	if found == nil {
		t.Fatal("expected block with heading 'type Handler'")
	}
	if found.Kind != "type" {
		t.Errorf("kind = %q, want %q", found.Kind, "type")
	}
	if !strings.Contains(found.Content, "Handler processes requests") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "Handle(r Request) Response") {
		t.Errorf("content should contain interface methods, got %q", found.Content)
	}
}

func TestGoChunker_ExportedConst(t *testing.T) {
	src := `package server

// Version is the current server version.
const Version = "1.0.0"
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "const Version")
	if found == nil {
		t.Fatal("expected block with heading 'const Version'")
	}
	if found.Kind != "const" {
		t.Errorf("kind = %q, want %q", found.Kind, "const")
	}
	if !strings.Contains(found.Content, "Version is the current server version") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, `Version = "1.0.0"`) {
		t.Errorf("content should contain const value, got %q", found.Content)
	}
}

func TestGoChunker_ExportedVar(t *testing.T) {
	src := `package server

// DefaultPort is the default listening port.
var DefaultPort = 8080
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "var DefaultPort")
	if found == nil {
		t.Fatal("expected block with heading 'var DefaultPort'")
	}
	if found.Kind != "var" {
		t.Errorf("kind = %q, want %q", found.Kind, "var")
	}
	if !strings.Contains(found.Content, "DefaultPort is the default listening port") {
		t.Errorf("content should contain doc comment, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "DefaultPort = 8080") {
		t.Errorf("content should contain var value, got %q", found.Content)
	}
}

func TestGoChunker_CobraCommand(t *testing.T) {
	src := `package main

import "github.com/spf13/cobra"

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server",
		Long:  "Start the MCP server with stdio or HTTP transport.",
	}
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("cli.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "command: serve")
	if found == nil {
		t.Fatal("expected block with heading 'command: serve'")
	}
	if found.Kind != "command" {
		t.Errorf("kind = %q, want %q", found.Kind, "command")
	}
	if !strings.Contains(found.Content, "CLI Command: serve") {
		t.Errorf("content should contain 'CLI Command: serve', got %q", found.Content)
	}
	if !strings.Contains(found.Content, "Short: Start the MCP server") {
		t.Errorf("content should contain Short field, got %q", found.Content)
	}
	if !strings.Contains(found.Content, "Long: Start the MCP server with stdio or HTTP transport.") {
		t.Errorf("content should contain Long field, got %q", found.Content)
	}
}

func TestGoChunker_MCPTool(t *testing.T) {
	src := `package main

import "github.com/modelcontextprotocol/go-sdk/mcp"

func registerTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_page",
		Description: "Get a page with its block tree",
	}, handleGetPage)
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "tool: get_page")
	if found == nil {
		t.Fatal("expected block with heading 'tool: get_page'")
	}
	if found.Kind != "tool" {
		t.Errorf("kind = %q, want %q", found.Kind, "tool")
	}
	if !strings.Contains(found.Content, "MCP Tool: get_page") {
		t.Errorf("content should contain 'MCP Tool: get_page', got %q", found.Content)
	}
	if !strings.Contains(found.Content, "Description: Get a page with its block tree") {
		t.Errorf("content should contain Description field, got %q", found.Content)
	}
}

func TestGoChunker_SyntaxError(t *testing.T) {
	src := `package main

func broken( {
	// invalid syntax
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("broken.go", []byte(src))
	if err == nil {
		t.Fatal("expected error for syntax error, got nil")
	}
	if blocks != nil {
		t.Errorf("expected nil blocks on error, got %d blocks", len(blocks))
	}
	if !strings.Contains(err.Error(), "broken.go") {
		t.Errorf("error should contain filename, got %q", err.Error())
	}
}

func TestGoChunker_EmptyFile(t *testing.T) {
	src := `package empty
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("empty.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if blocks == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks for empty file, got %d", len(blocks))
	}
}

func TestGoChunker_NoDocComment(t *testing.T) {
	src := `package server

func Serve(addr string) error {
	return nil
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "func Serve")
	if found == nil {
		t.Fatal("expected block for exported function without doc comment")
	}
	if found.Kind != "func" {
		t.Errorf("kind = %q, want %q", found.Kind, "func")
	}
	// Content should be just the signature, no doc comment prefix.
	if !strings.Contains(found.Content, "func Serve(addr string) error") {
		t.Errorf("content should contain signature, got %q", found.Content)
	}
	// Should NOT contain "//" since there's no doc comment.
	if strings.Contains(found.Content, "//") {
		t.Errorf("content should not contain comment markers when no doc, got %q", found.Content)
	}
}

func TestGoChunker_IsTestFile(t *testing.T) {
	gc := &GoChunker{}

	tests := []struct {
		filename string
		want     bool
	}{
		{"foo_test.go", true},
		{"foo.go", false},
		{"test.go", false},
		{"path/to/bar_test.go", true},
		{"path/to/bar.go", false},
		{"_test.go", true},
	}

	for _, tt := range tests {
		got := gc.IsTestFile(tt.filename)
		if got != tt.want {
			t.Errorf("IsTestFile(%q) = %v, want %v", tt.filename, got, tt.want)
		}
	}
}

func TestGoChunker_Extensions(t *testing.T) {
	gc := &GoChunker{}
	exts := gc.Extensions()
	if len(exts) != 1 {
		t.Fatalf("expected 1 extension, got %d", len(exts))
	}
	if exts[0] != ".go" {
		t.Errorf("extension = %q, want %q", exts[0], ".go")
	}
}

func TestGoChunker_Language(t *testing.T) {
	gc := &GoChunker{}
	lang := gc.Language()
	if lang != "go" {
		t.Errorf("language = %q, want %q", lang, "go")
	}
}

func TestGoChunker_MultipleExports(t *testing.T) {
	src := `package api

// Client is the API client.
type Client struct {
	BaseURL string
}

// NewClient creates a new Client.
func NewClient(url string) *Client {
	return &Client{BaseURL: url}
}

// Get fetches a resource.
func (c *Client) Get(path string) ([]byte, error) {
	return nil, nil
}

// DefaultTimeout is the default request timeout.
const DefaultTimeout = 30
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("api.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: type Client, func NewClient, func (*Client) Get, const DefaultTimeout
	wantHeadings := []string{
		"type Client",
		"func NewClient",
		"func (*Client) Get",
		"const DefaultTimeout",
	}
	for _, h := range wantHeadings {
		if findBlockByHeading(blocks, h) == nil {
			t.Errorf("missing block with heading %q", h)
		}
	}
}

func TestGoChunker_UnexportedConst(t *testing.T) {
	src := `package server

const internalLimit = 100

// PublicLimit is the public rate limit.
const PublicLimit = 1000
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, b := range blocks {
		if strings.Contains(b.Heading, "internalLimit") {
			t.Errorf("unexported const 'internalLimit' should not appear in blocks")
		}
	}

	found := findBlockByHeading(blocks, "const PublicLimit")
	if found == nil {
		t.Fatal("expected block for exported const 'PublicLimit'")
	}
}

func TestGoChunker_ValueReceiverMethod(t *testing.T) {
	src := `package server

// String returns the string representation.
func (s Server) String() string {
	return "server"
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := findBlockByHeading(blocks, "func (Server) String")
	if found == nil {
		t.Fatal("expected block with heading 'func (Server) String'")
	}
	if !strings.Contains(found.Content, "func (s Server) String() string") {
		t.Errorf("content should contain value receiver signature, got %q", found.Content)
	}
}

func TestGoChunker_CobraCommandNoUse(t *testing.T) {
	// A cobra.Command without a Use field should be skipped.
	src := `package main

import "github.com/spf13/cobra"

func newCmd() *cobra.Command {
	return &cobra.Command{
		Short: "A command without Use",
	}
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("cli.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, b := range blocks {
		if b.Kind == "command" {
			t.Errorf("should not extract command without Use field, got heading %q", b.Heading)
		}
	}
}

func TestGoChunker_MCPToolNoName(t *testing.T) {
	// An mcp.Tool without a Name field should be skipped.
	src := `package main

import "github.com/modelcontextprotocol/go-sdk/mcp"

func register(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Description: "A tool without a name",
	}, handler)
}
`
	gc := &GoChunker{}
	blocks, err := gc.ChunkFile("server.go", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, b := range blocks {
		if b.Kind == "tool" {
			t.Errorf("should not extract tool without Name field, got heading %q", b.Heading)
		}
	}
}

// --- Registry Tests ---

func TestRegistry_Get(t *testing.T) {
	// GoChunker is registered via init(), so "go" should be available.
	c, ok := Get("go")
	if !ok {
		t.Fatal("expected Go chunker to be registered")
	}
	if c.Language() != "go" {
		t.Errorf("language = %q, want %q", c.Language(), "go")
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	c, ok := Get("brainfuck")
	if ok {
		t.Fatal("expected unknown language to return false")
	}
	if c != nil {
		t.Error("expected nil chunker for unknown language")
	}
}

func TestRegistry_ForExtension(t *testing.T) {
	c, ok := ForExtension(".go")
	if !ok {
		t.Fatal("expected .go extension to be registered")
	}
	if c.Language() != "go" {
		t.Errorf("language = %q, want %q", c.Language(), "go")
	}
}

func TestRegistry_ForExtensionUnknown(t *testing.T) {
	c, ok := ForExtension(".rs")
	if ok {
		t.Fatal("expected unknown extension to return false")
	}
	if c != nil {
		t.Error("expected nil chunker for unknown extension")
	}
}

func TestRegistry_Languages(t *testing.T) {
	langs := Languages()
	found := false
	for _, l := range langs {
		if l == "go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'go' in Languages(), got %v", langs)
	}
}

func TestRegistry_ExtensionMap(t *testing.T) {
	m := ExtensionMap()
	lang, ok := m[".go"]
	if !ok {
		t.Fatal("expected .go in ExtensionMap()")
	}
	if lang != "go" {
		t.Errorf("ExtensionMap()['.go'] = %q, want %q", lang, "go")
	}
}

func TestRegistry_DuplicatePanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "duplicate") {
			t.Errorf("panic message should mention 'duplicate', got %q", msg)
		}
	}()

	// Attempt to register a second Go chunker — should panic.
	Register(&GoChunker{})
}

// --- Test Helpers ---

// findBlock returns the first block with the given Kind, or nil.
func findBlock(blocks []Block, kind string) *Block {
	for i := range blocks {
		if blocks[i].Kind == kind {
			return &blocks[i]
		}
	}
	return nil
}

// findBlockByHeading returns the first block with the given Heading, or nil.
func findBlockByHeading(blocks []Block, heading string) *Block {
	for i := range blocks {
		if blocks[i].Heading == heading {
			return &blocks[i]
		}
	}
	return nil
}
