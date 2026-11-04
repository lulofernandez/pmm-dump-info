// Package dump opens a pmm-dump file: gzip-compressed tar, optionally
// encrypted with openssl-compatible AES-256-CTR (pmm-dump --encryption),
// on its own or inside a .zip that customers use to upload several dumps.
package dump

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrEncrypted is returned when the file is encrypted and no password was given.
var ErrEncrypted = errors.New("the dump is encrypted: pass the password with -pass (pmm-dump prints it at export time)")

// Same parameters as pmm-dump pkg/encryption.
const (
	iterations = 10000
	saltSize   = 8
)

// File is an open dump.
type File struct {
	Tar       *tar.Reader
	Encrypted bool
	f         io.Closer
	gz        *gzip.Reader
}

// Open opens path and returns a tar reader over its content.
func Open(path, pass string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return open(f, path, pass)
}

// IsZip tells whether path is a zip archive (local file header magic).
func IsZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	_, err = io.ReadFull(f, head)
	return err == nil && bytes.Equal(head, []byte("PK\x03\x04"))
}

// ZipMembers opens a zip and returns its regular files, without directories
// and the macOS metadata a zip made in Finder carries (__MACOSX/, ._*).
func ZipMembers(path string) (*zip.ReadCloser, []*zip.File, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, nil, err
	}
	var out []*zip.File
	for _, m := range z.File {
		base := m.Name[strings.LastIndex(m.Name, "/")+1:]
		if m.FileInfo().IsDir() || strings.HasPrefix(m.Name, "__MACOSX/") || strings.HasPrefix(base, "._") {
			continue
		}
		out = append(out, m)
	}
	return z, out, nil
}

// OpenZipMember opens one dump stored in a zip; nothing is extracted to disk.
func OpenZipMember(m *zip.File, pass string) (*File, error) {
	rc, err := m.Open()
	if err != nil {
		return nil, err
	}
	return open(rc, m.Name, pass)
}

func open(f io.ReadCloser, path, pass string) (*File, error) {
	br := bufio.NewReaderSize(f, 1<<20)
	head, err := br.Peek(16)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: too short to be a pmm-dump file", path)
	}

	d := &File{f: f}
	var r io.Reader = br
	if bytes.HasPrefix(head, []byte("Salted__")) {
		d.Encrypted = true
		if pass == "" {
			f.Close()
			return nil, ErrEncrypted
		}
		salt := head[8 : 8+saltSize]
		br.Discard(8 + saltSize)
		k, err := pbkdf2.Key(sha256.New, pass, salt, iterations, 48)
		if err != nil {
			f.Close()
			return nil, err
		}
		block, err := aes.NewCipher(k[:32])
		if err != nil {
			f.Close()
			return nil, err
		}
		r = &cipher.StreamReader{S: cipher.NewCTR(block, k[32:]), R: br}
	} else if head[0] != 0x1f || head[1] != 0x8b {
		f.Close()
		return nil, fmt.Errorf("%s: not a gzip file nor an encrypted pmm-dump", path)
	}

	d.gz, err = gzip.NewReader(r)
	if err != nil {
		f.Close()
		if d.Encrypted {
			return nil, fmt.Errorf("cannot decrypt the dump, wrong password? (%v)", err)
		}
		return nil, err
	}
	d.Tar = tar.NewReader(d.gz)
	return d, nil
}

// Close closes the underlying file.
func (d *File) Close() error {
	d.gz.Close()
	return d.f.Close()
}
