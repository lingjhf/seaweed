// Package tus provides a client for SeaweedFS TUS resumable upload support.
//
// It implements the SeaweedFS filer TUS endpoints for options discovery, upload
// creation, chunk patching, status checks, resume, and termination. Each request
// option type can carry a per-request Authorization header.
package tus
