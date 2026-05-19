// Package filer provides a path-based client for SeaweedFS filer HTTP APIs.
//
// The filer client supports file writes, appends, multipart uploads to
// directory or file paths, reads, metadata, directory listing, walking, copy,
// move, delete, mkdir, and SeaweedFS header tags. Each request option type can
// carry a per-request Authorization header for SeaweedFS deployments that
// enable JWT-secured filer access.
package filer
