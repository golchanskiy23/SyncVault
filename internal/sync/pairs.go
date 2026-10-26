package sync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LocalDevice — ID встроенного устройства: компьютер, на котором запущен сервер
const LocalDevice = "local"

var ErrPairNotFound = errors.New("pair not found")

// PairSide — одна сторона связки: папка на устройстве или папка на Google Drive
type PairSide struct {
	Device   string `json:"device,omitempty"`    // "local" или ID агента
	Path     string `json:"path,omitempty"`      // папка на устройстве
	Drive    string `json:"drive,omitempty"`     // email Google аккаунта
	FolderID string `json:"folder_id,omitempty"` // папка на Drive ("" — корень)
	UserID   string `json:"user_id,omitempty"`   // владелец токена Drive (из JWT)
}

func (s PairSide) String() string {
	if s.Drive != "" {
		folder := s.FolderID
		if folder == "" {
			folder = "root"
		}
		return "drive:" + s.Drive + "/" + folder
	}
	return s.Device + ":" + s.Path
}

// Pair — связка двух папок, синхронизируемых между собой
type Pair struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	A               PairSide   `json:"a"`
	B               PairSide   `json:"b"`
	CreatedAt       time.Time  `json:"created_at"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	LastStatus      string     `json:"last_status"` // pending | ok | error
	LastError       string     `json:"last_error"`
	LastTransferred int        `json:"last_transferred"`
	LastDeleted     int        `json:"last_deleted"`
	LastConflicts   int        `json:"last_conflicts"`
}

func (p Pair) label() string {
	if p.Name != "" {
		return fmt.Sprintf("#%d %s", p.ID, p.Name)
	}
	return fmt.Sprintf("#%d %s ↔ %s", p.ID, p.A, p.B)
}

// PairStore хранит связки и результат их последней синхронизации
type PairStore interface {
	CreatePair(ctx context.Context, p *Pair) error
	ListPairs(ctx context.Context) ([]Pair, error)
	GetPair(ctx context.Context, id int64) (*Pair, error) // ErrPairNotFound
	DeletePair(ctx context.Context, id int64) error
	SavePairResult(ctx context.Context, id int64, at time.Time, status, errMsg string, res *SyncResult) error
}

// PairManager создаёт, проверяет и синхронизирует связки
type PairManager struct {
	engine     *SyncEngine
	devices    *NodeRegistry // агенты на других устройствах
	pairs      PairStore
	drive      DriveNodeFactory // nil — Google Drive не настроен
	localRoot  string           // корень папок этого компьютера; "" — устройство local выключено
	agentToken string
}

func NewPairManager(engine *SyncEngine, devices *NodeRegistry, pairs PairStore, localRoot, agentToken string) *PairManager {
	return &PairManager{engine: engine, devices: devices, pairs: pairs, localRoot: localRoot, agentToken: agentToken}
}

func (m *PairManager) SetDriveFactory(f DriveNodeFactory) { m.drive = f }

func (m *PairManager) LocalRoot() string { return m.localRoot }

func sideNodeID(pairID int64, key string) string {
	return fmt.Sprintf("pair%d:%s", pairID, key)
}

// node создаёт Node для стороны связки
func (m *PairManager) node(pairID int64, key string, side PairSide) (Node, error) {
	id := sideNodeID(pairID, key)
	switch {
	case side.Drive != "":
		if m.drive == nil {
			return nil, fmt.Errorf("google drive is not configured")
		}
		return m.drive.NewDriveNode(driveInfo(id, side))
	case side.Device == LocalDevice:
		dir, err := m.localDir(side.Path)
		if err != nil {
			return nil, err
		}
		return NewSimpleStorage(id, dir), nil
	default:
		info, ok := m.devices.Info(side.Device)
		if !ok {
			return nil, fmt.Errorf("device %s is not registered (is its agent running?)", side.Device)
		}
		return NewHTTPAgentFolder(id, info.Endpoint, m.agentToken, side.Path), nil
	}
}

func driveInfo(id string, side PairSide) NodeInfo {
	return NodeInfo{ID: id, Type: "google_drive", AccountID: side.Drive, UserID: side.UserID, FolderID: side.FolderID}
}

func (m *PairManager) localDir(path string) (string, error) {
	if m.localRoot == "" {
		return "", fmt.Errorf("local device is disabled (sync.localRoot is not set)")
	}
	return ResolveInside(m.localRoot, path)
}

// ListDirs возвращает подпапки папки path на устройстве
func (m *PairManager) ListDirs(ctx context.Context, device, path string) ([]string, error) {
	if device == LocalDevice {
		if m.localRoot == "" {
			return nil, fmt.Errorf("local device is disabled (sync.localRoot is not set)")
		}
		return ListSubdirs(m.localRoot, path)
	}
	info, ok := m.devices.Info(device)
	if !ok {
		return nil, fmt.Errorf("device %s: %w", device, os.ErrNotExist)
	}
	return NewHTTPAgentNode(device, info.Endpoint, m.agentToken).ListDirs(ctx, path)
}

// validateSide проверяет сторону связки и приводит путь к каноническому виду
func (m *PairManager) validateSide(ctx context.Context, side *PairSide) error {
	switch {
	case side.Drive != "" && side.Device != "":
		return fmt.Errorf("side must be either a device folder or a google drive folder, not both")
	case side.Drive != "":
		if m.drive == nil {
			return fmt.Errorf("google drive is not configured")
		}
		if side.UserID == "" {
			return fmt.Errorf("google drive side requires an authenticated user")
		}
		return m.drive.CheckDriveNode(ctx, driveInfo("", *side))
	case side.Device == "":
		return fmt.Errorf("side requires device (and path) or drive")
	case side.Device == LocalDevice:
		dir, err := m.localDir(side.Path)
		if err != nil {
			return err
		}
		fi, err := os.Stat(dir)
		if err != nil || !fi.IsDir() {
			return fmt.Errorf("folder %s does not exist on this computer (only folders inside %s are available)", dir, m.localRoot)
		}
		side.Path = dir
		return nil
	default:
		info, ok := m.devices.Info(side.Device)
		if !ok {
			return fmt.Errorf("device %s is not registered (is its agent running?)", side.Device)
		}
		if side.Path != "" && filepath.IsAbs(side.Path) && info.Root != "" {
			rel, err := filepath.Rel(info.Root, side.Path)
			if err != nil || !filepath.IsLocal(rel) {
				return fmt.Errorf("folder %s is outside of %s on device %s", side.Path, info.Root, side.Device)
			}
			side.Path = filepath.ToSlash(rel)
		}
		if side.Path != "" {
			side.Path = filepath.ToSlash(filepath.Clean(side.Path))
		}
		vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if _, err := NewHTTPAgentNode(side.Device, info.Endpoint, m.agentToken).ListDirs(vctx, side.Path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("folder %q does not exist on device %s", side.Path, side.Device)
			}
			return fmt.Errorf("device %s is not reachable: %w", side.Device, err)
		}
		return nil
	}
}

// overlaps сообщает, что стороны указывают на одну папку или одна вложена в другую
func overlaps(a, b PairSide) bool {
	if a.Drive != "" || b.Drive != "" {
		return a.Drive != "" && a.Drive == b.Drive && a.FolderID == b.FolderID
	}
	if a.Device != b.Device {
		return false
	}
	pa, pb := strings.Trim(a.Path, "/"), strings.Trim(b.Path, "/")
	return pa == pb || pa == "" || pb == "" || strings.HasPrefix(pa+"/", pb+"/") || strings.HasPrefix(pb+"/", pa+"/")
}

// Create проверяет стороны и сохраняет связку
func (m *PairManager) Create(ctx context.Context, p *Pair) error {
	if err := m.validateSide(ctx, &p.A); err != nil {
		return fmt.Errorf("side a: %w", err)
	}
	if err := m.validateSide(ctx, &p.B); err != nil {
		return fmt.Errorf("side b: %w", err)
	}
	if overlaps(p.A, p.B) {
		return fmt.Errorf("sides point to the same folder or one is inside the other")
	}
	p.LastStatus = "pending"
	return m.pairs.CreatePair(ctx, p)
}

// Delete удаляет связку и её состояние синхронизации
func (m *PairManager) Delete(ctx context.Context, id int64) error {
	if err := m.pairs.DeletePair(ctx, id); err != nil {
		return err
	}
	m.engine.ForgetNode(sideNodeID(id, "a"))
	m.engine.ForgetNode(sideNodeID(id, "b"))
	return nil
}

func (m *PairManager) List(ctx context.Context) ([]Pair, error) { return m.pairs.ListPairs(ctx) }

func (m *PairManager) Get(ctx context.Context, id int64) (*Pair, error) { return m.pairs.GetPair(ctx, id) }

// PairsUsingDevice возвращает ID связок, в которых участвует устройство
func (m *PairManager) PairsUsingDevice(ctx context.Context, device string) ([]int64, error) {
	pairs, err := m.pairs.ListPairs(ctx)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, p := range pairs {
		if p.A.Device == device || p.B.Device == device {
			ids = append(ids, p.ID)
		}
	}
	return ids, nil
}

// Run синхронизирует связку и сохраняет результат
func (m *PairManager) Run(ctx context.Context, p Pair) (*SyncResult, error) {
	res, err := m.run(ctx, p)

	status, msg := "ok", ""
	switch {
	case err != nil:
		status, msg = "error", err.Error()
	case len(res.Errors) > 0:
		status = "error"
		msg = res.Errors[0].Error()
		if len(res.Errors) > 1 {
			msg += fmt.Sprintf(" (and %d more)", len(res.Errors)-1)
		}
	}
	if serr := m.pairs.SavePairResult(ctx, p.ID, time.Now(), status, msg, res); serr != nil {
		log.Printf("Pair %s: failed to save result: %v", p.label(), serr)
	}

	if err != nil {
		log.Printf("Pair %s: sync failed: %v", p.label(), err)
	} else {
		for _, e := range res.Errors {
			log.Printf("Pair %s: %v", p.label(), e)
		}
		log.Printf("Pair %s: transferred=%d deleted=%d conflicts=%d errors=%d",
			p.label(), res.Transferred, res.Deleted, len(res.Conflicts), len(res.Errors))
	}
	return res, err
}

func (m *PairManager) run(ctx context.Context, p Pair) (*SyncResult, error) {
	a, err := m.node(p.ID, "a", p.A)
	if err != nil {
		return nil, fmt.Errorf("side a: %w", err)
	}
	b, err := m.node(p.ID, "b", p.B)
	if err != nil {
		return nil, fmt.Errorf("side b: %w", err)
	}
	return m.engine.Sync(ctx, a, b)
}

// RunAll синхронизирует все связки по очереди
func (m *PairManager) RunAll(ctx context.Context) {
	pairs, err := m.pairs.ListPairs(ctx)
	if err != nil {
		log.Printf("Sync: failed to list pairs: %v", err)
		return
	}
	if len(pairs) == 0 {
		log.Printf("Sync: no pairs configured")
		return
	}

	start := time.Now()
	failed := 0
	for _, p := range pairs {
		if ctx.Err() != nil {
			return
		}
		if res, err := m.Run(ctx, p); err != nil || len(res.Errors) > 0 {
			failed++
		}
	}
	log.Printf("Sync: %d pairs done in %s, %d with errors", len(pairs), time.Since(start).Round(time.Second), failed)
}
