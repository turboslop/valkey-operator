# Contributing

Thanks for improving valkey-operator. Keep changes small, reproducible, and tied to an issue when possible.

## Development Setup

Install the tools listed in the README prerequisites, then verify the repository:

```sh
make V=1
make test
golangci-lint run
```

`make test` uses envtest and excludes the e2e suite. Run the e2e tests separately against a Kind or minikube cluster:

```sh
make test-e2e
```

For minikube, point the image loader at the active profile:

```sh
make test-e2e E2E_CLUSTER_RUNTIME=minikube MINIKUBE_PROFILE=minikube
```

## Pull Requests

Before opening a pull request:

- Use a conventional PR title such as `fix: validate shard count`.
- Link the related issue in the PR body.
- Include the validation commands you ran.
- Add tests or explain why tests are not practical for the change.

## Issues

Bug reports should include a minimal Valkey custom resource, Kubernetes/operator versions, logs, and expected behavior. Feature requests should include acceptance criteria so maintainers can tell when the work is complete.

## Security

Do not report critical vulnerabilities in public issues. Follow `SECURITY.md` for disclosure guidance.
