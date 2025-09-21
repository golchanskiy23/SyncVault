# SyncVault Makefile

.PHONY: test test-middleware test-handlers test-integration test-coverage build run clean help

# Переменные
BINARY_NAME=syncvault
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Основные команды
all: test

# Тесты с покрытием
test-coverage:
	@echo "Running tests with coverage..."
	go test ./internal/app/tests/... -cover -coverprofile=$(COVERAGE_FILE) -coverpkg=./internal/app/...
	go tool cover -func=$(COVERAGE_FILE)

# Запуск всех тестов
test:
	@echo "Running all tests..."
	go test ./internal/app/tests/... -v

# Middleware тесты
test-middleware:
	@echo "Running middleware tests..."
	go test ./internal/app/tests/... -run TestMiddleware -v

# HTTP handlers тесты
test-handlers:
	@echo "Running HTTP handlers tests..."
	go test ./internal/app/tests/... -run TestHTTPHandlers -v

# Integration тесты
test-integration:
	@echo "Running integration tests..."
	go test ./internal/app/tests/... -run TestIntegration -v

# Сборка приложения
build:
	@echo "🔨 Building application..."
	go build -o $(BINARY_NAME) ./cmd/server
	@echo "✅ Build completed: $(BINARY_NAME)"

# Запуск приложения
run:
	@echo "🚀 Starting application..."
	go run ./cmd/server

# Запуск с отладкой
debug:
	@echo "🐛 Starting application in debug mode..."
	go run ./cmd/server -debug

# Проверка кода
lint:
	@echo "🔍 Running linter..."
	golangci-lint run ./...
	@echo "✅ Linting completed"

# Форматирование кода
fmt:
	@echo "📝 Formatting code..."
	go fmt ./...
	@echo "✅ Code formatted"

# Проверка зависимостей
deps:
	@echo "📦 Checking dependencies..."
	go mod verify
	go mod tidy
	@echo "✅ Dependencies checked"

# Обновление зависимостей
update-deps:
	@echo "📦 Updating dependencies..."
	go get -u ./...
	go mod tidy
	@echo "✅ Dependencies updated"

# Генерация моков
generate-mocks:
	@echo "🎭 Generating mocks..."
	mockgen -source=internal/usecase/interfaces.go -destination=internal/usecase/mocks/mock_file.go -package=mocks
	@echo "✅ Mocks generated"

# Очистка
clean:
	@echo "🧹 Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -f $(COVERAGE_FILE)
	rm -f $(COVERAGE_HTML)
	rm -f $(COVERAGE_FILE)_integration
	rm -f $(COVERAGE_HTML)_integration
	go clean -cache
	@echo "✅ Clean completed"

# Очистка тестовых данных
clean-testdata:
	@echo "🧹 Cleaning test data..."
	rm -f internal/app/testdata/*.golden
	@echo "✅ Test data cleaned"

# Установка зависимостей для тестов
install-test-deps:
	@echo "📦 Installing test dependencies..."
	go install github.com/golang/mock/mockgen@latest
	go get github.com/stretchr/testify
	go get github.com/golang/mock/gomock
	@echo "✅ Test dependencies installed"

# Запуск тестов в Docker
test-docker:
	@echo "🐳 Running tests in Docker..."
	docker-compose -f docker-compose.test.yml up --build --abort-on-container-exit
	@echo "✅ Docker tests completed"

# Сборка Docker образа
docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t syncvault:latest .
	@echo "✅ Docker image built"

# Запуск в Docker
docker-run:
	@echo "🐳 Running in Docker..."
	docker run -p 8080:8080 syncvault:latest

# Проверка безопасности
security:
	@echo "🔒 Running security scan..."
	gosec ./...
	@echo "✅ Security scan completed"

# Бенчмарки
benchmark:
	@echo "⚡ Running benchmarks..."
	go test -bench=. -benchmem ./...
	@echo "✅ Benchmarks completed"

# Профилирование
profile:
	@echo "📊 Profiling application..."
	go run ./cmd/server -cpuprofile=cpu.prof -memprofile=mem.prof
	@echo "✅ Profiling started"

# Анализ зависимостей
analyze-deps:
	@echo "📊 Analyzing dependencies..."
	go mod graph | dot -Tpng -o dependency-graph.png
	@echo "✅ Dependency graph generated: dependency-graph.png"

# Валидация OpenAPI спецификации
validate-openapi:
	@echo "📋 Validating OpenAPI specification..."
	swagctl validate docs/swagger.json
	@echo "✅ OpenAPI specification validated"

# Генерация документации
generate-docs:
	@echo "📚 Generating documentation..."
	swag init -g cmd/server/main.go -o docs/
	@echo "✅ Documentation generated"

# Полный цикл разработки
dev: fmt lint test-unit build run
	@echo "🔄 Full development cycle completed"

# CI/CD пайплайн
ci: deps lint test-unit test-integration build security
	@echo "🚀 CI pipeline completed"

# Показать помощь
help:
	@echo "📋 SyncVault Makefile"
	@echo ""
	@echo "Available commands:"
	@echo "  🧪 test          - Run all tests"
	@echo "  🧪 test-unit     - Run unit tests with coverage"
	@echo "  🔄 test-integration - Run integration tests"
	@echo "  📊 test-coverage  - Run tests with coverage report"
	@echo "  📄 test-golden   - Run tests with golden files update"
	@echo "  🔨 build         - Build the application"
	@echo "  🚀 run           - Run the application"
	@echo "  🐛 debug         - Run in debug mode"
	@echo "  🔍 lint          - Run linter"
	@echo "  📝 fmt           - Format code"
	@echo "  📦 deps          - Check dependencies"
	@echo "  📦 update-deps   - Update dependencies"
	@echo "  🎭 generate-mocks - Generate mocks"
	@echo "  🧹 clean         - Clean build artifacts"
	@echo "  🐳 docker        - Docker commands"
	@echo "  🔒 security      - Security scan"
	@echo "  ⚡ benchmark     - Run benchmarks"
	@echo "  📊 profile       - Profile application"
	@echo "  🚀 dev           - Full development cycle"
	@echo "  🚀 ci            - CI/CD pipeline"
