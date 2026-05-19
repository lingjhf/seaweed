package seaweed_test

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/lingjhf/seaweed/filer"
	"github.com/lingjhf/seaweed/master"
	"github.com/lingjhf/seaweed/volume"
)

func TestNativeAPISurfaceAlignment(t *testing.T) {
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	readerType := reflect.TypeOf((*io.Reader)(nil)).Elem()
	errorType := reflect.TypeOf((*error)(nil)).Elem()

	requireMethod(t, reflect.TypeOf((*master.Client)(nil)), "Submit",
		[]reflect.Type{
			ctxType,
			reflect.TypeOf(""),
			readerType,
			reflect.TypeOf(master.SubmitOptions{}),
		},
		[]reflect.Type{
			reflect.TypeOf((*master.SubmitResponse)(nil)),
			errorType,
		},
	)

	requireMethod(t, reflect.TypeOf((*volume.Client)(nil)), "Head",
		[]reflect.Type{
			ctxType,
			reflect.TypeOf(""),
			reflect.TypeOf(volume.HeadOptions{}),
		},
		[]reflect.Type{
			reflect.TypeOf(http.Header{}),
			errorType,
		},
	)
	requireMethod(t, reflect.TypeOf((*volume.Client)(nil)), "Delete",
		[]reflect.Type{
			ctxType,
			reflect.TypeOf(""),
			reflect.TypeOf(volume.DeleteOptions{}),
		},
		[]reflect.Type{
			errorType,
		},
	)

	requireMethod(t, reflect.TypeOf((*filer.Client)(nil)), "UploadMultipart",
		[]reflect.Type{
			ctxType,
			reflect.TypeOf(""),
			readerType,
			reflect.TypeOf(filer.MultipartUploadOptions{}),
		},
		[]reflect.Type{
			reflect.TypeOf((*filer.WriteResult)(nil)),
			errorType,
		},
	)

	requireFields(t, reflect.TypeOf(volume.PutOptions{}),
		"Fsync",
		"Replicate",
		"ModifiedAtSecond",
		"ChunkManifest",
		"SeaweedHeaders",
		"Authorization",
	)
	requireFields(t, reflect.TypeOf(volume.GetOptions{}),
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
	requireFields(t, reflect.TypeOf(volume.DeleteOptions{}), "Authorization")
	requireFields(t, reflect.TypeOf(filer.MultipartUploadOptions{}), "Filename")
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
	for i := 0; i < method.Type.NumOut(); i++ {
		gotOut = append(gotOut, method.Type.Out(i))
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
