package filer

import (
	"net/url"
	"testing"
)

func BenchmarkEscapePath(b *testing.B) {
	paths := []string{
		"/docs/report.txt",
		"/docs/folder with spaces/report 2026.txt",
		"/中文/目录/文件.txt",
		"/deep/a/b/c/d/e/f/g/file.txt",
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, path := range paths {
			if _, err := escapePath(path); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkPutQuery(b *testing.B) {
	offset := int64(1024)
	opts := PutOptions{
		DataCenter:         "dc1",
		Rack:               "rack1",
		DataNode:           "node1",
		Collection:         "photos",
		Replication:        "001",
		TTL:                "3d",
		MaxMB:              32,
		Mode:               "0755",
		Offset:             &offset,
		Fsync:              true,
		SaveInside:         true,
		SkipCheckParentDir: true,
	}

	b.ReportAllocs()
	for b.Loop() {
		query := putQuery(opts)
		if query.Get("collection") == "" {
			b.Fatal("missing collection")
		}
	}
}

func BenchmarkDeleteQuery(b *testing.B) {
	opts := DeleteOptions{
		Recursive:            true,
		IgnoreRecursiveError: true,
		SkipChunkDeletion:    true,
	}

	b.ReportAllocs()
	for b.Loop() {
		query := url.Values{}
		addBool(query, "recursive", opts.Recursive)
		addBool(query, "ignoreRecursiveError", opts.IgnoreRecursiveError)
		addBool(query, "skipChunkDeletion", opts.SkipChunkDeletion)
		if query.Get("recursive") == "" {
			b.Fatal("missing recursive")
		}
	}
}
