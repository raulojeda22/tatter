package tatter

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
)

const bufDef int64 = 4096
const maxBuf int64 = 64 * 1024 * 1024 // 64MiB
const diskWrites = 64
const threads = 3

// Calculates the buffer size that is going to be used to write to disk.
// The buffer size increases with file size to increase performance.
// With higher buffer, less write operations to disk. The const variable
// diskWrites stablishes how many write operations are expected for files
// not too small and not too large. Smaller files need less writes, since
// bufSize has a min value of bufDef, and large files (larger than
// diskWrites * maxBuf) will need more writes.
// Take into account that the total memory that the program will use will
// be more than bufSize * threads bytes.
func calcBuf(fileSize int64) int64 {
	bufSize := bufDef
	if fileSize > 0 {
		bufSize = bufDef + (fileSize / diskWrites)
	}
	if bufSize > maxBuf {
		bufSize = maxBuf
	}
	return bufSize
}

// Shreds file, overwriting its content using the given rand source,
// until a given size, writing in batches of the given buffer size.
// Errors are sent through a channel.
func shredProc(f *os.File, size int64, bufSize int64, randSrc interface{ io.Reader }, errs chan error) {
	if f == nil {
		errs <- errors.New("file is nil")
		return
	}
	if bufSize < 1 {
		errs <- errors.New("buffer must be greater than 0")
		return
	}
	rem := size % bufSize
	b := make([]byte, bufSize)
	sz := bufSize
	var j int64
	for j = 0; j < size; j += bufSize {
		if bufSize+j > size {
			sz = rem
		}
		if _, err := randSrc.Read(b[:sz]); err != nil { // b[:sz] when slicing from right O(1)
			errs <- err
			return
		}
		if _, err := f.WriteAt(b[:sz], j); err != nil {
			errs <- err
			return
		}
	}
	errs <- nil
}

// Shreds file, overwriting its content given const threads times
// with random data. Uses n threads, each one overwriting the file once.
func shredFile(f *os.File) error {
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	bufSize := calcBuf(stat.Size())
	errors := make(chan error)
	for i := 0; i < threads; i++ {
		go shredProc(f, stat.Size(), bufSize, rand.Reader, errors)
	}
	for i := 0; i < threads; i++ {
		if err = <-errors; err != nil {
			return err
		}
	}
	return nil
}

// Shreds file with given path string and removes it.
// If it fails at any step of the process, the file could have
// not been shreded correctly, it will not be removed.
func Shred(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 644)
	defer f.Close()
	if err == nil {
		err = shredFile(f)
		if err == nil {
			err = os.Remove(path)
		}
	}
	return err
}
