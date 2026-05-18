# Changelog

This project is in the `0.x` development line. Minor releases can include breaking API changes while the SDK is being shaped against SeaweedFS behavior.

## Unreleased

### Breaking Changes

- Replaced native single-endpoint fields with endpoint lists for master, volume, filer, and TUS clients.
- Redesigned the Filer resource API:
  - `filer.PutOptions` is now `filer.WriteOptions`.
  - `filer.WriteResponse` is now `filer.WriteResult`.
  - `Client.Append` now accepts `filer.AppendOptions`.
  - `Client.Head` now returns `*filer.HeadResult`.
  - `Client.List` is now `Client.ListPage`.

### Added

- Endpoint policy support for failover, round-robin, health checks, and circuit breakers.
- Native endpoint failover for retryable requests.
- Circuit breaker half-open request limiting.
- Tuned default HTTP transport with larger idle connection pools.
- Filer typed metadata fields, `Tags`, and paginated `Walk`.
- TUS creation-with-upload by default, with chunked upload still available through `ChunkSize`.
- Benchmarks for HTTP, path escaping, and TUS upload paths.
- CI target aligned with unit, race, and vet checks.

### Changed

- Coverage gate now requires at least `90.0%` combined production coverage from unit and integration profiles.
- README examples now cover Blob, Filer, TUS, S3/IAM, endpoint policy, and validation flows.

### Fixed

- Endpoint circuit breaker now enforces configured half-open request limits.

