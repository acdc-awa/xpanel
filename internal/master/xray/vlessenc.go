package xray

import (
	"github.com/acdc-awa/xpanel/internal/master/protocols"
)

// GenerateVlessEnc 代理调用 protocols.GenerateVlessEnc，保持对外部 API 的签名兼容。
func GenerateVlessEnc() (decryption, encryption string, err error) {
	return protocols.GenerateVlessEnc()
}

// VlessDecryptionFromSettings 代理调用 protocols.VlessDecryptionFromSettings。
func VlessDecryptionFromSettings(settingsJSON string) (string, error) {
	return protocols.VlessDecryptionFromSettings(settingsJSON)
}

// VlessEncEnabled 代理调用 protocols.VlessEncEnabled。
func VlessEncEnabled(decryption string) bool {
	return protocols.VlessEncEnabled(decryption)
}

// ValidateVlessEncDecryption 代理调用 protocols.ValidateVlessEncDecryption。
func ValidateVlessEncDecryption(decryption string) error {
	return protocols.ValidateVlessEncDecryption(decryption)
}

// VlessEncClientFromDecryption 代理调用 protocols.VlessEncClientFromDecryption。
func VlessEncClientFromDecryption(decryption string) (string, error) {
	return protocols.VlessEncClientFromDecryption(decryption)
}
