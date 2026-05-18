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

By default, `seaweed.New` uses an SDK HTTP client with a larger idle connection pool than Go's default transport. Use `seaweed.NewHTTPClient` when passing the same tuned client to direct package constructors.

Native SeaweedFS clients accept endpoint lists, for example `MasterURLs`, `VolumeURLs`, `FilerURLs`, and direct client `BaseURLs`. S3/IAM clients still use a single AWS SDK endpoint.

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

Health checks and circuit breakers are opt-in. Non-retryable write requests still use one selected endpoint and are not replayed across endpoints.

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

Use the filer client for path-based file operations, metadata, directories, and tags.

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

entry, err := client.Filer().Stat(ctx, "/sdk/hello.txt", filer.StatOptions{})
if err != nil {
    return err
}
fmt.Println(entry.FullPath, entry.FileSize)

list, err := client.Filer().ListPage(ctx, "/sdk", filer.ListOptions{Limit: 100})
if err != nil {
    return err
}
for _, entry := range list.Entries {
    fmt.Println(entry.FullPath)
}
```

### TUS Resumable Uploads

`Upload` uses SeaweedFS creation-with-upload by default. Set `ChunkSize` when you want explicit chunked PATCH uploads.

```go
ctx := context.Background()
data := "large upload payload"

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
```

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
Coverage gates require at least `90.0%` combined production coverage from unit and integration profiles, excluding `examples/*` and `internal/testweed`.

## Notes

- S3/IAM uses AWS SDK for Go v2 and path-style S3 addressing.
- IAM defaults to the S3 endpoint because SeaweedFS embeds IAM in the S3 server by default.
- TUS implements SeaweedFS-supported TUS 1.0.0 operations: creation, creation-with-upload, offset upload, resume, and termination.
