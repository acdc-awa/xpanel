// Package backup 提供主控 SQLite 数据库的定时/手动备份（VACUUM INTO 一致性快照 + 保留轮转）。
package backup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	_ "github.com/glebarez/sqlite" // 注册 sqlite driver
	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/master/services"
)

// tsRe 备份文件名格式：panel-20060102-150405.db。
var tsRe = regexp.MustCompile(`^panel-\d{8}-\d{6}\.db$`)

// BackupInfo 备份文件元信息。
type BackupInfo struct {
	File      string    `json:"file"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Service 备份服务：快照 / 列表 / 轮转（调度器见 scheduler.go）。
type Service struct {
	dsn   string
	dir   string
	keep  int
	audit *services.AuditService

	mu  sync.Mutex       // 串行化快照与轮转
	now func() time.Time // 可注入（测试）
}

// New 创建备份服务；备份目录不存在则创建。
func New(dsn string, cfg config.Backup, audit *services.AuditService) (*Service, error) {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建备份目录失败: %w", err)
	}
	return &Service{
		dsn:   dsn,
		dir:   cfg.Dir,
		keep:  cfg.Keep,
		audit: audit,
		now:   time.Now,
	}, nil
}

// Snapshot 立即执行一次一致性备份，返回新备份信息。
func (s *Service) Snapshot() (BackupInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

func (s *Service) snapshotLocked() (BackupInfo, error) {
	ts := s.now()
	name := ts.Format("panel-20060102-150405.db")
	dst := filepath.Join(s.dir, name)

	// 独立连接执行 VACUUM INTO（在线一致性快照，WAL 数据也包含）
	src, err := sql.Open("sqlite", s.dsn)
	if err != nil {
		s.log("backup.create", "failed", fmt.Sprintf("打开源库失败: %v", err))
		return BackupInfo{}, err
	}
	defer src.Close()
	if _, err := src.Exec(fmt.Sprintf(`VACUUM INTO %q`, dst)); err != nil {
		s.log("backup.create", "failed", fmt.Sprintf("VACUUM INTO 失败: %v", err))
		return BackupInfo{}, err
	}

	// 自检：重新打开快照跑 integrity_check
	if err := verify(dst); err != nil {
		_ = os.Remove(dst)
		s.log("backup.create", "failed", fmt.Sprintf("快照自检失败: %v", err))
		return BackupInfo{}, err
	}

	info := BackupInfo{File: name, CreatedAt: ts}
	if st, err := os.Stat(dst); err == nil {
		info.Size = st.Size()
	}
	s.log("backup.create", "ok", name)
	if err := s.rotateLocked(); err != nil {
		s.log("backup.rotate", "failed", err.Error())
	}
	return info, nil
}

// verify 打开备份文件执行 integrity_check。
func verify(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var res string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("integrity_check = %q", res)
	}
	return nil
}

// List 按时间倒序返回全部备份。
func (s *Service) List() ([]BackupInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listLocked()
}

// listLocked 无锁实现（调用方必须已持有 s.mu）。
func (s *Service) listLocked() ([]BackupInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var items []BackupInfo
	for _, e := range entries {
		if e.IsDir() || !tsRe.MatchString(e.Name()) {
			continue
		}
		info := BackupInfo{File: e.Name()}
		if st, err := e.Info(); err == nil {
			info.Size = st.Size()
			info.CreatedAt = st.ModTime()
		}
		items = append(items, info)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

// rotateLocked 删除超出 keep 的最旧备份。
func (s *Service) rotateLocked() error {
	items, err := s.listLocked()
	if err != nil {
		return err
	}
	if s.keep <= 0 || len(items) <= s.keep {
		return nil
	}
	for _, it := range items[s.keep:] {
		if err := os.Remove(filepath.Join(s.dir, it.File)); err != nil {
			return err
		}
	}
	return nil
}

// OpenFile 校验文件名并返回备份绝对路径（供下载端点；防路径穿越）。
func (s *Service) OpenFile(name string) (string, error) {
	if !tsRe.MatchString(name) {
		return "", fmt.Errorf("非法备份文件名: %q", name)
	}
	p := filepath.Join(s.dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("备份不存在: %s", name)
	}
	return p, nil
}

// log 写审计日志（audit 可为 nil）。
func (s *Service) log(action, result, detail string) {
	if s.audit == nil {
		return
	}
	s.audit.Log("system", 0, action, fmt.Sprintf("%s %s", result, detail), "")
}
