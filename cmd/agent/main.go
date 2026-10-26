// cmd/agent — агент синхронизации.
// Запускается на каждой машине которую нужно включить в сеть синхронизации.
//
// Использование:
//
//	go run ./cmd/agent \
//	  --id=office-server \
//	  --root=/srv \
//	  --port=9100 \
//	  --server=192.168.1.10:8081 \
//	  --token=<sync.agentToken из config.yml сервера>
//
// Параметры можно задать и через окружение: AGENT_ID, AGENT_ROOT, AGENT_PORT,
// AGENT_SERVER, AGENT_ADVERTISE, AGENT_TOKEN (удобно для Docker).
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"syncvault/internal/sync/agent"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	nodeID := flag.String("id", env("AGENT_ID", ""), "уникальный ID этого узла (например: office-server)")
	rootPath := flag.String("root", env("AGENT_ROOT", ""),
		"базовая папка: связки создаются для её подпапок (по умолчанию — домашняя папка)")
	port := flag.String("port", env("AGENT_PORT", "9100"), "порт HTTP агента")
	serverAddr := flag.String("server", env("AGENT_SERVER", "localhost:8081"), "адрес сервера SyncVault (host:port)")
	advertise := flag.String("advertise", env("AGENT_ADVERTISE", ""),
		"URL, по которому сервер ходит к агенту (например http://100.64.0.2:9100 в Tailscale); пусто — IP определит сервер")
	token := flag.String("token", env("AGENT_TOKEN", ""), "общий секрет агента (sync.agentToken в config.yml сервера)")
	flag.Parse()

	if *nodeID == "" || *token == "" {
		log.Fatal("--id и --token обязательны")
	}
	if *rootPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("--root не задан и домашняя папка не определена: %v", err)
		}
		*rootPath = home
	}
	abs, err := filepath.Abs(*rootPath)
	if err != nil {
		log.Fatalf("invalid --root: %v", err)
	}
	*rootPath = abs

	a := agent.NewHTTPAgent(*nodeID, *rootPath, *port, *serverAddr, *advertise, *token)
	if err := a.Run(); err != nil {
		log.Fatalf("Agent failed: %v", err)
	}
}
