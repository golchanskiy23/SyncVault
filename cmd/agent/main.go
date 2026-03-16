// cmd/agent — агент синхронизации.
// Запускается на каждой машине которую нужно включить в сеть синхронизации.
//
// Использование:
//
//	go run cmd/agent/main.go \
//	  --id=laptop-home \
//	  --root=/home/bloom/all/all_files/Резюме \
//	  --port=9100 \
//	  --server=localhost:8081
package main

import (
	"flag"
	"log"

	"syncvault/internal/sync/agent"
)

func main() {
	nodeID := flag.String("id", "", "уникальный ID этого узла (например: laptop-home)")
	rootPath := flag.String("root", "", "локальная папка для синхронизации")
	port := flag.String("port", "9100", "порт HTTP агента")
	serverAddr := flag.String("server", "localhost:8081", "адрес middleware сервера")
	flag.Parse()

	if *nodeID == "" || *rootPath == "" {
		log.Fatal("--id и --root обязательны")
	}

	a := agent.NewHTTPAgent(*nodeID, *rootPath, *port, *serverAddr)
	if err := a.Run(); err != nil {
		log.Fatalf("Agent failed: %v", err)
	}
}
