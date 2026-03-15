package ports

import (
	"context"
	"io"
	"time"

	"syncvault/internal/domain/valueobjects"
)

type Storage interface {
	PutFile(ctx context.Context, path valueobjects.FilePath, content io.Reader, size int64) error
	GetFile(ctx context.Context, path valueobjects.FilePath) (io.ReadCloser, int64, error)
	DeleteFile(ctx context.Context, path valueobjects.FilePath) error
	FileExists(ctx context.Context, path valueobjects.FilePath) (bool, error)

	CreateDir(ctx context.Context, path valueobjects.FilePath) error
	ListDir(ctx context.Context, path valueobjects.FilePath) ([]FileInfo, error)

	GetFileInfo(ctx context.Context, path valueobjects.FilePath) (*FileInfo, error)
	GetSpaceInfo(ctx context.Context) (*SpaceInfo, error)

	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected(ctx context.Context) bool
}

type FileInfo struct {
	Path        valueobjects.FilePath
	Size        int64
	ModifiedAt  time.Time
	IsDir       bool
	Hash        valueobjects.FileHash
	Permissions string
}

type SpaceInfo struct {
	TotalSpace int64
	UsedSpace  int64
	FreeSpace  int64
}

type EventBus interface {
	Publish(ctx context.Context, event interface{}) error
	PublishAsync(ctx context.Context, event interface{}) error

	Subscribe(ctx context.Context, eventType string, handler EventHandler) error
	Unsubscribe(ctx context.Context, eventType string, handler EventHandler) error

	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsRunning() bool
}

type EventHandler interface {
	Handle(ctx context.Context, event interface{}) error
	EventType() string
}

type EventHandlerFunc func(ctx context.Context, event interface{}) error

func (f EventHandlerFunc) Handle(ctx context.Context, event interface{}) error {
	return f(ctx, event)
}

func (f EventHandlerFunc) EventType() string {
	return ""
}

type Cache interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string) (interface{}, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)

	SetMultiple(ctx context.Context, items map[string]interface{}, ttl time.Duration) error
	GetMultiple(ctx context.Context, keys []string) (map[string]interface{}, error)
	DeleteMultiple(ctx context.Context, keys []string) error

	GetByPattern(ctx context.Context, pattern string) (map[string]interface{}, error)
	DeleteByPattern(ctx context.Context, pattern string) error

	Clear(ctx context.Context) error
	TTL(ctx context.Context, key string) (time.Duration, error)

	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected(ctx context.Context) bool
}

type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

type ConfigProvider interface {
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	GetDuration(key string) time.Duration
	GetStringSlice(key string) []string
	IsSet(key string) bool
	Set(key string, value interface{})
}

type MetricsCollector interface {
	Counter(name string, tags map[string]string) Counter
	Gauge(name string, tags map[string]string) Gauge
	Histogram(name string, tags map[string]string) Histogram
	Timer(name string, tags map[string]string) Timer
}

type Counter interface {
	Inc()
	Add(value float64)
}

type Gauge interface {
	Set(value float64)
	Inc()
	Dec()
}

type Histogram interface {
	Observe(value float64)
}

type Timer interface {
	Time() func()
	Record(duration time.Duration)
}
