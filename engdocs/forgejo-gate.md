# The Forgejo Landing Gate

This fork lands changes on a local **Forgejo** server, not on GitHub. GitHub
Actions is disabled on the GitHub fork, so the workflows under `.github/` do not
run here; Forgejo runs the gate instead.

## How a change lands

1. The landing worker pushes a candidate branch named `land/<bead>` (for
   example `land/be-bl8`).
2. Forgejo runs the single `gate` job in `.forgejo/workflows/gate.yml`. It
   checks out the branch and runs `make gate`, which runs the lint tier
   (`make ci-pr-lint`) and then the unit test tier (`make test`).
3. The job is the required status context `ci / gate (push)`.
4. The branch is merged through a pull request against the protected `main`
   branch, and the gate must be green before the merge is allowed.

## Mirrors and publishing

GitHub `main` is a read-only mirror of the Forgejo `main`; push changes to
Forgejo, never to GitHub.

Nothing in this fork deploys or publishes. There is no release, PyPI, or Pages
publishing from it, and the gate uses no secrets or cloud credentials.
