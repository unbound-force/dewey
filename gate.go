package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

// errAutoAnswerGateRefused is returned by the `gate auto-answer`
// command when the gate refuses. main() logs it and exits 1; the
// tool-rendered verdict was already printed to stdout, so the error
// carries no user-facing detail.
var errAutoAnswerGateRefused = errors.New("auto-answer gate refused")

// envTokenRe matches an agents entry that is exactly one environment
// interpolation token. Per CA-FR-007 interpolation applies only when
// the whole entry is the token.
var envTokenRe = regexp.MustCompile(`^\{env:([A-Za-z_][A-Za-z0-9_]*)\}$`)

// envTokenAnyRe detects a partial interpolation anywhere in an entry,
// which is unverifiable rather than matchable.
var envTokenAnyRe = regexp.MustCompile(`\{env:`)

// defaultAutoAnswerAgents is the plugin's documented default agent
// list when its config file or `agents` key is absent.
var defaultAutoAnswerAgents = []string{"autonomy"}

// gateOptions injects the gate's external inputs so evaluation is
// testable without touching the real environment or filesystem
// layout. Empty fields fall back to the process environment and the
// current working directory.
type gateOptions struct {
	// configDir overrides the OpenCode global config directory. When
	// empty it is resolved from $XDG_CONFIG_HOME or $HOME.
	configDir string
	// workDir is where .opencode/agents/curator.md is looked up.
	// Defaults to the process working directory.
	workDir string
	// lookupEnv reads an environment variable and reports whether it
	// is set. Defaults to os.LookupEnv.
	lookupEnv func(string) (string, bool)
}

// gateVerdict is the evaluated outcome rendered by the command.
type gateVerdict struct {
	Pass            bool
	ConfigFile      string
	EffectiveAgents []string
	DefaultApplied  bool
	Registration    string
	Warnings        []string
	Reason          string
	Fix             string
}

// evaluateAutoAnswerGate deterministically evaluates whether the
// curator agent is configured for automatic question answering, per
// CA-FR-007. Every failure path is fail-closed: an unverifiable input
// refuses rather than assuming auto-answering is off.
func evaluateAutoAnswerGate(opts gateOptions) gateVerdict {
	lookup := opts.lookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	workDir := opts.workDir
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return gateVerdict{
				Reason: "cannot determine the working directory: " + err.Error(),
				Fix:    "Run the gate from a readable working directory, then re-run /dewey-distill.",
			}
		}
		workDir = wd
	}

	configDir := opts.configDir
	if configDir == "" {
		resolved, err := resolveOpenCodeConfigDir(lookup)
		if err != nil {
			return gateVerdict{
				Reason: "cannot resolve the OpenCode config directory: " + err.Error(),
				Fix:    "Set XDG_CONFIG_HOME or HOME to an absolute path, then re-run /dewey-distill.",
			}
		}
		configDir = resolved
	}

	verdict := gateVerdict{ConfigFile: filepath.Join(configDir, "auto-question.jsonc")}
	// Advisory only: registration state never changes the verdict.
	verdict.Registration, verdict.Warnings = scanPluginRegistration(workDir, configDir)

	agents, defaultApplied, refuseVerdictResult, ok := resolvePluginAgents(verdict.ConfigFile, lookup, verdict)
	if !ok {
		return refuseVerdictResult
	}
	verdict.EffectiveAgents = agents
	verdict.DefaultApplied = defaultApplied

	if refusal, refused := agentListRefusal(agents, verdict.ConfigFile); refused {
		verdict.Reason = refusal.Reason
		verdict.Fix = refusal.Fix
		return verdict
	}

	if err := verifyCuratorDefinition(workDir); err != nil {
		verdict.Reason = "the curator agent definition is not resolvable: " + err.Error()
		verdict.Fix = "Ensure .opencode/agents/curator.md exists with `mode: subagent`, then re-run /dewey-distill."
		return verdict
	}

	verdict.Pass = true
	return verdict
}

// resolvePluginAgents determines the effective agent list from the
// plugin config: the documented default when the file is absent, the
// parsed list when present, or a refusal verdict when the path cannot
// be stat'd, is a directory, or cannot be interpreted.
func resolvePluginAgents(path string, lookup func(string) (string, bool), base gateVerdict) ([]string, bool, gateVerdict, bool) {
	info, statErr := os.Stat(path)
	switch {
	case statErr != nil && os.IsNotExist(statErr):
		return append([]string(nil), defaultAutoAnswerAgents...), true, base, true
	case statErr != nil:
		base.Reason = fmt.Sprintf("cannot stat %s: %v", path, statErr)
		base.Fix = fmt.Sprintf("Make %s reachable, then re-run /dewey-distill.", path)
		return nil, false, base, false
	case !info.Mode().IsRegular():
		base.Reason = fmt.Sprintf("%s is not a regular file", path)
		base.Fix = fmt.Sprintf("Replace %s with the plugin config file or remove it, then re-run /dewey-distill.", path)
		return nil, false, base, false
	}
	return readPluginAgents(path, lookup, base)
}

// agentListRefusal returns a refusal fragment when the effective agent
// list contains curator or the wildcard.
func agentListRefusal(agents []string, configFile string) (gateVerdict, bool) {
	for _, agent := range agents {
		switch agent {
		case "*":
			return gateVerdict{
				Reason: fmt.Sprintf("the %q agent list includes the wildcard `*`", configFile),
				Fix:    fmt.Sprintf("Replace the wildcard with an explicit list that omits curator in %s, then re-run /dewey-distill.", configFile),
			}, true
		case "curator":
			return gateVerdict{
				Reason: fmt.Sprintf("the %q agent list includes curator", configFile),
				Fix:    fmt.Sprintf("Remove curator from the agents list in %s, then re-run /dewey-distill.", configFile),
			}, true
		}
	}
	return gateVerdict{}, false
}

// resolveOpenCodeConfigDir resolves the OpenCode global config
// directory from the XDG base-directory spec: $XDG_CONFIG_HOME when
// set and absolute, otherwise $HOME/.config. A relative value resolves
// against the process CWD, which can point inside the vault, so it is
// rejected as unverifiable.
func resolveOpenCodeConfigDir(lookup func(string) (string, bool)) (string, error) {
	if v, ok := lookup("XDG_CONFIG_HOME"); ok && v != "" {
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("XDG_CONFIG_HOME %q is not an absolute path", v)
		}
		return filepath.Join(v, "opencode"), nil
	}
	if v, ok := lookup("HOME"); ok && v != "" {
		if !filepath.IsAbs(v) {
			return "", fmt.Errorf("HOME %q is not an absolute path", v)
		}
		return filepath.Join(v, ".config", "opencode"), nil
	}
	return "", errors.New("neither XDG_CONFIG_HOME nor HOME is set")
}

// readPluginAgents reads and interprets a plugin config file. It
// returns the normalized effective agent list and whether the
// documented default applied. The boolean is false when the config is
// unverifiable, in which case the returned verdict carries the
// refusal.
func readPluginAgents(path string, lookup func(string) (string, bool), base gateVerdict) ([]string, bool, gateVerdict, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		base.Reason = fmt.Sprintf("the plugin config exists but cannot be read: %v", err)
		base.Fix = fmt.Sprintf("Make %s readable, then re-run /dewey-distill.", path)
		return nil, false, base, false
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &obj); err != nil {
		base.Reason = fmt.Sprintf("the plugin config is not valid JSONC: %v", err)
		base.Fix = fmt.Sprintf("Fix the JSON syntax in %s, then re-run /dewey-distill.", path)
		return nil, false, base, false
	}
	if obj == nil {
		base.Reason = fmt.Sprintf("%s contains JSON null, not a configuration object", path)
		base.Fix = fmt.Sprintf("Replace the null value in %s with a JSON object, then re-run /dewey-distill.", path)
		return nil, false, base, false
	}

	raw, ok := obj["agents"]
	if !ok {
		return append([]string(nil), defaultAutoAnswerAgents...), true, base, true
	}

	if isJSONNull(raw) {
		base.Reason = fmt.Sprintf("the agents value in %s is JSON null, not an array of strings", path)
		base.Fix = fmt.Sprintf("Set agents to a JSON array of strings in %s, then re-run /dewey-distill.", path)
		return nil, false, base, false
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		base.Reason = fmt.Sprintf("the agents value in %s is not an array of strings", path)
		base.Fix = fmt.Sprintf("Set agents to a JSON array of strings in %s, then re-run /dewey-distill.", path)
		return nil, false, base, false
	}

	agents := make([]string, 0, len(items))
	for _, item := range items {
		if isJSONNull(item) {
			base.Reason = fmt.Sprintf("the agents value in %s contains a null element, not a string", path)
			base.Fix = fmt.Sprintf("Set agents to a JSON array of strings in %s, then re-run /dewey-distill.", path)
			return nil, false, base, false
		}
		var entry string
		if err := json.Unmarshal(item, &entry); err != nil {
			base.Reason = fmt.Sprintf("the agents value in %s is not an array of strings", path)
			base.Fix = fmt.Sprintf("Set agents to a JSON array of strings in %s, then re-run /dewey-distill.", path)
			return nil, false, base, false
		}
		resolved, err := resolveAgentEntry(entry, lookup)
		if err != nil {
			base.Reason = fmt.Sprintf("cannot interpret the agents entry %q in %s: %v", entry, path, err)
			base.Fix = fmt.Sprintf("Fix the agents entry in %s, then re-run /dewey-distill.", path)
			return nil, false, base, false
		}
		agents = append(agents, normalizeAgentEntry(resolved))
	}
	return agents, false, base, true
}

// isJSONNull reports whether a raw JSON value is the literal null.
// encoding/json maps null into a slice or string as a no-op with no
// error, so a null value would otherwise slip through as an empty
// list or empty entry on the fail-closed path.
func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// resolveAgentEntry resolves a full-entry `{env:VAR}` interpolation. A
// partial interpolation, or an unset variable, is unverifiable.
func resolveAgentEntry(entry string, lookup func(string) (string, bool)) (string, error) {
	trimmed := strings.TrimSpace(entry)
	if m := envTokenRe.FindStringSubmatch(trimmed); m != nil {
		v, ok := lookup(m[1])
		if !ok {
			return "", fmt.Errorf("environment variable %s is unset", m[1])
		}
		return v, nil
	}
	if envTokenAnyRe.MatchString(trimmed) {
		return "", errors.New("the entry embeds a partial environment interpolation")
	}
	return trimmed, nil
}

// normalizeAgentEntry trims whitespace and case-folds an agent entry so
// the gate's matching agrees with the plugin's.
func normalizeAgentEntry(entry string) string {
	return strings.ToLower(strings.TrimSpace(entry))
}

// verifyCuratorDefinition confirms the curator binding is resolvable:
// .opencode/agents/curator.md exists under workDir and its frontmatter
// declares mode: subagent.
func verifyCuratorDefinition(workDir string) error {
	path := filepath.Join(workDir, ".opencode", "agents", "curator.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	block, _, err := splitFrontmatter(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	value, ok := frontmatterValue(block, "mode")
	if !ok {
		return fmt.Errorf("%s has no mode in its frontmatter", path)
	}
	if value != "subagent" {
		return fmt.Errorf("%s declares mode %q, not subagent", path, value)
	}
	return nil
}

// frontmatterValue returns a top-level key's value from a YAML
// frontmatter block. It tolerates an optional space before the colon,
// an inline comment, and surrounding quotes.
func frontmatterValue(block, key string) (string, bool) {
	for _, line := range strings.Split(block, "\n") {
		name, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) != key {
			continue
		}
		if i := strings.Index(value, "#"); i >= 0 {
			value = value[:i]
		}
		return strings.Trim(strings.TrimSpace(value), `"'`), true
	}
	return "", false
}

// scanPluginRegistration reports whether opencode-auto-question is
// registered in the project or global OpenCode configs or discovered
// from a plugins directory. The result is advisory context only; an
// unreadable or unparseable config yields a warning, never a refusal.
func scanPluginRegistration(workDir, configDir string) (string, []string) {
	var warnings []string
	registered := false
	for _, path := range pluginConfigCandidates(workDir, configDir) {
		found, warning := scanPluginConfigFile(path)
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if found {
			registered = true
		}
	}
	if scanPluginDirs(pluginDiscoveryDirs(workDir, configDir)) {
		registered = true
	}
	if registered {
		return "registered", warnings
	}
	return "absent", warnings
}

// pluginConfigCandidates lists the project and global OpenCode config
// files scanned for plugin registration.
func pluginConfigCandidates(workDir, configDir string) []string {
	candidates := []string{
		filepath.Join(workDir, ".opencode", "opencode.json"),
		filepath.Join(workDir, ".opencode", "opencode.jsonc"),
		filepath.Join(workDir, "opencode.json"),
		filepath.Join(workDir, "opencode.jsonc"),
	}
	if configDir != "" {
		candidates = append(candidates,
			filepath.Join(configDir, "opencode.json"),
			filepath.Join(configDir, "opencode.jsonc"),
		)
	}
	return candidates
}

// pluginDiscoveryDirs lists the directories scanned for discovered
// plugin entries.
func pluginDiscoveryDirs(workDir, configDir string) []string {
	dirs := []string{filepath.Join(workDir, ".opencode", "plugins")}
	if configDir != "" {
		dirs = append(dirs, filepath.Join(configDir, "plugins"))
	}
	return dirs
}

// scanPluginConfigFile reports whether one OpenCode config registers
// the auto-question plugin, and returns a warning when the file exists
// but cannot be read or parsed.
func scanPluginConfigFile(path string) (bool, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return false, fmt.Sprintf("cannot read %s: %v", path, err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(stripJSONC(data), &obj); err != nil {
		return false, fmt.Sprintf("cannot parse %s: %v", path, err)
	}
	for _, key := range []string{"plugin", "plugins"} {
		if raw, ok := obj[key]; ok && pluginListMatches(raw) {
			return true, ""
		}
	}
	return false, ""
}

// scanPluginDirs reports whether any discovery-directory entry names
// the auto-question plugin.
func scanPluginDirs(dirs []string) bool {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if pluginNameMatches(entry.Name()) {
				return true
			}
		}
	}
	return false
}

// pluginListMatches reports whether a plugin/plugins array names the
// auto-question plugin. Entries may be strings or objects carrying a
// package field.
func pluginListMatches(raw json.RawMessage) bool {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return false
	}
	for _, item := range items {
		var name string
		if err := json.Unmarshal(item, &name); err == nil {
			if pluginNameMatches(name) {
				return true
			}
			continue
		}
		var object struct {
			Package string `json:"package"`
		}
		if err := json.Unmarshal(item, &object); err == nil && object.Package != "" {
			if pluginNameMatches(object.Package) {
				return true
			}
		}
	}
	return false
}

// pluginNameMatches reports whether a plugin reference names the
// auto-question plugin after stripping a leading disable marker and
// comparing the final path segment.
func pluginNameMatches(reference string) bool {
	trimmed := strings.TrimSpace(reference)
	trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	if i := strings.Index(trimmed, "@"); i >= 0 {
		trimmed = trimmed[:i]
	}
	trimmed = strings.TrimPrefix(trimmed, "npm:")
	trimmed = strings.ToLower(trimmed)
	trimmed = strings.TrimSuffix(trimmed, ".js")
	trimmed = strings.TrimSuffix(trimmed, ".ts")
	return trimmed == "opencode-auto-question" || trimmed == "auto-question"
}

// stripJSONC removes // and /* */ comments and trailing commas from a
// JSONC document in one pass, preserving string contents and byte
// positions. Go's stdlib encoding/json does not accept JSONC, so the
// normalizer is hand-rolled to keep the change dependency-free. It
// must run before json.Unmarshal.
func stripJSONC(src []byte) []byte {
	out := append([]byte(nil), src...)
	inString := false
	escaped := false
	lastComma := -1
	valueSinceComma := false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			lastComma = -1
			valueSinceComma = true
		case '/':
			i = blankCommentAt(out, i)
		case ',':
			lastComma = i
			valueSinceComma = false
		case ' ', '\t', '\n', '\r':
			// Whitespace between a comma and a closer is irrelevant.
		default:
			if (c == '}' || c == ']') && lastComma >= 0 && !valueSinceComma {
				out[lastComma] = ' '
			}
			lastComma = -1
			valueSinceComma = true
		}
	}
	return out
}

// blankCommentAt blanks the comment starting at i (a '/'), dispatching
// on the following byte, and returns the last byte consumed.
func blankCommentAt(out []byte, i int) int {
	if i+1 >= len(out) {
		return i
	}
	switch out[i+1] {
	case '/':
		return blankLineComment(out, i)
	case '*':
		return blankBlockComment(out, i)
	}
	return i
}

// blankLineComment blanks a // comment starting at i and returns the
// index of its terminating newline (or the last byte).
func blankLineComment(out []byte, i int) int {
	for i < len(out) && out[i] != '\n' {
		out[i] = ' '
		i++
	}
	return i
}

// blankBlockComment blanks a /* */ comment starting at i and returns
// the index of its closing '/'.
func blankBlockComment(out []byte, i int) int {
	out[i] = ' '
	out[i+1] = ' '
	i += 2
	for i < len(out)-1 && (out[i] != '*' || out[i+1] != '/') {
		if out[i] != '\n' {
			out[i] = ' '
		}
		i++
	}
	if i < len(out)-1 {
		out[i] = ' '
		out[i+1] = ' '
		i++
	}
	return i
}

// renderGateVerdict renders the tool-owned verdict text (DE-003). The
// command relays this verbatim; config-supplied values are sanitized
// so a pathological entry cannot spoof the output.
func renderGateVerdict(v gateVerdict) string {
	var b strings.Builder
	if v.Pass {
		b.WriteString("Auto-answer gate: PASS\n")
	} else {
		b.WriteString("Auto-answer gate: REFUSED\n")
	}
	if v.ConfigFile != "" {
		fmt.Fprintf(&b, "Config file: %s\n", safeValue(v.ConfigFile))
	}
	if v.Pass {
		fmt.Fprintf(&b, "Effective agents: %s\n", formatAgents(v.EffectiveAgents))
		if v.DefaultApplied {
			b.WriteString("Default applied: yes\n")
		} else {
			b.WriteString("Default applied: no\n")
		}
		b.WriteString("Curator definition: resolved\n")
	} else {
		fmt.Fprintf(&b, "Reason: %s\n", safeValue(v.Reason))
		if v.Fix != "" {
			fmt.Fprintf(&b, "Fix: %s\n", safeValue(v.Fix))
		}
	}
	if v.Registration != "" {
		fmt.Fprintf(&b, "Registration: %s\n", safeValue(v.Registration))
	}
	if len(v.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, warning := range v.Warnings {
			fmt.Fprintf(&b, "  - %s\n", safeValue(warning))
		}
	}
	return b.String()
}

// formatAgents renders the effective list with each entry quoted, so
// an entry containing punctuation is unambiguous.
func formatAgents(agents []string) string {
	quoted := make([]string, len(agents))
	for i, agent := range agents {
		quoted[i] = strconv.Quote(agent)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// safeValue strips control characters from config-supplied text so a
// refusal cannot be spoofed with newlines or ANSI sequences.
func safeValue(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// newGateCmd creates the `dewey gate` command group.
func newGateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gate",
		Short: "Deterministic startup gates",
		Long: `Deterministic startup gates consumed by Dewey commands.

The gates print a tool-rendered verdict and communicate the result
through their exit code, so command prose and scripts consume the
verdict directly rather than reconstructing it.`,
	}
	cmd.AddCommand(newGateAutoAnswerCmd())
	return cmd
}

// newGateAutoAnswerCmd creates `dewey gate auto-answer`, the mandatory
// /dewey-distill startup gate.
func newGateAutoAnswerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto-answer",
		Short: "Refuse when the curator agent is configured for automatic question answering",
		Long: `Evaluate whether the opencode-auto-question plugin is configured to
auto-answer questions for the curator agent.

Exit 0 means the curator is not configured for auto-answering (or the
documented default applies and the curator definition resolves). Exit 1
means the gate refuses: the plugin config lists curator or the wildcard,
or the configuration cannot be verified. Refusal is fail-closed — an
unreadable, unparseable, or unresolvable configuration never passes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAutoAnswerGate(cmd, gateOptions{})
		},
	}
	return cmd
}

// runAutoAnswerGate evaluates the gate, writes the tool-rendered
// verdict to the command's output, and maps a refusal to an error so
// the process exits non-zero. A verdict write failure is itself an
// error: the consumer keys on both the exit code and the rendered
// output, so an unverifiable verdict must fail closed.
func runAutoAnswerGate(cmd *cobra.Command, opts gateOptions) error {
	verdict := evaluateAutoAnswerGate(opts)
	if _, err := fmt.Fprint(cmd.OutOrStdout(), renderGateVerdict(verdict)); err != nil {
		return fmt.Errorf("write gate verdict: %w", err)
	}
	if !verdict.Pass {
		return errAutoAnswerGateRefused
	}
	return nil
}
