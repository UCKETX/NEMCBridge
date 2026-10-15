package main

import (
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
)

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
