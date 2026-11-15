// Workflow template validation for the weekly unattended distill trigger
// (specs/018-weekly-distill-automation T014): asserts the structural
// contract from contracts/workflow-template.md and executes the template's
// own trigger script against a fake gh to verify the FR-003/SC-002 dedup
// invariant over a 4-week simulated schedule with overlapping triggers.
//
// PARALLEL SAFETY: Tests in this file MUST NOT use t.Parallel().
// They execute the template script with a PATH-injected fake gh; a parallel
// neighbor sharing PATH resolution could observe the fake binary.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const distillWorkflowTemplatePath = ".github/workflows/dewey-distill.yml.example"

// fakeGhScript implements just enough of the gh CLI for the trigger script:
// the `issue list` projection (--json number --jq), `issue comment`, and
// `issue create`. State lives in $FAKE_GH_STATE:
//
//	open  — number of the single open labeled issue (absent = none)
//	next  — next issue number to allocate
//	log   — one line per mutating call:
//	          "create <n> <label> <assignee>" | "comment <n> <body>"
//
// create refuses when an open labeled issue already exists: the dedup
// invariant is the trigger script's job, so a violation must fail the run
// loudly (set -eu propagates) rather than silently open a second issue.
const fakeGhScript = `#!/bin/sh
state="$FAKE_GH_STATE"
cmd="$1 $2"
case "$cmd" in
"issue list")
  # gh evaluates --jq client-side; emit the '.[0].number // empty'
  # projection for the single open labeled issue.
  if [ -f "$state/open" ]; then
    cat "$state/open"
  fi
  ;;
"issue comment")
  n="$3"
  if [ ! -f "$state/open" ] || [ "$(cat "$state/open")" != "$n" ]; then
    echo "fake gh: comment target #$n is not the open issue" >&2
    exit 1
  fi
  printf 'comment %s %s\n' "$n" "$5" >>"$state/log"
  ;;
"issue create")
  if [ -f "$state/open" ]; then
    echo "fake gh: dedup invariant violated - issue #$(cat "$state/open") is still open" >&2
    exit 1
  fi
  label=""
  assignee=""
  shift 2
  while [ $# -gt 0 ]; do
    case "$1" in
      --label) label="$2"; shift ;;
      --assignee) assignee="$2"; shift ;;
    esac
    shift
  done
  n=$(cat "$state/next" 2>/dev/null || echo 1)
  echo $((n + 1)) >"$state/next"
  echo "$n" >"$state/open"
  printf 'create %s %s %s\n' "$n" "$label" "$assignee" >>"$state/log"
  ;;
*)
  echo "fake gh: unexpected arguments: $*" >&2
  exit 2
  ;;
esac
`

type distillWorkflowStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

type distillWorkflowJob struct {
	Steps []distillWorkflowStep `yaml:"steps"`
}

type distillWorkflow struct {
	Name string `yaml:"name"`
	// On is a map so a bare `workflow_dispatch:` (null value) is
	// distinguishable from an absent key.
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
	Jobs map[string]distillWorkflowJob `yaml:"jobs"`
}

// loadDistillWorkflowTemplate parses the shipped template. Tests run from
// the repository root (package main), so the documented relative path works.
func loadDistillWorkflowTemplate(t *testing.T) distillWorkflow {
	t.Helper()
	raw, err := os.ReadFile(distillWorkflowTemplatePath)
	if err != nil {
		t.Fatalf("read %s: %v", distillWorkflowTemplatePath, err)
	}
	var wf distillWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("parse %s: %v", distillWorkflowTemplatePath, err)
	}
	return wf
}

// distillTriggerScript returns the run script of the distill-trigger job.
func distillTriggerScript(t *testing.T, wf distillWorkflow) string {
	t.Helper()
	job, ok := wf.Jobs["distill-trigger"]
	if !ok {
		t.Fatal("template has no distill-trigger job")
	}
	if len(job.Steps) == 0 {
		t.Fatal("distill-trigger job has no steps")
	}
	script := job.Steps[0].Run
	if script == "" {
		t.Fatal("distill-trigger first step has no run script")
	}
	return script
}

// runDistillTrigger executes the template's trigger script with the fake gh
// on PATH and the fixture state directory.
func runDistillTrigger(t *testing.T, script, stateDir, binDir string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_GH_STATE="+stateDir,
		"GH_TOKEN=fixture-token",
		"GH_REPO=example/fixture",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("distill-trigger script failed: %v\noutput:\n%s", err, out)
	}
}

// readFixtureLog reads the fake gh mutation log, one line per call.
func readFixtureLog(t *testing.T, stateDir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(stateDir, "log"))
	if err != nil {
		t.Fatalf("read fake gh log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// TestDistillWorkflowTemplate_Exists verifies the template ships at the
// documented path (contracts/workflow-template.md Tests).
func TestDistillWorkflowTemplate_Exists(t *testing.T) {
	if _, err := os.Stat(distillWorkflowTemplatePath); err != nil {
		t.Fatalf("workflow template missing at documented path %s: %v", distillWorkflowTemplatePath, err)
	}
}

// TestDistillWorkflowTemplate_Structure verifies the template's triggers,
// permissions, concurrency serialization, and trigger commands against
// contracts/workflow-template.md.
func TestDistillWorkflowTemplate_Structure(t *testing.T) {
	wf := loadDistillWorkflowTemplate(t)

	if wf.Name != "dewey-distill" {
		t.Errorf("workflow name = %q, want %q", wf.Name, "dewey-distill")
	}
	schedule, ok := wf.On["schedule"].([]any)
	if !ok || len(schedule) != 1 {
		t.Fatalf("on.schedule = %#v, want one entry", wf.On["schedule"])
	}
	entry, ok := schedule[0].(map[string]any)
	if !ok {
		t.Fatalf("on.schedule[0] = %#v, want a cron mapping", schedule[0])
	}
	if entry["cron"] != "0 9 * * 1" {
		t.Errorf("on.schedule[0].cron = %v, want 0 9 * * 1 (editable)", entry["cron"])
	}
	// workflow_dispatch carries no value; presence of the key is the trigger.
	if _, ok := wf.On["workflow_dispatch"]; !ok {
		t.Error("workflow_dispatch trigger missing")
	}

	wantPerms := map[string]string{
		"contents":      "read",
		"issues":        "write",
		"pull-requests": "write",
	}
	for key, want := range wantPerms {
		if got := wf.Permissions[key]; got != want {
			t.Errorf("permissions[%q] = %q, want %q", key, got, want)
		}
	}

	// The concurrency group serializes a manual re-trigger racing the
	// schedule so the check-then-create window cannot double-issue.
	if wf.Concurrency.Group != "dewey-distill-trigger" {
		t.Errorf("concurrency group = %q, want dewey-distill-trigger", wf.Concurrency.Group)
	}
	if wf.Concurrency.CancelInProgress {
		t.Error("concurrency cancel-in-progress = true, want false (queued runs must not be cancelled)")
	}

	script := distillTriggerScript(t, wf)
	for _, want := range []string{
		"gh issue list --label dewey-distill --state open",
		"gh issue comment",
		"<!-- dewey-distill-trigger --> /dewey-distill-auto #",
		"gh issue create --label dewey-distill --assignee \"@me\"",
		"--title \"Weekly dewey-distill\"",
		"--body \"Dispatch: /dewey-distill-auto\"",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("trigger script missing contract command %q;\nscript:\n%s", want, script)
		}
	}
}

// TestDistillWorkflowTemplate_DedupInvariant executes the template's own
// trigger script against a fake gh across a 4-week simulated schedule with
// overlapping triggers (SC-002): at most one open dewey-distill issue at
// any time, and a second trigger posts a re-trigger comment instead of a
// duplicate issue.
func TestDistillWorkflowTemplate_DedupInvariant(t *testing.T) {
	wf := loadDistillWorkflowTemplate(t)
	script := distillTriggerScript(t, wf)

	// Fixture: state dir holds gh's issue world; bin dir holds the fake gh.
	stateDir := t.TempDir()
	binDir := t.TempDir()
	fakeGh := filepath.Join(binDir, "gh")
	if err := os.WriteFile(fakeGh, []byte(fakeGhScript), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	// Week 1: no open issue — the trigger creates one, labeled and
	// assigned to the workflow actor.
	runDistillTrigger(t, script, stateDir, binDir)
	lines := readFixtureLog(t, stateDir)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "create 1 dewey-distill @me") {
		t.Fatalf("week 1: want a single create of issue 1 with label and assignee, got %v", lines)
	}

	// Week 2: issue #1 still open — second trigger posts a re-trigger
	// comment, not a duplicate issue.
	runDistillTrigger(t, script, stateDir, binDir)
	lines = readFixtureLog(t, stateDir)
	if len(lines) != 2 {
		t.Fatalf("week 2: want exactly one new log entry, got %v", lines)
	}
	if want := "comment 1 <!-- dewey-distill-trigger --> /dewey-distill-auto #1"; lines[1] != want {
		t.Errorf("week 2: re-trigger comment = %q, want %q", lines[1], want)
	}

	// Week 3: the human closed issue #1 — a fresh trigger opens a new
	// issue. (Simulates closing the review queue between weeks.)
	if err := os.Remove(filepath.Join(stateDir, "open")); err != nil {
		t.Fatalf("close issue 1: %v", err)
	}
	runDistillTrigger(t, script, stateDir, binDir)
	lines = readFixtureLog(t, stateDir)
	if len(lines) != 3 || !strings.HasPrefix(lines[2], "create 2 dewey-distill @me") {
		t.Fatalf("week 3: want create of issue 2, got %v", lines)
	}

	// Week 4 plus overlapping triggers (a manual re-trigger racing the
	// schedule, serialized by the concurrency group): with issue #2 open,
	// every fire must comment — the fake gh fails the run if create is
	// ever called while an issue is open, so a duplicate issue fails the
	// test rather than slipping through.
	for fire := 0; fire < 3; fire++ {
		runDistillTrigger(t, script, stateDir, binDir)
	}
	lines = readFixtureLog(t, stateDir)
	if len(lines) != 6 {
		t.Fatalf("overlapping triggers: want 3 comments after week 4 (6 log lines total), got %v", lines)
	}
	for i, line := range lines[3:] {
		if want := "comment 2 <!-- dewey-distill-trigger --> /dewey-distill-auto #2"; line != want {
			t.Errorf("overlapping trigger %d: comment = %q, want %q", i+1, line, want)
		}
	}

	// Invariant across the whole schedule: two issues ever created (weeks
	// 1 and 3), both singletons in their window, four re-trigger comments.
	creates, comments := 0, 0
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "create "):
			creates++
		case strings.HasPrefix(line, "comment "):
			comments++
		}
	}
	if creates != 2 || comments != 4 {
		t.Errorf("schedule totals: %d creates, %d comments; want 2 creates, 4 comments", creates, comments)
	}
}
