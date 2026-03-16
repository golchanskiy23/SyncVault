package main

import (
	"os"

	"syncvault/internal/auth"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "server" {
		// Запуск тестового сервера
		auth.RunTestServer()
	} else {
		// Запуск автоматических тестов
		auth.RunTestsWithServer()
	}
}
