package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

func terminalColorEnabled() bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return false
	}
	return enableTerminalColor()
}

func printStartupBanner(w io.Writer, color bool) {
	font := map[rune][]string{
		'N': {"10001", "11001", "10101", "10011", "10001"},
		'E': {"11111", "10000", "11110", "10000", "11111"},
		'M': {"10001", "11011", "10101", "10001", "10001"},
		'C': {"01111", "10000", "10000", "10000", "01111"},
		'B': {"11110", "10001", "11110", "10001", "11110"},
		'R': {"11110", "10001", "11110", "10100", "10010"},
		'I': {"11111", "00100", "00100", "00100", "11111"},
		'D': {"11110", "10001", "10001", "10001", "11110"},
		'G': {"01111", "10000", "10111", "10001", "01111"},
	}
	const title = "NEMCBRIDGE"
	paint := func(text, code string) string {
		if color {
			return "\x1b[" + code + "m" + text + "\x1b[0m"
		}
		return text
	}
	width := len(title)*6 - 1 + 4
	fmt.Fprintln(w)
	fmt.Fprintln(w, paint("╔"+strings.Repeat("═", width)+"╗", "38;2;53;211;220"))
	for row := 0; row < 5; row++ {
		fmt.Fprint(w, paint("║  ", "38;2;53;211;220"))
		for index, letter := range title {
			pixels := strings.NewReplacer("1", "█", "0", " ").Replace(font[letter][row])
			code := fmt.Sprintf("38;2;%d;%d;%d", 104-index*6, 155+index*8, 214)
			fmt.Fprint(w, paint(pixels, code))
			if index < len(title)-1 {
				fmt.Fprint(w, " ")
			}
		}
		fmt.Fprintln(w, paint("  ║", "38;2;53;211;220"))
	}
	fmt.Fprintln(w, paint("╚"+strings.Repeat("═", width)+"╝", "38;2;53;211;220"))
	shortCommit := commit
	if len(shortCommit) > 7 {
		shortCommit = shortCommit[:7]
	}
	fmt.Fprintf(w, "  %s %s  ·  %s  ·  %s/%s\n", paint("NEMCBridge", "1;36"), version, shortCommit, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintln(w, "  网易版 ↔ 国际版 Bedrock · 在同一个世界相遇")
	fmt.Fprintln(w)
}

// Include every active IPv4 interface so LAN and VPN addresses can both be used.
func consoleIPv4Addresses() []net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var addresses []net.IP
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() || !ip.IsGlobalUnicast() {
				continue
			}
			if !seen[ip.String()] {
				seen[ip.String()] = true
				addresses = append(addresses, ip.To4())
			}
		}
	}
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].String() < addresses[j].String() })
	return addresses
}

// Retain Host validation for DNS rebinding protection while allowing LAN access.
func consoleHosts(port int, addresses []net.IP) map[string]bool {
	hosts := map[string]bool{}
	for _, host := range []string{"127.0.0.1", "localhost", "0.0.0.0"} {
		hosts[net.JoinHostPort(host, strconv.Itoa(port))] = true
	}
	for _, ip := range addresses {
		hosts[net.JoinHostPort(ip.String(), strconv.Itoa(port))] = true
	}
	return hosts
}

func printConsoleURLs(w io.Writer, port int, addresses []net.IP) {
	fmt.Fprintf(w, "NEMCBridge 控制台监听：0.0.0.0:%d\n", port)
	fmt.Fprintf(w, "本机访问：http://127.0.0.1:%d/\n", port)
	for _, ip := range addresses {
		fmt.Fprintf(w, "局域网访问：http://%s/\n", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	}
	if len(addresses) == 0 {
		fmt.Fprintln(w, "未发现可用的局域网 IPv4 地址，请检查网络连接。")
	}
}
