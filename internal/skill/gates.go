package skill

import (
	"fmt"

	"github.com/BertramPetersen/gatehouse/internal/config"
	"github.com/BertramPetersen/gatehouse/internal/types"
)

// GatesName is the skill directory and frontmatter name, so agents expose it as
// the /gatehouse-gates command.
const GatesName = "gatehouse-gates"

// GatesDescription is the frontmatter description. This skill is not
// model-invocable, so the description is a listing entry for a human choosing
// it rather than a trigger an agent matches against.
const GatesDescription = "Add a repository gate to .gatehouse.yaml from a description of what it should enforce: choosing command or instructions, the anchor step, a valid name, the size limits, and the trust rules that decide when it takes effect. Invoke with /gatehouse-gates."

// Gates teaches an agent to author a repository gate. Model invocation is
// disabled deliberately: a gate defines what validating a repository means, and
// declaring one is an owner's decision, so this must run because a human asked
// for it and never because a task looked related. Authoring a gate is also not
// something a validation run should ever do to itself.
var Gates = Skill{
	Name:                   GatesName,
	Description:            GatesDescription,
	Body:                   gatesBody,
	DisableModelInvocation: true,
}

// gatesBody is the instruction text an agent reads when the skill activates. It
// carries the rules an agent cannot infer from the config file it is editing:
// the activation boundary, the valid anchors, the naming syntax, and the size
// caps. Keep it static - no live state belongs here.
//
// The caps are interpolated from the constants the parser actually enforces, so
// raising one cannot leave the installed skill teaching agents a stale limit.
var gatesBody = fmt.Sprintf(`
# gatehouse-gates

Add a repository gate to a repository's `+"`.gatehouse.yaml`"+` from a description of
what it should enforce. A gate is an extra check that runs inside the gatehouse
pipeline, before the branch is pushed, alongside the core steps.

Gates are repo-local. They live in the repository's own `+"`.gatehouse.yaml`"+`, and
there is no global or machine-wide gates setting, because a gate declares what
validating *this* repository means.

## 1. Read before you write

Read the repository's existing `+"`.gatehouse.yaml`"+` first. You need it to pick a
`+"`name`"+` that is not already taken, to check the count against the cap below, and
to match the file's existing comment style.

If the file has no `+"`gates:`"+` key, add one at the top level. Never create a second
`+"`gates:`"+` block.

## 2. Decide the shape

Every gate sets `+"`name`"+`, `+"`after`"+`, and exactly one of `+"`command`"+` or `+"`instructions`"+`.

Use `+"`command`"+` when the rule is mechanically checkable. It runs in the run
worktree and passes on exit code 0:

    gates:
      - name: mutation-budget
        after: test
        command: "make mutation"

Use `+"`instructions`"+` when judging the rule requires reading the change. An agent
evaluates the diff against that text alone and reports structured findings; on
that turn it is told to report only and to leave the worktree alone:

    gates:
      - name: no-cli-imports
        after: lint
        instructions: |
          No package under internal/ may import internal/cli.

Prefer `+"`command`"+` whenever a deterministic check exists or is cheap to write: it
is faster and its verdict is reproducible. Reach for `+"`instructions`"+` for rules
about structure, layering, or naming that no exit code captures.

Do not restate what a core step already does. A gate earns its place by checking
something the core pipeline does not.

## 3. Choose the anchor

`+"`after`"+` names the core step the gate runs immediately after. Valid anchors are
`+"`rebase`"+`, `+"`review`"+`, `+"`test`"+`, `+"`document`"+`, `+"`lint`"+`. Pick the step whose subject
matter the gate extends: a coverage or mutation budget after `+"`test`"+`, an
architectural rule after `+"`lint`"+`.

`+"`intent`"+` and the delivery tail (`+"`push`"+`, `+"`pr`"+`, `+"`ci`"+`) cannot be anchored. A gate
after push would be validating a branch the world can already see, and `+"`intent`"+`
establishes the acceptance criteria the later gates check against.

Gates are inserted into the run's step sequence and never replace, reorder, or
remove a core step, so configuring gates can only make a pass mean *more*. There
is no way to switch a core step off here; to drop one for a single run the user
passes `+"`--skip`"+` instead.

## 4. Name it

`+"`name`"+` must be lowercase letters, digits, and inner hyphens, at most %d
characters, unique within the file, and not a core step name. The name becomes
the gate's step identity as `+"`gate.<anchor>.<name>`"+`, which is also its log
filename, so it has to stay path-safe.

## 5. Respect the limits

At most %d gates are allowed, and an `+"`instructions`"+` value may not exceed %d
bytes, because it shares the agent prompt's budget.

Merge-conflict markers are stripped from `+"`instructions`"+`, and a value left empty
once they are removed is rejected, so write the rule without them.

A malformed entry fails when the config is parsed and the run aborts before any
gate starts. That holds for whichever copy is parsed, including a pushed
branch's, so a broken gate surfaces before it merges.

## 6. Tell the user how it activates

This is the step most often missed. A gate is honored only from the trusted
default-branch copy of `+"`.gatehouse.yaml`"+`, regardless of `+"`allow_repo_commands`"+`,
because it either executes shell on the daemon host or steers a gate agent.

So once you have edited the file on a branch, state plainly that the gate takes
effect when the change reaches the default branch, and that adding it on a
feature branch does not affect that branch's own run.

A run also resolves its gate list once, when it starts, and keeps it for its
whole lifetime including across a daemon restart, so adding a gate never
retargets a run that is already in flight or parked.

## 7. Set expectations about failure

A failing gate parks for a decision instead of auto-fixing, and every finding an
agent gate raises is escalated the same way whatever action the agent itself
assigned. A gate states a repository rule, so deciding that the change should be
altered to satisfy it is the author's call, never the pipeline's.

Answering that decision with `+"`fix`"+` is the authorization: the gate runs a fix turn
against the reported findings and its own requirement, then re-runs its check.
Answering `+"`approve`"+` accepts the change as it stands.

A gate cannot be pre-skipped either: neither `+"`--skip`"+` nor the `+"`gatehouse.skip=`"+`
push option accepts a gate step name.

## 8. Harden it when a contributor must not be able to weaken it

The trust boundary protects the gate's *declaration*, not the repository files a
`+"`command`"+` gate goes on to invoke. The command runs in the worktree checked out at
the pushed head, so whoever can edit the script or make target it calls can still
change what it actually checks.

An `+"`instructions`"+` gate's agent runs in that same worktree, so with
`+"`disable_project_settings`"+` left at its default the branch's own `+"`AGENTS.md`"+`,
`+"`CLAUDE.md`"+`, or harness project settings reach it alongside the trusted rule and
can steer the verdict.

When that matters, state the rule as `+"`instructions`"+` and set
`+"`disable_project_settings: true`"+` on the trusted copy, or point `+"`command`"+` at logic
that does not live in the repository.

`+"`disable_project_settings`"+` is not scoped to the gate you are adding. It is a
repository-wide setting that applies to every pipeline agent step, and it fails
the run closed on a harness without verified suppression - only `+"`claude`"+`,
`+"`codex`"+`, and `+"`pi`"+` qualify today, and only while `+"`agent_args_override`"+` does not
override the knob. On a repository configured for any other agent, turning it on
means every later run stops before its first step instead of hardening one gate.
So check the configured agent first, and tell the user it changes the whole
pipeline rather than presenting it as gate-local hardening.

## 9. Verify

Each gate keeps its own step log, read with:

    gatehouse axi logs --step gate.<anchor>.<name>

A gate declared as `+"`name: mutation-budget`"+` with `+"`after: test`"+` is read as
`+"`gate.test.mutation-budget`"+`.
`, types.MaxCustomGateLabelLen, config.MaxGates, config.MaxGateInstructionsBytes)
