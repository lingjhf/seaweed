# Release Readiness

This project is in the `0.x` development line. Do not tag, push, or publish a release unless explicitly requested.

## Required Gates

Run the CI-aligned gate first:

```bash
make ci
```

Run the full local release gate with a real SeaweedFS `weed` binary:

```bash
WEED_BINARY=./weed make release-check
```

`release-check` runs unit tests, race tests, `go vet`, integration tests, and the combined production coverage gate.

## Documentation Checks

Before release, verify:

- `CHANGELOG.md` has an `Unreleased` section describing user-visible changes.
- `README.md` examples compile through `go test ./...`.
- Integration tests use a real `weed` binary through `WEED_BINARY`.

## Version Rules

- Use plain semantic version tags such as `0.2.0` or `1.0.0`.
- Do not use a `v` prefix.
- Do not reuse or decrease an existing tag.
- For the current `0.x` development line, SDK APIs can change while the client is being shaped.
- For a stable `1.x` line, breaking SDK APIs require a major version.

## Current Release Notes

The current unreleased changes include API updates for endpoint configuration, S3/IAM endpoint policy, Blob lookup caching, TUS uploads, and the Filer client. Do not publish a release until the full gate passes and the changelog matches the current API.
