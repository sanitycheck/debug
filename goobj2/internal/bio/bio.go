// Code generated by golang.org/x/tools/cmd/bundle. DO NOT EDIT.
//go:generate bundle -o bio.go -prefix= cmd/internal/bio

// Package bio implements common I/O abstractions used within the Go toolchain.
//

package bio

import (
	"bufio"
	"io"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
)

// Reader implements a seekable buffered io.Reader.
type Reader struct {
	f *os.File
	*bufio.Reader
}

// Writer implements a seekable buffered io.Writer.
type Writer struct {
	f *os.File
	*bufio.Writer
}

// Create creates the file named name and returns a Writer
// for that file.
func Create(name string) (*Writer, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	return &Writer{f: f, Writer: bufio.NewWriter(f)}, nil
}

// Open returns a Reader for the file named name.
func Open(name string) (*Reader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return &Reader{f: f, Reader: bufio.NewReader(f)}, nil
}

func (r *Reader) MustSeek(offset int64, whence int) int64 {
	if whence == 1 {
		offset -= int64(r.Buffered())
	}
	off, err := r.f.Seek(offset, whence)
	if err != nil {
		log.Fatalf("seeking in output: %v", err)
	}
	r.Reset(r.f)
	return off
}

func (w *Writer) MustSeek(offset int64, whence int) int64 {
	if err := w.Flush(); err != nil {
		log.Fatalf("writing output: %v", err)
	}
	off, err := w.f.Seek(offset, whence)
	if err != nil {
		log.Fatalf("seeking in output: %v", err)
	}
	return off
}

func (r *Reader) Offset() int64 {
	off, err := r.f.Seek(0, 1)
	if err != nil {
		log.Fatalf("seeking in output [0, 1]: %v", err)
	}
	off -= int64(r.Buffered())
	return off
}

func (w *Writer) Offset() int64 {
	if err := w.Flush(); err != nil {
		log.Fatalf("writing output: %v", err)
	}
	off, err := w.f.Seek(0, 1)
	if err != nil {
		log.Fatalf("seeking in output [0, 1]: %v", err)
	}
	return off
}

func (r *Reader) Close() error {
	return r.f.Close()
}

func (w *Writer) Close() error {
	err := w.Flush()
	err1 := w.f.Close()
	if err == nil {
		err = err1
	}
	return err
}

func (r *Reader) File() *os.File {
	return r.f
}

func (w *Writer) File() *os.File {
	return w.f
}

// Slice reads the next length bytes of r into a slice.
//
// This slice may be backed by mmap'ed memory. Currently, this memory
// will never be unmapped. The second result reports whether the
// backing memory is read-only.
func (r *Reader) Slice(length uint64) ([]byte, bool, error) {
	if length == 0 {
		return []byte{}, false, nil
	}

	data, ok := r.sliceOS(length)
	if ok {
		return data, true, nil
	}

	data = make([]byte, length)
	_, err := io.ReadFull(r, data)
	if err != nil {
		return nil, false, err
	}
	return data, false, nil
}

// SliceRO returns a slice containing the next length bytes of r
// backed by a read-only mmap'd data. If the mmap cannot be
// established (limit exceeded, region too small, etc) a nil slice
// will be returned. If mmap succeeds, it will never be unmapped.
func (r *Reader) SliceRO(length uint64) []byte {
	data, ok := r.sliceOS(length)
	if ok {
		return data
	}
	return nil
}

// mmapLimit is the maximum number of mmaped regions to create before
// falling back to reading into a heap-allocated slice. This exists
// because some operating systems place a limit on the number of
// distinct mapped regions per process. As of this writing:
//
//  Darwin    unlimited
//  DragonFly   1000000 (vm.max_proc_mmap)
//  FreeBSD   unlimited
//  Linux         65530 (vm.max_map_count) // TODO: query /proc/sys/vm/max_map_count?
//  NetBSD    unlimited
//  OpenBSD   unlimited
var mmapLimit int32 = 1<<31 - 1

func init() {
	// Linux is the only practically concerning OS.
	if runtime.GOOS == "linux" {
		mmapLimit = 30000
	}
}

func (r *Reader) sliceOS(length uint64) ([]byte, bool) {
	// For small slices, don't bother with the overhead of a
	// mapping, especially since we have no way to unmap it.
	const threshold = 16 << 10
	if length < threshold {
		return nil, false
	}

	// Have we reached the mmap limit?
	if atomic.AddInt32(&mmapLimit, -1) < 0 {
		atomic.AddInt32(&mmapLimit, 1)
		return nil, false
	}

	// Page-align the offset.
	off := r.Offset()
	align := syscall.Getpagesize()
	aoff := off &^ int64(align-1)

	data, err := syscall.Mmap(int(r.f.Fd()), aoff, int(length+uint64(off-aoff)), syscall.PROT_READ, syscall.MAP_SHARED|syscall.MAP_FILE)
	if err != nil {
		return nil, false
	}

	data = data[off-aoff:]
	r.MustSeek(int64(length), 1)
	return data, true
}

// MustClose closes Closer c and calls log.Fatal if it returns a non-nil error.
func MustClose(c io.Closer) {
	if err := c.Close(); err != nil {
		log.Fatal(err)
	}
}

// MustWriter returns a Writer that wraps the provided Writer,
// except that it calls log.Fatal instead of returning a non-nil error.
func MustWriter(w io.Writer) io.Writer {
	return mustWriter{w}
}

type mustWriter struct {
	w io.Writer
}

func (w mustWriter) Write(b []byte) (int, error) {
	n, err := w.w.Write(b)
	if err != nil {
		log.Fatal(err)
	}
	return n, nil
}

func (w mustWriter) WriteString(s string) (int, error) {
	n, err := io.WriteString(w.w, s)
	if err != nil {
		log.Fatal(err)
	}
	return n, nil
}
