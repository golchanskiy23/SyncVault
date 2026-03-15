package aggregates

import (
	"time"

	"syncvault/internal/domain/valueobjects"
)

type SyncJobStatus string

const (
	SyncJobStatusPending   SyncJobStatus = "pending"
	SyncJobStatusRunning   SyncJobStatus = "running"
	SyncJobStatusCompleted SyncJobStatus = "completed"
	SyncJobStatusFailed    SyncJobStatus = "failed"
	SyncJobStatusCancelled SyncJobStatus = "cancelled"
)

type FileOperationStatus string

const (
	FileOperationStatusPending   FileOperationStatus = "pending"
	FileOperationStatusRunning   FileOperationStatus = "running"
	FileOperationStatusCompleted FileOperationStatus = "completed"
	FileOperationStatusFailed    FileOperationStatus = "failed"
	FileOperationStatusSkipped   FileOperationStatus = "skipped"
)

type FileOperation struct {
	id          valueobjects.FileID
	fileID      valueobjects.FileID
	operation   string
	status      FileOperationStatus
	startedAt   *time.Time
	completedAt *time.Time
	errorMsg    string
	attempts    int
	maxAttempts int
}

func NewFileOperation(fileID valueobjects.FileID, operation string) *FileOperation {
	return &FileOperation{
		id:          valueobjects.NewFileID(),
		fileID:      fileID,
		operation:   operation,
		status:      FileOperationStatusPending,
		maxAttempts: 3,
	}
}

func (op *FileOperation) ID() valueobjects.FileID {
	return op.id
}

func (op *FileOperation) FileID() valueobjects.FileID {
	return op.fileID
}

func (op *FileOperation) Operation() string {
	return op.operation
}

func (op *FileOperation) Status() FileOperationStatus {
	return op.status
}

func (op *FileOperation) StartedAt() *time.Time {
	return op.startedAt
}

func (op *FileOperation) CompletedAt() *time.Time {
	return op.completedAt
}

func (op *FileOperation) ErrorMsg() string {
	return op.errorMsg
}

func (op *FileOperation) Attempts() int {
	return op.attempts
}

func (op *FileOperation) MaxAttempts() int {
	return op.maxAttempts
}

func (op *FileOperation) Start() {
	if op.status == FileOperationStatusPending {
		now := time.Now()
		op.status = FileOperationStatusRunning
		op.startedAt = &now
		op.attempts++
	}
}

func (op *FileOperation) Complete() {
	if op.status == FileOperationStatusRunning {
		now := time.Now()
		op.status = FileOperationStatusCompleted
		op.completedAt = &now
	}
}

func (op *FileOperation) Fail(errorMsg string) {
	if op.status == FileOperationStatusRunning {
		op.status = FileOperationStatusFailed
		op.errorMsg = errorMsg
		if op.attempts >= op.maxAttempts {
			now := time.Now()
			op.completedAt = &now
		}
	}
}

func (op *FileOperation) Retry() {
	if op.status == FileOperationStatusFailed && op.attempts < op.maxAttempts {
		op.status = FileOperationStatusPending
		op.errorMsg = ""
	}
}

func (op *FileOperation) Skip(reason string) {
	now := time.Now()
	op.status = FileOperationStatusSkipped
	op.completedAt = &now
	op.errorMsg = reason
}

func (op *FileOperation) CanRetry() bool {
	return op.status == FileOperationStatusFailed && op.attempts < op.maxAttempts
}

func (op *FileOperation) IsCompleted() bool {
	return op.status == FileOperationStatusCompleted || op.status == FileOperationStatusSkipped
}

func (op *FileOperation) IsFailed() bool {
	return op.status == FileOperationStatusFailed && op.attempts >= op.maxAttempts
}

type SyncJob struct {
	id          valueobjects.SyncJobID
	sourceNode  valueobjects.StorageNodeID
	targetNode  valueobjects.StorageNodeID
	status      SyncJobStatus
	operations  []*FileOperation
	createdAt   time.Time
	startedAt   *time.Time
	completedAt *time.Time
	errorMsg    string
	progress    int
	total       int
}

func NewSyncJob(
	sourceNode, targetNode valueobjects.StorageNodeID,
	fileIDs []valueobjects.FileID,
) *SyncJob {
	operations := make([]*FileOperation, len(fileIDs))
	for i, fileID := range fileIDs {
		operations[i] = NewFileOperation(fileID, "sync")
	}

	return &SyncJob{
		id:         valueobjects.NewSyncJobID(),
		sourceNode: sourceNode,
		targetNode: targetNode,
		status:     SyncJobStatusPending,
		operations: operations,
		createdAt:  time.Now(),
		total:      len(fileIDs),
		progress:   0,
	}
}

func (job *SyncJob) ID() valueobjects.SyncJobID {
	return job.id
}

func (job *SyncJob) SourceNode() valueobjects.StorageNodeID {
	return job.sourceNode
}

func (job *SyncJob) TargetNode() valueobjects.StorageNodeID {
	return job.targetNode
}

func (job *SyncJob) Status() SyncJobStatus {
	return job.status
}

func (job *SyncJob) Operations() []*FileOperation {
	return job.operations
}

func (job *SyncJob) CreatedAt() time.Time {
	return job.createdAt
}

func (job *SyncJob) StartedAt() *time.Time {
	return job.startedAt
}

func (job *SyncJob) CompletedAt() *time.Time {
	return job.completedAt
}

func (job *SyncJob) ErrorMsg() string {
	return job.errorMsg
}

func (job *SyncJob) Progress() int {
	return job.progress
}

func (job *SyncJob) Total() int {
	return job.total
}

func (job *SyncJob) Start() {
	if job.status == SyncJobStatusPending {
		now := time.Now()
		job.status = SyncJobStatusRunning
		job.startedAt = &now
	}
}

func (job *SyncJob) Complete() {
	if job.status == SyncJobStatusRunning {
		now := time.Now()
		job.status = SyncJobStatusCompleted
		job.completedAt = &now
	}
}

func (job *SyncJob) Fail(errorMsg string) {
	if job.status == SyncJobStatusRunning {
		now := time.Now()
		job.status = SyncJobStatusFailed
		job.completedAt = &now
		job.errorMsg = errorMsg
	}
}

func (job *SyncJob) Cancel() {
	if job.status == SyncJobStatusPending || job.status == SyncJobStatusRunning {
		now := time.Now()
		job.status = SyncJobStatusCancelled
		job.completedAt = &now
	}
}

func (job *SyncJob) UpdateProgress() {
	completed := 0
	for _, op := range job.operations {
		if op.IsCompleted() {
			completed++
		}
	}
	job.progress = completed

	if completed == len(job.operations) && job.status == SyncJobStatusRunning {
		job.Complete()
	}
}

func (job *SyncJob) GetPendingOperations() []*FileOperation {
	var pending []*FileOperation
	for _, op := range job.operations {
		if op.Status() == FileOperationStatusPending {
			pending = append(pending, op)
		}
	}
	return pending
}

func (job *SyncJob) GetFailedOperations() []*FileOperation {
	var failed []*FileOperation
	for _, op := range job.operations {
		if op.IsFailed() {
			failed = append(failed, op)
		}
	}
	return failed
}

func (job *SyncJob) GetRunningOperations() []*FileOperation {
	var running []*FileOperation
	for _, op := range job.operations {
		if op.Status() == FileOperationStatusRunning {
			running = append(running, op)
		}
	}
	return running
}

func (job *SyncJob) HasFailures() bool {
	for _, op := range job.operations {
		if op.IsFailed() {
			return true
		}
	}
	return false
}

func (job *SyncJob) IsCompleted() bool {
	return job.status == SyncJobStatusCompleted ||
		job.status == SyncJobStatusFailed ||
		job.status == SyncJobStatusCancelled
}

func (job *SyncJob) GetProgressPercentage() float64 {
	if job.total == 0 {
		return 0
	}
	return float64(job.progress) / float64(job.total) * 100
}
