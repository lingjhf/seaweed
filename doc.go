// Package seaweed provides Go clients for SeaweedFS native HTTP APIs and the
// SeaweedFS S3/IAM compatibility endpoints.
//
// The root Client composes service clients for master, volume, blob, filer, TUS,
// S3, and IAM. Use the root client when one process talks to several SeaweedFS
// services with shared HTTP, retry, authentication, and endpoint policy
// settings. Direct subpackage clients are available when only one SeaweedFS
// service is needed.
package seaweed
