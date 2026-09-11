// 备份上传与数据库恢复。
//
// 恢复采取「暂存 + 启动时替换」而非热替换：SQLite 文件被运行中的进程持有（含 -wal/-shm），
// 直接在运行期改名会让连接指向被替换的 inode。流程：
//  1. ScheduleRestore 校验目标备份 → 对当前库做一次安全快照 → 复制到 <dsn>.restore 并落 <dsn>.restore-pending；
//  2. 进程收到 SIGTERM 优雅退出，由容器 restart 策略拉起；
//  3. 新进程在 db.Open 之前调用 ApplyPendingRestore，校验暂存文件后原子替换并清除标记。
//
// 任何一步校验失败都保留当前数据库并清理暂存/标记（失败安全），绝不把坏文件顶上生产库。
package backup

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/glebarez/sqlite" // 注册 sqlite driver（校验备份文件用）

	"github.com/acdc-awa/xpanel/internal/models"
	pkgdb "github.com/acdc-awa/xpanel/internal/pkg/db"
)

// MaxUploadBytes 上传备份文件体积上限（1 GiB）。面板库通常几十 MB，留足余量。
const MaxUploadBytes int64 = 1 << 30

const (
	restoreMarkerSuffix  = ".restore-pending"
	restoreStagingSuffix = ".restore"
)

// requiredTables 判定「是不是一块面板数据库」的最小表集合。
var requiredTables = []string{"users", "settings", "servers"}

func restoreMarkerPath(dsn string) string  { return dsn + restoreMarkerSuffix }
func restoreStagingPath(dsn string) string { return dsn + restoreStagingSuffix }

// checkIntegrity 对已打开的连接执行 integrity_check。
func checkIntegrity(db *sql.DB) error {
	var res string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("integrity_check = %q", res)
	}
	return nil
}

// validatePanelDB 校验一个 .db 文件是否为完整、可被当前面板读取的面板数据库：
// integrity_check + 必需表存在 + 记录的最低兼容 schema 版本不高于当前面板。
func validatePanelDB(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := checkIntegrity(db); err != nil {
		return fmt.Errorf("数据库文件损坏: %w", err)
	}
	for _, t := range requiredTables {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, t).Scan(&n); err != nil {
			return fmt.Errorf("检查表 %s 失败: %w", t, err)
		}
		if n == 0 {
			return fmt.Errorf("不是有效的面板数据库（缺少表 %s）", t)
		}
	}
	// 备份由更新版本面板写过且做了不兼容迁移时拒绝，避免把新库顶上旧面板。
	var minRaw sql.NullString
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='schema_min_compatible' LIMIT 1`).Scan(&minRaw); err == nil && minRaw.Valid {
		if min, perr := strconv.Atoi(strings.TrimSpace(minRaw.String)); perr == nil && min > models.DBSchemaVersion {
			return fmt.Errorf("备份来自更新版本的面板（最低兼容 schema v%d > 当前 v%d），请先升级面板后再恢复", min, models.DBSchemaVersion)
		}
	}
	return nil
}

// Upload 把上传的 .db 文件校验后落入备份目录，成为一份可下载/可恢复的普通备份。
func (s *Service) Upload(r io.Reader, size int64) (BackupInfo, error) {
	if size > MaxUploadBytes {
		return BackupInfo{}, fmt.Errorf("文件过大（上限 %d MB）", MaxUploadBytes/1024/1024)
	}
	if !pkgdb.SupportsOnlineSnapshot(s.driver) {
		return BackupInfo{}, fmt.Errorf("当前数据库驱动 %q 不支持文件级备份（仅 sqlite）", s.driver)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tmp, err := os.CreateTemp(s.dir, ".upload-*.tmp")
	if err != nil {
		return BackupInfo{}, err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // 成功改名后为空操作

	n, err := io.Copy(tmp, io.LimitReader(r, MaxUploadBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return BackupInfo{}, err
	}
	if n > MaxUploadBytes {
		return BackupInfo{}, fmt.Errorf("文件过大（上限 %d MB）", MaxUploadBytes/1024/1024)
	}
	if err := validatePanelDB(tmpPath); err != nil {
		return BackupInfo{}, err
	}

	// 时间戳命名与自动备份一致；同秒冲突则顺延，保证 List/下载/恢复都能识别。
	ts := s.now()
	var dst string
	for i := 0; i < 120; i++ {
		cand := ts.Add(time.Duration(i) * time.Second)
		p := filepath.Join(s.dir, cand.Format("panel-20060102-150405.db"))
		if _, statErr := os.Stat(p); os.IsNotExist(statErr) {
			dst, ts = p, cand
			break
		}
	}
	if dst == "" {
		return BackupInfo{}, fmt.Errorf("无法分配备份文件名")
	}
	if err := replaceFile(tmpPath, dst); err != nil {
		return BackupInfo{}, err
	}
	info := BackupInfo{File: filepath.Base(dst), CreatedAt: ts}
	if st, err := os.Stat(dst); err == nil {
		info.Size = st.Size()
	}
	s.log("backup.upload", "ok", info.File)
	if err := s.rotateLocked(); err != nil {
		s.log("backup.rotate", "failed", err.Error())
	}
	return info, nil
}

// ScheduleRestore 安排一次数据库恢复：校验目标备份 → 当前库安全快照 → 复制到暂存路径并落标记。
// 返回的安全快照信息供调用方提示用户（恢复后若发现问题可用它回退）。
func (s *Service) ScheduleRestore(name string) (BackupInfo, error) {
	if !pkgdb.SupportsOnlineSnapshot(s.driver) {
		return BackupInfo{}, fmt.Errorf("当前数据库驱动 %q 不支持数据库恢复（仅 sqlite）", s.driver)
	}
	src, err := s.OpenFile(name)
	if err != nil {
		return BackupInfo{}, err
	}
	if err := validatePanelDB(src); err != nil {
		return BackupInfo{}, err
	}
	safety, err := s.Snapshot()
	if err != nil {
		return BackupInfo{}, fmt.Errorf("恢复前安全备份失败: %w", err)
	}
	if err := copyFile(src, restoreStagingPath(s.dsn)); err != nil {
		return BackupInfo{}, fmt.Errorf("暂存待恢复文件失败: %w", err)
	}
	if err := os.WriteFile(restoreMarkerPath(s.dsn), []byte(name), 0o644); err != nil {
		_ = os.Remove(restoreStagingPath(s.dsn))
		return BackupInfo{}, fmt.Errorf("写入恢复标记失败: %w", err)
	}
	s.log("backup.restore", "scheduled", fmt.Sprintf("%s (safety=%s)", name, safety.File))
	return safety, nil
}

// ApplyPendingRestore 在打开数据库之前应用一次待处理恢复（main 在 db.Open 前调用）。
// 返回 true 表示数据库文件已被替换。校验失败时保留原库并清理标记，返回错误但进程可继续启动。
func ApplyPendingRestore(dsn string) (bool, error) {
	if _, err := os.ReadFile(restoreMarkerPath(dsn)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	staging := restoreStagingPath(dsn)
	if err := validatePanelDB(staging); err != nil {
		_ = os.Remove(staging)
		_ = os.Remove(restoreMarkerPath(dsn))
		return false, fmt.Errorf("待恢复文件校验失败，已保留当前数据库: %w", err)
	}
	// 旧库的 WAL/SHM 与替换后的文件不匹配，必须先清理。
	_ = os.Remove(dsn + "-wal")
	_ = os.Remove(dsn + "-shm")
	if err := replaceFile(staging, dsn); err != nil {
		_ = os.Remove(staging)
		_ = os.Remove(restoreMarkerPath(dsn))
		return false, err
	}
	_ = os.Remove(restoreMarkerPath(dsn))
	return true, nil
}

// replaceFile 用 src 覆盖 dst：Unix 下 rename 直接覆盖；Windows 下 rename 不能覆盖已存在文件，
// 退化为先删后改名（恢复路径在启动前调用，无并发读者，可接受非原子）。
func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}

// copyFile 复制文件：先写临时文件再改名，避免留下半成品。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return replaceFile(tmp, dst)
}
