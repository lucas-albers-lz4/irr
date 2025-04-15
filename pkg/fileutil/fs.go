package fileutil

import (
	"io"
	"os"

	"github.com/spf13/afero"
)

// File represents a file in the filesystem.
type File interface {
	io.Closer
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Writer
	Name() string
	ReadAt(b []byte, off int64) (n int, err error)
	Readdirnames(n int) ([]string, error)
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(size int64) error
	WriteAt(b []byte, off int64) (n int, err error)
	WriteString(s string) (ret int, err error)
}

// FS defines the filesystem operations needed by IRR
type FS interface {
	Create(name string) (File, error)
	Mkdir(name string, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Open(name string) (File, error)
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	Remove(name string) error
	RemoveAll(path string) error
	Rename(oldname, newname string) error
	Stat(name string) (os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

// AferoFile adapts an afero.File to our File interface
type AferoFile struct {
	afero.File
}

// AferoFS adapts an afero.Fs to our FS interface
type AferoFS struct {
	fs afero.Fs
}

// NewAferoFS creates a new AferoFS instance wrapping the provided afero.Fs
func NewAferoFS(fs afero.Fs) *AferoFS {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	return &AferoFS{fs: fs}
}

// Create creates a file
func (a *AferoFS) Create(name string) (File, error) {
	file, err := a.fs.Create(name)
	if err != nil {
		return nil, err
	}
	return &AferoFile{File: file}, nil
}

// Mkdir creates a directory
func (a *AferoFS) Mkdir(name string, perm os.FileMode) error {
	return a.fs.Mkdir(name, perm)
}

// MkdirAll creates a directory with all parent directories
func (a *AferoFS) MkdirAll(path string, perm os.FileMode) error {
	return a.fs.MkdirAll(path, perm)
}

// Open opens a file
func (a *AferoFS) Open(name string) (File, error) {
	file, err := a.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &AferoFile{File: file}, nil
}

// OpenFile opens a file with specific flags and permissions
func (a *AferoFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	file, err := a.fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &AferoFile{File: file}, nil
}

// Remove removes a file
func (a *AferoFS) Remove(name string) error {
	return a.fs.Remove(name)
}

// RemoveAll removes a file or directory and all its contents
func (a *AferoFS) RemoveAll(path string) error {
	return a.fs.RemoveAll(path)
}

// Rename renames a file
func (a *AferoFS) Rename(oldname, newname string) error {
	return a.fs.Rename(oldname, newname)
}

// Stat returns file info
func (a *AferoFS) Stat(name string) (os.FileInfo, error) {
	return a.fs.Stat(name)
}

// ReadFile reads a file
func (a *AferoFS) ReadFile(filename string) ([]byte, error) {
	return afero.ReadFile(a.fs, filename)
}

// WriteFile writes a file
func (a *AferoFS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(a.fs, filename, data, perm)
}

// DefaultFS is the default filesystem implementation used throughout the codebase
var DefaultFS FS = NewAferoFS(afero.NewOsFs())

// SetFS replaces the default filesystem with the provided one and returns a cleanup function
func SetFS(fs FS) func() {
	oldFS := DefaultFS
	DefaultFS = fs
	return func() {
		DefaultFS = oldFS
	}
}
