// Package gateguidance owns the shared phase-ownership contract rendered into
// every validation-step prompt and the installed gatehouse skill.
package gateguidance

import "fmt"

// SkillBoundary is the static defense-in-depth guard installed for agents.
// Runtime classification remains authoritative; GATEHOUSE_GATE alone never
// decides whether an ordinary operator command is allowed.
const SkillBoundary = `
## Active validation-step boundary

A gatehouse validation-step agent is already inside an active outer run. It
must inspect, fix, and return only its assigned phase. It must never initialize,
start, reattach, rerun, respond to, synchronize, abort, eject, or directly push
a gatehouse pipeline. Delivery requirements in user intent remain
acceptance context, but the outer executor alone performs the other validation,
push, PR, and CI phases.

` + "`GATEHOUSE_GATE`" + ` is fast diagnostic evidence, not authorization by
itself. The runtime combines managed Git identity with authenticated process
ancestry. If a pipeline-control command returns
` + "`error.code: nested_gate_context`" + `, stop immediately and
return control to the outer executor. Safe inspection remains available through
` + "`gatehouse axi status`" + `, ` + "`gatehouse axi logs`" + `, help, and
` + "`gatehouse doctor`" + `.
`

// PromptBoundary is prepended centrally to every concrete agent invocation.
func PromptBoundary(phase string) string {
	if phase == "" {
		phase = "current"
	}
	return fmt.Sprintf(`Gate-step phase boundary:
- You are the %s phase inside an already active gatehouse run. Inspect, fix, and return only this assigned phase.
- Never invoke gatehouse init, axi run, rerun, respond, sync, abort, eject, or directly push a gate. Never initialize or control another pipeline.
- Delivery requirements in user intent remain authoritative acceptance context for evaluating this change. Do not personally execute other validation, push, PR, or CI phases; the outer executor alone owns every phase other than this assigned one.
- When this phase is complete, return its requested structured result to the outer executor.

`, phase)
}
