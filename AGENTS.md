# Repository Instructions

## Pull Requests and Commits

PR titles and commit subjects must use Conventional Commits format:

```text
<type>(<scope>): <description>
```

Allowed types are `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `ci`, `perf`, `style`, `build`, and `revert`. The scope is optional, but the colon and description are required.

Examples:

```text
test(e2e): add minikube smoke test
ci: pin minikube version
fix(controller): preserve generated password
```

Do not prefix PR titles with `[codex]` or similar labels. Use the same Conventional Commit style for the PR title as for the commit subject.

## Pre-Push Validation

Before pushing a branch, run the Go test suite and Go lint for the changed code. Prefer repository targets when they exist:

```text
make test
make lint
```

If a full e2e run is relevant, run it separately against a prepared cluster. If a validation command cannot be run, mention the exact blocker in the PR body or final handoff.
