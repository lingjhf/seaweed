// Package volume provides a client for direct SeaweedFS volume server HTTP APIs.
//
// Use this package when a file ID and target volume endpoint are already known.
// The root blob client handles assignment and lookup when those steps should be
// managed automatically. Put, Get, Head, and Delete expose the documented
// volume query parameters and request headers, including per-request
// Authorization. Status returns typed local disk and volume metadata.
package volume
