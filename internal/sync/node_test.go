package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeAPI — in-memory ComplexStorageAPI с семантикой Drive: элементы адресуются по ID,
// в одной папке допустимы одинаковые имена
type fakeAPI struct {
	nextID int
	items  map[string]*fakeItem
}

type fakeItem struct {
	RemoteFile
	parent  string
	content []byte
}

func newFakeAPI() *fakeAPI { return &fakeAPI{items: map[string]*fakeItem{}} }

func (f *fakeAPI) add(name, parent string, isDir bool, content []byte) string {
	f.nextID++
	id := fmt.Sprintf("id%d", f.nextID)
	sum := sha256.Sum256(content)
	f.items[id] = &fakeItem{
		RemoteFile: RemoteFile{ID: id, Path: name, IsDir: isDir, Hash: hex.EncodeToString(sum[:]), Size: int64(len(content)), ModTime: time.Now()},
		parent:     parent,
		content:    content,
	}
	return id
}

func (f *fakeAPI) ListFiles(_ context.Context, folderID string) ([]RemoteFile, error) {
	var out []RemoteFile
	for _, it := range f.items {
		if it.parent == folderID {
			out = append(out, it.RemoteFile)
		}
	}
	return out, nil
}

func (f *fakeAPI) Download(_ context.Context, fileID, localPath string) error {
	return os.WriteFile(localPath, f.items[fileID].content, 0644)
}

func (f *fakeAPI) Upload(_ context.Context, localPath, folderID string) (string, error) {
	b, err := os.ReadFile(localPath)
	if err != nil {
		return "", err
	}
	parts := strings.Split(localPath, string(os.PathSeparator))
	return f.add(parts[len(parts)-1], folderID, false, b), nil
}

func (f *fakeAPI) Update(_ context.Context, fileID, localPath string) error {
	b, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	it := f.items[fileID]
	it.content, it.Hash, it.Size = b, hex.EncodeToString(sum[:]), int64(len(b))
	return nil
}

func (f *fakeAPI) Delete(_ context.Context, fileID string) error {
	delete(f.items, fileID)
	return nil
}

func (f *fakeAPI) MkDir(_ context.Context, name, parentID string) (string, error) {
	return f.add(name, parentID, true, nil), nil
}

func (f *fakeAPI) count(name string, isDir bool) int {
	n := 0
	for _, it := range f.items {
		if it.Path == name && it.IsDir == isDir {
			n++
		}
	}
	return n
}

func TestComplexStorage_WriteReadDelete(t *testing.T) {
	ctx := context.Background()
	api := newFakeAPI()
	cs := NewComplexStorage("drive", "fake", t.TempDir(), api)

	write := func(path, content string) {
		t.Helper()
		if err := cs.WriteFile(ctx, path, strings.NewReader(content)); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write("docs/sub/a.txt", "v1")
	write("docs/b.txt", "b")
	write("docs/sub/a.txt", "v2") // перезапись — без дубликата файла

	if api.count("a.txt", false) != 1 || api.count("docs", true) != 1 || api.count("sub", true) != 1 {
		t.Fatalf("duplicates created: a.txt=%d docs=%d sub=%d", api.count("a.txt", false), api.count("docs", true), api.count("sub", true))
	}

	r, err := cs.ReadFile(ctx, "docs/sub/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "v2" {
		t.Fatalf("read %q, want v2", b)
	}

	entries, err := cs.ListFiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2 files", entries)
	}

	if err := cs.DeleteFile(ctx, "docs/sub/a.txt"); err != nil {
		t.Fatal(err)
	}
	if api.count("a.txt", false) != 0 {
		t.Fatal("file not deleted")
	}
	if _, err := cs.ReadFile(ctx, "docs/sub/a.txt"); err == nil {
		t.Fatal("deleted file still readable")
	}
}

func TestSync_LocalToComplexStorage(t *testing.T) {
	dir := t.TempDir()
	api := newFakeAPI()
	e := NewSyncEngine(NewInMemoryStateStore(), KeepNewest)
	local, remote := NewSimpleStorage("local", dir), NewComplexStorage("drive", "fake", t.TempDir(), api)

	writeFile(t, dir, "x/one.txt", "1", time.Time{})
	api.add("two.txt", "", false, []byte("2"))

	mustSync(t, e, local, remote)
	if readFile(t, dir, "two.txt") != "2" || api.count("one.txt", false) != 1 {
		t.Fatal("files not exchanged with complex storage")
	}

	// Хеши совпадают с локальными SHA-256 — повторная синхронизация ничего не передаёт
	if res := mustSync(t, e, local, remote); res.Transferred != 0 {
		t.Fatalf("second sync transferred %d files", res.Transferred)
	}

	os.Remove(dir + "/x/one.txt")
	if res := mustSync(t, e, local, remote); res.Deleted != 1 || api.count("one.txt", false) != 0 {
		t.Fatalf("local deletion not propagated to complex storage: %+v", res)
	}
}

func TestSimpleStorage_RejectsPathsOutsideRoot(t *testing.T) {
	ctx := context.Background()
	s := NewSimpleStorage("a", t.TempDir())
	for _, p := range []string{"../escape.txt", "sub/../../escape.txt", "/etc/passwd"} {
		if err := s.WriteFile(ctx, p, strings.NewReader("x")); err == nil {
			t.Errorf("WriteFile(%q) succeeded, want error", p)
		}
		if _, err := s.ReadFile(ctx, p); err == nil {
			t.Errorf("ReadFile(%q) succeeded, want error", p)
		}
	}
	if err := s.WriteFile(ctx, "ok/inside.txt", strings.NewReader("x")); err != nil {
		t.Fatalf("WriteFile inside root: %v", err)
	}
}
