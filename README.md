# SeaweedFS Go SDK

Go client SDK for SeaweedFS native HTTP APIs plus SeaweedFS S3/IAM compatibility.

This project is in the `0.x` development line. Public APIs can change between minor versions while the SDK is being shaped against real SeaweedFS behavior.

## Features

- Master client: assign, lookup, `/submit`, status, health, volume management helpers.
- Volume client: direct put, get, head, delete, official read/write options, typed status.
- Blob client: assign/lookup plus volume upload, read failover, head, delete, optional master-issued volume authorization.
- Filer client: put, append, multipart file or directory upload, get, head, stat, list, mkdir, delete, copy, move, tagging, per-request authorization.
- TUS client: native SeaweedFS resumable upload support for `/.tus`.
- S3/IAM clients: AWS SDK v2 clients configured for SeaweedFS endpoints.

## Basic Usage

Install the SDK:

```bash
go get github.com/lingjhf/seaweed
```

Create a root client for the SeaweedFS services you use:

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs:  []string{"http://127.0.0.1:9333"},
    FilerURLs:   []string{"http://127.0.0.1:8888"},
    TUSBasePath: "/.tus",
    S3URLs:      []string{"http://127.0.0.1:8333"},

    AccessKeyID:     "seaweed_admin",
    SecretAccessKey: "seaweed_secret",
    Region:          "us-east-1",
})
if err != nil {
    return err
}
defer client.Close()
```

See `examples/basic` and `examples/s3`.

Direct package clients such as `master.New`, `volume.New`, `filer.New`, and `tus.New` return `(*Client, error)` and accept standard `*http.Client` configuration. They do not expose SDK internal transport types.

By default, `seaweed.New` uses an SDK HTTP client with a larger idle connection pool than Go's default transport. Use `seaweed.NewHTTPClient` when passing the same tuned client to direct package constructors.

Native master, volume, blob, and filer clients support SeaweedFS JWT-secured deployments. Master assign and lookup responses expose the documented `Authorization` response header, volume and filer option structs accept per-request `Authorization`, and blob clients can pass master-issued volume tokens automatically with `EnableVolumeAuthorization`.

SeaweedFS clients accept endpoint lists, for example `MasterURLs`, `VolumeURLs`, `FilerURLs`, `S3URLs`, `IAMURLs`, and direct client `BaseURLs`. If `IAMURLs` is empty, IAM uses `S3URLs` because SeaweedFS serves IAM from the S3 service by default.

Endpoint lists use failover by default. Enable round-robin when you want retryable read requests to start from a different endpoint on each call:

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{
        "http://master-1:9333",
        "http://master-2:9333",
    },
    EndpointPolicy: seaweed.EndpointPolicy{
        Mode: seaweed.EndpointPolicyRoundRobin,
    },
})
if err != nil {
    return err
}
defer client.Close()
```

Health checks and circuit breakers are opt-in. Native non-retryable write requests still use one selected endpoint and are not replayed across endpoints. S3 and IAM clients rely on the AWS SDK retryer; the SDK resolves an endpoint before SigV4 signing for each SDK attempt.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://master-1:9333", "http://master-2:9333"},
    FilerURLs:  []string{"http://filer-1:8888", "http://filer-2:8888"},
    EndpointPolicy: seaweed.EndpointPolicy{
        HealthCheck: seaweed.EndpointHealthCheckPolicy{
            Enabled:          true,
            Interval:         5 * time.Second,
            Timeout:          time.Second,
            FailureThreshold: 2,
            SuccessThreshold: 1,
        },
        CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
            Enabled:          true,
            FailureThreshold: 3,
            OpenTimeout:      30 * time.Second,
        },
    },
})
if err != nil {
    return err
}
defer client.Close()
```

Use service-specific endpoint policies when S3 or IAM should scale independently from the native clients. S3 and IAM treat transport errors, HTTP 429, and HTTP 5xx as endpoint failures; HTTP 4xx responses are request-level failures and do not open the endpoint circuit breaker.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://master-1:9333"},
    S3URLs: []string{
        "http://s3-1:8333",
        "http://s3-2:8333",
    },
    AccessKeyID:     "seaweed_admin",
    SecretAccessKey: "seaweed_secret",
    S3EndpointPolicy: seaweed.EndpointPolicy{
        Mode: seaweed.EndpointPolicyRoundRobin,
        CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
            Enabled:          true,
            FailureThreshold: 2,
            OpenTimeout:      10 * time.Second,
        },
    },
})
if err != nil {
    return err
}
defer client.Close()

s3Client, err := client.S3(ctx)
if err != nil {
    return err
}
_, err = s3Client.ListBuckets(ctx, nil)
```

Blob reads discover volume replicas through master lookup. `Get` and `Head` use all returned locations for read failover; `Delete` uses the selected endpoint only. Use `BlobEndpointPolicy` when the blob read path should differ from the global policy, and set `BlobLocationCacheTTL` when cached volume locations should refresh periodically.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://master-1:9333", "http://master-2:9333"},
    BlobEndpointPolicy: seaweed.EndpointPolicy{
        Mode: seaweed.EndpointPolicyRoundRobin,
        CircuitBreaker: seaweed.EndpointCircuitBreakerPolicy{
            Enabled:          true,
            FailureThreshold: 2,
            OpenTimeout:      10 * time.Second,
        },
    },
    BlobLocationCacheTTL: 30 * time.Second,
})
if err != nil {
    return err
}
defer client.Close()
```

For SeaweedFS deployments with JWT-secured volume access, enable blob authorization or pass the master-issued token into direct volume requests:

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs:                []string{"http://master-1:9333"},
    EnableVolumeAuthorization: true,
})
if err != nil {
    return err
}
defer client.Close()

put, err := client.Blob().Put(ctx, strings.NewReader("secure-data"), blob.PutOptions{
    ContentLength: int64(len("secure-data")),
})
if err != nil {
    return err
}
fmt.Println(put.FileID)
```

Filer requests accept the token issued for your filer access policy on every operation:

```go
filerToken := "Bearer <jwt>"

_, err := client.Filer().Put(ctx, "/secure/hello.txt", strings.NewReader("secure-data"), filer.WriteOptions{
    ContentLength: int64(len("secure-data")),
    Authorization: filerToken,
})
if err != nil {
    return err
}

entry, err := client.Filer().Stat(ctx, "/secure/hello.txt", filer.StatOptions{
    Authorization: filerToken,
})
if err != nil {
    return err
}
fmt.Println(entry.FullPath)
```

## Usage Examples

### Blob Upload

Use the blob client when you want SeaweedFS to assign a file id and write directly to a volume server.

```go
ctx := context.Background()
data := "hello seaweedfs"

put, err := client.Blob().Put(ctx, strings.NewReader(data), blob.PutOptions{
    Collection:    "sdk",
    ContentType:   "text/plain",
    ContentLength: int64(len(data)),
    Filename:      "hello.txt",
})
if err != nil {
    return err
}

resp, err := client.Blob().Get(ctx, put.FileID, blob.GetOptions{})
if err != nil {
    return err
}
defer resp.Body.Close()

body, err := io.ReadAll(resp.Body)
if err != nil {
    return err
}
fmt.Println(string(body))
```

### Filer Files

Use the filer client for path-based file operations, metadata, directories, and tags. `ListPage` maps to one SeaweedFS filer page; use `Walk` when you want the SDK to follow `lastFileName` pagination for you.

```go
ctx := context.Background()
data := "hello filer"

_, err := client.Filer().Put(ctx, "/sdk/hello.txt", strings.NewReader(data), filer.WriteOptions{
    ContentType:   "text/plain",
    ContentLength: int64(len(data)),
    SeaweedHeaders: map[string]string{
        "Owner": "sdk",
    },
})
if err != nil {
    return err
}

_, err = client.Filer().UploadMultipart(ctx, "/sdk/uploads/multipart.txt", strings.NewReader(data), filer.MultipartUploadOptions{
    FileContentType: "text/plain",
    SeaweedHeaders: map[string]string{
        "Owner": "sdk",
    },
})
if err != nil {
    return err
}

head, err := client.Filer().Head(ctx, "/sdk/hello.txt", filer.HeadOptions{})
if err != nil {
    return err
}
fmt.Println(head.Header.Get("Seaweed-Owner"), head.Tags["Owner"])

entry, err := client.Filer().Stat(ctx, "/sdk/hello.txt", filer.StatOptions{})
if err != nil {
    return err
}
fmt.Println(entry.FullPath, entry.FileSize)

page, err := client.Filer().ListPage(ctx, "/sdk", filer.ListOptions{Limit: 100})
if err != nil {
    return err
}
for _, entry := range page.Entries {
    fmt.Println(entry.FullPath)
}

err = client.Filer().Walk(ctx, "/sdk", filer.WalkOptions{Limit: 100}, func(entry filer.Entry) error {
    fmt.Println(entry.FullPath)
    return nil
})
if err != nil {
    return err
}
```

### TUS Resumable Uploads

`Upload` uses SeaweedFS creation-with-upload by default. Set `ChunkSize` when you want explicit chunked PATCH uploads. This maps to SeaweedFS' TUS `OPTIONS`, `POST`, `HEAD`, `PATCH`, and `DELETE` endpoints under `/.tus`.

The SDK intentionally models the TUS subset SeaweedFS currently declares: `creation`, `creation-with-upload`, and `termination`. It does not send checksum, defer-length, expiration, or concatenation extension headers unless SeaweedFS adds and declares those capabilities in the future.

When using multiple filer endpoints, treat the upload `Location` returned by `Create`, `CreateWithUpload`, or `Upload` as the authoritative address for follow-up `HEAD`, `PATCH`, `Resume`, and `Terminate` calls. SeaweedFS stores resumable upload sessions on the filer that created them, so the SDK preserves that returned URL instead of rewriting it to another filer endpoint.

```go
ctx := context.Background()
data := "large upload payload"

options, err := client.TUS().Options(ctx, tus.OptionsOptions{})
if err != nil {
    return err
}
fmt.Println(options.ExtensionList, options.SupportsCreationWithUpload, options.MaxSize)

upload, err := client.TUS().Upload(ctx, "/sdk/video.mp4", strings.NewReader(data), tus.UploadOptions{
    Size: int64(len(data)),
    Metadata: map[string]string{
        "filename": "video.mp4",
    },
})
if err != nil {
    return err
}
fmt.Println(upload.Location, upload.Offset)

chunked, err := client.TUS().Upload(ctx, "/sdk/chunked.bin", strings.NewReader(data), tus.UploadOptions{
    Size:      int64(len(data)),
    ChunkSize: 8 << 20,
})
if err != nil {
    return err
}
fmt.Println(chunked.Location)
```

Resume an existing upload with an `io.ReadSeeker`:

```go
file, err := os.Open("video.mp4")
if err != nil {
    return err
}
defer file.Close()

status, err := client.TUS().Resume(ctx, upload.Location, file, tus.ResumeOptions{
    ChunkSize: 8 << 20,
})
if err != nil {
    return err
}
fmt.Println(status.Offset)
```

For secured filer deployments, pass the same per-request `Authorization` value through the TUS option structs:

```go
tusToken := "Bearer <jwt>"

upload, err := client.TUS().Upload(ctx, "/secure/video.mp4", strings.NewReader(data), tus.UploadOptions{
    Size:          int64(len(data)),
    Authorization: tusToken,
})
if err != nil {
    return err
}

status, err := client.TUS().Head(ctx, upload.Location, tus.HeadOptions{
    Authorization: tusToken,
})
if err != nil {
    return err
}
fmt.Println(status.Offset)
```

### S3 Compatibility

The SDK returns AWS SDK v2 clients configured for SeaweedFS path-style S3.

```go
s3Client, err := client.S3(ctx)
if err != nil {
    return err
}

_, _ = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
    Bucket: aws.String("sdk-example"),
})

_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
    Bucket:      aws.String("sdk-example"),
    Key:         aws.String("hello.txt"),
    Body:        strings.NewReader("hello s3"),
    ContentType: aws.String("text/plain"),
})
if err != nil {
    return err
}
```

### Direct Package Clients

Use direct clients when you want to wire only one SeaweedFS API surface.

```go
httpClient := seaweed.NewHTTPClient(seaweed.DefaultHTTPClientConfig())

masterClient, err := master.New(master.Config{
    BaseURLs:   []string{"http://127.0.0.1:9333"},
    HTTPClient: httpClient,
})
if err != nil {
    return err
}

assigned, err := masterClient.Assign(ctx, master.AssignOptions{
    Collection: "sdk",
})
if err != nil {
    return err
}
fmt.Println(assigned.FID)

submitted, err := masterClient.Submit(ctx, "hello.txt", strings.NewReader("hello master"), master.SubmitOptions{
    FileContentType: "text/plain",
})
if err != nil {
    return err
}
fmt.Println(submitted.FID, submitted.Size)

dirStatus, err := masterClient.DirStatus(ctx)
if err != nil {
    return err
}
fmt.Println(dirStatus.Topology.Free, dirStatus.Topology.Max)

volumeStatus, err := masterClient.VolumeStatus(ctx)
if err != nil {
    return err
}
fmt.Println(volumeStatus.Volumes.Free, volumeStatus.Volumes.Max)

volumeURL := assigned.URL
if !strings.Contains(volumeURL, "://") {
    volumeURL = "http://" + volumeURL
}
volumeClient, err := volume.New(volume.Config{
    BaseURLs:   []string{volumeURL},
    HTTPClient: httpClient,
})
if err != nil {
    return err
}

volumeData := "hello volume"
put, err := volumeClient.Put(ctx, assigned.FID, strings.NewReader(volumeData), volume.PutOptions{
    ContentType:   "text/plain",
    ContentLength: int64(len(volumeData)),
    Fsync:         true,
    SeaweedHeaders: map[string]string{
        "Owner": "sdk",
    },
})
if err != nil {
    return err
}
fmt.Println(put.Name, put.Size)

serverStatus, err := volumeClient.Status(ctx)
if err != nil {
    return err
}
fmt.Println(serverStatus.Version, len(serverStatus.Volumes))

partial, err := volumeClient.Get(ctx, assigned.FID, volume.GetOptions{
    Range: "bytes=0-4",
})
if err != nil {
    return err
}
defer partial.Body.Close()
```

### Error Handling

Use `errors.As` to distinguish HTTP status failures from SeaweedFS JSON API errors.

```go
_, err = client.Filer().Put(ctx, "/sdk/hello.txt", strings.NewReader(data), filer.WriteOptions{
    ContentLength: int64(len(data)),
})
if err != nil {
    var apiErr *seaweed.APIError
    if errors.As(err, &apiErr) {
        fmt.Println(apiErr.Message)
        return err
    }

    var httpErr *seaweed.Error
    if errors.As(err, &httpErr) {
        fmt.Println(httpErr.StatusCode, httpErr.Body)
        return err
    }

    return err
}
```

## Validation

The full local gate used before every commit is:

```bash
make lint
make ci
make test
make test-race
make vet
WEED_BINARY=./weed make integration
WEED_BINARY=./weed make coverage
WEED_BINARY=./weed make release-check
```

Integration tests require a real SeaweedFS `weed` binary. The test harness resolves `WEED_BINARY` first and then project-root `./weed`.
`make ci` runs `golangci-lint`, unit tests, race tests, and `go vet`.
Coverage gates require at least `94.1%` combined production coverage from unit and integration profiles, excluding `examples/*` and `internal/testweed`.
See `CHANGELOG.md` and `RELEASE.md` before cutting a release.

## Notes

- JSON responses with an `error` field are returned as `*seaweed.APIError`, including status-only operations such as delete, mkdir, and TUS termination.
- Master, volume, and filer clients track the documented native HTTP API surface; debug-only pretty output is intentionally not modeled.
- Volume health checks use `/healthz`; use `Volume().Status(ctx)` for full disk and volume metadata.
- S3/IAM uses AWS SDK for Go v2 and path-style S3 addressing.
- IAM defaults to `S3URLs` because SeaweedFS embeds IAM in the S3 server by default.
- TUS implements SeaweedFS-supported TUS 1.0.0 operations: creation, creation-with-upload, offset upload, resume, and termination.
