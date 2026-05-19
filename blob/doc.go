// Package blob provides file ID based upload, read, head, and delete helpers.
//
// The blob client asks master for file assignment or volume lookup, then talks
// directly to volume servers. It is the convenient path for applications that do
// not want to manage volume locations themselves. EnableVolumeAuthorization
// makes the client use master-issued per-file Authorization headers for
// secured volume reads, writes, and deletes.
package blob
