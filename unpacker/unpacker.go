package unpacker

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
)

var (
	// ErrNoUnpackers ...
	ErrNoUnpackers = errors.New("No unpackers found in system PATH")

	// ErrUnpackFailed ...
	ErrUnpackFailed = errors.New("Unpacking failed")
)

// Format ...
type Format interface {
	Unpack(ctx context.Context, src, dest string, w io.Writer) error
	CheckPath(path string) (string, bool)
	Installed() bool
}

// Target ...
type Target struct {
	format Format
	path   string
}

func (t *Target) String() string {
	return t.path
}

func getFormats() []Format {
	unpackers := [...]Format{
		&cmdRAR{
			name:    "rar",
			ext:     regexp.MustCompile(`^\.(rar|r\d\d|\d\d\d)$`),
			command: "unrar",
		},
		&cmdZIP{
			name:    "zip",
			ext:     regexp.MustCompile(`^\.zip$`),
			command: "unzip",
		},
	}
	return unpackers[:]
}

func recursiveDir(path string, cb func(string) error) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fileInfo.IsDir() {
		if err = cb(path); err != nil {
			return err
		}
	} else {
		fileInfo, err := ioutil.ReadDir(path)
		if err != nil {
			return err
		}

		for _, de := range fileInfo {
			err = recursiveDir(filepath.Join(path, de.Name()), cb)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Unpacker ...
type Unpacker struct {
	Formats []Format
}

// New ...
func New() (*Unpacker, error) {
	u := &Unpacker{}
	for _, uc := range getFormats() {
		if uc.Installed() {
			u.Formats = append(u.Formats, uc)
		}
	}

	if u.Empty() {
		return nil, ErrNoUnpackers
	}

	return u, nil
}

// Empty ...
func (u *Unpacker) Empty() bool {
	return len(u.Formats) == 0
}

// Identify ...
func (u *Unpacker) Identify(path string) *Target {
	for _, format := range u.Formats {
		if newPath, ok := format.CheckPath(path); ok {
			return &Target{
				format: format,
				path:   newPath,
			}
		}
	}
	return nil
}

// ScanPath ...
func (u *Unpacker) ScanPath(ctx context.Context, path string) ([]*Target, error) {

	paths := make(map[string]*Target)
	err := recursiveDir(path, func(path string) error {
		if target := u.Identify(path); target != nil {
			if t := paths[target.path]; t == nil {
				paths[target.path] = target
			}
		}
		return ctx.Err()
	})

	var out []*Target
	for _, v := range paths {
		out = append(out, v)
	}

	return out, err
}

// Unpack ...
func (t *Target) Unpack(ctx context.Context, dest string, w io.Writer) error {
	return t.format.Unpack(ctx, filepath.Dir(t.path), dest, w)
}
