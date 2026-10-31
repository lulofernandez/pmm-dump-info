// Package dump opens a pmm-dump file: gzip-compressed tar, optionally
// encrypted with openssl-compatible AES-256-CTR (pmm-dump --encryption).
package dump

import (
	"archive/tar"
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
	f         *os.File
	gz        *gzip.Reader
}

// Open opens path and returns a tar reader over its content.
func Open(path, pass string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
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
