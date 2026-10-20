package contracts

import "fmt"

// CACertDir 节点上 CA 根证书的落盘目录（与 agent 的 certs_dir 默认值一致）。
const CACertDir = "/etc/xray/certs/ca"

// CACertPath 托管 CA 在节点上的固定路径：
// 主控生成出站 tlsSettings.certificates[].certificateFile 时使用，
// agent 的 push_ca 按同一约定落盘到 <certs_dir>/ca/<id>.crt。
func CACertPath(id uint64) string {
	return fmt.Sprintf("%s/%d.crt", CACertDir, id)
}
