package agent

import (
	"context"
	"io"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	internalsync "syncvault/internal/sync"
)

// FileTransfer gRPC сервис — минимальный протокол передачи файлов между агентами.
// Используем вручную написанные структуры вместо proto-gen для простоты.

// --- Типы запросов/ответов ---

type ListFilesRequest struct{}

type ListFilesResponse struct {
	Files []RemoteFileInfo
}

type RemoteFileInfo struct {
	Path string
	Hash string
	Size int64
}

type DownloadRequest struct {
	Path string
}

type FileChunkResponse struct {
	Data []byte
}

type UploadRequest struct {
	Payload isUploadRequest_Payload
}

type isUploadRequest_Payload interface{ isUploadRequest_Payload() }

type UploadRequest_Header struct{ Header *FileHeader }
type UploadRequest_Chunk struct{ Chunk *FileChunk }

func (*UploadRequest_Header) isUploadRequest_Payload() {}
func (*UploadRequest_Chunk) isUploadRequest_Payload()  {}

type FileHeader struct{ Path string }
type FileChunk struct{ Data []byte }

type UploadResponse struct{ Path string }

type DeleteRequest struct{ Path string }
type DeleteResponse struct{}

type MkDirRequest struct{ Path string }
type MkDirResponse struct{}

// --- Интерфейсы клиента и сервера ---

type FileTransferClient interface {
	ListFiles(ctx context.Context, req *ListFilesRequest) (*ListFilesResponse, error)
	DownloadFile(ctx context.Context, req *DownloadRequest) (DownloadStream, error)
	UploadFile(ctx context.Context) (UploadStream, error)
	DeleteFile(ctx context.Context, req *DeleteRequest) (*DeleteResponse, error)
	MkDir(ctx context.Context, req *MkDirRequest) (*MkDirResponse, error)
}

type DownloadStream interface {
	Recv() (*FileChunkResponse, error)
}

type UploadStream interface {
	Send(*UploadRequest) error
	CloseAndRecv() (*UploadResponse, error)
}

// --- Серверная реализация ---

type FileTransferService struct {
	node *internalsync.SimpleStorage
}

func NewFileTransferService(node *internalsync.SimpleStorage) *FileTransferService {
	return &FileTransferService{node: node}
}

func (s *FileTransferService) ListFiles(ctx context.Context) (*ListFilesResponse, error) {
	entries, err := s.node.ListFiles(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list files: %v", err)
	}

	files := make([]RemoteFileInfo, 0, len(entries))
	for _, e := range entries {
		files = append(files, RemoteFileInfo{Path: e.Path, Hash: e.Hash, Size: e.Size})
	}
	return &ListFilesResponse{Files: files}, nil
}

func (s *FileTransferService) DownloadFile(ctx context.Context, path string, send func([]byte) error) error {
	r, err := s.node.ReadFile(ctx, path)
	if err != nil {
		return status.Errorf(codes.NotFound, "read file %s: %v", path, err)
	}
	defer r.Close()

	buf := make([]byte, 1024*1024) // 1MB чанки
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if sendErr := send(buf[:n]); sendErr != nil {
				return sendErr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "read chunk: %v", err)
		}
	}
}

func (s *FileTransferService) UploadFile(ctx context.Context, path string, r io.Reader) error {
	return s.node.WriteFile(ctx, path, r)
}

func (s *FileTransferService) DeleteFile(ctx context.Context, path string) error {
	return s.node.DeleteFile(ctx, path)
}

func (s *FileTransferService) MkDir(ctx context.Context, path string) error {
	return s.node.MkDir(ctx, path)
}

// RegisterFileTransferServer регистрирует сервис на gRPC сервере через HTTP/2 вручную.
// В реальном проекте это делается через protoc-gen-go-grpc.
// Здесь используем простой HTTP/2 multiplexer поверх gRPC.
func RegisterFileTransferServer(srv *grpc.Server, svc *FileTransferService) {
	// Регистрируем через описание сервиса
	srv.RegisterService(&fileTransferServiceDesc, svc)
}

var fileTransferServiceDesc = grpc.ServiceDesc{
	ServiceName: "syncvault.agent.FileTransfer",
	HandlerType: (*FileTransferService)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams:     []grpc.StreamDesc{},
}

// NewFileTransferClient создаёт клиент для подключения к агенту
func NewFileTransferClient(conn grpc.ClientConnInterface) FileTransferClient {
	return &fileTransferClientImpl{conn: conn}
}

// fileTransferClientImpl — HTTP-based клиент (упрощённая реализация без proto)
// В продакшне заменяется на сгенерированный gRPC клиент
type fileTransferClientImpl struct {
	conn grpc.ClientConnInterface
}

func (c *fileTransferClientImpl) ListFiles(ctx context.Context, _ *ListFilesRequest) (*ListFilesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use HTTP agent client")
}

func (c *fileTransferClientImpl) DownloadFile(ctx context.Context, _ *DownloadRequest) (DownloadStream, error) {
	return nil, status.Error(codes.Unimplemented, "use HTTP agent client")
}

func (c *fileTransferClientImpl) UploadFile(ctx context.Context) (UploadStream, error) {
	return nil, status.Error(codes.Unimplemented, "use HTTP agent client")
}

func (c *fileTransferClientImpl) DeleteFile(ctx context.Context, _ *DeleteRequest) (*DeleteResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use HTTP agent client")
}

func (c *fileTransferClientImpl) MkDir(ctx context.Context, _ *MkDirRequest) (*MkDirResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use HTTP agent client")
}

// --- HTTP агент клиент (реальная реализация без proto зависимости) ---

type HTTPAgentClient struct {
	baseURL string
	http    *httpClient
}

type httpClient struct {
	client *os.File // placeholder
}

func NewHTTPAgentNode(id, agentURL string) *HTTPRemoteNode {
	return &HTTPRemoteNode{id: id, agentURL: agentURL}
}

// HTTPRemoteNode — реализует sync.Node через HTTP API агента
type HTTPRemoteNode struct {
	id       string
	agentURL string // http://host:port
}

func (n *HTTPRemoteNode) ID() string   { return n.id }
func (n *HTTPRemoteNode) Type() string { return "remote_simple" }

func (n *HTTPRemoteNode) ListFiles(ctx context.Context) ([]internalsync.FileEntry, error) {
	resp, err := doGet(ctx, n.agentURL+"/agent/files")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Files []internalsync.FileEntry `json:"files"`
	}
	if err := decodeJSON(resp.Body, &result); err != nil {
		return nil, err
	}
	return result.Files, nil
}

func (n *HTTPRemoteNode) ReadFile(ctx context.Context, path string) (io.ReadCloser, error) {
	resp, err := doGet(ctx, n.agentURL+"/agent/files/download?path="+urlEncode(path))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (n *HTTPRemoteNode) WriteFile(ctx context.Context, path string, r io.Reader) error {
	return doUpload(ctx, n.agentURL+"/agent/files/upload?path="+urlEncode(path), r)
}

func (n *HTTPRemoteNode) DeleteFile(ctx context.Context, path string) error {
	return doDelete(ctx, n.agentURL+"/agent/files/delete?path="+urlEncode(path))
}

func (n *HTTPRemoteNode) MkDir(ctx context.Context, path string) error {
	return doPost(ctx, n.agentURL+"/agent/mkdir?path="+urlEncode(path), nil)
}
