//go:build !windows

package secret

import (
	"os"

	"lobbykeeper/internal/fileutil"
)

func StorageDescription() string { return "以仅当前用户可读写的 0600 权限保存" }

func Save(path, token string) error { return fileutil.AtomicWrite(path, []byte(token), 0600) }
func Load(path string) (string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(b), err
}
