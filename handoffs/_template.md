# Handoff — <branch-slug>

<!--
One handoff file per branch, at `handoffs/<branch-slug>.md`. Resolve the slug with:
  bash/zsh:   slug=$(git branch --show-current | tr '/' '-'); slug=${slug:-detached-$(git rev-parse --short HEAD)}
  PowerShell: $slug = (git branch --show-current) -replace '/','-'
              if (-not $slug) { $slug = "detached-$(git rev-parse --short HEAD)" }
The detached fallback is load-bearing: `--show-current` prints nothing on a
detached checkout, so without it every detached HEAD shares one `handoffs/.md`
and silently overwrites the others' state.
Copy this template to that path when you start a feature; keep it updated as you go.
Per-branch paths never collide, so parallel branches/worktrees never merge-conflict
on their handoffs. Do NOT funnel session notes into one shared file. For cross-repo
status, use the issue tracker (e.g. Linear), not a committed file.

Handoffs are WORKING STATE, NOT HISTORY: `handoffs/*` is gitignored, so your
handoff never lands in a PR or piles up on the default branch. Only this
`_template.md` seed is committed. Anything here that outlives the branch must
graduate before you finish — decisions to the decision log, system knowledge to
the spec, process knowledge to the agent instructions. If it only lives here, it
dies with the branch.
-->

## Current Objective

- **Last updated:** YYYY-MM-DD HH:MM
- **Active feature:** [Linear ticket id — title]
- **Goal:** [what this branch is trying to accomplish]
- **Status:** [in progress / blocked / ready for review]

## Done This Session

- [x] [completed item]

## In Progress / Next

1. [next action]
2. [following action]

## Blockers / Risks

- [blocker: impact / mitigation]

## Decisions Made

- **[decision]** — context, alternatives considered. (Durable decisions graduate
  to a decision log / spec; don't leave them buried here.)

## Files Changed

- `path/to/file` — [what changed]

## Verification Evidence

| Check | Command | Result | Notes |
|---|---|---|---|
|  |  |  |  |

## Next-Session Startup

1. Read `AGENTS.md`.
2. Read the Linear ticket for the active work and its done criteria.
3. Read this handoff (`handoffs/<branch-slug>.md` for your branch).
4. Run `./init.sh` (or the documented verification command) before editing.
