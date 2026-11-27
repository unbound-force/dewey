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
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/unbound-force/dewey/v3/tools"
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

// autoAnswerConfigFile is the auto-question plugin's config file name
// within an OpenCode global config directory; every candidate path the
// gate evaluates joins it.
const autoAnswerConfigFile = "auto-question.jsonc"

// gateOptions injects the gate's external inputs so evaluation is
// testable without touching the real environment or filesystem
// layout. Empty fields fall back to the process environment and the
// current working directory.
type gateOptions struct {
	// configDir overrides the OpenCode global config directory. When
	// set, evaluation collapses to exactly that one directory — one
	// candidate file, no environment resolution, no additional
	// checked paths — the single-path seam every existing test pins.
	// When empty, evaluation resolves candidates from the environment:
	// the AUTO_QUESTION_CONFIG file when set, the home-directory path
	// always, the OPENCODE_CONFIG_DIR file when set, and the XDG path
	// when XDG_CONFIG_HOME differs from the home directory.
	configDir string
	// workDir is where .opencode/agents/curator.md is looked up.
	// Defaults to the process working directory.
	workDir string
	// lookupEnv reads an environment variable and reports whether it
	// is set. Defaults to os.LookupEnv.
	lookupEnv func(string) (string, bool)
	// supervisorConfigDir overrides the opencode-supervisor global
	// config directory (normally <config root>/opencode/supervisor) for
	// the distill-unattended gate. When set, evaluation checks exactly
	// that directory's config.json and skips environment resolution.
	// Repository-local config is never a candidate in any mode — the
	// widening rule reads global config only (FR-016).
	supervisorConfigDir string
	// vaultDir overrides the vault root for the distill-unattended
	// gate's refusal artifact. Empty selects the working directory —
	// the unattended command dispatches in the repository.
	vaultDir string
}

// gateVerdict is the evaluated outcome rendered by the command.
type gateVerdict struct {
	Pass bool
	// ConfigFile is the first checked config file — the
	// AUTO_QUESTION_CONFIG file when set, else the home-directory
	// candidate, or the injected configDir's file when evaluation
	// collapses to a single path. EffectiveAgents and DefaultApplied
	// follow the plugin's merge rule over the checked files.
	ConfigFile string
	// AdditionalConfigFiles lists the checked config files after
	// ConfigFile, in evaluation order, so the verdict names every
	// file the gate evaluated — on pass and refusal alike.
	AdditionalConfigFiles []string
	EffectiveAgents       []string
	DefaultApplied        bool
	Registration          string
	Warnings              []string
	Reason                string
	Fix                   string
}

// evaluateAutoAnswerGate deterministically evaluates whether the
// curator agent is configured for automatic question answering, per
// CA-FR-007. It checks every file a plugin process could read given
// this environment: the AUTO_QUESTION_CONFIG file when set, the
// home-directory config path always, the OPENCODE_CONFIG_DIR file when
// set, and the XDG path when XDG_CONFIG_HOME resolves elsewhere. The
// effective agent list and default-applied flag follow the plugin's
// merge rule (the AUTO_QUESTION_CONFIG file alone, else the
// OPENCODE_CONFIG_DIR file's agents over the home-directory file's);
// every checked file is a refusal input that no clean file can mask,
// and the verdict names every file checked, on pass and refusal alike. An injected gateOptions.configDir collapses
// evaluation to exactly that one directory and skips environment
// resolution. Every failure path is fail-closed: an unverifiable
// input refuses rather than assuming auto-answering is off.
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

	// Build the ordered candidate list per CA-FR-007: the
	// AUTO_QUESTION_CONFIG file when set, the home-directory file
	// always, the OPENCODE_CONFIG_DIR file when set, and the XDG file
	// when XDG_CONFIG_HOME resolves elsewhere — every file a plugin
	// process could read given this environment. An injected
	// configDir collapses evaluation to exactly that one candidate and
	// skips environment resolution.
	configDir := opts.configDir
	var candidates []gateCandidate
	// scanDir is the single global config directory the advisory
	// registration scan receives, per design D6: the injected
	// configDir when present, else the XDG-first pick from the
	// resolved pair — the same directory the scan received before
	// multi-path evaluation. Registration never changes the verdict,
	// so the scan is deliberately not widened to the full candidate
	// set; that would only churn advisory warning output.
	scanDir := configDir
	if configDir == "" {
		dirs, err := resolvePluginConfigDirs(lookup)
		if err != nil {
			return gateVerdict{
				Reason: "cannot resolve the OpenCode config directory: " + err.Error(),
				Fix:    "Set HOME to a non-empty absolute path, and XDG_CONFIG_HOME to an absolute path when set, then re-run /dewey-distill.",
			}
		}
		candidates, err = resolveGateCandidates(dirs, lookup)
		if err != nil {
			return gateVerdict{
				Reason: "cannot resolve the plugin config location: " + err.Error(),
				Fix:    "Set AUTO_QUESTION_CONFIG and OPENCODE_CONFIG_DIR to absolute paths (or ~/...), or unset them, then re-run /dewey-distill.",
			}
		}
		// XDG-first per D6: the XDG directory when it differs from
		// the home-directory directory, else the home-directory
		// directory — identical strings when the two coincide, so
		// the value the scan receives never changes.
		scanDir = dirs.xdgDir
		if scanDir == "" {
			scanDir = dirs.homeDir
		}
	} else {
		candidates = []gateCandidate{{path: filepath.Join(configDir, autoAnswerConfigFile), role: roleHome}}
	}

	// The first candidate is the verdict's config file — the
	// AUTO_QUESTION_CONFIG file when set, else the home-directory
	// file, or the injected config directory's file. The remaining
	// candidates, in evaluation order, are additionally checked files:
	// the verdict names every checked path on pass and refusal, and an
	// injected configDir yields exactly one candidate so the
	// single-path seam reports none.
	verdict := gateVerdict{ConfigFile: candidates[0].path}
	for _, c := range candidates[1:] {
		verdict.AdditionalConfigFiles = append(verdict.AdditionalConfigFiles, c.path)
	}
	// Advisory only: registration state never changes the verdict.
	verdict.Registration, verdict.Warnings = scanPluginRegistration(workDir, scanDir)

	// Evaluate every candidate before deciding, then refuse on the
	// first offending or unverifiable file in candidate order, so the
	// refusal names exactly one deterministic file.
	results := make([]gateCandidateResult, 0, len(candidates))
	for _, c := range candidates {
		results = append(results, evaluateGateCandidate(c, lookup, verdict))
	}

	// The reported effective agents and default-applied flag are the
	// ones the plugin computes (design D3); refusal below is a union
	// over every candidate regardless.
	verdict.EffectiveAgents, verdict.DefaultApplied = effectiveGateAgents(candidates, results)

	// No clean file masks an offending or unverifiable one.
	for _, result := range results {
		if !result.ok {
			return result.refusal
		}
		if refusal, refused := agentListRefusal(result.agents, result.path); refused {
			verdict.Reason = refusal.Reason
			verdict.Fix = refusal.Fix
			return verdict
		}
	}

	if err := verifyCuratorDefinition(workDir); err != nil {
		verdict.Reason = "the curator agent definition is not resolvable: " + err.Error()
		verdict.Fix = "Ensure .opencode/agents/curator.md exists with `mode: subagent`, then re-run /dewey-distill."
		return verdict
	}

	verdict.Pass = true
	return verdict
}

// gateCandidateResult is the per-file outcome of evaluating one plugin
// config candidate: the normalized effective agent list when the file
// is verifiable, or the fail-closed refusal verdict naming it when it
// is not. Every candidate's result is a refusal input — an offending
// or unverifiable file at any checked path refuses the gate — while
// the reported effective agents come from the plugin's merge rule
// (effectiveGateAgents).
type gateCandidateResult struct {
	// path is the evaluated candidate; a refusal names it so the
	// reason and the fix point at exactly one deterministic file.
	path string
	// agents is the normalized effective agent list, valid when ok.
	agents []string
	// defaultApplied reports whether the documented default applied.
	defaultApplied bool
	// refusal is the fail-closed verdict for an unverifiable file,
	// valid when ok is false.
	refusal gateVerdict
	// ok reports whether the candidate could be verified.
	ok bool
}

// gateCandidateRole is the plugin config source a checked file stands
// for. It decides the file's part in the reported effective agent list
// (design D3) and whether a missing file refuses (design D4).
type gateCandidateRole int

const (
	// roleExplicit is the AUTO_QUESTION_CONFIG file: the plugin reads
	// it alone, and a missing one is an error.
	roleExplicit gateCandidateRole = iota
	// roleHome is $HOME/.config/opencode/auto-question.jsonc.
	roleHome
	// roleConfigDir is $OPENCODE_CONFIG_DIR/auto-question.jsonc,
	// merged over the home-directory file.
	roleConfigDir
	// roleXDG is $XDG_CONFIG_HOME/opencode/auto-question.jsonc: never
	// read by the plugin, checked as a refusal input only.
	roleXDG
)

// gateCandidate is one checked plugin config file and its source.
type gateCandidate struct {
	path string
	role gateCandidateRole
}

// resolveGateCandidates returns the ordered plugin config candidates
// per CA-FR-007 and design D1: the AUTO_QUESTION_CONFIG file when set,
// the home-directory file always, the OPENCODE_CONFIG_DIR file when
// set, and the XDG file when dirs carries one. A candidate whose
// cleaned path equals an earlier one's is dropped (lexical comparison,
// no symlink evaluation: aliasing can only over-check). With neither
// variable set the result is exactly the home-then-XDG pair evaluated
// before. A value that is relative after ~ expansion is an error: the
// plugin resolves it against a working directory the gate cannot
// observe.
func resolveGateCandidates(dirs pluginConfigDirs, lookup func(string) (string, bool)) ([]gateCandidate, error) {
	home, _ := lookup("HOME") // validated absolute by resolvePluginConfigDirs
	var candidates []gateCandidate
	add := func(path string, role gateCandidateRole) {
		path = filepath.Clean(path)
		for _, c := range candidates {
			if c.path == path {
				return
			}
		}
		candidates = append(candidates, gateCandidate{path: path, role: role})
	}

	explicit, err := pluginSourceEnv(lookup, "AUTO_QUESTION_CONFIG", home)
	if err != nil {
		return nil, err
	}
	configDir, err := pluginSourceEnv(lookup, "OPENCODE_CONFIG_DIR", home)
	if err != nil {
		return nil, err
	}
	if explicit != "" {
		add(explicit, roleExplicit)
	}
	add(filepath.Join(dirs.homeDir, autoAnswerConfigFile), roleHome)
	if configDir != "" {
		add(filepath.Join(configDir, autoAnswerConfigFile), roleConfigDir)
	}
	if dirs.xdgDir != "" {
		add(filepath.Join(dirs.xdgDir, autoAnswerConfigFile), roleXDG)
	}
	return candidates, nil
}

// pluginSourceEnv reads one of the plugin's config-location variables
// the way the plugin does: trimmed, blank meaning unset, and ~ or ~/...
// expanded against home. It returns "" when the variable is unset or
// blank, and an error when the expanded value is not absolute.
func pluginSourceEnv(lookup func(string) (string, bool), name, home string) (string, error) {
	raw, ok := lookup(name)
	value := strings.TrimSpace(raw)
	if !ok || value == "" {
		return "", nil
	}
	if value == "~" {
		value = home
	} else if strings.HasPrefix(value, "~/") {
		value = filepath.Join(home, value[2:])
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s %q is not an absolute path", name, raw)
	}
	return value, nil
}

// effectiveGateAgents returns the effective agent list and
// default-applied flag the plugin computes (design D3): the
// AUTO_QUESTION_CONFIG file alone when it is a candidate; otherwise
// the OPENCODE_CONFIG_DIR file when it defines agents, else the
// home-directory file. The XDG file never contributes. Results that
// could not be verified contribute nothing; they refuse the gate.
func effectiveGateAgents(candidates []gateCandidate, results []gateCandidateResult) ([]string, bool) {
	var deciding *gateCandidateResult
	for i, c := range candidates {
		result := &results[i]
		switch c.role {
		case roleExplicit:
			if result.ok {
				return result.agents, result.defaultApplied
			}
			return nil, false
		case roleHome:
			if deciding == nil && result.ok {
				deciding = result
			}
		case roleConfigDir:
			if result.ok && !result.defaultApplied {
				deciding = result
			}
		}
	}
	if deciding == nil {
		return nil, false
	}
	return deciding.agents, deciding.defaultApplied
}

// evaluateGateCandidate evaluates one plugin config candidate
// through the existing per-file logic — stat, JSONC parse, agents
// extraction, {env:VAR} interpolation, trim and case-fold — wrapping
// resolvePluginAgents so the candidate loop in evaluateAutoAnswerGate
// stays focused. base is the partial verdict carrying the checked-file
// set; a per-file refusal inherits it, so a refusal still names every
// file the gate checked.
func evaluateGateCandidate(c gateCandidate, lookup func(string) (string, bool), base gateVerdict) gateCandidateResult {
	path := c.path
	if c.role == roleExplicit {
		// The plugin treats a missing AUTO_QUESTION_CONFIG file as an
		// error (design D4): refuse rather than apply the default.
		if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
			base.Reason = fmt.Sprintf("AUTO_QUESTION_CONFIG names %s, which does not exist", path)
			base.Fix = "Point AUTO_QUESTION_CONFIG at an existing plugin config or unset it, then re-run /dewey-distill."
			return gateCandidateResult{path: path, refusal: base}
		}
	}
	agents, defaultApplied, refusal, ok := resolvePluginAgents(path, lookup, base)
	return gateCandidateResult{
		path:           path,
		agents:         agents,
		defaultApplied: defaultApplied,
		refusal:        refusal,
		ok:             ok,
	}
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

// pluginConfigDirs is the ordered pair of OpenCode global config
// directories the gate evaluates, per CA-FR-007: the home-directory
// directory is always present, and the XDG directory joins the pair
// only when XDG_CONFIG_HOME resolves elsewhere. The AUTO_QUESTION_CONFIG
// and OPENCODE_CONFIG_DIR sources are added by resolveGateCandidates.
type pluginConfigDirs struct {
	// homeDir is $HOME/.config/opencode — the path the plugin reads
	// unless AUTO_QUESTION_CONFIG is set, so it is never skipped.
	homeDir string
	// xdgDir is $XDG_CONFIG_HOME/opencode when XDG_CONFIG_HOME is set,
	// non-empty, absolute, and lexically different from homeDir; empty
	// when XDG_CONFIG_HOME yields no second candidate.
	xdgDir string
}

// resolvePluginConfigDirs resolves the ordered pair of OpenCode plugin
// config directories, per CA-FR-007. The home directory,
// $HOME/.config/opencode, is always resolved: the plugin loads
// join(homedir(), ".config", "opencode") unless AUTO_QUESTION_CONFIG is
// set, and ~ in the other sources expands against it, so HOME must be
// set, non-empty, and absolute — including when
// XDG_CONFIG_HOME is set, because an unresolvable HOME leaves the path
// the plugin actually reads unverifiable. The XDG directory,
// $XDG_CONFIG_HOME/opencode, joins the pair only when XDG_CONFIG_HOME
// is set, non-empty, absolute, and lexically different from the home
// directory; an unset or empty XDG_CONFIG_HOME yields no XDG directory.
// Coincidence is decided by comparing the constructed directory paths
// as strings, never by evaluating symlinks: lexical comparison can only
// over-check, it can never treat two different files as one. A relative
// HOME or XDG_CONFIG_HOME fails resolution: a relative value resolves
// against the process CWD, which can point inside the vault, so it is
// rejected as unverifiable.
func resolvePluginConfigDirs(lookup func(string) (string, bool)) (pluginConfigDirs, error) {
	home, ok := lookup("HOME")
	if !ok || home == "" {
		return pluginConfigDirs{}, errors.New("HOME is not set or is empty")
	}
	if !filepath.IsAbs(home) {
		return pluginConfigDirs{}, fmt.Errorf("HOME %q is not an absolute path", home)
	}
	dirs := pluginConfigDirs{homeDir: filepath.Join(home, ".config", "opencode")}

	xdg, ok := lookup("XDG_CONFIG_HOME")
	if !ok || xdg == "" {
		return dirs, nil
	}
	if !filepath.IsAbs(xdg) {
		return pluginConfigDirs{}, fmt.Errorf("XDG_CONFIG_HOME %q is not an absolute path", xdg)
	}
	if xdgDir := filepath.Join(xdg, "opencode"); xdgDir != dirs.homeDir {
		dirs.xdgDir = xdgDir
	}
	return dirs, nil
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
		for _, path := range v.AdditionalConfigFiles {
			fmt.Fprintf(&b, "Also checked: %s\n", safeValue(path))
		}
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
	cmd.AddCommand(newGateDistillUnattendedCmd())
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

// --- distill-unattended gate (018-weekly-distill-automation, distill-auto-runner-neutral) ---

// supervisorConfigFile is the opencode-supervisor global config file
// name. opencode-supervisor (v2.9.1+, jflowers/opencode-supervisor#51)
// reads command-scoped profiles only from this one strict-JSON file in
// its global config directory (see supervisorConfigPath); profiles in
// repository-local config are never honored.
const supervisorConfigFile = "config.json"

// distillCommandProfile is the slash-command name a supervisor profile's
// match.command must name (FR-016): only a global opencode-supervisor
// profile matching this command may narrow or widen the distiller's
// allowlist.
const distillCommandProfile = "dewey-distill-auto"

// errDistillUnattendedGateRefused is returned by `gate
// distill-unattended` when the gate refuses. main() logs it and exits 1;
// the tool-rendered verdict was already printed to stdout, so the error
// carries no user-facing detail.
var errDistillUnattendedGateRefused = errors.New("distill-unattended gate refused")

// distillUnattendedVerdict is the evaluated outcome rendered by the
// distill-unattended gate.
type distillUnattendedVerdict struct {
	Pass bool
	// ConfigFile is the one global supervisor config file the gate
	// checked — named on pass and refusal alike. Empty only when the
	// location could not be resolved. Repository-local config is never
	// a candidate (FR-016: only global config can widen).
	ConfigFile string
	// Reason carries the refusal cause.
	Reason string
	// Fix names the remediation on refusal.
	Fix string
}

// supervisorConfigPath resolves the one config file opencode-supervisor
// reads, exactly as the supervisor resolves it:
// $XDG_CONFIG_HOME/opencode/supervisor/config.json when XDG_CONFIG_HOME
// is set, non-blank and absolute; otherwise
// $HOME/.config/opencode/supervisor/config.json. A blank or relative
// XDG_CONFIG_HOME is ignored, the same as unset (the supervisor ignores
// it too). A missing or relative HOME, when the home path is needed, is
// unverifiable and fails resolution.
func supervisorConfigPath(lookup func(string) (string, bool)) (string, error) {
	if xdg, ok := lookup("XDG_CONFIG_HOME"); ok && strings.TrimSpace(xdg) != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "opencode", "supervisor", supervisorConfigFile), nil
	}
	home, ok := lookup("HOME")
	if !ok || home == "" {
		return "", errors.New("HOME is not set or is empty")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("HOME %q is not an absolute path", home)
	}
	return filepath.Join(home, ".config", "opencode", "supervisor", supervisorConfigFile), nil
}

// supervisorProfileFix is the remediation for a config that does not
// carry the distill command profile: it names the file and the minimal
// profile to add.
func supervisorProfileFix(path string) string {
	return fmt.Sprintf(`Add a profile matching the command to %s, e.g. {"profiles": [{"name": "distill", "match": {"command": "%s"}}]}, then re-run /dewey-distill-auto.`, path, distillCommandProfile)
}

// supervisorHasDistillProfile reports whether the supervisor config
// carries a profile whose match.command, trimmed and with its leading
// "/"s stripped (the supervisor's own normalization), names
// dewey-distill-auto. The file must be strict JSON with an object at
// the top level; a "profiles" key that is present but not an array is
// an error (fail-closed). An absent or null "profiles" is simply no
// profile. Elements are decoded leniently — a non-object element or a
// missing or non-string match.command does not match — so only a match
// is decisive, mirroring the supervisor's find-first-match. The
// profile's other contents (traceCheck, ...) are the supervisor's
// contract, not the gate's.
func supervisorHasDistillProfile(data []byte) (bool, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return false, fmt.Errorf("not valid JSON with an object at the top level: %w", err)
	}
	raw, ok := top["profiles"]
	if !ok || isJSONNull(raw) {
		return false, nil
	}
	var profiles []json.RawMessage
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return false, errors.New(`"profiles" is not an array`)
	}
	for _, p := range profiles {
		var profile struct {
			Match struct {
				Command any `json:"command"`
			} `json:"match"`
		}
		if json.Unmarshal(p, &profile) != nil {
			continue
		}
		command, ok := profile.Match.Command.(string)
		if !ok {
			continue
		}
		if strings.TrimLeft(strings.TrimSpace(command), "/") == distillCommandProfile {
			return true, nil
		}
	}
	return false, nil
}

// evaluateDistillUnattendedGate deterministically evaluates whether the
// global opencode-supervisor config carries a profile matching the
// dewey-distill-auto command (FR-016). It checks exactly the one file
// the supervisor reads (supervisorConfigPath) — repository-local config
// is never read, so repository content cannot widen the distiller's
// privileges. Every failure path is fail-closed: an absent file or a
// file without the profile refuses with the supervisor-unavailable
// reason; an unreadable file, invalid JSON, or a non-array "profiles"
// refuses naming the file. An injected gateOptions.supervisorConfigDir
// collapses evaluation to that directory's config.json.
func evaluateDistillUnattendedGate(opts gateOptions) distillUnattendedVerdict {
	lookup := opts.lookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	var path string
	if opts.supervisorConfigDir != "" {
		path = filepath.Join(opts.supervisorConfigDir, supervisorConfigFile)
	} else {
		var err error
		path, err = supervisorConfigPath(lookup)
		if err != nil {
			return distillUnattendedVerdict{
				Reason: "cannot resolve the supervisor config location: " + err.Error(),
				Fix:    "Set HOME to a non-empty absolute path (or XDG_CONFIG_HOME to an absolute path), then re-run /dewey-distill-auto.",
			}
		}
	}
	verdict := distillUnattendedVerdict{ConfigFile: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			verdict.Reason = tools.NoOpReasonSupervisorUnavailable
			verdict.Fix = supervisorProfileFix(path)
			return verdict
		}
		verdict.Reason = fmt.Sprintf("the supervisor config exists but cannot be read: %v", err)
		verdict.Fix = fmt.Sprintf("Make %s readable, then re-run /dewey-distill-auto.", path)
		return verdict
	}
	has, err := supervisorHasDistillProfile(data)
	if err != nil {
		verdict.Reason = "the supervisor config is invalid: " + err.Error()
		verdict.Fix = fmt.Sprintf(`Fix %s so it is strict JSON with a "profiles" array, then re-run /dewey-distill-auto.`, path)
		return verdict
	}
	if !has {
		verdict.Reason = tools.NoOpReasonSupervisorUnavailable
		verdict.Fix = supervisorProfileFix(path)
		return verdict
	}
	verdict.Pass = true
	return verdict
}

// recordSupervisorNoOp writes the FR-021 no-op artifact for a refused
// distill-unattended gate: a run that never started is still a run, and
// the artifact is its sole audit surface. Returns the artifact path.
func recordSupervisorNoOp(vaultDir string) (string, error) {
	record := tools.DistillRunRecord{
		RunAt:      time.Now().UTC(),
		ScopeSize:  0,
		Themes:     []string{},
		PRsTouched: []string{},
		NoOpReason: tools.NoOpReasonSupervisorUnavailable,
	}
	return tools.WriteDistillRunArtifact(vaultDir, record)
}

// renderDistillUnattendedVerdict renders the tool-owned verdict text.
// The command relays this verbatim; config-supplied values are
// sanitized so a pathological entry cannot spoof the output. On
// refusal the verdict carries the checked file, reason, fix and the
// no-op artifact path (when recorded).
func renderDistillUnattendedVerdict(v distillUnattendedVerdict, artifact string) string {
	var b strings.Builder
	if v.Pass {
		b.WriteString("Distill-unattended gate: PASS\n")
		fmt.Fprintf(&b, "Config file: %s\n", safeValue(v.ConfigFile))
		fmt.Fprintf(&b, "Command profile: %s\n", distillCommandProfile)
		return b.String()
	}
	b.WriteString("Distill-unattended gate: REFUSED\n")
	if v.ConfigFile != "" {
		fmt.Fprintf(&b, "Checked: %s\n", safeValue(v.ConfigFile))
	}
	fmt.Fprintf(&b, "Reason: %s\n", safeValue(v.Reason))
	if v.Fix != "" {
		fmt.Fprintf(&b, "Fix: %s\n", safeValue(v.Fix))
	}
	if artifact != "" {
		fmt.Fprintf(&b, "Artifact: %s\n", safeValue(artifact))
	}
	return b.String()
}

// runDistillUnattendedGate evaluates the gate, records the refusal
// no-op artifact into the vault (working directory — the unattended
// command dispatches in the repository; the same CWD convention the
// auto-answer gate uses for the curator definition), writes the
// tool-rendered verdict, and maps a refusal to an error so the process
// exits non-zero. A verdict-write failure is itself an error: the
// consumer keys on both the exit code and the rendered output, so an
// unverifiable verdict must fail closed. An artifact-write failure
// does not change the refusal — it surfaces as a warning line in the
// verdict so the audit gap is visible, not silent.
func runDistillUnattendedGate(cmd *cobra.Command, opts gateOptions) error {
	verdict := evaluateDistillUnattendedGate(opts)
	var artifact string
	var artifactWarning string
	if !verdict.Pass {
		vaultDir := opts.vaultDir
		if vaultDir == "" {
			wd, err := os.Getwd()
			if err != nil {
				artifactWarning = "could not resolve the working directory for the no-op artifact: " + err.Error()
			} else {
				vaultDir = wd
			}
		}
		if vaultDir != "" {
			path, err := recordSupervisorNoOp(vaultDir)
			if err != nil {
				artifactWarning = "could not record the no-op artifact: " + err.Error()
			} else {
				artifact = path
			}
		}
	}
	out := renderDistillUnattendedVerdict(verdict, artifact)
	if artifactWarning != "" {
		out += "Warning: " + artifactWarning + "\n"
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), out); err != nil {
		return fmt.Errorf("write gate verdict: %w", err)
	}
	if !verdict.Pass {
		return errDistillUnattendedGateRefused
	}
	return nil
}

// newGateDistillUnattendedCmd creates `dewey gate distill-unattended`,
// the /dewey-distill-auto startup gate (018-weekly-distill-automation
// FR-016/FR-017 fail-closed posture).
func newGateDistillUnattendedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "distill-unattended",
		Short: "Refuse until the global opencode-supervisor command profile exists",
		Long: `Evaluate whether the global opencode-supervisor config carries a
profile matching the dewey-distill-auto command — the sole authority that
may widen the distiller agent's allowlist (FR-016).

The gate checks exactly the one file the supervisor reads:
$XDG_CONFIG_HOME/opencode/supervisor/config.json when XDG_CONFIG_HOME is
set, non-blank and absolute, otherwise
$HOME/.config/opencode/supervisor/config.json. It passes when that file
is strict JSON whose "profiles" array holds a profile with
"match": {"command": "dewey-distill-auto"} (a leading "/" is ignored):

  {"profiles": [{"name": "distill",
                 "match": {"command": "dewey-distill-auto"},
                 "traceCheck": {"exemptTools": ["dewey_*", "read", "grep", "glob"]}}]}

Exit 0 means the profile is present. Exit 1 means the gate refuses: the
file is absent, has no matching profile, or is unreadable or invalid.
Refusal is fail-closed — unattended mode stays disabled and the gate
records a no-op run artifact under .uf/dewey/distill-runs/ in the
working directory. Repository-local config is never read: repository
content cannot widen the allowlist.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDistillUnattendedGate(cmd, gateOptions{})
		},
	}
	return cmd
}
