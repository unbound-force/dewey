package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
