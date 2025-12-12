.PHONY: proto clean-proto install-tools test-proto

# Пути
PROTO_DIR := .
OUTPUT_DIR := .
DOCS_DIR := ../../../docs

# Инструменты
PROTOC := protoc
PROTOC_GEN_GO := $(shell go env GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc
PROTOC_GEN_GRPC_GATEWAY := $(shell go env GOPATH)/bin/protoc-gen-grpc-gateway
PROTOC_GEN_OPENAPIV2 := $(shell go env GOPATH)/bin/protoc-gen-openapiv2

# Proto файлы
COMMON_PROTOS := $(shell find $(PROTO_DIR)/common -name "*.proto")
STORAGE_PROTOS := $(shell find $(PROTO_DIR)/storage -name "*.proto")
SYNC_PROTOS := $(shell find $(PROTO_DIR)/sync -name "*.proto")
NOTIFICATION_PROTOS := $(shell find $(PROTO_DIR)/notification -name "*.proto")
HEALTH_PROTOS := $(shell find $(PROTO_DIR)/health -name "*.proto")
ALL_PROTOS := $(COMMON_PROTOS) $(STORAGE_PROTOS) $(SYNC_PROTOS) $(NOTIFICATION_PROTOS) $(HEALTH_PROTOS)

# Цели

proto: install-tools
	@echo "🔧 Generating protobuf files..."
	@mkdir -p $(OUTPUT_DIR)
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(OUTPUT_DIR) \
		--grpc-gateway_opt=paths=source_relative \
		--openapiv2_out=$(DOCS_DIR) \
		--openapiv2_opt=merge_file_name=syncvault_api \
		--openapiv2_opt=merge_file_name=syncvault_openapi \
		$(ALL_PROTOS)
	@echo "✅ Protobuf generation completed"
	@echo "📄 Generated files:"
	@find $(OUTPUT_DIR) -name "*.pb.go" -o -name "*_grpc.pb.go" -o -name "*.pb.gw.go" | sort
	@echo "📚 OpenAPI docs generated in $(DOCS_DIR)"

clean-proto:
	@echo "🧹 Cleaning generated protobuf files..."
	@find $(OUTPUT_DIR) -name "*.pb.go" -delete
	@find $(OUTPUT_DIR) -name "*_grpc.pb.go" -delete
	@find $(OUTPUT_DIR) -name "*.pb.gw.go" -delete
	@rm -f $(DOCS_DIR)/syncvault_api.swagger.json
	@rm -f $(DOCS_DIR)/syncvault_openapi.swagger.json
	@echo "✅ Cleaned protobuf files"

install-tools:
	@echo "📦 Installing protobuf tools..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
	@echo "✅ Tools installed"

test-proto: proto
	@echo "🧪 Testing protobuf compilation..."
	@go build ./...
	@echo "✅ Protobuf compilation test passed"

validate-proto:
	@echo "🔍 Validating proto files..."
	@for proto in $(ALL_PROTOS); do \
		echo "Validating $$proto..."; \
		$(PROTOC) --proto_path=$(PROTO_DIR) --descriptor_set_out=/dev/null $$proto; \
	done
	@echo "✅ All proto files are valid"

docs: proto
	@echo "📚 Generating documentation..."
	@mkdir -p $(DOCS_DIR)
	@echo "📄 API documentation generated in $(DOCS_DIR)"

# Генерация по отдельным пакетам
proto-common: install-tools
	@echo "🔧 Generating common proto files..."
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		$(COMMON_PROTOS)

proto-storage: install-tools proto-common
	@echo "🔧 Generating storage proto files..."
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(OUTPUT_DIR) \
		--grpc-gateway_opt=paths=source_relative \
		$(STORAGE_PROTOS)

proto-sync: install-tools proto-common
	@echo "🔧 Generating sync proto files..."
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(OUTPUT_DIR) \
		--grpc-gateway_opt=paths=source_relative \
		$(SYNC_PROTOS)

proto-notification: install-tools proto-common
	@echo "🔧 Generating notification proto files..."
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(OUTPUT_DIR) \
		--grpc-gateway_opt=paths=source_relative \
		$(NOTIFICATION_PROTOS)

proto-health: install-tools proto-common
	@echo "🔧 Generating health proto files..."
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(OUTPUT_DIR) \
		--grpc-gateway_opt=paths=source_relative \
		$(HEALTH_PROTOS)

# Help
help:
	@echo "📋 Available commands:"
	@echo "  proto          - Generate all protobuf files"
	@echo "  proto-common   - Generate common protobuf files"
	@echo "  proto-storage  - Generate storage protobuf files"
	@echo "  proto-sync     - Generate sync protobuf files"
	@echo "  proto-notification - Generate notification protobuf files"
	@echo "  proto-health   - Generate health protobuf files"
	@echo "  clean-proto    - Clean generated protobuf files"
	@echo "  install-tools  - Install protobuf tools"
	@echo "  test-proto     - Test protobuf compilation"
	@echo "  validate-proto - Validate proto files"
	@echo "  docs           - Generate documentation"
	@echo "  help           - Show this help message"
