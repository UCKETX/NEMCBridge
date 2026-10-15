//go:build windows

package secret

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"nemcbridge/internal/fileutil"
)

type dataBlob struct {
	Size uint32
	Data *byte
}

var cryptDLL = syscall.NewLazyDLL("crypt32.dll")
var protectProc = cryptDLL.NewProc("CryptProtectData")
var unprotectProc = cryptDLL.NewProc("CryptUnprotectData")
var localFree = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")

func StorageDescription() string { return "使用当前 Windows 用户的 DPAPI 密钥加密" }

func cryptData(input []byte, protect bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("令牌为空")
	}
	in := dataBlob{uint32(len(input)), &input[0]}
	out := dataBlob{}
	proc := unprotectProc
	if protect {
		proc = protectProc
	}
	r, _, err := proc.Call(uintptr(unsafe.Pointer(&in)), 0, 0, 0, 0, 1, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, fmt.Errorf("Windows 令牌加密/解密失败: %v", err)
	}
	defer localFree.Call(uintptr(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func Save(path, token string) error {
	b, err := cryptData([]byte(token), true)
	if err != nil {
		return err
	}
	return fileutil.AtomicWrite(path, b, 0600)
}
func Load(path string) (string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	plain, err := cryptData(b, false)
	return string(plain), err
}
