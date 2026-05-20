// Package tus provides a client for SeaweedFS TUS resumable upload support.
//
// It implements the SeaweedFS filer TUS subset for options discovery, upload
// creation, creation-with-upload, chunk patching, status checks, resume, and
// termination. SeaweedFS currently declares the creation,
// creation-with-upload, and termination extensions; this package does not send
// checksum, defer-length, expiration, or concatenation extension headers.
// Each request option type can carry a per-request Authorization header.
package tus
