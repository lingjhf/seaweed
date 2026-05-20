package seaweed_test

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/lingjhf/seaweed/blob"
	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/tus"
	"github.com/lingjhf/seaweed/volume"
)

func TestNativeAPISurfaceAlignment(t *testing.T) {
	ctxType := reflect.TypeFor[context.Context]()
	readerType := reflect.TypeFor[io.Reader]()
	readSeekerType := reflect.TypeFor[io.ReadSeeker]()
	errorType := reflect.TypeFor[error]()

	requireMethod(t, reflect.TypeFor[*master.Client](), "Submit",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			readerType,
			reflect.TypeFor[master.SubmitOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*master.SubmitResponse](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*master.Client](), "Vacuum",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[master.VacuumOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*master.Client](), "DeleteCollection",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[master.DeleteCollectionOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)

	requireMethod(t, reflect.TypeFor[*volume.Client](), "Head",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[volume.HeadOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[http.Header](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*volume.Client](), "Delete",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[volume.DeleteOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)

	requireMethod(t, reflect.TypeFor[*filer.Client](), "UploadMultipart",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			readerType,
			reflect.TypeFor[filer.MultipartUploadOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*filer.WriteResult](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "Head",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[filer.HeadOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*filer.HeadResult](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "Tags",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[filer.HeadOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[map[string]string](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "Copy",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[string](),
			reflect.TypeFor[filer.CopyOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "Move",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[string](),
			reflect.TypeFor[filer.MoveOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "SetTags",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[map[string]string](),
			reflect.TypeFor[filer.TagOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "DeleteTags",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[filer.TagOptions](),
			reflect.TypeFor[[]string](),
		},
		[]reflect.Type{
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*filer.Client](), "Mkdir",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[filer.MkdirOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)

	requireMethod(t, reflect.TypeFor[*tus.Client](), "Options",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[tus.OptionsOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Options](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "Create",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[tus.CreateOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Upload](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "CreateWithUpload",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			readerType,
			reflect.TypeFor[tus.CreateOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Upload](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "Head",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[tus.HeadOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Status](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "Patch",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[int64](),
			readerType,
			reflect.TypeFor[int64](),
			reflect.TypeFor[tus.PatchOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Status](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "Terminate",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			reflect.TypeFor[tus.TerminateOptions](),
		},
		[]reflect.Type{
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "Upload",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			readerType,
			reflect.TypeFor[tus.UploadOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Upload](),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeFor[*tus.Client](), "Resume",
		[]reflect.Type{
			ctxType,
			reflect.TypeFor[string](),
			readSeekerType,
			reflect.TypeFor[tus.ResumeOptions](),
		},
		[]reflect.Type{
			reflect.TypeFor[*tus.Status](),
			errorType,
		},
	)

	requireFields(t, reflect.TypeFor[volume.PutOptions](),
		"Fsync",
		"Replicate",
		"ModifiedAtSecond",
		"ChunkManifest",
		"SeaweedHeaders",
		"Authorization",
	)
	requireFields(t, reflect.TypeFor[volume.GetOptions](),
		"ReadDeleted",
		"Width",
		"Height",
		"Mode",
		"CropX1",
		"CropY1",
		"CropX2",
		"CropY2",
		"ChunkManifest",
		"IfModifiedSince",
		"IfNoneMatch",
		"AcceptEncoding",
		"Authorization",
	)
	requireFields(t, reflect.TypeFor[volume.DeleteOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[blob.Config](), "EnableVolumeAuthorization")
	requireFields(t, reflect.TypeFor[master.AssignOptions](),
		"Count",
		"Collection",
		"DataCenter",
		"Rack",
		"DataNode",
		"Replication",
		"TTL",
		"Preallocate",
		"MemoryMapMaxSizeMB",
		"WritableVolumeCount",
		"Disk",
	)
	requireFields(t, reflect.TypeFor[master.LookupOptions](), "Collection", "FileID", "Read")
	requireFields(t, reflect.TypeFor[master.GrowOptions](),
		"Count",
		"Collection",
		"DataCenter",
		"Rack",
		"DataNode",
		"Replication",
		"TTL",
		"Preallocate",
		"MemoryMapMaxSizeMB",
		"Disk",
	)
	requireFields(t, reflect.TypeFor[master.SubmitOptions](), "FieldName", "FileContentType")
	requireFields(t, reflect.TypeFor[master.VacuumOptions](), "GarbageThreshold")
	requireFields(t, reflect.TypeFor[master.DeleteCollectionOptions](), "Collection")
	requireFields(t, reflect.TypeFor[filer.WriteOptions](),
		"DataCenter",
		"Rack",
		"DataNode",
		"Collection",
		"Replication",
		"TTL",
		"MaxMB",
		"Mode",
		"Offset",
		"Fsync",
		"SaveInside",
		"SkipCheckParentDir",
		"ContentType",
		"ContentDisposition",
		"SeaweedHeaders",
		"ContentLength",
		"Authorization",
	)
	requireFields(t, reflect.TypeFor[filer.AppendOptions](),
		"DataCenter",
		"Rack",
		"DataNode",
		"Collection",
		"Replication",
		"TTL",
		"MaxMB",
		"Mode",
		"Fsync",
		"SaveInside",
		"SkipCheckParentDir",
		"ContentType",
		"ContentDisposition",
		"SeaweedHeaders",
		"ContentLength",
		"Authorization",
	)
	requireFields(t, reflect.TypeFor[filer.MultipartUploadOptions](),
		"DataCenter",
		"Rack",
		"DataNode",
		"Collection",
		"Replication",
		"TTL",
		"MaxMB",
		"Mode",
		"Fsync",
		"SaveInside",
		"SkipCheckParentDir",
		"Filename",
		"FileContentType",
		"FieldName",
		"SeaweedHeaders",
		"Authorization",
	)
	requireFields(t, reflect.TypeFor[filer.GetOptions](), "ResponseContentDisposition", "ResolveManifest", "Authorization")
	requireFields(t, reflect.TypeFor[filer.HeadOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[filer.StatOptions](), "ResolveManifest", "Authorization")
	requireFields(t, reflect.TypeFor[filer.ListOptions](), "Limit", "LastFileName", "NamePattern", "NamePatternExclude", "Authorization")
	requireFields(t, reflect.TypeFor[filer.WalkOptions](), "Limit", "LastFileName", "NamePattern", "NamePatternExclude", "Authorization")
	requireFields(t, reflect.TypeFor[filer.DeleteOptions](), "Recursive", "IgnoreRecursiveError", "SkipChunkDeletion", "Authorization")
	requireFields(t, reflect.TypeFor[filer.CopyOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[filer.MoveOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[filer.TagOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[filer.MkdirOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[tus.OptionsOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[tus.CreateOptions](), "Size", "Metadata", "Authorization")
	requireFields(t, reflect.TypeFor[tus.HeadOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[tus.PatchOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[tus.TerminateOptions](), "Authorization")
	requireFields(t, reflect.TypeFor[tus.UploadOptions](), "Size", "ChunkSize", "Metadata", "Authorization")
	requireFields(t, reflect.TypeFor[tus.ResumeOptions](), "ChunkSize", "Authorization")

	requireJSONTags(t, reflect.TypeFor[master.AssignResponse](), map[string]string{
		"Count":         "count",
		"FID":           "fid",
		"URL":           "url",
		"PublicURL":     "publicUrl",
		"Authorization": "-",
	})
	requireJSONTags(t, reflect.TypeFor[master.LookupResponse](), map[string]string{
		"Locations":     "locations",
		"Authorization": "-",
	})
	requireJSONTags(t, reflect.TypeFor[master.Location](), map[string]string{
		"URL":        "url",
		"PublicURL":  "publicUrl",
		"DataCenter": "dataCenter,omitempty",
		"Rack":       "rack,omitempty",
	})
	requireJSONTags(t, reflect.TypeFor[master.ClusterStatus](), map[string]string{
		"IsLeader": "IsLeader",
		"Leader":   "Leader",
		"Peers":    "Peers",
	})
	requireJSONTags(t, reflect.TypeFor[master.DirStatusResponse](), map[string]string{
		"Topology": "Topology",
		"Version":  "Version",
	})
	requireJSONTags(t, reflect.TypeFor[master.VolumeInfo](), map[string]string{
		"ID":                "Id",
		"Size":              "Size",
		"ReplicaPlacement":  "ReplicaPlacement",
		"RepType":           "RepType",
		"TTL":               "Ttl",
		"DiskType":          "DiskType",
		"Collection":        "Collection",
		"Version":           "Version",
		"FileCount":         "FileCount",
		"DeleteCount":       "DeleteCount",
		"DeletedByteCount":  "DeletedByteCount",
		"ReadOnly":          "ReadOnly",
		"CompactRevision":   "CompactRevision",
		"ModifiedAtSecond":  "ModifiedAtSecond",
		"RemoteStorageName": "RemoteStorageName",
		"RemoteStorageKey":  "RemoteStorageKey",
	})
	requireJSONTags(t, reflect.TypeFor[volume.StatusResponse](), map[string]string{
		"DiskStatuses": "DiskStatuses",
		"Version":      "Version",
		"Volumes":      "Volumes",
	})
	requireJSONTags(t, reflect.TypeFor[volume.DiskStatus](), map[string]string{
		"Dir":         "dir",
		"All":         "all",
		"Used":        "used",
		"Free":        "free",
		"PercentFree": "percent_free",
		"PercentUsed": "percent_used",
		"DiskType":    "disk_type",
	})
	requireJSONTags(t, reflect.TypeFor[volume.VolumeInfo](), map[string]string{
		"ID":                "Id",
		"Size":              "Size",
		"ReplicaPlacement":  "ReplicaPlacement",
		"RepType":           "RepType",
		"TTL":               "Ttl",
		"DiskType":          "DiskType",
		"DiskID":            "DiskId",
		"Collection":        "Collection",
		"Version":           "Version",
		"FileCount":         "FileCount",
		"DeleteCount":       "DeleteCount",
		"DeletedByteCount":  "DeletedByteCount",
		"ReadOnly":          "ReadOnly",
		"CompactRevision":   "CompactRevision",
		"ModifiedAtSecond":  "ModifiedAtSecond",
		"RemoteStorageName": "RemoteStorageName",
		"RemoteStorageKey":  "RemoteStorageKey",
	})
	requireJSONTags(t, reflect.TypeFor[filer.Entry](), map[string]string{
		"FullPath":        "FullPath",
		"Mtime":           "Mtime",
		"Crtime":          "Crtime",
		"Mode":            "Mode",
		"Mime":            "Mime",
		"Replication":     "Replication",
		"Collection":      "Collection",
		"TtlSec":          "TtlSec",
		"DiskType":        "DiskType",
		"UserName":        "UserName",
		"GroupNames":      "GroupNames",
		"UID":             "Uid",
		"GID":             "Gid",
		"SymlinkTarget":   "SymlinkTarget",
		"MD5":             "Md5",
		"FileSize":        "FileSize",
		"Rdev":            "Rdev",
		"Inode":           "Inode",
		"Extended":        "Extended",
		"Content":         "Content",
		"Chunks":          "chunks",
		"HardLinkID":      "HardLinkId",
		"HardLinkCounter": "HardLinkCounter",
		"Remote":          "Remote",
		"Quota":           "Quota",
	})
}

func requireMethod(t *testing.T, receiver reflect.Type, name string, wantIn []reflect.Type, wantOut []reflect.Type) {
	t.Helper()

	method, ok := receiver.MethodByName(name)
	if !ok {
		t.Fatalf("%s.%s is missing", receiver, name)
	}

	gotIn := make([]reflect.Type, 0, method.Type.NumIn()-1)
	for i := 1; i < method.Type.NumIn(); i++ {
		gotIn = append(gotIn, method.Type.In(i))
	}
	if !reflect.DeepEqual(gotIn, wantIn) {
		t.Fatalf("%s.%s inputs = %v, want %v", receiver, name, gotIn, wantIn)
	}

	gotOut := make([]reflect.Type, 0, method.Type.NumOut())
	for out := range method.Type.Outs() {
		gotOut = append(gotOut, out)
	}
	if !reflect.DeepEqual(gotOut, wantOut) {
		t.Fatalf("%s.%s outputs = %v, want %v", receiver, name, gotOut, wantOut)
	}
}

func requireFields(t *testing.T, typ reflect.Type, names ...string) {
	t.Helper()

	for _, name := range names {
		if _, ok := typ.FieldByName(name); !ok {
			t.Fatalf("%s.%s is missing", typ, name)
		}
	}
}

func requireJSONTags(t *testing.T, typ reflect.Type, tags map[string]string) {
	t.Helper()

	for name, want := range tags {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("%s.%s is missing", typ, name)
		}
		if got := field.Tag.Get("json"); got != want {
			t.Fatalf("%s.%s json tag = %q, want %q", typ, name, got, want)
		}
	}
}
