package config

import (
	"os"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	cfg := Default()

	if cfg.HTTP.Host != "" {
		t.Errorf("Expected empty host, got %s", cfg.HTTP.Host)
	}

	if cfg.HTTP.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.HTTP.Port)
	}

	if cfg.HTTP.ReadTimeout != 15*time.Second {
		t.Errorf("Expected ReadTimeout 15s, got %v", cfg.HTTP.ReadTimeout)
	}

	if cfg.HTTP.WriteTimeout != 15*time.Second {
		t.Errorf("Expected WriteTimeout 15s, got %v", cfg.HTTP.WriteTimeout)
	}

	if cfg.HTTP.IdleTimeout != 60*time.Second {
		t.Errorf("Expected IdleTimeout 60s, got %v", cfg.HTTP.IdleTimeout)
	}

	if cfg.Shutdown.Timeout != 30*time.Second {
		t.Errorf("Expected Shutdown timeout 30s, got %v", cfg.Shutdown.Timeout)
	}
}

func TestAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		host     string
		port     int
		expected string
	}{
		{
			name:     "empty host",
			host:     "",
			port:     8080,
			expected: ":8080",
		},
		{
			name:     "localhost",
			host:     "localhost",
			port:     3000,
			expected: "localhost:3000",
		},
		{
			name:     "IP address",
			host:     "192.168.1.100",
			port:     9000,
			expected: "192.168.1.100:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.HTTP.Host = tt.host
			cfg.HTTP.Port = tt.port

			result := cfg.Address()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Parallel()

	// Создаем временный YAML файл
	yamlContent := `
http:
  host: "localhost"
  port: 9000
  readTimeout: "30s"
  writeTimeout: "30s"
  idleTimeout: "120s"
shutdown:
  timeout: "60s"
`

	tmpFile, err := os.CreateTemp("", "config_*.yml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		t.Fatalf("Failed to write YAML: %v", err)
	}
	tmpFile.Close()

	// Загружаем конфигурацию
	cfg, err := LoadFromFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Проверяем значения
	if cfg.HTTP.Host != "localhost" {
		t.Errorf("Expected host localhost, got %s", cfg.HTTP.Host)
	}

	if cfg.HTTP.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.HTTP.Port)
	}

	if cfg.HTTP.ReadTimeout != 30*time.Second {
		t.Errorf("Expected ReadTimeout 30s, got %v", cfg.HTTP.ReadTimeout)
	}

	if cfg.HTTP.WriteTimeout != 30*time.Second {
		t.Errorf("Expected WriteTimeout 30s, got %v", cfg.HTTP.WriteTimeout)
	}

	if cfg.HTTP.IdleTimeout != 120*time.Second {
		t.Errorf("Expected IdleTimeout 120s, got %v", cfg.HTTP.IdleTimeout)
	}

	if cfg.Shutdown.Timeout != 60*time.Second {
		t.Errorf("Expected Shutdown timeout 60s, got %v", cfg.Shutdown.Timeout)
	}
}

func TestLoadFromFile_NotExists(t *testing.T) {
	t.Parallel()

	_, err := LoadFromFile("nonexistent_file.yml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}

	// Проверяем что ошибка содержит имя файла
	if !contains(err.Error(), "nonexistent_file.yml") {
		t.Errorf("Error should contain filename, got: %v", err)
	}
}

func TestLoadFromFile_InvalidYAML(t *testing.T) {
	t.Parallel()

	// Создаем файл с невалидным YAML
	invalidYAML := `
http:
  host: "localhost"
  port: 9000
  readTimeout: "30s"
  writeTimeout: "30s"
  idleTimeout: "120s"
shutdown:
  timeout: "60s"
  invalid_yaml: [unclosed
`

	tmpFile, err := os.CreateTemp("", "invalid_config_*.yml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(invalidYAML); err != nil {
		t.Fatalf("Failed to write YAML: %v", err)
	}
	tmpFile.Close()

	_, err = LoadFromFile(tmpFile.Name())
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}

	// Проверяем что ошибка связана с парсингом
	if !contains(err.Error(), "parse") && !contains(err.Error(), "yaml") {
		t.Errorf("Error should mention parsing, got: %v", err)
	}
}

func TestLoadFromFile_EmptyFile(t *testing.T) {
	t.Parallel()

	tmpFile, err := os.CreateTemp("", "empty_config_*.yml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	cfg, err := LoadFromFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load empty config: %v", err)
	}

	// Проверяем что значения по умолчанию не установлены (пустые)
	if cfg.HTTP.Port != 0 {
		t.Errorf("Expected port 0 for empty file, got %d", cfg.HTTP.Port)
	}
}

func TestConfig_Values(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.HTTP.Host = "0.0.0.0"
	cfg.HTTP.Port = 80
	cfg.HTTP.ReadTimeout = 5 * time.Second
	cfg.HTTP.WriteTimeout = 10 * time.Second
	cfg.HTTP.IdleTimeout = 30 * time.Second
	cfg.Shutdown.Timeout = 15 * time.Second

	// Проверяем что Address работает с кастомными значениями
	expected := "0.0.0.0:80"
	if cfg.Address() != expected {
		t.Errorf("Expected %s, got %s", expected, cfg.Address())
	}
}

// Вспомогательная функция для проверки подстроки
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
