package sync

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// AgentTokenHeader — заголовок с общим секретом агента и сервера
const AgentTokenHeader = "X-Agent-Token"

// ValidAgentToken сравнивает токены за постоянное время; пустой ожидаемый токен не принимается
func ValidAgentToken(expected, got string) bool {
	return expected != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}

// agentHTTPClient ограничивает подключение и ожидание ответа агента,
// но не время передачи тела — большие файлы могут идти долго
var agentHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		ResponseHeaderTimeout: 60 * time.Second,
	},
}

// =============================================================================
// HTTPRemoteNode — удалённая машина, на которой запущен агент (cmd/agent).
// Реализует Node через HTTP API агента (/agent/*).
// =============================================================================

type HTTPRemoteNode struct {
	id       string
	agentURL string // http://host:port
	token    string // токен агента
	root     string // подпапка внутри базовой папки агента ("" — вся базовая папка)
}

func NewHTTPAgentNode(id, agentURL, token string) *HTTPRemoteNode {
	return &HTTPRemoteNode{id: id, agentURL: agentURL, token: token}
}

// NewHTTPAgentFolder — узел для подпапки root внутри базовой папки агента
func NewHTTPAgentFolder(id, agentURL, token, root string) *HTTPRemoteNode {
	return &HTTPRemoteNode{id: id, agentURL: agentURL, token: token, root: root}
}

// ListDirs возвращает подпапки папки rel (относительно базовой папки агента);
// ошибка с os.ErrNotExist — папки нет
func (n *HTTPRemoteNode) ListDirs(ctx context.Context, rel string) ([]string, error) {
	rawURL := n.agentURL + "/agent/dirs?path=" + url.QueryEscape(rel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(AgentTokenHeader, n.token)
	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s: %w", rel, os.ErrNotExist)
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("agent dirs: %s: %s", resp.Status, msg)
	}
	var result struct {
		Dirs []string `json:"dirs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Dirs, nil
}

func (n *HTTPRemoteNode) ID() string   { return n.id }
func (n *HTTPRemoteNode) Type() string { return "remote_simple" }

func (n *HTTPRemoteNode) ListFiles(ctx context.Context) ([]FileEntry, error) {
	resp, err := n.do(ctx, http.MethodGet, "/agent/files", "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Files []FileEntry `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Files, nil
}

func (n *HTTPRemoteNode) ReadFile(ctx context.Context, path string) (io.ReadCloser, error) {
	resp, err := n.do(ctx, http.MethodGet, "/agent/files/download", path, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (n *HTTPRemoteNode) WriteFile(ctx context.Context, path string, r io.Reader) error {
	return n.doClose(ctx, http.MethodPost, "/agent/files/upload", path, r)
}

func (n *HTTPRemoteNode) DeleteFile(ctx context.Context, path string) error {
	return n.doClose(ctx, http.MethodDelete, "/agent/files/delete", path, nil)
}

func (n *HTTPRemoteNode) MkDir(ctx context.Context, path string) error {
	return n.doClose(ctx, http.MethodPost, "/agent/mkdir", path, nil)
}

// do выполняет запрос к агенту и возвращает ошибку для любого не-2xx ответа
func (n *HTTPRemoteNode) do(ctx context.Context, method, endpoint, path string, body io.Reader) (*http.Response, error) {
	query := url.Values{}
	if path != "" {
		query.Set("path", path)
	}
	if n.root != "" {
		query.Set("root", n.root)
	}
	rawURL := n.agentURL + endpoint
	if len(query) > 0 {
		rawURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	req.Header.Set(AgentTokenHeader, n.token)
	resp, err := agentHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("agent %s %s: %s: %s", method, endpoint, resp.Status, msg)
	}
	return resp, nil
}

func (n *HTTPRemoteNode) doClose(ctx context.Context, method, endpoint, path string, body io.Reader) error {
	resp, err := n.do(ctx, method, endpoint, path, body)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
