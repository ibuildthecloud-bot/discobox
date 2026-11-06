# Release UX re-review

This follow-up to the [2026-11-04 walk](../README.md) reviews the experience
after #632 (welcome removal), #633 (guided tour), #634 (minimal attach),
#635 (approval forms), #587 (required Docker UX smoke) and #592 (cold startup).
The original captures remain historical evidence; they do not describe the
current onboarding or approval forms. Owner acceptance is still outstanding.

## Capture protocol

Walked 2026-11-06. The development CLI/server reports `6521462cc791+dirty`:
that baseline plus this change's help and approval-row edits. All observations
used Linux. The baseline help capture predates those edits. Every host call
uses
`http://127.0.0.1:8080` explicitly. Disposable boxes
and harness configs isolate the walkthrough from the enclosing control plane.
PNG files are native Kitty 0.49.2 window screenshots, captured with scrot on
Xvfb using Mesa software rendering. Matching `.ansi` and `.txt` files are live
tmux pane captures. No image reconstruction or fixture rendering is used.

## Coverage to retain

These tests run in `ci:test` through the CLI module suite unless indicated.

| Experience | Regression coverage |
| --- | --- |
| Grouped help, aliases, hidden options, color policy | `cli/internal/cli/help_test.go` |
| send → send-key → wait → capture, resume point, timeout 124 | `cli/internal/cli/terminal_drive_test.go`; required `ci:ux-smoke` real primary terminal |
| exec and cold preparation | `cli/internal/cli/exec_test.go`, `provisionwatch_loop_test.go`, `server/internal/resources/sandboxes/attach_wait_test.go` and terminal-condition tests from #592 |
| Harness list, file origins, reset preserving personal overrides | `cli/internal/tui/harnesses_test.go`, `cli/internal/cli/tui_harnesses_test.go`; required `ci:ux-smoke` saves settings and a synthetic login, resets, and checks a fresh box |
| Changes band, explainer, displayed candidate save/discard/errors | `cli/internal/tui/harness_changes_test.go`, `cli/internal/cli/tui_harness_changes_test.go` |
| Console opens directly on boxes or the empty-project prompt | `cli/internal/tui/model_test.go` opening-state tests |
| Minimal run/attach with background services; actions open services; shells/banners restore chrome | `cli/internal/tui/bare_test.go` |
| Primary pre-harness tour gate and recovery | `sandbox-agent/terminal/primary_gate_test.go`, `sandbox-agent/cmd/discobox-sandbox-agent/primary_test.go`; see accepted launch contract in `docs/adr/26-11-05-719-a-repository-can-run-an-interactive-tour-before-its-primary-harness.md` |
| Adoption help, approval/denial/release, authority and pending errors | `cli/internal/cli/adoption_test.go`, `cli/internal/tui/adoptions_test.go`; required `ci:ux-smoke` checks real sandbox identity, parent changes and unchanged creator |
| Use, delegation, API access, adoption, host trust and refresh approvals | `cli/internal/tui/approval_controls_test.go` and request-specific tests; [full live form walk](../../635/README.md) |

The required Linux `ux-smoke` workflow calls `ci:ux-smoke` without opt-in gates.
It retains command output, screens and journals on failure; missing prerequisites
fail the lane. Model/render tests cover console interaction in addition to that
CLI/server/pool/sandbox integration lane.


## Findings and disposition

| Finding | Disposition |
| --- | --- |
| Explicit help value names bypass uppercase formatting (`--git-credentials strategy`) | Fixed in the shared help renderer; the regression fails before the fix and checks root/run/attach output |
| Unnamed single-server approvals draw a blank Server row | Fixed in credential use/delegation, API access and host-trust cards; tests cover unnamed and named servers; live API, combined credential and trust cards exercised |
| Restart during background apt work leaves dpkg interrupted | Filed [#761](https://github.com/discobox-ai/discobox/issues/761); the shell fixture was restarted before optional provisioning finished |
| Existing Docker pool stays not ready after a control-plane restart | Filed [#762](https://github.com/discobox-ai/discobox/issues/762); restarting only its pool agent restored readiness without restarting boxes |
| A fresh tour box's Go archive extraction fails with unexpected EOF | Filed [#763](https://github.com/discobox-ai/discobox/issues/763); the tour itself runs, but its background provision state is failed |

The service fixture explicitly declares `features: [core]`: omitting that list
waits on the default feature set, including the failed Go install. Its running
state is recorded in [service-state.txt](service-state.txt). No provisioning
recovery fix is claimed by this PR.

## Discovery and terminal commands

![Grouped colored help, including uppercase STRATEGY](help.png)

[Baseline help](help-before.png) · [Adoption help](adopt-help.png) ·
[send](send-help.txt) · [send-key](send-key-help.txt) ·
[capture](capture-help.txt) · [wait](wait-help.txt) ·
[exec](exec-help.txt) · [release](release-help.txt).

The real shell received send → send-key Enter → wait idle → capture.
[Output](terminal-output.txt) contains the separately constructed marker;
[wait](wait-output.txt) returned idle; [timeout](wait-timeout.txt) exited 124.
[Exec](exec-output.txt) returned its output. An exec issued while the fresh
shell was coming up displayed “waiting for the discobox to come up” and then
[completed](cold-exec.png). The retained screenshot shows completion, not the
transient waiting row. An [unknown terminal selector](missing-terminal.txt)
still failed explicitly.

## Console, minimal attachment and services

![Console opens directly on the box list, without a welcome card](console.png)

![Attach stays minimal with a running background service](attach-service-minimal.png)

![Actions exposes services](attach-actions.png)

![Service picker](services-picker.png)

![Choosing View output restores the wrapped UI and shows live service output](service-output.png)

[Service actions](service-selected.png) include view/start/stop/restart.
The [CLI log](service-output-cli.txt) records the same marker. Opening a
second shell restored two-pane chrome, while leaving the tour running:

![A real auxiliary shell beside the tour](tour-extra-shell.png)

## Tour before the harness

The exact public source `discobox-ai/tour` resolved to `main@dd09de6`.
The command used `run -d -H shell discobox-ai/tour`, followed by attach,
so the tour could be inspected before starting an agent account session.

![Greeting before the selected harness](tour-greeting.png)

![Six independent chapters and explicit continuation](tour-menu.png)

The walk viewed [what this box is](tour-box.png),
[opening a shell](tour-shell.png), the tools chapter and
[real tools picker](tools-picker.png), [secrets and requests](tour-requests.png),
[harnesses](tour-harnesses.png), and [leaving and returning](tour-returning.png).
The auxiliary shell wrote and read `tour-note.txt` in the copied checkout;
the [live diff tool](tour-diff-tool.png) showed that untracked file.
A replay after leaving the tour was also exercised.

The access chapter made a real, narrowly scoped disposable credential request;
mouse Deny left it denied, and the tour returned to its menu. A separately
requested API-access approval was accepted in the same box; it is not required
by the tour. Continue returned to the shell and wrote the durable completion
marker. [Completion evidence](tour-completion.txt) also shows a later direct
hook invocation returning without reopening the menu.

![Explicit Continue returns to the selected shell](tour-continued.png)

## Harness setup, review, saving and reset

The isolated `ux-582` variant followed the built-in Claude bundle, with its
own account state. It started signed out; no external account was used.

![Signed-out Claude and green changes band](harness-band.png)

![First-time sharing explanation](harness-explainer.png)

![Displayed immutable settings candidate](harness-review.png)

Saving from that review succeeded even after adding a personal file through
an independent harness update. [Saved state](harness-saved.json) contains the
reviewed light-theme settings and preserves `personal.txt`. The first
candidate's [CLI preview](harness-preview.txt) records its exact save command.
A later, distinct settings edit produced the first-time explainer, then was
[discarded](harness-discard.txt).

![Harness list](harnesses.png)

![Reset offers to keep personal files](reset-overrides.png)

Mouse/keyboard review saved the displayed candidate; the reset menu's default
“Reset, keep your files” removed configured settings and retained the personal
file ([state](harness-reset.json)). A subsequent CLI reset with
`--include-overrides` removed that file too ([state](harness-reset-all.json)).
The required smoke suite separately exercised synthetic credential save/reset;
this Claude walk changed settings only.

## Sandbox approval forms

These native screenshots replace the old adoption form in the original walk.
The broader [#635 form walkthrough](../../635/README.md) remains the evidence
for refresh, new-credential entry, picker extremes and read-only adoption;
those paths were not repeated here.

![Credential use, first of a combined request](credential-approval.png)

![Credential delegation, second of that request](delegation-approval.png)

![Final confirmation commits both decisions together](credential-confirmation.png)

The dummy `ux582-demo` value was `tour-example-only`, with only
`GET tour.discobox.invalid/demo` requested for use and delegation, for ten
minutes. Both parts remained pending until final consent; the
[CLI receipt](combined-approved-cli.txt) records approval. No network call
used that dummy credential, and no real account credential was used.

![API access after removing the blank Server row](api-approval.png)

[Before](api-approval-before.png) ·
[80×24 before reading](api-approval-80x24.png) ·
[80×24 after reading](api-reviewed-80x24.png) ·
[approval history](api-request-cli.txt).

The first API request was approved, access was disabled again, and a second
request was accepted after scrolling its compact summary. The actions stayed
visible while the unread-summary gate prevented premature decisions.

![Adoption identifies both boxes and effective authority](adoption-approval.png)

[80×24](adoption-approval-80x24.png) ·
[read to the end](adoption-reviewed-80x24.png).
The real requester adopted the separately created shell box. The child's
[before](adoption-before.json), [approved](adoption-approved.json) and
[released](adoption-released.json) records retain its user creator while the
parent changes. [Release output](release-output.txt) returned successfully.
A second request was denied ([requester output](adoption-denied-cli.txt));
[both decision records](adoption-requests.json) agree. The required smoke
suite additionally checks a nonempty sandbox creator staying unchanged.

![Trust request with a locally generated self-signed certificate](trust-approval.png)

The certificate came from an isolated OpenSSL listener in the dev pool.
The live form omitted the empty server row, showed its certificate/pin and
finite duration, and was denied ([CLI result](trust-request-cli.txt)). No
trust was granted and the temporary key is not included here.

## Validation and limits

- Full CLI and TUI package suites passed after both polish changes
  ([results](package-tests.txt)). Each new regression was observed failing
  before its fix and passing after it.
- `go tool task check-hooks` passed ([result](hooks.txt)).
- `ci:ux-smoke` passed all three real-box groups and cleanup
  ([summary](ux-smoke.txt)). Its binary was built from the starting baseline
  before the polish edits; final-head CI is recorded on the PR.
- The manual walk used the baseline plus this working tree. Reopening the
  client after the form edit exercised the changed API, credential and trust
  rendering. [Build version](build-version.txt), [dev build check](dev-build-check.txt).

Runtime verification was Linux-only. macOS/Windows, a fresh installer, external
agent sign-in/OAuth, real credential sharing, and experimental VM drivers were
not exercised. The tour's shell selection was tested; continuation into an agent
is covered by #633's delivery evidence, not repeated here. Refresh and the more
specialized request forms retain #635's evidence and automated coverage.

The three manual boxes, isolated harness and dummy credential were removed;
the test listener and private tmux/Xvfb session were stopped. Smoke cleanup
removed its separate boxes and pool. The development loop and its seeded pool
are retained.

**Owner review is outstanding.** These captures and passing checks are evidence
for that decision, not release sign-off.
