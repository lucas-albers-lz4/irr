package fileutil

import (
	"fmt"
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
		return nil, fmt.Errorf("failed to create file %s: %w", name, err)
	}
	return &AferoFile{File: file}, nil
}

// Mkdir creates a directory
func (a *AferoFS) Mkdir(name string, perm os.FileMode) error {
	err := a.fs.Mkdir(name, perm)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", name, err)
	}
	return nil
}

// MkdirAll creates a directory with all parent directories
func (a *AferoFS) MkdirAll(path string, perm os.FileMode) error {
	err := a.fs.MkdirAll(path, perm)
	if err != nil {
		return fmt.Errorf("failed to create directory path %s: %w", path, err)
	}
	return nil
}

// Open opens a file
func (a *AferoFS) Open(name string) (File, error) {
	file, err := a.fs.Open(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", name, err)
	}
	return &AferoFile{File: file}, nil
}

// OpenFile opens a file with specific flags and permissions
func (a *AferoFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	file, err := a.fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s with flags %d: %w", name, flag, err)
	}
	return &AferoFile{File: file}, nil
}

// Remove removes a file
func (a *AferoFS) Remove(name string) error {
	err := a.fs.Remove(name)
	if err != nil {
		return fmt.Errorf("failed to remove %s: %w", name, err)
	}
	return nil
}

// RemoveAll removes a file or directory and all its contents
func (a *AferoFS) RemoveAll(path string) error {
	err := a.fs.RemoveAll(path)
	if err != nil {
		return fmt.Errorf("failed to remove path %s: %w", path, err)
	}
	return nil
}

// Rename renames a file
func (a *AferoFS) Rename(oldname, newname string) error {
	err := a.fs.Rename(oldname, newname)
	if err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", oldname, newname, err)
	}
	return nil
}

// Stat returns file info
func (a *AferoFS) Stat(name string) (os.FileInfo, error) {
	info, err := a.fs.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", name, err)
	}
	return info, nil
}

// ReadFile reads a file
func (a *AferoFS) ReadFile(filename string) ([]byte, error) {
	data, err := afero.ReadFile(a.fs, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}
	return data, nil
}

// WriteFile writes a file
func (a *AferoFS) WriteFile(filename string, data []byte, perm os.FileMode) error {
	err := afero.WriteFile(a.fs, filename, data, perm)
	if err != nil {
		return fmt.Errorf("failed to write file %s: %w", filename, err)
	}
	return nil
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
