# Changelog

This project is in the `0.x` development line. Public APIs can change while the SDK is being shaped against SeaweedFS behavior.

## Unreleased

### Development API Changes

- JSON responses with an `error` field now return `*seaweed.APIError` instead of exposing response `Error` fields.
- Filer write results no longer expose `Error`; write API failures are returned as errors.
- Endpoint configuration uses endpoint lists for master, volume, filer, TUS, S3, and IAM clients.
- Filer uses a resource-operation API with explicit write results, append options, head results, and page-based listing.

### Added

- Volume server status now returns typed disk and volume metadata.
- Master directory status now returns typed topology metadata.
- Master volume status now returns typed volume placement metadata.
- Endpoint policy support for failover, round-robin, health checks, and circuit breakers.
- Native endpoint failover for retryable requests.
- S3 and IAM endpoint lists now support round-robin, health checks, and circuit breakers through AWS SDK endpoint resolution.
- Blob volume lookup cache now stores all master locations, supports read failover, `BlobEndpointPolicy`, and `BlobLocationCacheTTL`.
- Concurrent Blob cache misses for the same volume are coalesced into one master lookup.
- Circuit breaker half-open request limiting.
- Tuned default HTTP transport with larger idle connection pools.
- Filer typed metadata fields, `Tags`, and paginated `Walk`.
- Filer entries now expose `Uid` and `Gid` metadata as `UID` and `GID`.
- Filer now supports streaming multipart uploads with `UploadMultipart`.
- TUS creation-with-upload by default, with chunked upload still available through `ChunkSize`.
- Benchmarks for HTTP, Blob cached lookup, path escaping, and TUS upload paths.
- CI target aligned with unit, race, and vet checks.

### Changed

- Coverage gate now requires at least `92.0%` combined production coverage from unit and integration profiles.
- README examples now cover Blob, Filer, TUS, S3/IAM, endpoint policy, and validation flows.

### Fixed

- Endpoint circuit breaker now enforces configured half-open request limits.
