package attachments

import (
	"encoding/base64"
	"github.com/eosaios/eos/pkg/coreapi"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreAndEncodeDocuments(t *testing.T) {
	for _, name := range []string{"需求 说明.md", "report.pdf", "source.zip", ".env", "LICENSE"} {
		t.Run(name, func(t *testing.T) {
			data := []byte("document content 中文")
			file, err := Store(t.TempDir(), "../"+name, data)
			if err != nil {
				t.Fatal(err)
			}
			if file.Name != name || filepath.Base(file.Path) != name {
				t.Fatalf("name changed: %+v", file)
			}
			got, err := Encode(file)
			if err != nil {
				t.Fatal(err)
			}
			want := coreapi.Attachment{Name: name, Path: base64.StdEncoding.EncodeToString(data), MIME: file.MIME, Kind: file.Kind}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("attachment = %+v, want %+v", got, want)
			}
			decoded, err := base64.StdEncoding.DecodeString(got.Path)
			if err != nil || string(decoded) != string(data) {
				t.Fatalf("content = %q, %v", decoded, err)
			}
		})
	}
}

func TestEncodeRejectsMissingDirectoryAndOversize(t *testing.T) {
	dir := t.TempDir()
	large := filepath.Join(dir, "large.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(MaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(dir, "missing"), dir, large} {
		if _, err := Encode(File{Path: path}); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}
