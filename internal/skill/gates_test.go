package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGatesSkillFrontmatter pins the invocation contract. This skill is
// deliberately NOT model-invocable: authoring a repository gate changes what
// validating the repository means, so it runs when a human asks for it and
// never because an agent guessed the task looked related.
func TestGatesSkillFrontmatter(t *testing.T) {
	md := Gates.Markdown()
	if !strings.HasPrefix(md, "---\n") {
		t.Fatalf("SKILL.md must start with YAML frontmatter, got:\n%s", md[:min(40, len(md))])
	}
	for _, want := range []string{
		"name: " + GatesName + "\n",
		"description: " + GatesDescription + "\n",
		"user-invocable: true\n",
		"disable-model-invocation: true\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("frontmatter missing %q", want)
		}
	}
	if strings.Count(md, "---\n") < 2 {
		t.Errorf("frontmatter not closed with a second --- delimiter")
	}
	if strings.Contains(md, "internal: true") {
		t.Errorf("Gates.Markdown() must not be marked internal")
	}
}

// TestPipelineSkillStaysModelInvocable guards the asymmetry: the pipeline skill
// must keep activating on its own, because an agent asked to ship work should
// reach for the gate without being told.
func TestPipelineSkillStaysModelInvocable(t *testing.T) {
	if strings.Contains(Markdown(), "disable-model-invocation") {
		t.Errorf("the pipeline skill must remain model-invocable")
	}
}

// TestGatesSkillBodyCarriesTheLoadBearingRules pins the facts an agent cannot
// derive from the config file it is editing. Each of these has a wrong-by-
// default failure mode: writing a gate that silently never runs, that is
// rejected at parse time, or that a contributor can switch off.
func TestGatesSkillBodyCarriesTheLoadBearingRules(t *testing.T) {
	// Collapse whitespace: these are prose rules, and pinning where the text
	// happens to wrap would make every reflow of the body a test failure.
	md := flattenSpace(Gates.Markdown())
	for name, want := range map[string]string{
		"schema":              "exactly one of `command` or `instructions`",
		"valid anchors":       "`rebase`, `review`, `test`, `document`, `lint`",
		"refused anchors":     "cannot be anchored",
		"name syntax":         "lowercase letters, digits, and inner hyphens",
		"gate cap":            "At most 16 gates",
		"instructions cap":    "16,384 bytes",
		"activation rule":     "trusted default-branch copy",
		"no effect this run":  "does not affect that branch's own run",
		"pinned at creation":  "resolves its gate list once",
		"park not autofix":    "parks for a decision instead of auto-fixing",
		"additive only":       "never replace, reorder, or remove a core step",
		"repo local":          "no global or machine-wide",
		"log step name":       "gate.<anchor>.<name>",
		"hardening":           "disable_project_settings",
		"read existing first": "Read the repository's existing",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("gates skill body missing the %s rule: %q", name, want)
		}
	}
}

// TestGatesSkillBodyDoesNotInventCoreStepControl is the anti-assertion: a gate
// can only ADD a verdict, so the skill must never suggest it can turn a core
// step off. An agent that believes otherwise writes config the parser rejects.
func TestGatesSkillBodyDoesNotInventCoreStepControl(t *testing.T) {
	md := strings.ToLower(flattenSpace(Gates.Markdown()))
	for _, forbidden := range []string{
		"disable a core step",
		"skip a core step",
		"replace the review step",
	} {
		if strings.Contains(md, forbidden) {
			t.Errorf("gates skill must not imply core-step control: %q", forbidden)
		}
	}
}

// TestAllSkillsAreDistinctAndComplete proves the generator and installer have a
// single list to iterate, so a new skill cannot be rendered but not installed.
func TestAllSkillsAreDistinctAndComplete(t *testing.T) {
	all := All()
	if len(all) < 2 {
		t.Fatalf("All() returned %d skill(s), want the pipeline and gates skills", len(all))
	}
	seen := map[string]bool{}
	for _, s := range all {
		if s.Name == "" || s.Description == "" || s.Body == "" {
			t.Errorf("skill %q is incomplete", s.Name)
		}
		if seen[s.Name] {
			t.Errorf("duplicate skill name %q", s.Name)
		}
		seen[s.Name] = true
	}
	for _, want := range []string{Name, GatesName} {
		if !seen[want] {
			t.Errorf("All() missing skill %q", want)
		}
	}
}

// TestInstallWritesEverySkillIntoEveryBase is the drift guard between the
// generator's list and what a user actually receives from `gatehouse init`.
func TestInstallWritesEverySkillIntoEveryBase(t *testing.T) {
	root := t.TempDir()
	written, err := Install(root)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if got, want := len(written), len(All())*len(InstallBases); got != want {
		t.Fatalf("Install wrote %d path(s), want %d", got, want)
	}
	for _, s := range All() {
		for _, base := range InstallBases {
			rel := filepath.Join(base, s.Name, "SKILL.md")
			data, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			if string(data) != s.Markdown() {
				t.Errorf("%s content does not match %s Markdown()", rel, s.Name)
			}
		}
	}
}

// TestCommittedGatesSkillMatchesGenerator is the `make lint` drift check for the
// second skill: the committed file is generated, never hand-edited.
func TestCommittedGatesSkillMatchesGenerator(t *testing.T) {
	path := filepath.Join("..", "..", "skills", GatesName, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed skill: %v (run `make skill`)", err)
	}
	if string(data) != Gates.Markdown() {
		t.Errorf("%s is stale; run `make skill` and commit the result", path)
	}
}

// flattenSpace collapses every run of whitespace to a single space so a prose
// assertion matches regardless of where the source text wraps.
func flattenSpace(s string) string { return strings.Join(strings.Fields(s), " ") }
