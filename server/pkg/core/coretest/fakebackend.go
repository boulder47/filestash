// Package coretest provides in-memory test doubles for the core interfaces so
// handlers and plugins can be unit-tested without a real storage backend or
// network. Upstream Filestash ships no Go unit tests; this is the seed harness.
package coretest

import (
	"bytes"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/mickael-kerjean/filestash/server/pkg/core"
	"github.com/mickael-kerjean/filestash/server/pkg/utils"
)

// FakeBackend is a minimal, thread-safe, in-memory IBackend implementation.
// Paths are cleaned absolute paths ("/a/b.txt"); directories end with "/".
type FakeBackend struct {
	mu    sync.Mutex
	files map[string][]byte // "/a/b.txt" -> content
	dirs  map[string]bool   // "/a/" -> true
}

// compile-time assertion that we satisfy the interface
var _ core.IBackend = (*FakeBackend)(nil)

// NewFakeBackend returns an empty backend with "/" present.
func NewFakeBackend() *FakeBackend {
	return &FakeBackend{
		files: map[string][]byte{},
		dirs:  map[string]bool{"/": true},
	}
}

func clean(p string) string {
	if p == "" {
		return "/"
	}
	return path.Clean("/" + strings.TrimPrefix(p, "/"))
}

func (b *FakeBackend) Init(params map[string]string, app *core.App) (core.IBackend, error) {
	return b, nil
}

func (b *FakeBackend) LoginForm() core.Form {
	return core.Form{Elmnts: []core.FormElement{{Name: "type", Type: "hidden", Value: "fake"}}}
}

func (b *FakeBackend) Ls(p string) ([]os.FileInfo, error) {
	dir := clean(p)
	prefix := strings.TrimSuffix(dir, "/") + "/"
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := map[string]os.FileInfo{}
	for f := range b.files {
		if rest, ok := childUnder(f, prefix); ok {
			name, isDir := splitChild(rest)
			if _, dup := seen[name]; !dup {
				seen[name] = utils.File{FName: name, FType: ftype(isDir), FSize: int64(len(b.files[f]))}
			}
		}
	}
	for d := range b.dirs {
		if d == dir || d == "/" {
			continue
		}
		if rest, ok := childUnder(strings.TrimSuffix(d, "/"), prefix); ok {
			name, _ := splitChild(rest)
			if _, dup := seen[name]; !dup {
				seen[name] = utils.File{FName: name, FType: "directory"}
			}
		}
	}
	out := make([]os.FileInfo, 0, len(seen))
	for _, fi := range seen {
		out = append(out, fi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}

func (b *FakeBackend) Stat(p string) (os.FileInfo, error) {
	cp := clean(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if content, ok := b.files[cp]; ok {
		return utils.File{FName: path.Base(cp), FType: "file", FSize: int64(len(content))}, nil
	}
	if b.dirs[strings.TrimSuffix(cp, "/")+"/"] || cp == "/" {
		return utils.File{FName: path.Base(cp), FType: "directory"}, nil
	}
	return nil, os.ErrNotExist
}

func (b *FakeBackend) Cat(p string) (io.ReadCloser, error) {
	cp := clean(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	content, ok := b.files[cp]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (b *FakeBackend) Mkdir(p string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dirs[strings.TrimSuffix(clean(p), "/")+"/"] = true
	return nil
}

func (b *FakeBackend) Rm(p string) error {
	cp := clean(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.files, cp)
	delete(b.dirs, strings.TrimSuffix(cp, "/")+"/")
	for f := range b.files {
		if strings.HasPrefix(f, strings.TrimSuffix(cp, "/")+"/") {
			delete(b.files, f)
		}
	}
	return nil
}

func (b *FakeBackend) Mv(from, to string) error {
	cf, ct := clean(from), clean(to)
	b.mu.Lock()
	defer b.mu.Unlock()
	content, ok := b.files[cf]
	if !ok {
		return os.ErrNotExist
	}
	b.files[ct] = content
	delete(b.files, cf)
	return nil
}

func (b *FakeBackend) Save(p string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	cp := clean(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files[cp] = data
	return nil
}

func (b *FakeBackend) Touch(p string) error {
	cp := clean(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.files[cp]; !ok {
		b.files[cp] = []byte{}
	}
	return nil
}

// --- helpers ---

func ftype(isDir bool) string {
	if isDir {
		return "directory"
	}
	return "file"
}

// childUnder reports the remainder of full below prefix when full is within it.
func childUnder(full, prefix string) (string, bool) {
	if !strings.HasPrefix(full, prefix) {
		return "", false
	}
	return full[len(prefix):], true
}

// splitChild returns the first path segment of rest and whether more segments follow.
func splitChild(rest string) (name string, isDir bool) {
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i], true
	}
	return rest, false
}
