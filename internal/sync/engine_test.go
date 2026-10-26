package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, root, rel, content string, mtime time.Time) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(full, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func mustSync(t *testing.T, e *SyncEngine, a, b Node) *SyncResult {
	t.Helper()
	res, err := e.Sync(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("sync errors: %v", res.Errors)
	}
	return res
}

func newPair(t *testing.T, strategy ConflictStrategy) (*SyncEngine, string, string, Node, Node) {
	t.Helper()
	dirA, dirB := t.TempDir(), t.TempDir()
	e := NewSyncEngine(NewInMemoryStateStore(), strategy)
	return e, dirA, dirB, NewSimpleStorage("a", dirA), NewSimpleStorage("b", dirB)
}

func TestSync_InitialBidirectional(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "one.txt", "1", time.Time{})
	writeFile(t, dirA, "sub/two.txt", "2", time.Time{})
	writeFile(t, dirB, "three.txt", "3", time.Time{})

	res := mustSync(t, e, a, b)
	if res.Transferred != 3 {
		t.Fatalf("transferred = %d, want 3", res.Transferred)
	}
	for _, dir := range []string{dirA, dirB} {
		if readFile(t, dir, "one.txt") != "1" || readFile(t, dir, "sub/two.txt") != "2" || readFile(t, dir, "three.txt") != "3" {
			t.Fatalf("files not synced in %s", dir)
		}
	}

	if res := mustSync(t, e, a, b); res.Transferred != 0 {
		t.Fatalf("second sync transferred = %d, want 0", res.Transferred)
	}
}

func TestSync_TargetEditPropagatesBack(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "f.txt", "v1", time.Time{})
	mustSync(t, e, a, b)

	writeFile(t, dirB, "f.txt", "edited on b", time.Time{})
	res := mustSync(t, e, a, b)

	if len(res.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", res.Conflicts)
	}
	if got := readFile(t, dirA, "f.txt"); got != "edited on b" {
		t.Fatalf("a/f.txt = %q, edit on target was lost", got)
	}
}

func TestSync_ConcurrentEdit_KeepNewest(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "f.txt", "v1", time.Time{})
	mustSync(t, e, a, b)

	now := time.Now()
	writeFile(t, dirA, "f.txt", "older on a", now.Add(-time.Hour))
	writeFile(t, dirB, "f.txt", "newer on b", now)
	res := mustSync(t, e, a, b)

	if len(res.Conflicts) != 1 || res.Conflicts[0].Resolution != "kept_target_newer" {
		t.Fatalf("conflicts = %+v, want one kept_target_newer", res.Conflicts)
	}
	if readFile(t, dirA, "f.txt") != "newer on b" || readFile(t, dirB, "f.txt") != "newer on b" {
		t.Fatal("newest version did not win")
	}
	if res := mustSync(t, e, a, b); res.Transferred != 0 || len(res.Conflicts) != 0 {
		t.Fatalf("sync after resolution not idle: %+v", res)
	}
}

func TestSync_ConcurrentEdit_KeepBoth(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepBoth)
	writeFile(t, dirA, "f.txt", "v1", time.Time{})
	mustSync(t, e, a, b)

	writeFile(t, dirA, "f.txt", "from a", time.Time{})
	writeFile(t, dirB, "f.txt", "from b", time.Time{})
	res := mustSync(t, e, a, b)

	if len(res.Conflicts) != 1 || res.Conflicts[0].Resolution != "kept_both" {
		t.Fatalf("conflicts = %+v, want one kept_both", res.Conflicts)
	}
	if got := readFile(t, dirB, "f.txt.conflict.a"); got != "from a" {
		t.Fatalf("conflict copy = %q, want %q", got, "from a")
	}
	if readFile(t, dirA, "f.txt") != "from b" || readFile(t, dirB, "f.txt") != "from b" {
		t.Fatal("original files did not converge")
	}

	// Следующий проход разносит копию конфликта и больше не конфликтует
	res = mustSync(t, e, a, b)
	if len(res.Conflicts) != 0 {
		t.Fatalf("conflict repeated: %+v", res.Conflicts)
	}
	if got := readFile(t, dirA, "f.txt.conflict.a"); got != "from a" {
		t.Fatalf("a/f.txt.conflict.a = %q", got)
	}
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func TestSync_DeletePropagates(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "keep.txt", "k", time.Time{})
	writeFile(t, dirA, "gone.txt", "g", time.Time{})
	mustSync(t, e, a, b)

	os.Remove(filepath.Join(dirB, "gone.txt"))
	res := mustSync(t, e, a, b)
	if res.Deleted != 1 || exists(dirA, "gone.txt") {
		t.Fatalf("deletion on target not propagated: %+v", res)
	}

	// Повторная синхронизация ничего не воскрешает
	if res := mustSync(t, e, a, b); res.Transferred != 0 || exists(dirA, "gone.txt") || exists(dirB, "gone.txt") {
		t.Fatalf("deleted file came back: %+v", res)
	}
}

func TestSync_ModifiedAfterDeleteIsRestored(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "f.txt", "v1", time.Time{})
	mustSync(t, e, a, b)

	os.Remove(filepath.Join(dirB, "f.txt"))
	writeFile(t, dirA, "f.txt", "v2", time.Time{})
	res := mustSync(t, e, a, b)
	if res.Deleted != 0 || readFile(t, dirB, "f.txt") != "v2" {
		t.Fatalf("modified file should win over deletion: %+v", res)
	}
}

func TestSync_DeleteOfInitiallyIdenticalFile(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "same.txt", "x", time.Time{})
	writeFile(t, dirB, "same.txt", "x", time.Time{})
	if res := mustSync(t, e, a, b); res.Transferred != 0 {
		t.Fatalf("identical folders transferred %d files", res.Transferred)
	}

	os.Remove(filepath.Join(dirA, "same.txt"))
	if res := mustSync(t, e, a, b); res.Deleted != 1 || exists(dirB, "same.txt") {
		t.Fatalf("deletion of initially identical file not propagated: %+v", res)
	}
}

func TestSync_DeleteAcrossThreeNodes(t *testing.T) {
	dirs := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	e := NewSyncEngine(NewInMemoryStateStore(), KeepNewest)
	nodes := []Node{NewSimpleStorage("a", dirs[0]), NewSimpleStorage("b", dirs[1]), NewSimpleStorage("c", dirs[2])}

	writeFile(t, dirs[0], "f.txt", "v1", time.Time{})
	if _, err := e.SyncAll(context.Background(), nodes); err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !exists(d, "f.txt") {
			t.Fatalf("f.txt not replicated to %s", d)
		}
	}

	os.Remove(filepath.Join(dirs[1], "f.txt"))
	for i := 0; i < 2; i++ {
		if _, err := e.SyncAll(context.Background(), nodes); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range dirs {
		if exists(d, "f.txt") {
			t.Fatalf("f.txt resurrected in %s", d)
		}
	}
}

func TestSync_ForgetNodeDoesNotDeleteFromOthers(t *testing.T) {
	e, dirA, _, a, b := newPair(t, KeepNewest)
	writeFile(t, dirA, "f.txt", "v1", time.Time{})
	mustSync(t, e, a, b)

	// Узел b перерегистрирован на пустую папку: без ForgetNode её файлы сочлись бы удалёнными
	dirNew := t.TempDir()
	e.ForgetNode("b")
	res := mustSync(t, e, a, NewSimpleStorage("b", dirNew))
	if res.Deleted != 0 || !exists(dirA, "f.txt") || !exists(dirNew, "f.txt") {
		t.Fatalf("files were deleted after node relocation: %+v", res)
	}
}

func TestSync_EmptiedNodeDoesNotWipeOthers(t *testing.T) {
	e, dirA, dirB, a, b := newPair(t, KeepNewest)
	for i := 0; i <= wipeThreshold; i++ {
		writeFile(t, dirA, fmt.Sprintf("f%d.txt", i), "x", time.Time{})
	}
	mustSync(t, e, a, b)

	// Папка b опустела (например, не примонтирован диск)
	os.RemoveAll(dirB)
	os.MkdirAll(dirB, 0755)
	if _, err := e.Sync(context.Background(), a, b); err == nil {
		t.Fatal("expected sync to refuse mass deletion")
	}
	if !exists(dirA, "f0.txt") || !exists(dirA, fmt.Sprintf("f%d.txt", wipeThreshold)) {
		t.Fatal("files were deleted from the healthy node")
	}
}
