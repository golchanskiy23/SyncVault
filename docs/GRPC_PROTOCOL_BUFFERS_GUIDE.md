# Protocol Buffers 3 и gRPC: Полное руководство

## Содержание
- [Protocol Buffers 3](#protocol-buffers-3)
- [gRPC — 4 типа RPC](#grpc---4-типа-rpc)
- [Практическое применение в SyncVault](#практическое-применение-in-syncvault)
- [Примеры кода](#примеры-кода)

---

## Protocol Buffers 3

### Синтаксис proto3

Protocol Buffers (protobuf) — это бинарный формат сериализации данных от Google. Версия 3 (proto3) — текущий стандарт.

```protobuf
syntax = "proto3";

package syncvault.v1;

option go_package = "syncvault/internal/grpc/proto";

import "google/protobuf/timestamp.proto";
import "google/protobuf/duration.proto";
```

### Основные конструкции

#### Message и Field Numbers
```protobuf
message File {
  string id = 1;           // Уникальный номер поля
  string name = 2;         // Обязательный номер >= 1
  int64 size = 3;
  bytes content = 4;
}
```

**Правила field numbers:**
- Номера 1-15 используют 1 байт в кодировке
- Номера 16-2047 используют 2 байта
- Зарезервированы номера 19000-19999 (внутренние protobuf)
- Рекомендуется оставлять "дыры" для будущих полей

#### Scalar Types
```protobuf
message Types {
  double double_field = 1;    // 64-bit
  float float_field = 2;      // 32-bit
  int32 int32_field = 3;      // Variable length
  int64 int64_field = 4;      // Variable length
  uint32 uint32_field = 5;    // Variable length
  uint64 uint64_field = 6;    // Variable length
  sint32 sint32_field = 7;    // ZigZag encoding
  sint64 sint64_field = 8;    // ZigZag encoding
  fixed32 fixed32_field = 9;  // Always 4 bytes
  fixed64 fixed64_field = 10; // Always 8 bytes
  sfixed32 sfixed32_field = 11;// Always 4 bytes
  sfixed64 sfixed64_field = 12;// Always 8 bytes
  bool bool_field = 13;
  string string_field = 14;
  bytes bytes_field = 15;
}
```

#### Enum
```protobuf
enum FileStatus {
  FILE_STATUS_UNSPECIFIED = 0;  // Default value
  FILE_STATUS_CREATED = 1;
  FILE_STATUS_UPLOADED = 2;
  FILE_STATUS_DELETED = 3;
  FILE_STATUS_SYNCED = 4;
}

message File {
  string id = 1;
  FileStatus status = 2;
}
```

**Правила enum:**
- Первое значение должно быть 0 (default)
- Использовать UPPER_SNAKE_CASE
- Добавлять префикс для избежания конфликтов

#### Oneof
```protobuf
message FileOperation {
  oneof operation {
    CreateFile create = 1;
    UpdateFile update = 2;
    DeleteFile delete = 3;
  }
}

message CreateFile {
  string name = 1;
  bytes content = 2;
}

message UpdateFile {
  string id = 1;
  optional string new_name = 2;
}
```

#### Map
```protobuf
message FileMetadata {
  map<string, string> tags = 1;
  map<string, int32> versions = 2;
}
```

### Вложенные сообщения

```protobuf
message SyncEvent {
  string event_id = 1;
  google.protobuf.Timestamp created_at = 2;
  
  message FileChange {
    string file_id = 1;
    string old_path = 2;
    string new_path = 3;
  }
  
  repeated FileChange changes = 3;
}
```

### Repeated Fields
```protobuf
message FileList {
  repeated string file_ids = 1;
  repeated File files = 2;
}
```

### Well-Known Types

#### Timestamp
```protobuf
import "google/protobuf/timestamp.proto";

message File {
  string id = 1;
  google.protobuf.Timestamp created_at = 2;
  google.protobuf.Timestamp updated_at = 3;
}
```

#### Duration
```protobuf
import "google/protobuf/duration.proto";

message SyncSettings {
  google.protobuf.Duration sync_interval = 1;
  google.protobuf.Duration timeout = 2;
}
```

#### Empty
```protobuf
import "google/protobuf/empty.proto";

service FileService {
  rpc Ping(google.protobuf.Empty) returns (google.protobuf.Empty);
}
```

### Toolchain

#### Установка инструментов
```bash
# Protocol Buffers compiler
brew install protobuf  # macOS
apt-get install protobuf-compiler  # Ubuntu

# Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

#### Генерация Go кода
```bash
protoc \
  --go_out=. \
  --go_opt=paths=source_relative \
  --go-grpc_out=. \
  --go-grpc_opt=paths=source_relative \
  path/to/file.proto
```

#### Makefile пример
```makefile
.PHONY: proto
proto:
	protoc \
		--go_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_out=. \
		--go-grpc_opt=paths=source_relative \
		internal/grpc/proto/*.proto
```

### Backward Compatibility

#### ✅ Безопасные изменения
```protobuf
// v1
message File {
  string id = 1;
  string name = 2;
  int64 size = 3;
}

// v2 - backward compatible
message File {
  string id = 1;
  string name = 2;
  int64 size = 3;
  string description = 4;        // ✅ Добавлено новое поле
  repeated string tags = 5;      // ✅ Добавлено новое поле
}
```

#### ❌ Небезопасные изменения
```protobuf
// v1
message File {
  string id = 1;
  string name = 2;
  int64 size = 3;
}

// v2 - NOT backward compatible
message File {
  string id = 1;
  string name = 2;
  // int64 size = 3; ❌ Удалено поле
  string description = 4;        // ❌ Изменён номер поля
  int64 file_size = 5;           // ❌ Изменено имя поля
}
```

### Сравнение с JSON

| Характеристика | Protocol Buffers | JSON |
|---------------|------------------|------|
| **Размер** | ~3-10x меньше | Больше |
| **Скорость** | ~20-100x быстрее | Медленнее |
| **Типизация** | Строгая | Динамическая |
| **Схема** | Обязательная | Опциональная |
| **Читаемость** | Бинарная | Текстовая |
| **Поддержка** | Меньше языков | Универсальная |

#### Пример сравнения
```json
// JSON (91 байт)
{
  "id": "file123",
  "name": "document.pdf",
  "size": 1024,
  "status": "uploaded"
}
```

```protobuf
// Protobuf (≈30 байт)
message File {
  string id = 1;           // "file123"
  string name = 2;         // "document.pdf"
  int64 size = 3;          // 1024
  FileStatus status = 4;   // 2 (UPLOADED)
}
```

### Reserved

#### Резервирование удалённых полей
```protobuf
message File {
  string id = 1;
  string name = 2;
  // int64 size = 3; [deprecated=true]  // Удалено в v2
  int64 file_size = 4;      // Новое поле
  
  reserved 3;               // Резервирование номера
  reserved "size", "old_field"; // Резервирование имён
}
```

#### Best Practices
```protobuf
// ✅ Правильно
message File {
  string id = 1;
  string name = 2;
  int64 size = 3;
  
  reserved 4 to 10;         // Резерв на будущее
}

// ❌ Неправильно
message File {
  string id = 1;
  string name = 2;
  int64 size = 3;
  // Пропуск номеров без резервирования
  string description = 11;
}
```

---

## gRPC — 4 типа RPC

### 1. Unary RPC

Классический запрос-ответ, как в HTTP.

```protobuf
service FileService {
  rpc GetFile(GetFileRequest) returns (GetFileResponse);
}

message GetFileRequest {
  string file_id = 1;
}

message GetFileResponse {
  File file = 1;
}
```

#### Go реализация
```go
func (s *FileService) GetFile(ctx context.Context, req *pb.GetFileRequest) (*pb.GetFileResponse, error) {
    file, err := s.repo.GetByID(ctx, req.FileId)
    if err != nil {
        return nil, status.Error(codes.NotFound, "file not found")
    }
    
    return &pb.GetFileResponse{
        File: &pb.File{
            Id:   file.ID,
            Name: file.Name,
            Size: file.Size,
        },
    }, nil
}
```

#### Клиентский вызов
```go
resp, err := client.GetFile(ctx, &pb.GetFileRequest{
    FileId: "file123",
})
if err != nil {
    log.Fatalf("Failed to get file: %v", err)
}
log.Printf("File: %v", resp.File)
```

### 2. Server Streaming RPC

Один запрос → поток ответов.

```protobuf
service FileService {
  rpc ListFiles(ListFilesRequest) returns (stream FileResponse);
}

message ListFilesRequest {
  string user_id = 1;
  int32 limit = 2;
}

message FileResponse {
  File file = 1;
}
```

#### Go реализация
```go
func (s *FileService) ListFiles(req *pb.ListFilesRequest, stream pb.FileService_ListFilesServer) error {
    files, err := s.repo.GetByUserID(ctx, req.UserId, int(req.Limit))
    if err != nil {
        return status.Error(codes.Internal, "failed to list files")
    }
    
    for _, file := range files {
        err := stream.Send(&pb.FileResponse{
            File: &pb.File{
                Id:   file.ID,
                Name: file.Name,
                Size: file.Size,
            },
        })
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

#### Клиентский вызов
```go
stream, err := client.ListFiles(ctx, &pb.ListFilesRequest{
    UserId: "user123",
    Limit:  100,
})
if err != nil {
    log.Fatalf("Failed to list files: %v", err)
}

for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    if err != nil {
        log.Fatalf("Stream error: %v", err)
    }
    log.Printf("File: %v", resp.File)
}
```

### 3. Client Streaming RPC

Поток запросов → один ответ.

```protobuf
service FileService {
  rpc UploadFile(stream UploadFileRequest) returns (UploadFileResponse);
}

message UploadFileRequest {
  oneof data {
    FileMetadata metadata = 1;
    bytes chunk = 2;
  }
}

message FileMetadata {
  string name = 1;
  string mime_type = 2;
}

message UploadFileResponse {
  string file_id = 1;
  int64 size = 2;
}
```

#### Go реализация
```go
func (s *FileService) UploadFile(stream pb.FileService_UploadFileServer) error {
    var metadata *pb.FileMetadata
    var buffer []byte
    
    for {
        req, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        
        switch data := req.Data.(type) {
        case *pb.UploadFileRequest_Metadata:
            metadata = data.Metadata
        case *pb.UploadFileRequest_Chunk:
            buffer = append(buffer, data.Chunk...)
        }
    }
    
    if metadata == nil {
        return status.Error(codes.InvalidArgument, "metadata required")
    }
    
    fileID, err := s.storage.Store(ctx, metadata.Name, buffer)
    if err != nil {
        return status.Error(codes.Internal, "failed to store file")
    }
    
    return stream.SendAndClose(&pb.UploadFileResponse{
        FileId: fileID,
        Size:   int64(len(buffer)),
    })
}
```

#### Клиентский вызов
```go
stream, err := client.UploadFile(ctx)
if err != nil {
    log.Fatalf("Failed to upload: %v", err)
}

// Отправка метаданных
err = stream.Send(&pb.UploadFileRequest{
    Data: &pb.UploadFileRequest_Metadata{
        Metadata: &pb.FileMetadata{
            Name:     "document.pdf",
            MimeType: "application/pdf",
        },
    },
})
if err != nil {
    log.Fatalf("Failed to send metadata: %v", err)
}

// Отправка чанков
for _, chunk := range fileChunks {
    err = stream.Send(&pb.UploadFileRequest{
        Data: &pb.UploadFileRequest_Chunk{
            Chunk: chunk,
        },
    })
    if err != nil {
        log.Fatalf("Failed to send chunk: %v", err)
    }
}

resp, err := stream.CloseAndRecv()
if err != nil {
    log.Fatalf("Failed to close stream: %v", err)
}
log.Printf("File uploaded: %s", resp.FileId)
```

### 4. Bidirectional Streaming RPC

Поток запросов ↔ поток ответов.

```protobuf
service SyncService {
  rpc SyncEvents(stream SyncRequest) returns (stream SyncResponse);
}

message SyncRequest {
  oneof request {
    SubscribeEvents subscribe = 1;
    AckEvent ack = 2;
  }
}

message SubscribeEvents {
  string user_id = 1;
  google.protobuf.Timestamp since = 2;
}

message AckEvent {
  string event_id = 1;
}

message SyncResponse {
  oneof response {
    SyncEvent event = 1;
    Heartbeat heartbeat = 2;
  }
}

message SyncEvent {
  string event_id = 1;
  string type = 2;
  bytes data = 3;
}

message Heartbeat {
  google.protobuf.Timestamp timestamp = 1;
}
```

#### Go реализация
```go
func (s *SyncService) SyncEvents(stream pb.SyncService_SyncEventsServer) error {
    var userID string
    var eventChan chan *pb.SyncEvent
    
    for {
        req, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        
        switch request := req.Request.(type) {
        case *pb.SyncRequest_SubscribeEvents:
            userID = request.SubscribeEvents.UserId
            eventChan = s.eventManager.Subscribe(userID)
            
            go func() {
                for event := range eventChan {
                    stream.Send(&pb.SyncResponse{
                        Response: &pb.SyncResponse_Event{
                            Event: event,
                        },
                    })
                }
            }()
            
        case *pb.SyncRequest_AckEvent:
            s.eventManager.Acknowledge(request.AckEvent.EventId)
        }
    }
    
    if eventChan != nil {
        s.eventManager.Unsubscribe(userID, eventChan)
    }
    
    return nil
}
```

### gRPC Interceptors

#### Unary Interceptor
```go
func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
    start := time.Now()
    log.Printf("Calling %s", info.FullMethod)
    
    resp, err := handler(ctx, req)
    
    log.Printf("Completed %s in %v", info.FullMethod, time.Since(start))
    return resp, err
}

server := grpc.NewServer(
    grpc.UnaryInterceptor(loggingInterceptor),
)
```

#### Stream Interceptor
```go
func streamLoggingInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
    log.Printf("Starting stream: %s", info.FullMethod)
    
    err := handler(srv, ss)
    
    log.Printf("Completed stream: %s", info.FullMethod)
    return err
}

server := grpc.NewServer(
    grpc.StreamInterceptor(streamLoggingInterceptor),
)
```

#### Chain Interceptors
```go
func chainInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
        chain := handler
        for i := len(interceptors) - 1; i >= 0; i-- {
            chain = createChain(interceptors[i], chain, info)
        }
        return chain(ctx, req)
    }
}

func createChain(interceptor grpc.UnaryServerInterceptor, chain grpc.UnaryHandler, info *grpc.UnaryServerInfo) grpc.UnaryHandler {
    return func(ctx context.Context, req interface{}) (interface{}, error) {
        return interceptor(ctx, req, info, chain)
    }
}
```

### Metadata

#### Отправка metadata
```go
ctx := metadata.AppendToOutgoingContext(ctx, 
    "authorization", "Bearer token123",
    "request-id", "req-456",
    "client-version", "1.0.0",
)

resp, err := client.GetFile(ctx, req)
```

#### Получение metadata на сервере
```go
func (s *FileService) GetFile(ctx context.Context, req *pb.GetFileRequest) (*pb.GetFileResponse, error) {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok {
        return nil, status.Error(codes.Internal, "missing metadata")
    }
    
    auth := md.Get("authorization")
    if len(auth) == 0 {
        return nil, status.Error(codes.Unauthenticated, "missing auth token")
    }
    
    // Обработка запроса...
}
```

#### Отправка trailer metadata
```go
func (s *FileService) GetFile(ctx context.Context, req *pb.GetFileRequest) (*pb.GetFileResponse, error) {
    defer func() {
        if err := grpc.SetTrailer(ctx, metadata.Pairs(
            "processing-time", "150ms",
            "cache-hit", "true",
        )); err != nil {
            log.Printf("Failed to set trailer: %v", err)
        }
    }()
    
    // Обработка запроса...
}
```

### Status Codes

#### Основные коды статуса
```go
// Успешное выполнение
return &pb.Response{}, nil

// Ошибки клиента
return nil, status.Error(codes.InvalidArgument, "invalid file ID")
return nil, status.Error(codes.NotFound, "file not found")
return nil, status.Error(codes.PermissionDenied, "access denied")
return nil, status.Error(codes.Unauthenticated, "missing auth token")

// Ошибки сервера
return nil, status.Error(codes.Internal, "database error")
return nil, status.Error(codes.Unavailable, "service temporarily unavailable")
return nil, status.Error(codes.DeadlineExceeded, "operation timeout")
```

#### Детальная информация об ошибке
```go
return nil, status.Error(codes.InvalidArgument, 
    "invalid file ID: must be UUID format")
```

#### Custom error details
```go
import "google.golang.org/genproto/googleapis/rpc/errdetails"

err := &errdetails.BadRequest{
    FieldViolations: []*errdetails.BadRequest_FieldViolation{
        {
            Field:       "file_id",
            Description: "must be a valid UUID",
        },
    },
}

st, err := status.New(codes.InvalidArgument, "invalid request").WithDetails(err)
if err != nil {
    return nil, status.Error(codes.Internal, "internal error")
}

return nil, st.Err()
```

### gRPC-Gateway

#### Proto аннотации для REST
```protobuf
import "google/api/annotations.proto";

service FileService {
  rpc GetFile(GetFileRequest) returns (GetFileResponse) {
    option (google.api.http) = {
      get: "/v1/files/{file_id}"
    };
  }
  
  rpc CreateFile(CreateFileRequest) returns (CreateFileResponse) {
    option (google.api.http) = {
      post: "/v1/files"
      body: "*"
    };
  }
  
  rpc DeleteFile(DeleteFileRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/v1/files/{file_id}"
    };
  }
}
```

#### Генерация gateway
```bash
protoc \
  --grpc-gateway_out=. \
  --grpc-gateway_opt=paths=source_relative \
  --grpc-gateway_opt=grpc_api_configuration=proto/api_config.yaml \
  internal/grpc/proto/*.proto
```

#### HTTP ↔ gRPC маппинг
| gRPC | HTTP | Пример |
|------|------|--------|
| `GetFile(file_id: "123")` | `GET /v1/files/123` | Unary |
| `CreateFile(file)` | `POST /v1/files` | Unary |
| `ListFiles(user_id: "456")` | `GET /v1/files?user_id=456` | Unary |
| `UploadFile(stream)` | `POST /v1/files/upload` | Streaming |

### Reflection

#### Включение reflection
```go
import "google.golang.org/grpc/reflection"

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("Failed to listen: %v", err)
    }
    
    s := grpc.NewServer()
    pb.RegisterFileServiceServer(s, &fileService{})
    
    // Включение reflection
    reflection.Register(s)
    
    log.Println("gRPC server listening on :50051")
    if err := s.Serve(lis); err != nil {
        log.Fatalf("Failed to serve: %v", err)
    }
}
```

#### Тестирование с grpcurl
```bash
# Список сервисов
grpcurl -plaintext localhost:50051 list

# Список методов сервиса
grpcurl -plaintext localhost:50051 list syncvault.v1.FileService

# Описание метода
grpcurl -plaintext localhost:50051 describe syncvault.v1.FileService.GetFile

# Вызов метода
grpcurl -plaintext -d '{"file_id": "123"}' \
  localhost:50051 syncvault.v1.FileService.GetFile
```

---

## Практическое применение в SyncVault

### Структура proto файлов для SyncVault

```
internal/grpc/proto/
├── common/
│   ├── types.proto          # Общие типы
│   └── errors.proto         # Custom ошибки
├── file/
│   ├── service.proto        # File service
│   └── types.proto          # File-specific типы
├── sync/
│   ├── service.proto        # Sync service
│   └── events.proto         # Sync события
└── node/
    ├── service.proto        # Node management
    └── health.proto         # Health checks
```

### Пример: File Service для SyncVault

```protobuf
syntax = "proto3";

package syncvault.v1;

option go_package = "syncvault/internal/grpc/proto/file/v1;filev1";

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";
import "common/types.proto";

service FileService {
  // Unary RPC
  rpc GetFile(GetFileRequest) returns (GetFileResponse) {
    option (google.api.http) = {
      get: "/v1/files/{file_id}"
    };
  }
  
  // Server Streaming
  rpc ListFiles(ListFilesRequest) returns (stream ListFilesResponse) {
    option (google.api.http) = {
      get: "/v1/files"
    };
  }
  
  // Client Streaming
  rpc UploadFile(stream UploadFileRequest) returns (UploadFileResponse) {
    option (google.api.http) = {
      post: "/v1/files/upload"
    };
  }
  
  // Bidirectional Streaming
  rpc SyncFiles(stream SyncFilesRequest) returns (stream SyncFilesResponse);
}

message GetFileRequest {
  string file_id = 1;
  bool include_content = 2;
}

message GetFileResponse {
  File file = 1;
  bytes content = 2;
}

message ListFilesRequest {
  string user_id = 1;
  string node_id = 2;
  int32 limit = 3;
  string cursor = 4;
  repeated string filters = 5;
}

message ListFilesResponse {
  repeated File files = 1;
  string next_cursor = 2;
  int32 total_count = 3;
}

message UploadFileRequest {
  oneof data {
    FileMetadata metadata = 1;
    bytes chunk = 2;
  }
}

message FileMetadata {
  string name = 1;
  string mime_type = 2;
  int64 size = 3;
  map<string, string> tags = 4;
  string node_id = 5;
}

message UploadFileResponse {
  string file_id = 1;
  string version = 2;
  google.protobuf.Timestamp created_at = 3;
}

message SyncFilesRequest {
  oneof request {
    SyncStart start = 1;
    SyncAck ack = 2;
    SyncComplete complete = 3;
  }
}

message SyncStart {
  string node_id = 1;
  google.protobuf.Timestamp since = 2;
  repeated string file_ids = 3;
}

message SyncAck {
  string event_id = 1;
  bool success = 2;
  string error_message = 3;
}

message SyncComplete {
  string node_id = 1;
  int32 events_processed = 2;
}

message SyncFilesResponse {
  oneof response {
    SyncEvent event = 1;
    SyncStatus status = 2;
    Heartbeat heartbeat = 3;
  }
}

message SyncEvent {
  string event_id = 1;
  string type = 2;
  File file = 3;
  google.protobuf.Timestamp timestamp = 4;
}

message SyncStatus {
  int32 pending_events = 1;
  int32 processed_events = 2;
  google.protobuf.Timestamp last_sync = 3;
}

message Heartbeat {
  google.protobuf.Timestamp timestamp = 1;
}
```

### Пример: Sync Service для реального времени

```protobuf
syntax = "proto3";

package syncvault.v1;

option go_package = "syncvault/internal/grpc/proto/sync/v1;syncv1";

service SyncService {
  // Bidirectional streaming для real-time синхронизации
  rpc StreamSyncEvents(stream SyncEventRequest) returns (stream SyncEventResponse);
  
  // Server streaming для подписки на события
  rpc SubscribeEvents(SubscribeRequest) returns (stream EventResponse);
  
  // Unary RPC для получения статуса синхронизации
  rpc GetSyncStatus(GetSyncStatusRequest) returns (GetSyncStatusResponse);
}

message SyncEventRequest {
  oneof request {
    Subscribe subscribe = 1;
    Unsubscribe unsubscribe = 2;
    AckEvent ack = 3;
    Ping ping = 4;
  }
}

message Subscribe {
  string node_id = 1;
  repeated string event_types = 2;
  google.protobuf.Timestamp since = 3;
}

message Unsubscribe {
  string subscription_id = 1;
}

message AckEvent {
  string event_id = 1;
}

message Ping {
  string node_id = 1;
}

message SyncEventResponse {
  oneof response {
    Event event = 1;
    SubscriptionStatus status = 2;
    Pong pong = 3;
  }
}

message Event {
  string event_id = 1;
  string type = 2;
  string node_id = 3;
  bytes data = 4;
  google.protobuf.Timestamp timestamp = 5;
  map<string, string> metadata = 6;
}

message SubscriptionStatus {
  string subscription_id = 1;
  bool active = 2;
  int32 queued_events = 3;
}

message Pong {
  string node_id = 1;
  google.protobuf.Timestamp timestamp = 2;
}
```

---

## Примеры кода

### Makefile для генерации protobuf

```makefile
.PHONY: proto clean-proto

# Путь к proto файлам
PROTO_DIR := internal/grpc/proto
OUTPUT_DIR := internal/grpc/proto

# Плагины
PROTOC_GEN_GO := $(shell go env GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc
PROTOC_GEN_GRPC_GATEWAY := $(shell go env GOPATH)/bin/protoc-gen-grpc-gateway
PROTOC_GEN_OPENAPIV2 := $(shell go env GOPATH)/bin/protoc-gen-openapiv2

proto:
	@echo "Generating protobuf files..."
	@mkdir -p $(OUTPUT_DIR)
	@protoc \
		--proto_path=$(PROTO_DIR) \
		--go_out=$(OUTPUT_DIR) \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(OUTPUT_DIR) \
		--go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=$(OUTPUT_DIR) \
		--grpc-gateway_opt=paths=source_relative \
		--openapiv2_out=docs \
		--openapiv2_opt=merge_file_name=api \
		$(shell find $(PROTO_DIR) -name "*.proto")
	@echo "✅ Protobuf generation completed"

clean-proto:
	@echo "Cleaning generated protobuf files..."
	@find $(OUTPUT_DIR) -name "*.pb.go" -delete
	@find $(OUTPUT_DIR) -name "*_grpc.pb.go" -delete
	@find $(OUTPUT_DIR) -name "*.pb.gw.go" -delete
	@echo "✅ Cleaned protobuf files"

install-tools:
	@echo "Installing protobuf tools..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	@go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
	@echo "✅ Tools installed"
```

### Server с interceptor'ами

```go
package grpcserver

import (
	"context"
	"log"
	"time"
	
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/peer"
)

type Server struct {
	grpcServer *grpc.Server
}

func NewServer(fileService *FileService, syncService *SyncService) *Server {
	// Chain interceptors
	interceptors := []grpc.UnaryServerInterceptor{
		loggingInterceptor,
		authInterceptor,
		metricsInterceptor,
		rateLimitInterceptor,
	}
	
	chainInterceptor := chainUnaryInterceptors(interceptors...)
	
	s := grpc.NewServer(
		grpc.UnaryInterceptor(chainInterceptor),
		grpc.StreamInterceptor(streamLoggingInterceptor),
		grpc.MaxRecvMsgSize(4*1024*1024), // 4MB
		grpc.MaxSendMsgSize(4*1024*1024), // 4MB
	)
	
	// Регистрация сервисов
	pb.RegisterFileServiceServer(s, fileService)
	pb.RegisterSyncServiceServer(s, syncService)
	
	return &Server{grpcServer: s}
}

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	
	// Логирование запроса
	log.Printf("[GRPC] Started: %s", info.FullMethod)
	
	resp, err := handler(ctx, req)
	
	// Логирование ответа
	duration := time.Since(start)
	log.Printf("[GRPC] Completed: %s in %v", info.FullMethod, duration)
	
	if err != nil {
		log.Printf("[GRPC] Error: %s - %v", info.FullMethod, err)
	}
	
	return resp, err
}

func authInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Пропускаем health checks и reflection
	if info.FullMethod == "/grpc.health.v1.Health/Check" ||
		info.FullMethod == "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo" {
		return handler(ctx, req)
	}
	
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	
	authTokens := md.Get("authorization")
	if len(authTokens) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization token")
	}
	
	// Валидация токена
	if !validateToken(authTokens[0]) {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	
	// Добавление user ID в context
	userID := extractUserIDFromToken(authTokens[0])
	ctx = context.WithValue(ctx, "user_id", userID)
	
	return handler(ctx, req)
}

func metricsInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	
	resp, err := handler(ctx, req)
	
	duration := time.Since(start)
	
	// Запись метрик
	recordGRPCRequest(info.FullMethod, duration, err)
	
	return resp, err
}

func rateLimitInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return nil, status.Error(codes.Internal, "user ID not found")
	}
	
	// Проверка rate limit
	if !checkRateLimit(userID, info.FullMethod) {
		return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}
	
	return handler(ctx, req)
}

func chainUnaryInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		chain := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			chain = createChain(interceptors[i], chain, info)
		}
		return chain(ctx, req)
	}
}

func createChain(interceptor grpc.UnaryServerInterceptor, chain grpc.UnaryHandler, info *grpc.UnaryServerInfo) grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		return interceptor(ctx, req, info, chain)
	}
}
```

### Клиент с retry и connection pooling

```go
package grpcclient

import (
	"context"
	"time"
	
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type Client struct {
	conn   *grpc.ClientConn
	file   pb.FileServiceClient
	sync   pb.SyncServiceClient
	config ClientConfig
}

type ClientConfig struct {
	Address             string
	Timeout             time.Duration
	KeepAliveTime       time.Duration
	KeepAliveTimeout    time.Duration
	MaxRetries          int
	RetryBackoff         time.Duration
	EnableCompression  bool
}

func NewClient(config ClientConfig) (*Client, error) {
	// Настройки keepalive
	kacp := keepalive.ClientParameters{
		Time:                config.KeepAliveTime,
		Timeout:             config.KeepAliveTimeout,
		PermitWithoutStream: true,
	}
	
	// Опции подключения
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(kacp),
		grpc.WithBlock(),
		grpc.WithTimeout(config.Timeout),
	}
	
	if config.EnableCompression {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.UseCompressor("gzip")))
	}
	
	// Подключение с retry
	conn, err := grpc.Dial(config.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	
	return &Client{
		conn:   conn,
		file:   pb.NewFileServiceClient(conn),
		sync:   pb.NewSyncServiceClient(conn),
		config: config,
	}, nil
}

func (c *Client) GetFile(ctx context.Context, fileID string, includeContent bool) (*pb.File, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	
	var lastErr error
	
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := time.Duration(attempt) * c.config.RetryBackoff
			time.Sleep(backoff)
		}
		
		// Проверка соединения
		if c.conn.GetState() != connectivity.Ready {
			if err := c.waitForReady(ctx); err != nil {
				lastErr = err
				continue
			}
		}
		
		resp, err := c.file.GetFile(ctx, &pb.GetFileRequest{
			FileId:        fileID,
			IncludeContent: includeContent,
		})
		
		if err == nil {
			return resp.File, resp.Content, nil
		}
		
		// Проверка, стоит ли retry
		if !shouldRetry(err) {
			return nil, nil, err
		}
		
		lastErr = err
	}
	
	return nil, nil, fmt.Errorf("failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
}

func (c *Client) waitForReady(ctx context.Context) error {
	for {
		switch c.conn.GetState() {
		case connectivity.Ready:
			return nil
		case connectivity.TransientFailure:
			<-time.After(time.Second)
		case connectivity.Shutdown, connectivity.Connecting:
			return fmt.Errorf("connection is %s", c.conn.GetState())
		}
	}
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	
	status, ok := status.FromError(err)
	if !ok {
		return false
	}
	
	// Retry на эти ошибки
	switch status.Code() {
	case codes.DeadlineExceeded,
		 codes.Unavailable,
		 codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

func (c *Client) Close() error {
	return c.conn.Close()
}
```

### Streaming клиент для синхронизации

```go
package syncclient

import (
	"context"
	"log"
	"time"
	
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type SyncClient struct {
	client    pb.SyncServiceClient
	nodeID    string
	eventChan chan *pb.Event
}

func NewSyncClient(client pb.SyncServiceClient, nodeID string) *SyncClient {
	return &SyncClient{
		client:    client,
		nodeID:    nodeID,
		eventChan: make(chan *pb.Event, 100),
	}
}

func (s *SyncClient) StartSync(ctx context.Context) error {
	// Metadata для аутентификации
	md := metadata.New(map[string]string{
		"node-id": s.nodeID,
		"client-version": "1.0.0",
	})
	ctx = metadata.NewOutgoingContext(ctx, md)
	
	// Создание stream
	stream, err := s.client.StreamSyncEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to create sync stream: %w", err)
	}
	
	// Отправка subscribe
	subscribeReq := &pb.SyncEventRequest{
		Request: &pb.SyncEventRequest_Subscribe{
			Subscribe: &pb.Subscribe{
				NodeId:     s.nodeID,
				EventTypes: []string{"file_created", "file_updated", "file_deleted"},
				Since:      timestamppb.Now(),
			},
		},
	}
	
	if err := stream.Send(subscribeReq); err != nil {
		return fmt.Errorf("failed to send subscribe: %w", err)
	}
	
	// Запуск горутин для обработки
	go s.handleIncomingEvents(stream)
	go s.sendHeartbeats(stream)
	
	return nil
}

func (s *SyncClient) handleIncomingEvents(stream pb.SyncService_StreamSyncEventsClient) {
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			log.Printf("[SYNC] Stream closed by server")
			return
		}
		if err != nil {
			log.Printf("[SYNC] Stream error: %v", err)
			return
		}
		
		switch response := resp.Response.(type) {
		case *pb.SyncEventResponse_Event:
			// Обработка события
			s.eventChan <- response.Event
			
			// Отправка ACK
			ackReq := &pb.SyncEventRequest{
				Request: &pb.SyncEventRequest_AckEvent{
					AckEvent: &pb.AckEvent{
						EventId: response.Event.EventId,
						Success: true,
					},
				},
			}
			
			if err := stream.Send(ackReq); err != nil {
				log.Printf("[SYNC] Failed to send ACK: %v", err)
			}
			
		case *pb.SyncEventResponse_Status:
			log.Printf("[SYNC] Status: %+v", response.Status)
			
		case *pb.SyncEventResponse_Pong:
			log.Printf("[SYNC] Received pong: %v", response.Pong.Timestamp)
		}
	}
}

func (s *SyncClient) sendHeartbeats(stream pb.SyncService_StreamSyncEventsClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			pingReq := &pb.SyncEventRequest{
				Request: &pb.SyncEventRequest_Ping{
					Ping: &pb.Ping{
						NodeId: s.nodeID,
					},
				},
			}
			
			if err := stream.Send(pingReq); err != nil {
				log.Printf("[SYNC] Failed to send ping: %v", err)
				return
			}
		}
	}
}

func (s *SyncClient) Events() <-chan *pb.Event {
	return s.eventChan
}

func (s *SyncClient) Stop() error {
	close(s.eventChan)
	return nil
}
```

Этот документ предоставляет полное руководство по использованию Protocol Buffers 3 и gRPC в проекте SyncVault, включая теоретические основы, практические примеры и best practices для production-использования.
