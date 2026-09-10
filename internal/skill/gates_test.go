package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

// TestGatesSkillFrontmatter pins the invocation contract as a skill loader
// reads it: the header is parsed as YAML and the values are asserted, because
// a substring match reports green on a header no parser accepts. This skill is
// deliberately NOT model-invocable: authoring a repository gate changes what
// validating the repository means, so it runs when a human asks for it and
// never because an agent guessed the task looked related.
func TestGatesSkillFrontmatter(t *testing.T) {
	got := parseFrontmatter(t, Gates.Markdown())
	want := frontmatter{
		Name:                   GatesName,
		Description:            GatesDescription,
		UserInvocable:          true,
		DisableModelInvocation: true,
	}
	if got != want {
		t.Errorf("gates frontmatter = %+v, want %+v", got, want)
	}
	if strings.Contains(Gates.Markdown(), "internal: true") {
		t.Errorf("Gates.Markdown() must not be marked internal")
	}
}

// TestEverySkillRendersParseableFrontmatter is the render-boundary guard: a
// description carrying YAML syntax (a colon followed by a space is enough)
// must not produce a header a loader refuses, which would silently withhold
// the skill from the user.
func TestEverySkillRendersParseableFrontmatter(t *testing.T) {
	for _, sk := range All() {
		t.Run(sk.Name, func(t *testing.T) {
			got := parseFrontmatter(t, sk.Markdown())
			if got.Name != sk.Name {
				t.Errorf("name = %q, want %q", got.Name, sk.Name)
			}
			if got.Description != sk.Description {
				t.Errorf("description = %q, want %q", got.Description, sk.Description)
			}
			if !got.UserInvocable {
				t.Errorf("user-invocable = false, want true")
			}
			if got.DisableModelInvocation != sk.DisableModelInvocation {
				t.Errorf("disable-model-invocation = %v, want %v", got.DisableModelInvocation, sk.DisableModelInvocation)
			}
		})
	}
}

// TestFrontmatterSurvivesAYAMLHostileDescription reproduces the defect at the
// boundary rather than through today's descriptions, so rewording one cannot
// retire the guard.
func TestFrontmatterSurvivesAYAMLHostileDescription(t *testing.T) {
	hostile := `Do a thing: then #another, "quoted" - {inline: map} | and more`
	sk := Skill{Name: "hostile", Description: hostile, Body: "\nbody\n"}
	if got := parseFrontmatter(t, sk.Markdown()); got.Description != hostile {
		t.Errorf("description round-trip = %q, want %q", got.Description, hostile)
	}
}

// TestPipelineSkillStaysModelInvocable guards the asymmetry: the pipeline skill
// must keep activating on its own, because an agent asked to ship work should
// reach for the gate without being told.
func TestPipelineSkillStaysModelInvocable(t *testing.T) {
	if parseFrontmatter(t, Markdown()).DisableModelInvocation {
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
		"name length":         fmt.Sprintf("at most %d characters", types.MaxCustomGateLabelLen),
		"gate cap":            fmt.Sprintf("At most %d gates", config.MaxGates),
		"instructions cap":    fmt.Sprintf("may not exceed %d bytes", config.MaxGateInstructionsBytes),
		"activation rule":     "trusted default-branch copy",
		"no effect this run":  "does not affect that branch's own run",
		"pinned at creation":  "resolves its gate list once",
		"park not autofix":    "parks for a decision instead of auto-fixing",
		"additive only":       "never replace, reorder, or remove a core step",
		"repo local":          "no global or machine-wide",
		"log step name":       "gate.<anchor>.<name>",
		"hardening":           "disable_project_settings",
		"hardening scope":     "applies to every pipeline agent step",
		"hardening closes":    "fails the run closed on a harness without verified suppression",
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
//
// Line endings are normalized before comparing because the two sides cannot
// agree on them across platforms. Markdown() is always LF: the body is a raw
// string literal, and Go discards carriage returns inside those, so the
// generator emits LF even when its own source was checked out as CRLF. The
// committed file, by contrast, carries whatever the checkout wrote - CRLF under
// the `core.autocrlf=true` that Windows CI runners default to. Comparing raw
// bytes therefore asserts the checkout's eol configuration rather than drift.
func TestCommittedGatesSkillMatchesGenerator(t *testing.T) {
	path := filepath.Join("..", "..", "skills", GatesName, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed skill: %v (run `make skill`)", err)
	}
	if normalizeEOL(string(data)) != normalizeEOL(Gates.Markdown()) {
		t.Errorf("%s is stale; run `make skill` and commit the result", path)
	}
}

// normalizeEOL rewrites CRLF to LF so a generated file's content can be
// compared independently of how git's eol translation wrote it to disk.
func normalizeEOL(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// flattenSpace collapses every run of whitespace to a single space so a prose
// assertion matches regardless of where the source text wraps.
func flattenSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestGatesSkillExamplesAreAcceptedByTheRealConfigParser runs the examples the
// skill hands an agent through config.LoadRepoFromBytes, the same parser the
// daemon uses to read a repository's .gatehouse.yaml. The skill exists because
// an agent without these rules writes a gate that is rejected at parse time, so
// an example the parser refuses would teach exactly the failure it prevents.
// The derived step name is asserted too, because the skill's Verify section
// tells the user to read that gate's log with `axi logs --step gate.test.
// mutation-budget`, which only resolves if the parser agrees.
func TestGatesSkillExamplesAreAcceptedByTheRealConfigParser(t *testing.T) {
	examples := gateExamplesFrom(Gates.Markdown())
	if len(examples) < 2 {
		t.Fatalf("found %d gate example(s) in the skill body, want the command and instructions examples", len(examples))
	}
	var steps []string
	for i, ex := range examples {
		cfg, err := config.LoadRepoFromBytes([]byte(ex))
		if err != nil {
			t.Errorf("example %d is not valid repo config:\n%s\n%v", i, ex, err)
			continue
		}
		if len(cfg.Gates) != 1 {
			t.Errorf("example %d parsed to %d gate(s), want 1:\n%s", i, len(cfg.Gates), ex)
			continue
		}
		steps = append(steps, string(cfg.Gates[0].StepName()))
	}
	want := []string{"gate.test.mutation-budget", "gate.lint.no-cli-imports"}
	if len(steps) != len(want) {
		t.Fatalf("parsed step names = %v, want %v", steps, want)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Errorf("example %d step name = %q, want %q", i, steps[i], want[i])
		}
	}
	// The Verify section documents the first example's step name verbatim, so
	// the log command it tells the user to run must name the step the parser
	// actually produces.
	if !strings.Contains(flattenSpace(Gates.Markdown()), "`"+want[0]+"`") {
		t.Errorf("the Verify section no longer names %q, the step the parser derives", want[0])
	}
}

// gateExamplesFrom lifts every indented `gates:` example out of the rendered
// skill body and dedents it back into a standalone YAML document, so the
// examples can be fed to the real parser exactly as an agent would copy them.
func gateExamplesFrom(md string) []string {
	const indent = "    "
	var out []string
	lines := strings.Split(md, "\n")
	for i := 0; i < len(lines); i++ {
		if lines[i] != indent+"gates:" {
			continue
		}
		var block []string
		for ; i < len(lines); i++ {
			line := lines[i]
			if strings.TrimSpace(line) == "" || !strings.HasPrefix(line, indent) {
				break
			}
			block = append(block, strings.TrimPrefix(line, indent))
		}
		out = append(out, strings.Join(block, "\n")+"\n")
	}
	return out
}
