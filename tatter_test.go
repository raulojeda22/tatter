package tatter

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/iotest"
)

type TestShredTable struct {
	file, pattern string
	want          error
}

type TestBuffTable struct {
	name       string
	size, want int64
}

func copyFile(t *testing.T, from, to string) (*os.File, error) {
	t.Helper()
	src, err := os.Open(from)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(dst, src); err != nil {
		return nil, err
	}
	return dst, nil
}

func patternIn(t *testing.T, pattern string, f *os.File) bool {
	t.Helper()
	b := make([]byte, len(pattern))
	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("err: %v\n", err)
	}
	var i int64
	for i = 0; i < stat.Size(); i += int64(len(pattern)) {
		_, err = f.ReadAt(b, i)
		if err != nil {
			t.Fatalf("err: %v\n", err)
		}
		eq := true
		for j := 0; j < len(b); j++ {
			if b[j] != pattern[j] {
				eq = false
				break
			}
		}
		if eq {
			return true
		}
	}
	return false
}

func createNonWritable(t *testing.T, path string) (*os.File, error) {
	t.Helper()
	f, err := copyFile(t, "testdata/small.bin", path)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err := os.Chmod(path, 0400); err != nil {
		return nil, err
	}
	fro, err := os.OpenFile(path, os.O_RDONLY, 0400)
	if err != nil {
		return nil, err
	}
	if err = os.Remove(path); err != nil {
		return nil, err
	}
	return fro, err
}

func TestShred(t *testing.T) {
	var tests = []TestShredTable{
		{"small.bin", "Small123", nil},
		{"large.bin", "Large123/", nil},
		{"extra.bin", "Extra123/", nil},
		{"empty.bin", "", nil},
		{"nonexistent", "", &fs.PathError{}},
		{"", "", &fs.PathError{}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			f, err := copyFile(t, "testdata/"+tt.file, "testdata/test/"+tt.file)
			if err != nil && tt.want == nil {
				t.Fatalf("unexpected error openning file %s: %v\n", tt.file, err)
			}
			defer f.Close()
			if err := Shred("testdata/test/" + tt.file); (tt.want == nil && err != nil) || (tt.want != nil && errors.Is(err, tt.want)) {
				t.Fatalf("got: %v, want %v\n", err, tt.want)
			}
			if tt.want == nil && patternIn(t, tt.pattern, f) {
				t.Fatalf("pattern %s found in %s\n", tt.pattern, tt.file)
			}
			f.Close()
			if fileinfo, _ := os.Stat("testdata/test/" + tt.file); tt.want == nil && fileinfo != nil {
				t.Fatalf("file: %v, has not been removed\n", tt.file)
			}
		})
	}
}

func TestShredFileWriteError(t *testing.T) {
	testFile := "testdata/test/smallwrite.bin"
	f, err := createNonWritable(t, testFile)
	defer f.Close()
	if err != nil {
		t.Fatalf("non writable file not created")
	}
	if err := shredFile(f); err == nil {
		t.Fatalf("expected write err, got nil\n")
	}
}

func TestShredFileNon(t *testing.T) {
	if err := shredFile(nil); err == nil {
		t.Fatalf("expected *PathError err, got nil\n")
	}
}

func TestCalcBuf(t *testing.T) {
	var tests = []TestBuffTable{
		{"5GB", 5 * 1024 * 1024 * 1024, 64 * 1024 * 1024},
		{"0B", 0, 4096},
		{"Negative", -12340, 4096},
		{"Average", 134217728, 134217728/64 + 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := calcBuf(tt.size)
			if buf != tt.want {
				t.Fatalf("expected %d err, got %d\n", tt.want, buf)
			}
		})
	}
}

func TestShredProcRandError(t *testing.T) {
	errs := make(chan error)
	testFile := "testdata/test/smallwrite.bin"
	f, err := createNonWritable(t, testFile)
	defer f.Close()
	if err != nil {
		t.Fatalf("non writable file not created")
	}
	go shredProc(f, 10, 10, iotest.ErrReader(errors.New("Rand err")), errs)
	if err := <-errs; err == nil {
		t.Fatalf("expected rand err, got nil\n")
	}
}

func TestShredProcBuffer(t *testing.T) {
	errs := make(chan error)
	testFile := "testdata/test/smallwrite.bin"
	f, err := createNonWritable(t, testFile)
	defer f.Close()
	if err != nil {
		t.Fatalf("Non writable file not created")
	}
	go shredProc(f, 100, -1000, rand.Reader, errs)
	if err := <-errs; err == nil {
		t.Fatalf("expected buff err, got nil\n")
	}
}

func TestShredProcNilError(t *testing.T) {
	errs := make(chan error)
	go shredProc(nil, 10, 10, iotest.ErrReader(errors.New("Rand err")), errs)
	if err := <-errs; err == nil {
		t.Fatalf("expected file err, got nil\n")
	}
}
