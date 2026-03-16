// Package agent реализует gRPC агент синхронизации.
// Запускается на каждой машине (ноутбук, сервер и т.д.).
// Регистрируется на middleware сервере и принимает команды синхронизации.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	internalsync "syncvault/internal/sync"
)

// Agent — gRPC агент на машине пользователя.
// Предоставляет локальную файловую систему как Node для SyncEngine.
type Agent struct {
	nodeID     string
	rootPath   string
	serverAddr string // адрес middleware сервера
	grpcPort   string // порт на котором агент слушает входящие соединения
	node       *internalsync.SimpleStorage
}

func NewAgent(nodeID, rootPath, serverAddr, grpcPort string) *Agent {
	return &Agent{
		nodeID:     nodeID,
		rootPath:   rootPath,
		serverAddr: serverAddr,
		grpcPort:   grpcPort,
		node:       internalsync.NewSimpleStorage(nodeID, rootPath),
	}
}

// Start запускает агент: регистрируется на сервере и слушает команды
func (a *Agent) Start(ctx context.Context) error {
	// Регистрируемся на middleware сервере
	if err := a.register(ctx); err != nil {
		return fmt.Errorf("failed to register with server: %w", err)
	}

	// Запускаем heartbeat
	go a.heartbeatLoop(ctx)

	log.Printf("Agent %s started, root=%s, server=%s", a.nodeID, a.rootPath, a.serverAddr)
	return nil
}

// register отправляет информацию об узле на middleware сервер
func (a *Agent) register(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"id":        a.nodeID,
		"root_path": a.rootPath,
		"endpoint":  a.grpcPort,
		"type":      "simple",
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		"http://"+a.serverAddr+"/sync/nodes/local",
		bytesReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register failed: %s", string(b))
	}

	log.Printf("Agent %s registered on server %s", a.nodeID, a.serverAddr)
	return nil
}

// heartbeatLoop периодически сообщает серверу что агент онлайн
func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			body, _ := json.Marshal(map[string]string{"node_id": a.nodeID})
			req, err := http.NewRequestWithContext(ctx, "POST",
				"http://"+a.serverAddr+"/sync/nodes/heartbeat",
				bytesReader(body))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("Agent %s heartbeat failed: %v", a.nodeID, err)
				continue
			}
			resp.Body.Close()
		}
	}
}

// ListenGRPC запускает gRPC сервер для приёма файлов от других узлов
func (a *Agent) ListenGRPC(ctx context.Context) error {
	lis, err := net.Listen("tcp", ":"+a.grpcPort)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", a.grpcPort, err)
	}

	srv := grpc.NewServer()
	RegisterFileTransferServer(srv, NewFileTransferService(a.node))

	log.Printf("Agent %s gRPC listening on :%s", a.nodeID, a.grpcPort)

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()

	return srv.Serve(lis)
}

// RemoteNode — реализует sync.Node для удалённой машины через gRPC
type RemoteNode struct {
	id       string
	endpoint string // host:port агента
	client   FileTransferClient
}

func NewRemoteNode(id, endpoint string) (*RemoteNode, error) {
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent %s: %w", endpoint, err)
	}

	return &RemoteNode{
		id:       id,
		endpoint: endpoint,
		client:   NewFileTransferClient(conn),
	}, nil
}

func (n *RemoteNode) ID() string   { return n.id }
func (n *RemoteNode) Type() string { return "remote_simple" }

func (n *RemoteNode) ListFiles(ctx context.Context) ([]internalsync.FileEntry, error) {
	resp, err := n.client.ListFiles(ctx, &ListFilesRequest{})
	if err != nil {
		return nil, fmt.Errorf("remote list files: %w", err)
	}

	entries := make([]internalsync.FileEntry, 0, len(resp.Files))
	for _, f := range resp.Files {
		entries = append(entries, internalsync.FileEntry{
			Path: f.Path,
			Hash: f.Hash,
			Size: f.Size,
		})
	}
	return entries, nil
}

func (n *RemoteNode) ReadFile(ctx context.Context, path string) (io.ReadCloser, error) {
	stream, err := n.client.DownloadFile(ctx, &DownloadRequest{Path: path})
	if err != nil {
		return nil, fmt.Errorf("remote read file: %w", err)
	}

	// Собираем чанки во временный файл
	tmp, err := os.CreateTemp("", "syncvault-remote-*")
	if err != nil {
		return nil, err
	}

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return nil, err
		}
		if _, err := tmp.Write(chunk.Data); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return nil, err
		}
	}

	if _, err := tmp.Seek(0, 0); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, err
	}

	return &autoRemoveTmp{File: tmp}, nil
}

func (n *RemoteNode) WriteFile(ctx context.Context, path string, r io.Reader) error {
	stream, err := n.client.UploadFile(ctx)
	if err != nil {
		return fmt.Errorf("remote write file: %w", err)
	}

	// Отправляем заголовок
	if err := stream.Send(&UploadRequest{
		Payload: &UploadRequest_Header{
			Header: &FileHeader{Path: path},
		},
	}); err != nil {
		return err
	}

	// Отправляем данные чанками по 1MB
	buf := make([]byte, 1024*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if err := stream.Send(&UploadRequest{
				Payload: &UploadRequest_Chunk{
					Chunk: &FileChunk{Data: buf[:n]},
				},
			}); err != nil {
				return err
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	_, err = stream.CloseAndRecv()
	return err
}

func (n *RemoteNode) DeleteFile(ctx context.Context, path string) error {
	_, err := n.client.DeleteFile(ctx, &DeleteRequest{Path: path})
	return err
}

func (n *RemoteNode) MkDir(ctx context.Context, path string) error {
	_, err := n.client.MkDir(ctx, &MkDirRequest{Path: path})
	return err
}

// autoRemoveTmp удаляет временный файл после закрытия
type autoRemoveTmp struct{ *os.File }

func (f *autoRemoveTmp) Close() error {
	name := f.File.Name()
	err := f.File.Close()
	os.Remove(name)
	return err
}

func bytesReader(b []byte) io.Reader {
	return &bytesReaderImpl{data: b, pos: 0}
}

type bytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *bytesReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
