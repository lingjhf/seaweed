# SeaweedFS Go SDK

Go client SDK for SeaweedFS native HTTP APIs plus SeaweedFS S3/IAM compatibility.

This project is in the `0.x` development line. Public APIs can change between minor versions while the SDK is being shaped against real SeaweedFS behavior.

## Features

- Master client: assign, lookup, status, health, volume management helpers.
- Volume client: direct put, get, head, delete, status.
- Blob client: assign/lookup plus volume upload, read, delete.
- Filer client: put, append, get, head, stat, list, mkdir, delete, copy, move, tagging.
- TUS client: native SeaweedFS resumable upload support for `/.tus`.
- S3/IAM clients: AWS SDK v2 clients configured for SeaweedFS endpoints.

## Basic Usage

```go
client, err := seaweed.New(seaweed.Config{
    MasterURL:   "http://127.0.0.1:9333",
    FilerURL:    "http://127.0.0.1:8888",
    TUSBasePath: "/.tus",
    S3URL:       "http://127.0.0.1:8333",

    AccessKeyID:     "seaweed_admin",
    SecretAccessKey: "seaweed_secret",
    Region:          "us-east-1",
})
if err != nil {
    return err
}
```

See `examples/basic` and `examples/s3`.

Direct package clients such as `master.New`, `volume.New`, `filer.New`, and `tus.New` return `(*Client, error)` and accept standard `*http.Client` configuration. They do not expose SDK internal transport types.

## Validation

The full local gate used before every commit is:

```bash
make test
make test-race
make vet
WEED_BINARY=./weed make integration
WEED_BINARY=./weed make coverage
```

Integration tests require a real SeaweedFS `weed` binary. The test harness resolves `WEED_BINARY` first and then project-root `./weed`.
Coverage gates require at least `85.0%` combined production coverage from unit and integration profiles, excluding `examples/*` and `internal/testweed`.

## Notes

- S3/IAM uses AWS SDK for Go v2 and path-style S3 addressing.
- IAM defaults to the S3 endpoint because SeaweedFS embeds IAM in the S3 server by default.
- TUS implements SeaweedFS-supported TUS 1.0.0 operations: creation, creation-with-upload, offset upload, resume, and termination.
