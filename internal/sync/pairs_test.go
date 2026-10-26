package sync

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// memPairStore — PairStore в памяти для тестов
type memPairStore struct {
	next  int64
	pairs map[int64]*Pair
}

func newMemPairStore() *memPairStore { return &memPairStore{pairs: map[int64]*Pair{}} }

func (s *memPairStore) CreatePair(_ context.Context, p *Pair) error {
	s.next++
	p.ID, p.CreatedAt = s.next, time.Now()
	cp := *p
	s.pairs[p.ID] = &cp
	return nil
}

func (s *memPairStore) ListPairs(context.Context) ([]Pair, error) {
	var out []Pair
	for _, p := range s.pairs {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *memPairStore) GetPair(_ context.Context, id int64) (*Pair, error) {
	p, ok := s.pairs[id]
	if !ok {
		return nil, ErrPairNotFound
	}
	cp := *p
	return &cp, nil
}

func (s *memPairStore) DeletePair(_ context.Context, id int64) error {
	if _, ok := s.pairs[id]; !ok {
		return ErrPairNotFound
	}
	delete(s.pairs, id)
	return nil
}

func (s *memPairStore) SavePairResult(_ context.Context, id int64, at time.Time, status, msg string, res *SyncResult) error {
	p := s.pairs[id]
	p.LastSyncAt, p.LastStatus, p.LastError = &at, status, msg
	if res != nil {
		p.LastTransferred, p.LastDeleted = res.Transferred, res.Deleted
	}
	return nil
}

func newLocalManager(t *testing.T) (*PairManager, string) {
	t.Helper()
	root := t.TempDir()
	e := NewSyncEngine(NewInMemoryStateStore(), KeepNewest)
	return NewPairManager(e, NewNodeRegistry(), newMemPairStore(), root, "token"), root
}

func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func local(path string) PairSide { return PairSide{Device: LocalDevice, Path: path} }

func TestPairManager_IndependentPairs(t *testing.T) {
	ctx := context.Background()
	m, root := newLocalManager(t)
	mkdirs(t, root, "docs", "docs-backup", "photos", "photos-backup")
	writeFile(t, root, "docs/d.txt", "doc", time.Time{})
	writeFile(t, root, "photos/p.jpg", "photo", time.Time{})

	docs := &Pair{Name: "docs", A: local("docs"), B: local(filepath.Join(root, "docs-backup"))}
	photos := &Pair{Name: "photos", A: local("photos"), B: local("photos-backup")}
	for _, p := range []*Pair{docs, photos} {
		if err := m.Create(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.Name, err)
		}
	}
	// Относительные пути приводятся к абсолютным внутри localRoot
	if docs.A.Path != filepath.Join(root, "docs") {
		t.Fatalf("path not normalized: %q", docs.A.Path)
	}

	m.RunAll(ctx)

	if readFile(t, root, "docs-backup/d.txt") != "doc" || readFile(t, root, "photos-backup/p.jpg") != "photo" {
		t.Fatal("pairs not synced")
	}
	if exists(root, "docs-backup/p.jpg") || exists(root, "photos-backup/d.txt") {
		t.Fatal("files leaked between independent pairs")
	}

	pairs, _ := m.List(ctx)
	for _, p := range pairs {
		if p.LastStatus != "ok" || p.LastSyncAt == nil || p.LastTransferred != 1 {
			t.Fatalf("pair %d status not recorded: %+v", p.ID, p)
		}
	}
}

func TestPairManager_RejectsInvalidSides(t *testing.T) {
	ctx := context.Background()
	m, root := newLocalManager(t)
	mkdirs(t, root, "a", "a/sub", "b")
	outside := t.TempDir()

	cases := map[string]*Pair{
		"missing folder":   {A: local("a"), B: local("nope")},
		"outside root":     {A: local("a"), B: local(outside)},
		"dot-dot escape":   {A: local("a"), B: local("../etc")},
		"same folder":      {A: local("a"), B: local(filepath.Join(root, "a"))},
		"nested folder":    {A: local("a"), B: local("a/sub")},
		"whole root":       {A: local(""), B: local("b")},
		"unknown device":   {A: local("a"), B: PairSide{Device: "ghost", Path: "x"}},
		"empty side":       {A: local("a"), B: PairSide{}},
		"drive w/o config": {A: local("a"), B: PairSide{Drive: "x@gmail.com", UserID: "1"}},
	}
	for name, p := range cases {
		if err := m.Create(ctx, p); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestPairManager_DeleteForgetsState(t *testing.T) {
	ctx := context.Background()
	m, root := newLocalManager(t)
	mkdirs(t, root, "a", "b")
	writeFile(t, root, "a/f.txt", "x", time.Time{})

	p := &Pair{A: local("a"), B: local("b")}
	if err := m.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	m.RunAll(ctx)
	if err := m.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if states := m.engine.store.All(sideNodeID(p.ID, "a")); len(states) != 0 {
		t.Fatalf("state of deleted pair kept: %d entries", len(states))
	}
	if err := m.Delete(ctx, p.ID); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("second delete: %v", err)
	}
}

func TestResolveInside(t *testing.T) {
	base := "/home/u/sync"
	ok := map[string]string{
		"":                       base,
		".":                      base,
		"docs":                   base + "/docs",
		"docs/../photos":         base + "/photos",
		"/home/u/sync/docs/2024": base + "/docs/2024",
	}
	for in, want := range ok {
		got, err := ResolveInside(base, in)
		if err != nil || got != want {
			t.Errorf("ResolveInside(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"..", "../x", "docs/../../x", "/home/u/syncX", "/etc"} {
		if _, err := ResolveInside(base, in); err == nil {
			t.Errorf("ResolveInside(%q) succeeded, want error", in)
		}
	}
}
