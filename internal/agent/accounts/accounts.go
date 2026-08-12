// Package accounts 管理节点内部账户（relay 入站 UUID）的本地持久化。
//
// 权威源约定（05 号文档 §4）：节点本地文件 `internal_accounts.json` 是唯一权威，
// 主控 DB 以回执为准覆盖。文件权限 600，写入用 临时文件 + rename 原子替换。
package accounts

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store 内部账户存储（{tag: uuid}）。
type Store struct {
	path string
	mu   sync.Mutex
}

// New 创建存储（路径由 agent 配置注入，默认 /etc/xray-agent/internal_accounts.json）。
func New(path string) *Store {
	return &Store{path: path}
}

// Path 返回存储文件路径。
func (s *Store) Path() string { return s.path }

// Load 读取全部账户（文件不存在返回空 map；损坏文件返回错误，不静默覆盖）。
func (s *Store) Load() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() (map[string]string, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取内部账户文件失败: %w", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("内部账户文件损坏: %w", err)
	}
	return m, nil
}

// Set 写入（或覆盖）tag 的 UUID，原子落盘（临时文件 + rename，权限 600）。
func (s *Store) Set(tag, uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return err
	}
	m[tag] = uuid
	return s.saveLocked(m)
}

// Remove 删除 tag（文件已存在时幂等；删除后仍落盘）。
func (s *Store) Remove(tag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.loadLocked()
	if err != nil {
		return err
	}
	if _, ok := m[tag]; !ok {
		return nil
	}
	delete(m, tag)
	return s.saveLocked(m)
}

// saveLocked 原子写：同目录临时文件写入 + rename 替换（跨崩溃安全）。
func (s *Store) saveLocked(m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("原子替换失败: %w", err)
	}
	return nil
}

// NewUUID 生成 UUID v4（crypto/rand，不依赖 xray 二进制）。
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("随机数失败: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
