package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	internalsync "syncvault/internal/sync"
)

func TestAgent_FolderSyncThroughHTTP(t *testing.T) {
	ctx := context.Background()
	agentBase, local := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(agentBase, "projects", "docs"), 0755)
	os.WriteFile(filepath.Join(agentBase, "projects", "docs", "remote.txt"), []byte("remote"), 0644)
	os.WriteFile(filepath.Join(agentBase, "secret.txt"), []byte("outside of the pair folder"), 0644)
	os.WriteFile(filepath.Join(local, "local.txt"), []byte("local"), 0644)

	a := NewHTTPAgent("dev", agentBase, "0", "localhost:1", "", "secret-token")
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	// Без токена — 401
	resp, err := http.Get(srv.URL + "/agent/files")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", resp.StatusCode)
	}

	// Просмотр папок и проверка несуществующей
	client := internalsync.NewHTTPAgentNode("dev", srv.URL, "secret-token")
	dirs, err := client.ListDirs(ctx, "projects")
	if err != nil || len(dirs) != 1 || dirs[0] != "docs" {
		t.Fatalf("ListDirs = %v, %v", dirs, err)
	}
	if _, err := client.ListDirs(ctx, "nope"); !os.IsNotExist(err) && err == nil {
		t.Fatalf("ListDirs(nope) = %v, want not exist", err)
	}
	if _, err := client.ListDirs(ctx, "../.."); err == nil {
		t.Fatal("ListDirs outside base succeeded")
	}

	// Синхронизация подпапки агента с локальной папкой
	e := internalsync.NewSyncEngine(internalsync.NewInMemoryStateStore(), internalsync.KeepNewest)
	remote := internalsync.NewHTTPAgentFolder("pair1:b", srv.URL, "secret-token", "projects/docs")
	res, err := e.Sync(ctx, internalsync.NewSimpleStorage("pair1:a", local), remote)
	if err != nil || len(res.Errors) > 0 {
		t.Fatalf("sync: %v %v", err, res)
	}
	if b, _ := os.ReadFile(filepath.Join(local, "remote.txt")); string(b) != "remote" {
		t.Fatal("remote file not downloaded")
	}
	if b, _ := os.ReadFile(filepath.Join(agentBase, "projects", "docs", "local.txt")); string(b) != "local" {
		t.Fatal("local file not uploaded into the pair folder")
	}
	if _, err := os.Stat(filepath.Join(local, "secret.txt")); err == nil {
		t.Fatal("file outside of the pair folder was synced")
	}

	// Подпапка за пределами базовой папки агента отклоняется
	escape := internalsync.NewHTTPAgentFolder("x", srv.URL, "secret-token", "../")
	if _, err := escape.ListFiles(ctx); err == nil {
		t.Fatal("root outside base accepted")
	}
}
