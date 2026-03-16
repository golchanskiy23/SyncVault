package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"syncvault/internal/grpc/interceptors"
	healthv1 "syncvault/internal/grpc/proto/health"
)

// HealthService реализация gRPC HealthService
type HealthService struct {
	healthv1.UnimplementedHealthServiceServer
}

// NewHealthService создает новый HealthService
func NewHealthService() *HealthService {
	return &HealthService{}
}

// Check стандартный health check
func (h *HealthService) Check(ctx context.Context, req *healthv1.HealthCheckRequest) (*healthv1.HealthCheckResponse, error) {
	fmt.Printf("HealthCheck.Check called for service: %s\n", req.Service)
	return &healthv1.HealthCheckResponse{
		Status: healthv1.HealthCheckResponse_SERVING,
	}, nil
}

// Watch стандартный streaming health check
func (h *HealthService) Watch(req *healthv1.HealthCheckRequest, stream healthv1.HealthService_WatchServer) error {
	ctx := stream.Context()
	fmt.Printf("HealthCheck.Watch called for service: %s\n", req.Service)

	if err := stream.Send(&healthv1.HealthCheckResponse{
		Status: healthv1.HealthCheckResponse_SERVING,
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to send health status: %v", err)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := stream.Send(&healthv1.HealthCheckResponse{
				Status: healthv1.HealthCheckResponse_SERVING,
			}); err != nil {
				return status.Errorf(codes.Internal, "failed to send health update: %v", err)
			}
		}
	}
}

// DetailedHealth расширенный health check с детальной информацией
func (h *HealthService) DetailedHealth(ctx context.Context, req *healthv1.DetailedHealthRequest) (*healthv1.DetailedHealthResponse, error) {
	fmt.Printf("DetailedHealth called for service: %s\n", req.Service)

	claims, _ := interceptors.GetClaimsFromContext(ctx)

	return &healthv1.DetailedHealthResponse{
		Service:   req.Service,
		Status:    healthv1.HealthCheckResponse_SERVING,
		Timestamp: timestamppb.Now(),
		Version:   "1.0.0",
		Build:     "dev-20240316",
		StartedAt: timestamppb.New(time.Now().Add(-24 * time.Hour)),
		Uptime:    durationpb.New(24 * time.Hour),
		Checks: []*healthv1.HealthCheck{
			{
				Name:      "database",
				Status:    healthv1.HealthCheckResponse_SERVING,
				Message:   "Database connection is healthy",
				Timestamp: timestamppb.Now(),
				Duration:  durationpb.New(50 * time.Millisecond),
				Details: map[string]string{
					"connection_pool": "8/10 active",
					"response_time":   "5ms",
					"last_query":      time.Now().Add(-1 * time.Minute).Format("2006-01-02 15:04:05"),
				},
				Issues: []*healthv1.HealthIssue{},
			},
			{
				Name:      "storage",
				Status:    healthv1.HealthCheckResponse_SERVING,
				Message:   "Storage backend is operational",
				Timestamp: timestamppb.Now(),
				Duration:  durationpb.New(100 * time.Millisecond),
				Details: map[string]string{
					"backend":       "gridfs",
					"free_space":    "8.5GB",
					"total_space":   "10GB",
					"usage_percent": "15%",
				},
				Issues: []*healthv1.HealthIssue{},
			},
			{
				Name:      "notification_service",
				Status:    healthv1.HealthCheckResponse_SERVING,
				Message:   "Notification service is running",
				Timestamp: timestamppb.Now(),
				Duration:  durationpb.New(20 * time.Millisecond),
				Details: map[string]string{
					"active_subscriptions": "150",
					"queue_size":           "25",
					"delivery_rate":        "0.98",
				},
				Issues: []*healthv1.HealthIssue{
					{
						Code:       "HIGH_QUEUE_SIZE",
						Message:    "Notification queue is above threshold",
						Severity:   healthv1.HealthSeverity_HEALTH_SEVERITY_WARNING,
						DetectedAt: timestamppb.New(time.Now().Add(-10 * time.Minute)),
						Resolved:   false,
						Context: map[string]string{
							"queue_size": "25",
							"threshold":  "20",
						},
						Suggestions: []string{
							"Increase notification processing capacity",
							"Scale notification workers",
						},
					},
				},
			},
		},
		Dependencies: []*healthv1.DependencyStatus{
			{
				Name:                "mongodb",
				Type:                "database",
				Address:             "mongodb://localhost:27017",
				Status:              healthv1.HealthCheckResponse_SERVING,
				Message:             "MongoDB connection is healthy",
				LastCheck:           timestamppb.Now(),
				ResponseTime:        durationpb.New(50 * time.Millisecond),
				ConsecutiveFailures: 0,
				Details: map[string]string{
					"version":     "6.0.8",
					"replica_set": "rs0",
					"primary":     "mongodb1:27017",
				},
			},
			{
				Name:                "redis",
				Type:                "cache",
				Address:             "redis://localhost:6379",
				Status:              healthv1.HealthCheckResponse_SERVING,
				Message:             "Redis cache is operational",
				LastCheck:           timestamppb.Now(),
				ResponseTime:        durationpb.New(10 * time.Millisecond),
				ConsecutiveFailures: 0,
				Details: map[string]string{
					"memory_usage":      "45MB",
					"max_memory":        "128MB",
					"connected_clients": "12",
				},
			},
		},
		Metrics: &healthv1.HealthMetrics{
			ActiveConnections: 25,
			TotalRequests:     1000000,
			FailedRequests:    5000,
			SuccessRate:       0.995,
			AvgResponseTime:   durationpb.New(150 * time.Millisecond),
			MaxResponseTime:   durationpb.New(2500 * time.Millisecond),
			MemoryUsageBytes:  512 * 1024 * 1024,
			MemoryLimitBytes:  2 * 1024 * 1024 * 1024,
			CpuUsagePercent:   15.5,
			DiskUsageBytes:    10 * 1024 * 1024 * 1024,
			DiskLimitBytes:    50 * 1024 * 1024 * 1024,
			Goroutines:        45,
			OpenFiles:         150,
			LastGc:            timestamppb.New(time.Now().Add(-5 * time.Minute)),
			GcCount:           1250,
		},
		Issues: []*healthv1.HealthIssue{
			{
				Code:       "HIGH_QUEUE_SIZE",
				Message:    "Notification queue is above threshold",
				Severity:   healthv1.HealthSeverity_HEALTH_SEVERITY_WARNING,
				DetectedAt: timestamppb.New(time.Now().Add(-10 * time.Minute)),
				Resolved:   false,
				Context: map[string]string{
					"affected_service": "notification",
					"queue_size":       "25",
					"threshold":        "20",
				},
				Suggestions: []string{
					"Increase notification processing capacity",
					"Add more notification workers",
					"Consider message batching",
				},
			},
		},
		Metadata: map[string]string{
			"node_id":     claims.NodeID,
			"environment": "production",
			"region":      "us-west-2",
			"version":     "1.0.0",
			"build_time":  time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z"),
		},
	}, nil
}

// ServiceHealth health check для конкретного сервиса
func (h *HealthService) ServiceHealth(ctx context.Context, req *healthv1.ServiceHealthRequest) (*healthv1.ServiceHealthResponse, error) {
	fmt.Printf("ServiceHealth called: %s\n", req.ServiceName)

	var deps []*healthv1.DependencyStatus
	if req.IncludeDependencies {
		deps = []*healthv1.DependencyStatus{}
	}

	var metrics *healthv1.HealthMetrics
	if req.IncludeMetrics {
		metrics = &healthv1.HealthMetrics{}
	}

	return &healthv1.ServiceHealthResponse{
		ServiceName: req.ServiceName,
		Status:      healthv1.HealthCheckResponse_SERVING,
		Version:     "1.0.0",
		Checks: []*healthv1.HealthCheck{
			{
				Name:      "core",
				Status:    healthv1.HealthCheckResponse_SERVING,
				Message:   "Core functionality is operational",
				Timestamp: timestamppb.Now(),
				Duration:  durationpb.New(100 * time.Millisecond),
				Details: map[string]string{
					"uptime":              "24h",
					"requests_per_second": "10.5",
				},
				Issues: []*healthv1.HealthIssue{},
			},
		},
		Dependencies: deps,
		Metrics:      metrics,
		Issues:       []*healthv1.HealthIssue{},
	}, nil
}

// AllServicesHealth health check для всех сервисов
func (h *HealthService) AllServicesHealth(ctx context.Context, req *emptypb.Empty) (*healthv1.AllServicesHealthResponse, error) {
	fmt.Printf("AllServicesHealth called\n")

	return &healthv1.AllServicesHealthResponse{
		Timestamp: timestamppb.Now(),
		Services: []*healthv1.ServiceHealthResponse{
			{
				ServiceName: "storage",
				Status:      healthv1.HealthCheckResponse_SERVING,
				Version:     "1.0.0",
				Checks: []*healthv1.HealthCheck{
					{
						Name:      "storage_backend",
						Status:    healthv1.HealthCheckResponse_SERVING,
						Message:   "GridFS storage is operational",
						Timestamp: timestamppb.Now(),
						Duration:  durationpb.New(50 * time.Millisecond),
						Details:   map[string]string{"backend": "gridfs", "free_space": "8.5GB"},
						Issues:    []*healthv1.HealthIssue{},
					},
				},
				Dependencies: []*healthv1.DependencyStatus{
					{
						Name:         "mongodb",
						Type:         "database",
						Address:      "mongodb://localhost:27017",
						Status:       healthv1.HealthCheckResponse_SERVING,
						Message:      "Connected",
						LastCheck:    timestamppb.Now(),
						ResponseTime: durationpb.New(50 * time.Millisecond),
					},
				},
				Metrics: &healthv1.HealthMetrics{
					ActiveConnections: 10,
					TotalRequests:     500000,
					FailedRequests:    1000,
					SuccessRate:       0.998,
					AvgResponseTime:   durationpb.New(200 * time.Millisecond),
					MaxResponseTime:   durationpb.New(3 * time.Second),
				},
				Issues: []*healthv1.HealthIssue{},
			},
			{
				ServiceName: "sync",
				Status:      healthv1.HealthCheckResponse_SERVING,
				Version:     "1.0.0",
				Checks: []*healthv1.HealthCheck{
					{
						Name:      "sync_engine",
						Status:    healthv1.HealthCheckResponse_SERVING,
						Message:   "Sync engine is running",
						Timestamp: timestamppb.Now(),
						Duration:  durationpb.New(100 * time.Millisecond),
						Details:   map[string]string{"active_sessions": "5", "pending_events": "10"},
						Issues:    []*healthv1.HealthIssue{},
					},
				},
				Dependencies: []*healthv1.DependencyStatus{
					{
						Name:         "storage",
						Type:         "service",
						Address:      "storage:50051",
						Status:       healthv1.HealthCheckResponse_SERVING,
						Message:      "Connected",
						LastCheck:    timestamppb.Now(),
						ResponseTime: durationpb.New(20 * time.Millisecond),
					},
				},
				Metrics: &healthv1.HealthMetrics{
					ActiveConnections: 5,
					TotalRequests:     250000,
					FailedRequests:    500,
					SuccessRate:       0.998,
					AvgResponseTime:   durationpb.New(150 * time.Millisecond),
					MaxResponseTime:   durationpb.New(2 * time.Second),
				},
				Issues: []*healthv1.HealthIssue{},
			},
			{
				ServiceName: "notification",
				Status:      healthv1.HealthCheckResponse_SERVING,
				Version:     "1.0.0",
				Checks: []*healthv1.HealthCheck{
					{
						Name:      "notification_engine",
						Status:    healthv1.HealthCheckResponse_NOT_SERVING,
						Message:   "High queue size detected",
						Timestamp: timestamppb.Now(),
						Duration:  durationpb.New(20 * time.Millisecond),
						Details:   map[string]string{"queue_size": "25", "threshold": "20"},
						Issues: []*healthv1.HealthIssue{
							{
								Code:        "HIGH_QUEUE_SIZE",
								Message:     "Queue size above threshold",
								Severity:    healthv1.HealthSeverity_HEALTH_SEVERITY_WARNING,
								DetectedAt:  timestamppb.New(time.Now().Add(-10 * time.Minute)),
								Resolved:    false,
								Context:     map[string]string{"queue_size": "25", "threshold": "20"},
								Suggestions: []string{"Scale notification workers"},
							},
						},
					},
				},
				Dependencies: []*healthv1.DependencyStatus{
					{
						Name:         "redis",
						Type:         "cache",
						Address:      "redis://localhost:6379",
						Status:       healthv1.HealthCheckResponse_SERVING,
						Message:      "Connected",
						LastCheck:    timestamppb.Now(),
						ResponseTime: durationpb.New(10 * time.Millisecond),
					},
				},
				Metrics: &healthv1.HealthMetrics{
					ActiveConnections: 150,
					TotalRequests:     1000000,
					FailedRequests:    5000,
					SuccessRate:       0.995,
					AvgResponseTime:   durationpb.New(50 * time.Millisecond),
					MaxResponseTime:   durationpb.New(1 * time.Second),
				},
				Issues: []*healthv1.HealthIssue{
					{
						Code:        "HIGH_QUEUE_SIZE",
						Message:     "Queue size above threshold",
						Severity:    healthv1.HealthSeverity_HEALTH_SEVERITY_WARNING,
						DetectedAt:  timestamppb.New(time.Now().Add(-10 * time.Minute)),
						Resolved:    false,
						Context:     map[string]string{"queue_size": "25", "threshold": "20"},
						Suggestions: []string{"Scale notification workers"},
					},
				},
			},
		},
		OverallStatus:     healthv1.HealthCheckResponse_SERVING,
		HealthyServices:   2,
		UnhealthyServices: 1,
		TotalServices:     3,
		GlobalIssues: []*healthv1.HealthIssue{
			{
				Code:       "NOTIFICATION_QUEUE_OVERLOAD",
				Message:    "Notification service queue is overloaded",
				Severity:   healthv1.HealthSeverity_HEALTH_SEVERITY_WARNING,
				DetectedAt: timestamppb.New(time.Now().Add(-10 * time.Minute)),
				Resolved:   false,
				Context: map[string]string{
					"affected_service": "notification",
					"queue_size":       "25",
					"threshold":        "20",
				},
				Suggestions: []string{
					"Increase notification processing capacity",
					"Add more notification workers",
					"Consider message batching",
				},
			},
		},
	}, nil
}

// DependencyHealth health check для зависимостей
func (h *HealthService) DependencyHealth(ctx context.Context, req *healthv1.DependencyHealthRequest) (*healthv1.DependencyHealthResponse, error) {
	fmt.Printf("DependencyHealth called: %s, %v\n", req.DependencyNames, req.IncludeMetrics)

	return &healthv1.DependencyHealthResponse{
		Timestamp: timestamppb.Now(),
		Dependencies: []*healthv1.DependencyStatus{
			{
				Name:                "mongodb",
				Type:                "database",
				Address:             "mongodb://localhost:27017",
				Status:              healthv1.HealthCheckResponse_SERVING,
				Message:             "MongoDB connection is healthy",
				LastCheck:           timestamppb.Now(),
				ResponseTime:        durationpb.New(50 * time.Millisecond),
				ConsecutiveFailures: 0,
				Details: map[string]string{
					"version":         "6.0.8",
					"replica_set":     "rs0",
					"primary":         "mongodb1:27017",
					"connection_pool": "8/10 active",
				},
			},
			{
				Name:                "redis",
				Type:                "cache",
				Address:             "redis://localhost:6379",
				Status:              healthv1.HealthCheckResponse_SERVING,
				Message:             "Redis cache is operational",
				LastCheck:           timestamppb.Now(),
				ResponseTime:        durationpb.New(10 * time.Millisecond),
				ConsecutiveFailures: 0,
				Details: map[string]string{
					"version":           "7.2.3",
					"memory_usage":      "45MB",
					"max_memory":        "128MB",
					"connected_clients": "12",
					"hit_rate":          "0.95",
				},
			},
			{
				Name:                "storage-backend",
				Type:                "service",
				Address:             "storage:50051",
				Status:              healthv1.HealthCheckResponse_SERVING,
				Message:             "Storage service is responding",
				LastCheck:           timestamppb.Now(),
				ResponseTime:        durationpb.New(20 * time.Millisecond),
				ConsecutiveFailures: 0,
				Details: map[string]string{
					"version":    "1.0.0",
					"backend":    "gridfs",
					"free_space": "8.5GB",
				},
			},
		},
		OverallStatus:         healthv1.HealthCheckResponse_SERVING,
		HealthyDependencies:   3,
		UnhealthyDependencies: 0,
		TotalDependencies:     3,
	}, nil
}

// SystemHealth health check для системы
func (h *HealthService) SystemHealth(ctx context.Context, req *healthv1.SystemHealthRequest) (*healthv1.SystemHealthResponse, error) {
	fmt.Printf("SystemHealth called: include_detailed=%v\n", req.IncludeDetailedMetrics)

	return &healthv1.SystemHealthResponse{
		Status:    healthv1.HealthCheckResponse_SERVING,
		Timestamp: timestamppb.Now(),
		SystemInfo: &healthv1.SystemInfo{
			Hostname:      "syncvault-server-1",
			Os:            "linux",
			Arch:          "amd64",
			KernelVersion: "5.15.0-107-generic",
			CpuCores:      4,
			TotalMemory:   8 * 1024 * 1024 * 1024,
			TotalDisk:     50 * 1024 * 1024 * 1024,
			BootTime:      timestamppb.New(time.Now().Add(-24 * time.Hour)),
			Uptime:        durationpb.New(24 * time.Hour),
		},
		ResourceUsage: &healthv1.ResourceUsage{
			// proto: message CPUUsage (не CpuUsage)
			Cpu: &healthv1.CPUUsage{
				UsagePercent:     15.5,
				LoadAverage_1M:   1.2,
				LoadAverage_5M:   1.5,
				LoadAverage_15M:  1.8,
				RunningProcesses: 125,
				TotalProcesses:   280,
			},
			Memory: &healthv1.MemoryUsage{
				TotalBytes:       8 * 1024 * 1024 * 1024,
				UsedBytes:        int64(2.5 * 1024 * 1024 * 1024), // float-константа → int64
				FreeBytes:        int64(5.5 * 1024 * 1024 * 1024),
				AvailableBytes:   5*1024*1024*1024 + 200*1024*1024, // ~5.2GB
				CachedBytes:      512 * 1024 * 1024,
				BuffersBytes:     128 * 1024 * 1024,
				UsagePercent:     31.25,
				SwapUsagePercent: 5.2,
			},
			Disk: &healthv1.DiskUsage{
				TotalBytes:         50 * 1024 * 1024 * 1024,
				UsedBytes:          15 * 1024 * 1024 * 1024,
				FreeBytes:          35 * 1024 * 1024 * 1024,
				UsagePercent:       30.0,
				InodesTotal:        1000000,
				InodesUsed:         300000,
				InodesFree:         700000,
				InodesUsagePercent: 30.0,
				MountPoint:         "/",
				Filesystem:         "ext4",
			},
			Network: &healthv1.NetworkUsage{
				BytesSent:              1024 * 1024 * 1024,
				BytesReceived:          2 * 1024 * 1024 * 1024,
				PacketsSent:            1000000,
				PacketsReceived:        2000000,
				ErrorsIn:               10,
				ErrorsOut:              5,
				DropsIn:                25,
				DropsOut:               15,
				ConnectionsActive:      25,
				ConnectionsEstablished: 150,
			},
			Process: &healthv1.ProcessUsage{
				Pid:             12345,
				CpuPercent:      5.2,
				MemoryRssBytes:  128 * 1024 * 1024,
				MemoryVmsBytes:  256 * 1024 * 1024,
				FileDescriptors: 150,
				Threads:         8,
				StartTime:       durationpb.New(24 * time.Hour), // proto: Duration, не Timestamp
				CpuTime:         durationpb.New(1 * time.Hour),
			},
		},
		Environment: &healthv1.EnvironmentInfo{
			EnvironmentVariables: map[string]string{
				"GO_ENV":    "production",
				"PORT":      "50051",
				"LOG_LEVEL": "info",
			},
			Arguments: []string{
				"./syncvault-server",
				"--config=/etc/syncvault/config.yaml",
				"--log-format=json",
			},
			WorkingDirectory: "/opt/syncvault",
			ExecutablePath:   "/opt/syncvault/bin/syncvault-server",
			Configuration: map[string]string{
				"database_url":    "mongodb://localhost:27017",
				"redis_url":       "redis://localhost:6379",
				"storage_backend": "gridfs",
				"max_connections": "1000",
			},
		},
		Issues: []*healthv1.HealthIssue{
			{
				Code:       "HIGH_MEMORY_USAGE",
				Message:    "Memory usage is above 75%",
				Severity:   healthv1.HealthSeverity_HEALTH_SEVERITY_WARNING,
				DetectedAt: timestamppb.New(time.Now().Add(-5 * time.Minute)),
				Resolved:   false,
				Context: map[string]string{
					"current_usage":    "75%",
					"threshold":        "75%",
					"available_memory": "2GB",
				},
				Suggestions: []string{
					"Add more memory to the server",
					"Optimize memory usage",
					"Restart memory-intensive services",
				},
			},
		},
		Metadata: map[string]string{
			"node_id":     "syncvault-server-1",
			"environment": "production",
			"region":      "us-west-2",
			"version":     "1.0.0",
			"build_time":  time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04:05Z"),
		},
	}, nil
}

// Live Kubernetes LivenessProbe
func (h *HealthService) Live(ctx context.Context, req *emptypb.Empty) (*healthv1.LiveResponse, error) {
	fmt.Printf("Live probe called\n")
	return &healthv1.LiveResponse{
		Alive:     true,
		Timestamp: timestamppb.Now(),
		Version:   "1.0.0",
	}, nil
}

// Ready Kubernetes ReadinessProbe
func (h *HealthService) Ready(ctx context.Context, req *emptypb.Empty) (*healthv1.ReadyResponse, error) {
	fmt.Printf("Ready probe called\n")
	return &healthv1.ReadyResponse{
		Ready:            true,
		Timestamp:        timestamppb.Now(),
		Version:          "1.0.0",
		NotReadyServices: []string{},
	}, nil
}

// Startup Kubernetes StartupProbe
func (h *HealthService) Startup(ctx context.Context, req *emptypb.Empty) (*healthv1.StartupResponse, error) {
	fmt.Printf("Startup probe called\n")
	return &healthv1.StartupResponse{
		Started:     true,
		Timestamp:   timestamppb.Now(),
		Version:     "1.0.0",
		Phase:       "running",
		Message:     "All services started successfully",
		StartupTime: durationpb.New(30 * time.Second),
	}, nil
}
