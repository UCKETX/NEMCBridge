//go:build !windows

package main

import "os"

func secretStorageDescription() string { return "以仅当前用户可读写的 0600 权限保存" }

func saveSecret(path, token string) error { return atomicWrite(path, []byte(token), 0600) }
func loadSecret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(b), err
}
