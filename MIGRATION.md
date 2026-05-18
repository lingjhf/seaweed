# Migration Guide

This guide covers breaking changes in the current `0.x` development line.

## Endpoint Configuration

Clients now use endpoint lists instead of single endpoint fields.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://127.0.0.1:9333"},
    FilerURLs:  []string{"http://127.0.0.1:8888"},
})
```

Use `EndpointPolicy` to configure endpoint selection and health behavior.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://master-1:9333", "http://master-2:9333"},
    EndpointPolicy: seaweed.EndpointPolicy{
        Mode: seaweed.EndpointPolicyRoundRobin,
    },
})
```

S3 and IAM endpoints also moved to lists. Replace `S3URL` with `S3URLs` and `IAMURL` with `IAMURLs`. When `IAMURLs` is empty, `IAM()` uses `S3URLs`.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://127.0.0.1:9333"},
    S3URLs:     []string{"http://127.0.0.1:8333"},
})
```

S3 and IAM now use endpoint policy through AWS SDK v2 endpoint resolution. Configure `S3EndpointPolicy` or `IAMEndpointPolicy` when those clients need their own round-robin, health check, or circuit breaker behavior.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://127.0.0.1:9333"},
    S3URLs:     []string{"http://s3-1:8333", "http://s3-2:8333"},
    S3EndpointPolicy: seaweed.EndpointPolicy{
        Mode: seaweed.EndpointPolicyRoundRobin,
    },
})
```

Blob reads use master lookup results as a per-volume location cache. Configure `BlobEndpointPolicy` when the Blob read path needs its own selection behavior, and `BlobLocationCacheTTL` when lookup locations should be refreshed periodically.

```go
client, err := seaweed.New(seaweed.Config{
    MasterURLs: []string{"http://master-1:9333", "http://master-2:9333"},
    BlobEndpointPolicy: seaweed.EndpointPolicy{
        Mode: seaweed.EndpointPolicyRoundRobin,
    },
    BlobLocationCacheTTL: 30 * time.Second,
})
```

## Filer API

The Filer client now uses a resource-operation API with explicit result and page types.

### Writes

```go
_, err := client.Filer().Put(ctx, "/docs/report.txt", body, filer.WriteOptions{
    ContentType:   "text/plain",
    ContentLength: size,
})
```

`Append` no longer accepts write offsets. Use `AppendOptions`.

```go
_, err := client.Filer().Append(ctx, "/docs/report.txt", body, filer.AppendOptions{
    ContentType:   "text/plain",
    ContentLength: size,
})
```

### Head And Tags

`Head` now returns `*filer.HeadResult`, preserving raw headers while exposing parsed SeaweedFS tags.

```go
head, err := client.Filer().Head(ctx, "/docs/report.txt")
if err != nil {
    return err
}
fmt.Println(head.Header.Get("Seaweed-Owner"), head.Tags["Owner"])
```

Use `Tags` when only SeaweedFS tags are needed.

```go
tags, err := client.Filer().Tags(ctx, "/docs/report.txt")
```

### Listing

`List` was renamed to `ListPage` to make single-page behavior explicit.

```go
page, err := client.Filer().ListPage(ctx, "/docs", filer.ListOptions{
    Limit: 100,
})
```

Use `Walk` when the SDK should follow `lastFileName` pagination.

```go
err := client.Filer().Walk(ctx, "/docs", filer.WalkOptions{Limit: 100}, func(entry filer.Entry) error {
    fmt.Println(entry.FullPath)
    return nil
})
```
