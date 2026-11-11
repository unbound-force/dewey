package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/spf13/cobra"
)

// mapEnv returns a lookupEnv backed by a fixed map.
func mapEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

// newGateFixture creates a temporary work directory with a resolvable
// curator definition and a config directory, returning both.
func newGateFixture(t *testing.T, curatorMode string) (workDir, configDir string) {
	t.Helper()
	workDir = t.TempDir()
	configDir = t.TempDir()
	agentsDir := filepath.Join(workDir, ".opencode", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("create agents dir: %v", err)
	}
	definition := "---\ndescription: test agent\nmode: " + curatorMode + "\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "curator.md"), []byte(definition), 0o644); err != nil {
		t.Fatalf("write curator definition: %v", err)
	}
	return workDir, configDir
}

// writePluginConfig writes auto-question.jsonc into configDir.
func writePluginConfig(t *testing.T, configDir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(configDir, "auto-question.jsonc"), []byte(content), 0o644); err != nil {
		t.Fatalf("write plugin config: %v", err)
	}
}

func TestGateAutoAnswer_AbsentConfigPassesOnDefault(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if !v.Pass {
		t.Fatalf("expected pass on absent config, got refusal: %s", v.Reason)
	}
	if !v.DefaultApplied {
		t.Error("expected the documented default to apply")
	}
	if len(v.EffectiveAgents) != 1 || v.EffectiveAgents[0] != "autonomy" {
		t.Errorf("effective agents = %v, want [autonomy]", v.EffectiveAgents)
	}
}

func TestGateAutoAnswer_CleanListPasses(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	writePluginConfig(t, configDir, `{"agents": ["autonomy", "gaze-reporter"]}`)
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if !v.Pass {
		t.Fatalf("expected pass on clean list, got refusal: %s", v.Reason)
	}
	if v.DefaultApplied {
		t.Error("default should not apply when a list is configured")
	}
	if len(v.EffectiveAgents) != 2 || v.EffectiveAgents[0] != "autonomy" || v.EffectiveAgents[1] != "gaze-reporter" {
		t.Errorf("effective agents = %v, want [autonomy gaze-reporter]", v.EffectiveAgents)
	}
}

func TestGateAutoAnswer_CuratorListedRefused(t *testing.T) {
	for _, entry := range []string{"curator", "Curator", " curator ", "CURATOR"} {
		t.Run(entry, func(t *testing.T) {
			workDir, configDir := newGateFixture(t, "subagent")
			writePluginConfig(t, configDir, `{"agents": ["autonomy", "`+entry+`"]}`)
			v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
			if v.Pass {
				t.Fatalf("expected refusal for entry %q", entry)
			}
			if !strings.Contains(v.Reason, "curator") {
				t.Errorf("reason %q does not name curator", v.Reason)
			}
			if v.Fix == "" {
				t.Error("refusal must name a fix")
			}
		})
	}
}

func TestGateAutoAnswer_WildcardRefused(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	writePluginConfig(t, configDir, `{"agents": ["*"]}`)
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if v.Pass {
		t.Fatal("expected refusal for wildcard")
	}
}

func TestGateAutoAnswer_UnreadableConfigRefused(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: file permissions do not reject reads")
	}
	workDir, configDir := newGateFixture(t, "subagent")
	path := filepath.Join(configDir, "auto-question.jsonc")
	writePluginConfig(t, configDir, `{"agents": ["autonomy"]}`)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if v.Pass {
		t.Fatal("expected refusal on unreadable config")
	}
}

func TestGateAutoAnswer_InvalidJSONCRefused(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	writePluginConfig(t, configDir, `{"agents": [`)
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if v.Pass {
		t.Fatal("expected refusal on unparseable config")
	}
}

func TestGateAutoAnswer_AgentsNotStringArrayRefused(t *testing.T) {
	for name, content := range map[string]string{
		"string value":  `{"agents": "curator"}`,
		"number member": `{"agents": [1, 2]}`,
		"object member": `{"agents": [{"name": "autonomy"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			workDir, configDir := newGateFixture(t, "subagent")
			writePluginConfig(t, configDir, content)
			v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
			if v.Pass {
				t.Fatalf("expected refusal for %s", name)
			}
		})
	}
}

func TestGateAutoAnswer_EnvInterpolation(t *testing.T) {
	t.Run("resolves to curator", func(t *testing.T) {
		workDir, configDir := newGateFixture(t, "subagent")
		writePluginConfig(t, configDir, `{"agents": ["{env:AGENT}"]}`)
		v := evaluateAutoAnswerGate(gateOptions{
			configDir: configDir,
			workDir:   workDir,
			lookupEnv: mapEnv(map[string]string{"AGENT": "curator"}),
		})
		if v.Pass {
			t.Fatal("expected refusal when interpolation resolves to curator")
		}
	})
	t.Run("unset variable", func(t *testing.T) {
		workDir, configDir := newGateFixture(t, "subagent")
		writePluginConfig(t, configDir, `{"agents": ["{env:UNSET}"]}`)
		v := evaluateAutoAnswerGate(gateOptions{
			configDir: configDir,
			workDir:   workDir,
			lookupEnv: mapEnv(map[string]string{}),
		})
		if v.Pass {
			t.Fatal("expected refusal when the variable is unset")
		}
	})
}

func TestGateAutoAnswer_PartialInterpolationRefused(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	writePluginConfig(t, configDir, `{"agents": ["prefix-{env:X}"]}`)
	v := evaluateAutoAnswerGate(gateOptions{
		configDir: configDir,
		workDir:   workDir,
		lookupEnv: mapEnv(map[string]string{"X": "curator"}),
	})
	if v.Pass {
		t.Fatal("expected refusal for a partial interpolation")
	}
}

func TestGateAutoAnswer_ConfigDirUnresolvableRefused(t *testing.T) {
	workDir, _ := newGateFixture(t, "subagent")
	v := evaluateAutoAnswerGate(gateOptions{
		workDir:   workDir,
		lookupEnv: mapEnv(map[string]string{}),
	})
	if v.Pass {
		t.Fatal("expected refusal when neither XDG_CONFIG_HOME nor HOME is set")
	}
	if !strings.Contains(v.Reason, "config directory") {
		t.Errorf("reason %q does not name the config directory", v.Reason)
	}
}

func TestGateAutoAnswer_RelativeXDGRefused(t *testing.T) {
	workDir, _ := newGateFixture(t, "subagent")
	v := evaluateAutoAnswerGate(gateOptions{
		workDir:   workDir,
		lookupEnv: mapEnv(map[string]string{"XDG_CONFIG_HOME": "relative/config"}),
	})
	if v.Pass {
		t.Fatal("expected refusal for a relative XDG_CONFIG_HOME")
	}
}

func TestGateAutoAnswer_StatFailureRefused(t *testing.T) {
	workDir, _ := newGateFixture(t, "subagent")
	// A regular file used as the config directory makes the stat of
	// configDir/auto-question.jsonc fail with ENOTDIR.
	notADir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	v := evaluateAutoAnswerGate(gateOptions{configDir: notADir, workDir: workDir})
	if v.Pass {
		t.Fatal("expected refusal when the config path cannot be stat'd")
	}
}

func TestGateAutoAnswer_CuratorDefinitionMissingRefused(t *testing.T) {
	_, configDir := newGateFixture(t, "subagent")
	emptyWork := t.TempDir()
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: emptyWork})
	if v.Pass {
		t.Fatal("expected refusal when the curator definition is missing")
	}
}

func TestGateAutoAnswer_CuratorDefinitionWrongModeRefused(t *testing.T) {
	workDir, configDir := newGateFixture(t, "primary")
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if v.Pass {
		t.Fatal("expected refusal when the curator definition is not a subagent")
	}
}

func TestGateAutoAnswer_JSONCCommentsAndTrailingCommas(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	writePluginConfig(t, configDir, `{
		// a comment
		"agents": [
			"autonomy", // inline comment
			"gaze-reporter",
		],
	}`)
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	if !v.Pass {
		t.Fatalf("expected pass on valid JSONC, got refusal: %s", v.Reason)
	}
	if len(v.EffectiveAgents) != 2 {
		t.Errorf("effective agents = %v, want two entries", v.EffectiveAgents)
	}
}

func TestGateAutoAnswer_RegistrationAdvisory(t *testing.T) {
	t.Run("registered", func(t *testing.T) {
		workDir, configDir := newGateFixture(t, "subagent")
		config := `{"plugin": ["opencode-auto-question"]}`
		if err := os.WriteFile(filepath.Join(workDir, ".opencode", "opencode.json"), []byte(config), 0o644); err != nil {
			t.Fatalf("write project config: %v", err)
		}
		v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
		if !v.Pass {
			t.Fatalf("registration must not change the verdict: %s", v.Reason)
		}
		if v.Registration != "registered" {
			t.Errorf("registration = %q, want registered", v.Registration)
		}
	})
	t.Run("unparseable scan warns without changing verdict", func(t *testing.T) {
		workDir, configDir := newGateFixture(t, "subagent")
		if err := os.WriteFile(filepath.Join(workDir, ".opencode", "opencode.json"), []byte("{not json"), 0o644); err != nil {
			t.Fatalf("write project config: %v", err)
		}
		v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
		if !v.Pass {
			t.Fatalf("an unparseable scanned config must not change the verdict: %s", v.Reason)
		}
		if len(v.Warnings) == 0 {
			t.Error("expected a warning naming the unparseable scanned config")
		}
	})
}

func TestGateAutoAnswer_PluginArrayForms(t *testing.T) {
	cases := map[string]string{
		"v1 plugin array":     `{"plugin": ["opencode-auto-question"]}`,
		"v2 plugins array":    `{"plugins": ["opencode-auto-question"]}`,
		"object package":      `{"plugins": [{"package": "opencode-auto-question"}]}`,
		"disable marker":      `{"plugins": ["-opencode-auto-question"]}`,
		"scoped with version": `{"plugins": ["npm:opencode-auto-question@1.2.3"]}`,
		"final segment":       `{"plugins": ["./plugins/opencode-auto-question.js"]}`,
		"unrelated":           `{"plugins": ["some-other-plugin"]}`,
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			workDir, configDir := newGateFixture(t, "subagent")
			if err := os.WriteFile(filepath.Join(workDir, ".opencode", "opencode.json"), []byte(config), 0o644); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
			wantRegistered := name != "unrelated"
			if (v.Registration == "registered") != wantRegistered {
				t.Errorf("registration = %q, want registered=%v", v.Registration, wantRegistered)
			}
		})
	}
}

func TestStripJSONC_PreservesStringContents(t *testing.T) {
	src := []byte(`{"agents": ["a//b", "c/*d*/e"], /* tail */}`)
	got := string(stripJSONC(src))
	if !strings.Contains(got, `"a//b"`) || !strings.Contains(got, `"c/*d*/e"`) {
		t.Errorf("string contents were corrupted: %s", got)
	}
	if !strings.Contains(got, `"agents"`) {
		t.Errorf("structure was corrupted: %s", got)
	}
}

func TestRenderGateVerdict_Pass(t *testing.T) {
	rendered := renderGateVerdict(gateVerdict{
		Pass:            true,
		ConfigFile:      "/tmp/x.jsonc",
		EffectiveAgents: []string{"autonomy", "gaze-reporter"},
		DefaultApplied:  false,
		Registration:    "registered",
		Warnings:        []string{"cannot read /tmp/y: permission denied"},
	})
	for _, want := range []string{
		"Auto-answer gate: PASS",
		"Config file: /tmp/x.jsonc",
		`Effective agents: ["autonomy", "gaze-reporter"]`,
		"Default applied: no",
		"Curator definition: resolved",
		"Registration: registered",
		"Warnings:",
		"cannot read /tmp/y",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("pass verdict missing %q:\n%s", want, rendered)
		}
	}

	defaulted := renderGateVerdict(gateVerdict{
		Pass:            true,
		ConfigFile:      "c",
		EffectiveAgents: []string{"autonomy"},
		DefaultApplied:  true,
	})
	if !strings.Contains(defaulted, "Default applied: yes") {
		t.Error("default-applied yes missing from pass verdict")
	}
}

func TestRenderGateVerdict_SanitizesControlChars(t *testing.T) {
	rendered := renderGateVerdict(gateVerdict{
		Pass:       false,
		ConfigFile: "/tmp/x.jsonc",
		Reason:     "bad\nreason\x1b[31m",
		Fix:        "do the fix",
	})
	if strings.Contains(rendered, "\x1b") {
		t.Error("ANSI escape sequence was not stripped from the verdict")
	}
	if !strings.Contains(rendered, "Auto-answer gate: REFUSED") {
		t.Error("verdict header missing")
	}
}

func TestGateAutoAnswerCmd_RejectsArguments(t *testing.T) {
	cmd := newGateAutoAnswerCmd()
	cmd.SetArgs([]string{"extra"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected an error when arguments are supplied")
	}
}

// assertRefusal checks the documented refusal payload (CA-FR-007): a
// non-pass verdict that names the config file, a reason, and a fix.
func assertRefusal(t *testing.T, v gateVerdict) {
	t.Helper()
	if v.Pass {
		t.Fatal("expected a refusal verdict")
	}
	if v.ConfigFile == "" {
		t.Error("refusal must name the config file")
	}
	if v.Reason == "" {
		t.Error("refusal must name a reason")
	}
	if v.Fix == "" {
		t.Error("refusal must name a corrective action")
	}
}

func TestGateAutoAnswer_JSONNullRefused(t *testing.T) {
	cases := map[string]string{
		"root null":       `null`,
		"agents null":     `{"agents": null}`,
		"null element":    `{"agents": [null]}`,
		"null plus entry": `{"agents": ["autonomy", null]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			workDir, configDir := newGateFixture(t, "subagent")
			writePluginConfig(t, configDir, content)
			v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
			assertRefusal(t, v)
		})
	}
}

func TestGateAutoAnswer_ConfigPathIsDirectoryRefused(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	if err := os.Mkdir(filepath.Join(configDir, "auto-question.jsonc"), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	assertRefusal(t, v)
}

func TestGateAutoAnswer_NonRegularFileRefused(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	path := filepath.Join(configDir, "auto-question.jsonc")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("cannot create FIFO: %v", err)
	}
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	assertRefusal(t, v)
}

func TestGateAutoAnswer_TolerantModeParsing(t *testing.T) {
	cases := map[string]string{
		"space before colon": "mode : subagent",
		"inline comment":     "mode: subagent # the distill agent",
		"double quoted":      `mode: "subagent"`,
	}
	for name, modeLine := range cases {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			configDir := t.TempDir()
			dir := filepath.Join(workDir, ".opencode", "agents")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			definition := "---\ndescription: test\n" + modeLine + "\n---\nbody\n"
			if err := os.WriteFile(filepath.Join(dir, "curator.md"), []byte(definition), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
			if !v.Pass {
				t.Fatalf("expected pass for %q, got refusal: %s", modeLine, v.Reason)
			}
		})
	}
}

func TestGateAutoAnswer_FrontmatterInvalidRefused(t *testing.T) {
	cases := map[string]string{
		"no frontmatter": "body only, no delimiters\n",
		"unterminated":   "---\nmode: subagent\nbody\n",
	}
	for name, definition := range cases {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			configDir := t.TempDir()
			dir := filepath.Join(workDir, ".opencode", "agents")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "curator.md"), []byte(definition), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
			assertRefusal(t, v)
		})
	}
}

func TestRunAutoAnswerGate_CommandLayer(t *testing.T) {
	t.Run("pass prints verdict and returns nil", func(t *testing.T) {
		workDir, configDir := newGateFixture(t, "subagent")
		writePluginConfig(t, configDir, `{"agents": ["autonomy"]}`)
		var buf bytes.Buffer
		cmd := newGateAutoAnswerCmd()
		cmd.SetOut(&buf)
		err := runAutoAnswerGate(cmd, gateOptions{configDir: configDir, workDir: workDir})
		if err != nil {
			t.Fatalf("expected nil error on pass, got %v", err)
		}
		if !strings.Contains(buf.String(), "Auto-answer gate: PASS") {
			t.Errorf("verdict not written: %q", buf.String())
		}
	})
	t.Run("refusal returns errAutoAnswerGateRefused", func(t *testing.T) {
		workDir, configDir := newGateFixture(t, "subagent")
		writePluginConfig(t, configDir, `{"agents": ["curator"]}`)
		var buf bytes.Buffer
		cmd := newGateAutoAnswerCmd()
		cmd.SetOut(&buf)
		err := runAutoAnswerGate(cmd, gateOptions{configDir: configDir, workDir: workDir})
		if !errors.Is(err, errAutoAnswerGateRefused) {
			t.Fatalf("expected errAutoAnswerGateRefused, got %v", err)
		}
		if !strings.Contains(buf.String(), "Auto-answer gate: REFUSED") {
			t.Errorf("refusal verdict not written: %q", buf.String())
		}
	})
	t.Run("root command registers gate auto-answer", func(t *testing.T) {
		var gate *cobra.Command
		for _, c := range newRootCmd().Commands() {
			if c.Use == "gate" {
				gate = c
			}
		}
		if gate == nil {
			t.Fatal("newRootCmd does not register `gate`")
		}
		found := false
		for _, c := range gate.Commands() {
			if c.Use == "auto-answer" {
				found = true
			}
		}
		if !found {
			t.Error("`gate` does not register the auto-answer subcommand")
		}
	})
}

func TestGateAutoAnswer_RefusalPayload(t *testing.T) {
	workDir, configDir := newGateFixture(t, "subagent")
	writePluginConfig(t, configDir, `{"agents": ["*"]}`)
	v := evaluateAutoAnswerGate(gateOptions{configDir: configDir, workDir: workDir})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "auto-question.jsonc") {
		t.Errorf("reason %q does not name the config file", v.Reason)
	}
}

// newGateEnvFixture creates the multi-path gate fixture per design D5: a
// work directory with a resolvable curator definition, plus distinct HOME
// and XDG_CONFIG_HOME t.TempDir() trees holding real plugin config files.
// homeContent lands at <HOME>/.config/opencode/auto-question.jsonc and
// xdgContent at <XDG_CONFIG_HOME>/opencode/auto-question.jsonc; empty
// content leaves that file absent. The returned lookup injects both trees
// so evaluation runs the production environment resolution end to end,
// and the returned file paths are the exact candidates the gate must
// evaluate and name.
func newGateEnvFixture(t *testing.T, homeContent, xdgContent string) (workDir, homeFile, xdgFile string, lookup func(string) (string, bool)) {
	t.Helper()
	workDir, _ = newGateFixture(t, "subagent")
	homeRoot := t.TempDir()
	xdgRoot := t.TempDir()
	homeDir := filepath.Join(homeRoot, ".config", "opencode")
	xdgDir := filepath.Join(xdgRoot, "opencode")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create home-directory config dir: %v", err)
	}
	if err := os.MkdirAll(xdgDir, 0o755); err != nil {
		t.Fatalf("create XDG config dir: %v", err)
	}
	if homeContent != "" {
		writePluginConfig(t, homeDir, homeContent)
	}
	if xdgContent != "" {
		writePluginConfig(t, xdgDir, xdgContent)
	}
	lookup = mapEnv(map[string]string{
		"HOME":            homeRoot,
		"XDG_CONFIG_HOME": xdgRoot,
	})
	homeFile = filepath.Join(homeDir, autoAnswerConfigFile)
	xdgFile = filepath.Join(xdgDir, autoAnswerConfigFile)
	return
}

// assertNamesEveryCheckedFile checks acceptance criterion 4 for the
// two-path fixture (spec scenario "Verdict names every checked file"):
// the verdict names the authoritative home-directory file as the config
// file and the XDG file as the one additional checked path, and the
// rendered output prints both — home first — on pass and refusal alike.
func assertNamesEveryCheckedFile(t *testing.T, v gateVerdict, homeFile, xdgFile string) {
	t.Helper()
	if v.ConfigFile != homeFile {
		t.Errorf("ConfigFile = %q, want the home-directory file %q", v.ConfigFile, homeFile)
	}
	if want := []string{xdgFile}; !slices.Equal(v.AdditionalConfigFiles, want) {
		t.Errorf("AdditionalConfigFiles = %v, want exactly the XDG file %v", v.AdditionalConfigFiles, want)
	}
	rendered := renderGateVerdict(v)
	if !strings.Contains(rendered, "Config file: "+homeFile) {
		t.Errorf("rendered verdict missing the home-directory Config file line:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Also checked: "+xdgFile) {
		t.Errorf("rendered verdict missing the Also checked line for the XDG file:\n%s", rendered)
	}
	homeAt := strings.Index(rendered, "Config file: "+homeFile)
	xdgAt := strings.Index(rendered, "Also checked: "+xdgFile)
	if homeAt >= 0 && xdgAt >= 0 && homeAt > xdgAt {
		t.Errorf("rendered verdict must name the home-directory file before the XDG file:\n%s", rendered)
	}
}

// TestGateAutoAnswer_CleanXDGDoesNotMaskCuratorInHome covers the spec
// scenario "Clean XDG file does not mask curator in the home-directory
// file": a clean XDG config must not mask an offending home-directory
// config, and the refusal must name the home-directory file as the
// offender.
func TestGateAutoAnswer_CleanXDGDoesNotMaskCuratorInHome(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["autonomy", "curator"]}`,
		`{"agents": ["reviewer"]}`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "curator") {
		t.Errorf("reason %q must name the offending curator entry", v.Reason)
	}
	if !strings.Contains(v.Reason, homeFile) {
		t.Errorf("reason %q does not name the offending home-directory file", v.Reason)
	}
	if strings.Contains(v.Reason, xdgFile) {
		t.Errorf("reason %q must blame the home-directory file, not the clean XDG file", v.Reason)
	}
	if !strings.Contains(v.Fix, homeFile) {
		t.Errorf("fix %q does not name the offending home-directory file", v.Fix)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_CuratorInXDGDoesNotMaskCleanHome covers the spec
// scenario "Curator in the XDG file does not mask a clean home-directory
// file": an offending XDG config must refuse even when the home-directory
// config is clean, and the refusal must name the XDG file as the offender.
func TestGateAutoAnswer_CuratorInXDGDoesNotMaskCleanHome(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		`{"agents": ["autonomy", "curator"]}`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "curator") {
		t.Errorf("reason %q must name the offending curator entry", v.Reason)
	}
	if !strings.Contains(v.Reason, xdgFile) {
		t.Errorf("reason %q does not name the offending XDG file", v.Reason)
	}
	if strings.Contains(v.Reason, homeFile) {
		t.Errorf("reason %q must blame the XDG file, not the clean home-directory file", v.Reason)
	}
	if !strings.Contains(v.Fix, xdgFile) {
		t.Errorf("fix %q does not name the offending XDG file", v.Fix)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_CleanListAtEveryCheckedPath covers the spec scenario
// "Clean agent list at every checked path": clean lists everywhere pass,
// and the reported effective agents come from the home-directory file.
// The two fixtures carry distinct agent lists so authority leakage from
// the XDG file is detectable.
func TestGateAutoAnswer_CleanListAtEveryCheckedPath(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["autonomy", "builder"]}`,
		`{"agents": ["reviewer"]}`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	if !v.Pass {
		t.Fatalf("expected pass on clean lists at every checked path, got refusal: %s", v.Reason)
	}
	if want := []string{"autonomy", "builder"}; !slices.Equal(v.EffectiveAgents, want) {
		t.Errorf("effective agents = %v, want the home-directory list %v", v.EffectiveAgents, want)
	}
	if v.DefaultApplied {
		t.Error("default must not apply when the home-directory file configures a list")
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
	if rendered := renderGateVerdict(v); !strings.Contains(rendered, `Effective agents: ["autonomy", "builder"]`) {
		t.Errorf("rendered verdict must report the home-directory list:\n%s", rendered)
	}
}

// TestGateAutoAnswer_EffectiveAgentsFromHomeFile covers the spec scenario
// "Effective agents come from the home-directory file": with the
// home-directory config absent and a clean, parseable XDG config, the
// gate passes on the documented default. The XDG fixture carries a
// distinctive list so an implementation populating the effective list or
// default flag from the XDG file when the home file is absent is
// detectable.
func TestGateAutoAnswer_EffectiveAgentsFromHomeFile(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		"",
		`{"agents": ["reviewer"]}`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	if !v.Pass {
		t.Fatalf("expected pass when the home-directory file is absent, got refusal: %s", v.Reason)
	}
	if want := []string{"autonomy"}; !slices.Equal(v.EffectiveAgents, want) {
		t.Errorf("effective agents = %v, want the documented default %v", v.EffectiveAgents, want)
	}
	if !v.DefaultApplied {
		t.Error("the documented default must apply when the home-directory file is absent")
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
	rendered := renderGateVerdict(v)
	for _, want := range []string{
		`Effective agents: ["autonomy"]`,
		"Default applied: yes",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered verdict missing %q:\n%s", want, rendered)
		}
	}
}

// TestGateAutoAnswer_CoincidentXDGCheckedOnce covers the spec scenario
// "Coincident paths checked once": when XDG_CONFIG_HOME is set to
// $HOME/.config, the constructed XDG opencode directory is lexically the
// home-directory directory (design D3), so exactly one file is checked
// and the single-path output shape is preserved — no `Also checked:`
// line.
func TestGateAutoAnswer_CoincidentXDGCheckedOnce(t *testing.T) {
	workDir, homeFile, _, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		"",
	)
	// Point XDG_CONFIG_HOME exactly at $HOME/.config so the two
	// constructed opencode directories coincide lexically.
	coincident := func(key string) (string, bool) {
		if key == "XDG_CONFIG_HOME" {
			return filepath.Dir(filepath.Dir(homeFile)), true
		}
		return lookup(key)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: coincident})
	if !v.Pass {
		t.Fatalf("expected pass on a clean coincident config, got refusal: %s", v.Reason)
	}
	if v.ConfigFile != homeFile {
		t.Errorf("ConfigFile = %q, want the single coincident file %q", v.ConfigFile, homeFile)
	}
	if len(v.AdditionalConfigFiles) != 0 {
		t.Errorf("AdditionalConfigFiles = %v, want none when the paths coincide", v.AdditionalConfigFiles)
	}
	rendered := renderGateVerdict(v)
	for _, want := range []string{
		"Config file: " + homeFile,
		`Effective agents: ["reviewer"]`,
		"Default applied: no",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered verdict missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "Also checked:") {
		t.Errorf("coincident paths must not print an Also checked line:\n%s", rendered)
	}
	if want := []string{"reviewer"}; !slices.Equal(v.EffectiveAgents, want) {
		t.Errorf("effective agents = %v, want the coincident file's list %v", v.EffectiveAgents, want)
	}
	if v.DefaultApplied {
		t.Error("default must not apply when the coincident config lists agents")
	}
}

// TestGateAutoAnswer_BothPathsAbsentPassesOnDefault covers the spec
// scenario "Plugin config absent at every checked path": with no
// auto-question.jsonc at either checked path, the gate passes on the
// documented default with the default-applied flag set, and the verdict
// names both checked paths.
func TestGateAutoAnswer_BothPathsAbsentPassesOnDefault(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t, "", "")
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	if !v.Pass {
		t.Fatalf("expected pass when every checked file is absent, got refusal: %s", v.Reason)
	}
	if want := []string{"autonomy"}; !slices.Equal(v.EffectiveAgents, want) {
		t.Errorf("effective agents = %v, want the documented default %v", v.EffectiveAgents, want)
	}
	if !v.DefaultApplied {
		t.Error("the documented default must apply when every checked file is absent")
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
	rendered := renderGateVerdict(v)
	for _, want := range []string{
		`Effective agents: ["autonomy"]`,
		"Default applied: yes",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered verdict missing %q:\n%s", want, rendered)
		}
	}
}

// TestGateAutoAnswer_UnparseableXDGFileRefused covers the spec scenario
// "Unparseable plugin configuration" in the multi-path arrangement: an
// unparseable XDG file refuses even with a clean home-directory file,
// and the refusal names the XDG file as the offender.
func TestGateAutoAnswer_UnparseableXDGFileRefused(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		`{"agents": [`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "not valid JSONC") {
		t.Errorf("reason %q must name the unparseable JSONC", v.Reason)
	}
	if !strings.Contains(v.Fix, xdgFile) {
		t.Errorf("fix %q does not name the offending XDG file", v.Fix)
	}
	if strings.Contains(v.Fix, homeFile) {
		t.Errorf("fix %q must blame the XDG file, not the clean home-directory file", v.Fix)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_WildcardInHomeFileRefused covers the spec scenario
// "Wildcard configured for auto-answer" in the multi-path arrangement:
// a wildcard at the home-directory path refuses even with a clean XDG
// file, and the refusal names the home-directory file as the offender.
func TestGateAutoAnswer_WildcardInHomeFileRefused(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["*"]}`,
		`{"agents": ["reviewer"]}`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "wildcard") {
		t.Errorf("reason %q must name the wildcard", v.Reason)
	}
	if !strings.Contains(v.Reason, homeFile) {
		t.Errorf("reason %q does not name the offending home-directory file", v.Reason)
	}
	if strings.Contains(v.Reason, xdgFile) {
		t.Errorf("reason %q must blame the home-directory file, not the clean XDG file", v.Reason)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_HomeUnsetWithXDGSetRefused covers design D7 and the
// spec scenario "Configuration location unverifiable": the home-directory
// path is always checked, so an unset HOME refuses even when
// XDG_CONFIG_HOME points at a valid directory with a clean, parseable
// config — the path the plugin actually reads is unverifiable.
func TestGateAutoAnswer_HomeUnsetWithXDGSetRefused(t *testing.T) {
	workDir, _, _, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		`{"agents": ["builder"]}`,
	)
	noHome := func(key string) (string, bool) {
		if key == "HOME" {
			return "", false
		}
		return lookup(key)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: noHome})
	if v.Pass {
		t.Fatal("expected refusal when HOME is unset even with XDG_CONFIG_HOME set")
	}
	if !strings.Contains(v.Reason, "config directory") {
		t.Errorf("reason %q does not name the config directory", v.Reason)
	}
	if !strings.Contains(v.Reason, "HOME is not set or is empty") {
		t.Errorf("reason %q does not name the unset HOME", v.Reason)
	}
	if v.Fix == "" {
		t.Error("refusal must name a corrective action")
	}
}

// TestGateAutoAnswer_EnvInterpolationToCuratorInHomeFileRefused covers
// CA-FR-007's interpolated-curator refusal in the multi-path
// arrangement: a home-directory entry that is exactly `{env:AGENT}` and
// resolves to curator refuses even with a clean XDG file, and the
// refusal names the home-directory file as the offender.
func TestGateAutoAnswer_EnvInterpolationToCuratorInHomeFileRefused(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["{env:AGENT}"]}`,
		`{"agents": ["reviewer"]}`,
	)
	withAgent := func(key string) (string, bool) {
		if key == "AGENT" {
			return "curator", true
		}
		return lookup(key)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: withAgent})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "curator") {
		t.Errorf("reason %q must name the resolved curator entry", v.Reason)
	}
	if !strings.Contains(v.Reason, homeFile) {
		t.Errorf("reason %q does not name the offending home-directory file", v.Reason)
	}
	if strings.Contains(v.Reason, xdgFile) {
		t.Errorf("reason %q must blame the home-directory file, not the clean XDG file", v.Reason)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_XDGStatFailureRefused covers the spec scenario
// "Configuration location unverifiable" at the XDG path: a stat of the
// XDG auto-question.jsonc failing for a reason other than file-not-found
// refuses even with a clean home-directory file, and the refusal names
// the XDG path. The fixture's XDG opencode directory is replaced by a
// regular file, making the stat fail with ENOTDIR — a genuine
// non-not-found stat failure that stays deterministic on every host,
// including ones running as root, where 0o000 permission bits would not
// deny stat.
func TestGateAutoAnswer_XDGStatFailureRefused(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		"",
	)
	xdgDir := filepath.Dir(xdgFile)
	if err := os.Remove(xdgDir); err != nil {
		t.Fatalf("remove the fixture XDG directory: %v", err)
	}
	if err := os.WriteFile(xdgDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write a regular file at the XDG directory path: %v", err)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "cannot stat") {
		t.Errorf("reason %q must name the stat failure", v.Reason)
	}
	if !strings.Contains(v.Reason, xdgFile) {
		t.Errorf("reason %q does not name the XDG path", v.Reason)
	}
	if strings.Contains(v.Reason, homeFile) {
		t.Errorf("reason %q must blame the XDG path, not the clean home-directory file", v.Reason)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_RelativeHomeRefused covers design D7's relative-HOME
// refusal branch: HOME must be absolute in every environment-resolved
// configuration, so a relative HOME refuses even when XDG_CONFIG_HOME
// points at a valid directory with clean, parseable configs — a relative
// value resolves against the process CWD, which can point inside the
// vault. It also pins the corrective-action text for the config-directory
// resolution refusal: HOME is always required, and XDG_CONFIG_HOME is
// constrained only when set, so the fix never presents setting
// XDG_CONFIG_HOME as a remedy for an unresolvable HOME.
func TestGateAutoAnswer_RelativeHomeRefused(t *testing.T) {
	workDir, _, _, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		`{"agents": ["builder"]}`,
	)
	relativeHome := func(key string) (string, bool) {
		if key == "HOME" {
			return "relative/home", true
		}
		return lookup(key)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: relativeHome})
	if v.Pass {
		t.Fatal("expected refusal for a relative HOME even with XDG_CONFIG_HOME set")
	}
	if !strings.Contains(v.Reason, "config directory") {
		t.Errorf("reason %q does not name the config directory", v.Reason)
	}
	if !strings.Contains(v.Reason, `HOME "relative/home" is not an absolute path`) {
		t.Errorf("reason %q does not name the relative HOME", v.Reason)
	}
	wantFix := "Set HOME to a non-empty absolute path, and XDG_CONFIG_HOME to an absolute path when set, then re-run /dewey-distill."
	if v.Fix != wantFix {
		t.Errorf("fix = %q, want %q", v.Fix, wantFix)
	}
}

// TestGateAutoAnswer_EmptyHomeWithXDGSetRefused covers the remaining D7
// variant: HOME set but empty refuses even when XDG_CONFIG_HOME points at
// a valid directory with clean configs, sharing the unset-HOME refusal
// branch — the path the plugin actually reads is unverifiable.
func TestGateAutoAnswer_EmptyHomeWithXDGSetRefused(t *testing.T) {
	workDir, _, _, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		`{"agents": ["builder"]}`,
	)
	emptyHome := func(key string) (string, bool) {
		if key == "HOME" {
			return "", true
		}
		return lookup(key)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: emptyHome})
	if v.Pass {
		t.Fatal("expected refusal for an empty HOME even with XDG_CONFIG_HOME set")
	}
	if !strings.Contains(v.Reason, "config directory") {
		t.Errorf("reason %q does not name the config directory", v.Reason)
	}
	if !strings.Contains(v.Reason, "HOME is not set or is empty") {
		t.Errorf("reason %q does not name the empty HOME", v.Reason)
	}
	if v.Fix == "" {
		t.Error("refusal must name a corrective action")
	}
}

// TestGateAutoAnswer_RelativeXDGWithValidHomeRefused restores coverage for
// the relative-XDG_CONFIG_HOME refusal branch under the HOME-first
// resolution: with a valid absolute HOME, a relative XDG_CONFIG_HOME
// refuses and the reason names the relative value. The pre-existing
// TestGateAutoAnswer_RelativeXDGRefused sets no HOME, so under the new
// resolution it short-circuits at the HOME-unset check; this test reaches
// the IsAbs(XDG_CONFIG_HOME) branch it was written to pin.
func TestGateAutoAnswer_RelativeXDGWithValidHomeRefused(t *testing.T) {
	workDir, _, _, lookup := newGateEnvFixture(t,
		`{"agents": ["reviewer"]}`,
		`{"agents": ["builder"]}`,
	)
	relativeXDG := func(key string) (string, bool) {
		if key == "XDG_CONFIG_HOME" {
			return "relative/config", true
		}
		return lookup(key)
	}
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: relativeXDG})
	if v.Pass {
		t.Fatal("expected refusal for a relative XDG_CONFIG_HOME with a valid HOME")
	}
	if !strings.Contains(v.Reason, "config directory") {
		t.Errorf("reason %q does not name the config directory", v.Reason)
	}
	if !strings.Contains(v.Reason, `XDG_CONFIG_HOME "relative/config" is not an absolute path`) {
		t.Errorf("reason %q does not name the relative XDG_CONFIG_HOME", v.Reason)
	}
	if v.Fix == "" {
		t.Error("refusal must name a corrective action")
	}
}

// TestGateAutoAnswer_XDGUnsetCheckedOnce covers the default real-world
// arrangement through environment resolution — XDG_CONFIG_HOME unset or
// empty with a valid HOME — the first half of the spec scenario
// "Coincident paths checked once": exactly one candidate, so the rendered
// verdict is byte-identical to the single-path evaluation of the same
// home directory through the injected configDir seam, with no
// "Also checked:" line. The XDG fixture carries a distinct clean list, so
// an implementation consulting the XDG tree despite the unset variable is
// detectable.
func TestGateAutoAnswer_XDGUnsetCheckedOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		xdgSet bool
	}{
		{name: "unset", xdgSet: false},
		{name: "empty", xdgSet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workDir, homeFile, _, lookup := newGateEnvFixture(t,
				`{"agents": ["reviewer"]}`,
				`{"agents": ["builder"]}`,
			)
			noXDG := func(key string) (string, bool) {
				if key == "XDG_CONFIG_HOME" {
					return "", tc.xdgSet
				}
				return lookup(key)
			}
			v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: noXDG})
			if !v.Pass {
				t.Fatalf("expected pass with XDG_CONFIG_HOME %s and a valid HOME, got refusal: %s", tc.name, v.Reason)
			}
			if v.ConfigFile != homeFile {
				t.Errorf("ConfigFile = %q, want the home-directory file %q", v.ConfigFile, homeFile)
			}
			if len(v.AdditionalConfigFiles) != 0 {
				t.Errorf("AdditionalConfigFiles = %v, want none when XDG_CONFIG_HOME is %s", v.AdditionalConfigFiles, tc.name)
			}
			if want := []string{"reviewer"}; !slices.Equal(v.EffectiveAgents, want) {
				t.Errorf("effective agents = %v, want the home-directory list %v", v.EffectiveAgents, want)
			}
			rendered := renderGateVerdict(v)
			if strings.Contains(rendered, "Also checked:") {
				t.Errorf("a %s XDG_CONFIG_HOME must not print an Also checked line:\n%s", tc.name, rendered)
			}
			single := evaluateAutoAnswerGate(gateOptions{configDir: filepath.Dir(homeFile), workDir: workDir})
			if !single.Pass {
				t.Fatalf("expected the injected single-path evaluation to pass, got refusal: %s", single.Reason)
			}
			if other := renderGateVerdict(single); other != rendered {
				t.Errorf("env-resolved verdict must be byte-identical to the single-path verdict:\nenv-resolved:\n%s\nsingle-path:\n%s", rendered, other)
			}
		})
	}
}

// TestGateAutoAnswer_TwoOffendersNameHomeFirst pins design D2's
// first-offender determinism: when both checked files offend — curator in
// the home-directory file and the wildcard in the XDG file — the refusal
// names exactly one file, the home-directory file and its curator entry,
// never the XDG offender, so a home-then-XDG order reversal in the
// candidate loop is detectable.
func TestGateAutoAnswer_TwoOffendersNameHomeFirst(t *testing.T) {
	workDir, homeFile, xdgFile, lookup := newGateEnvFixture(t,
		`{"agents": ["autonomy", "curator"]}`,
		`{"agents": ["*"]}`,
	)
	v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
	assertRefusal(t, v)
	if !strings.Contains(v.Reason, "curator") {
		t.Errorf("reason %q must name the home-directory curator entry", v.Reason)
	}
	if !strings.Contains(v.Reason, homeFile) {
		t.Errorf("reason %q does not name the offending home-directory file", v.Reason)
	}
	if strings.Contains(v.Reason, xdgFile) {
		t.Errorf("reason %q must name only the first offender, not the XDG file", v.Reason)
	}
	if strings.Contains(v.Reason, "wildcard") {
		t.Errorf("reason %q must name the first offender's entry, not the XDG wildcard", v.Reason)
	}
	if !strings.Contains(v.Fix, homeFile) {
		t.Errorf("fix %q does not name the offending home-directory file", v.Fix)
	}
	assertNamesEveryCheckedFile(t, v, homeFile, xdgFile)
}

// TestGateAutoAnswer_RegistrationScanDirSelection pins design D6: the
// advisory registration scan receives the XDG directory when
// XDG_CONFIG_HOME is set and absolute, else the home-directory directory
// — the same directory the scan received before multi-path evaluation.
// Each subtest places the opencode.json registration marker in exactly
// one tree, so a drift in scanDir selection reports absent and fails.
// Registration is advisory only and never changes the verdict.
func TestGateAutoAnswer_RegistrationScanDirSelection(t *testing.T) {
	const marker = `{"plugin": ["opencode-auto-question"]}`
	t.Run("XDG set scans the XDG tree", func(t *testing.T) {
		workDir, _, xdgFile, lookup := newGateEnvFixture(t,
			`{"agents": ["reviewer"]}`,
			"",
		)
		if err := os.WriteFile(filepath.Join(filepath.Dir(xdgFile), "opencode.json"), []byte(marker), 0o644); err != nil {
			t.Fatalf("write XDG registration marker: %v", err)
		}
		v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: lookup})
		if !v.Pass {
			t.Fatalf("registration must not change the verdict: %s", v.Reason)
		}
		if v.Registration != "registered" {
			t.Errorf("registration = %q, want registered from the XDG-tree marker", v.Registration)
		}
	})
	t.Run("XDG unset scans the home tree", func(t *testing.T) {
		workDir, homeFile, _, lookup := newGateEnvFixture(t,
			`{"agents": ["reviewer"]}`,
			"",
		)
		noXDG := func(key string) (string, bool) {
			if key == "XDG_CONFIG_HOME" {
				return "", false
			}
			return lookup(key)
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(homeFile), "opencode.json"), []byte(marker), 0o644); err != nil {
			t.Fatalf("write home registration marker: %v", err)
		}
		v := evaluateAutoAnswerGate(gateOptions{workDir: workDir, lookupEnv: noXDG})
		if !v.Pass {
			t.Fatalf("registration must not change the verdict: %s", v.Reason)
		}
		if v.Registration != "registered" {
			t.Errorf("registration = %q, want registered from the home-tree marker", v.Registration)
		}
	})
}

// TestRenderGateVerdict_MultiFile covers design D4's multi-file render
// contract: with one additional checked file, the verdict prints the
// Config file line immediately followed by an Also checked line naming
// it — on pass and refusal shapes alike — and a control-character-laden
// additional path is rendered in its safeValue-sanitized form, never
// with the raw control bytes.
func TestRenderGateVerdict_MultiFile(t *testing.T) {
	const primary = "/home/alice/.config/opencode/auto-question.jsonc"
	const additional = "/xdg-config/opencode/auto-question.jsonc"
	adjacentLines := "Config file: " + primary + "\nAlso checked: " + additional + "\n"

	passed := renderGateVerdict(gateVerdict{
		Pass:                  true,
		ConfigFile:            primary,
		AdditionalConfigFiles: []string{additional},
		EffectiveAgents:       []string{"autonomy", "builder"},
		DefaultApplied:        false,
	})
	if !strings.Contains(passed, "Auto-answer gate: PASS") {
		t.Error("pass verdict header missing")
	}
	if !strings.Contains(passed, adjacentLines) {
		t.Errorf("pass verdict must name the additional file immediately after the config file:\n%s", passed)
	}

	refused := renderGateVerdict(gateVerdict{
		ConfigFile:            primary,
		AdditionalConfigFiles: []string{additional},
		Reason:                `the "` + primary + `" agent list includes curator`,
		Fix:                   "Remove curator from the agents list in " + primary + ", then re-run /dewey-distill.",
	})
	if !strings.Contains(refused, "Auto-answer gate: REFUSED") {
		t.Error("refusal verdict header missing")
	}
	if !strings.Contains(refused, adjacentLines) {
		t.Errorf("refusal verdict must name the additional file immediately after the config file:\n%s", refused)
	}

	// A control-character-laden additional path is sanitized per
	// safeValue: the carriage return maps to a space and the ANSI
	// escape is stripped, so the Also checked line never carries the
	// raw control bytes.
	sanitized := renderGateVerdict(gateVerdict{
		ConfigFile:            primary,
		AdditionalConfigFiles: []string{"/xdg/opencode\r/auto-question\x1b[31m.jsonc"},
		Reason:                "the plugin config is not valid JSONC",
	})
	if want := "Config file: " + primary + "\nAlso checked: /xdg/opencode /auto-question[31m.jsonc\n"; !strings.Contains(sanitized, want) {
		t.Errorf("Also checked line must carry the sanitized additional path:\n%s", sanitized)
	}
	if strings.Contains(sanitized, "\x1b") {
		t.Error("ANSI escape sequence was not stripped from the Also checked line")
	}
	if strings.Contains(sanitized, "\r") {
		t.Error("carriage return was not replaced in the Also checked line")
	}
}
