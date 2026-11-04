package inspect

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tinyDump is a pmm-dump with only meta.json.
func tinyDump(t *testing.T) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	meta := []byte(`{"version":{"git-branch":"main","git-commit":"abc"},"pmm-server-version":"3.9.0","arguments":"export --start-ts=S --end-ts=E","vm-data-format":"json"}`)
	if err := tw.WriteHeader(&tar.Header{Name: "meta.json", Mode: 0o644, Size: int64(len(meta)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write(meta)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestScanZip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "dumps.tar.zip")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, data := range map[string][]byte{
		"a.tar.gz":            tinyDump(t),
		"notes.txt":           []byte("not a dump at all"),
		"__MACOSX/._a.tar.gz": []byte("x"),
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	zw.Close()
	f.Close()

	entries, err := ScanZip(file, "", true, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (__MACOSX skipped)", len(entries))
	}
	for _, e := range entries {
		switch e.Name {
		case "a.tar.gz":
			if e.Err != nil || e.Result.Name != "dumps.tar.zip > a.tar.gz" || e.Result.PMMDump != "main@abc" || e.Result.Args.Range != "S -> E" {
				t.Errorf("a.tar.gz: err %v, result %+v", e.Err, e.Result)
			}
		case "notes.txt":
			if e.Err == nil || !strings.Contains(e.Err.Error(), "not a gzip file") {
				t.Errorf("notes.txt: err = %v", e.Err)
			}
		default:
			t.Errorf("unexpected entry %s", e.Name)
		}
	}
}
