package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// IPInfo 包含 IP 归属地与网络类型分析（美国家宽/机房判定）。
type IPInfo struct {
	IP            string `json:"ip"`
	CountryCode   string `json:"country_code"`
	Country       string `json:"country"`
	Region        string `json:"region"`
	City          string `json:"city"`
	ISP           string `json:"isp"`
	Org           string `json:"org"`
	AS            string `json:"as"`
	Hosting       bool   `json:"hosting"`
	Proxy         bool   `json:"proxy"`
	Mobile        bool   `json:"mobile"`
	IsResidential bool   `json:"is_residential"`
	IPType        string `json:"ip_type"` // "美国家宽", "住宅宽带", "机房 IP", "未知"
}

// 美国家宽主流运营商关键词（Comcast, Charter Spectrum, AT&T, Verizon, Cox 等）
var usResidentialKeywords = []string{
	"comcast", "xfinity", "charter", "spectrum", "at&t", "att ", "bellsouth",
	"verizon", "cox", "centurylink", "lumen", "frontier", "optimum", "suddenlink",
	"altice", "mediacom", "windstream", "tds telecom", "breezeline", "cogeco",
	"wow!", "wideopenwest", "starlink", "t-mobile", "astound", "rcn",
	"cable one", "sparklight", "consolidated", "brightspeed", "ziply",
}

// 常见机房与云厂商关键词（非家宽/数据中心）
var datacenterKeywords = []string{
	"amazon", "aws", "google", "cloud", "azure", "microsoft", "cloudflare",
	"digitalocean", "linode", "akamai", "vultr", "choopa", "ovh", "hetzner",
	"oracle", "leaseweb", "psychz", "quadranet", "cogent", "hurricane electric",
	"fastly", "hostinger", "contabo", "datacenter", "hosting", "server", "vps",
	"dedicated", "m247", "clouvider", "tzulo", "ipxo", "frantech", "buyvm",
	"kamatera", "serverius", "zenlayer", "colocrossing",
}

// 美国家宽常见主机名后缀（VPN Gate 等志愿者节点常用主机名特征）
var usResidentialHostSuffixes = []string{
	".comcast.net", ".res.rr.com", ".charter.com", ".att.net", ".verizon.net",
	".cox.net", ".sbcglobal.net", ".frontiernet.net", ".centurylink.net",
	".spectrum.com", ".windstream.net", ".suddenlink.net", ".optimum.net",
}

// JudgeIPType 根据 IP 属性、ISP/Org/AS 信息综合判定是否属于美国家宽。
func JudgeIPType(info *IPInfo) {
	if info == nil {
		return
	}
	isUS := strings.EqualFold(info.CountryCode, "US")
	blob := strings.ToLower(strings.Join([]string{info.ISP, info.Org, info.AS}, " "))

	// 1. 判断是否命中机房特征
	isDC := info.Hosting
	if !isDC {
		for _, kw := range datacenterKeywords {
			if strings.Contains(blob, kw) {
				isDC = true
				break
			}
		}
	}

	// 2. 判断是否命中家宽特征
	isRes := false
	if !isDC {
		if isUS {
			for _, kw := range usResidentialKeywords {
				if strings.Contains(blob, kw) {
					isRes = true
					break
				}
			}
			// 如果不是机房且不是代理，在美国 IP 下判定为住宅宽带
			if !isRes && !info.Hosting && !info.Proxy {
				isRes = true
			}
		} else {
			if !info.Hosting {
				isRes = true
			}
		}
	}

	info.IsResidential = isUS && isRes
	if isUS && isRes {
		info.IPType = "美国家宽"
	} else if isDC {
		info.IPType = "机房 IP"
	} else if isRes {
		info.IPType = "住宅宽带"
	} else {
		info.IPType = "普通网络"
	}
}

// isUSResidentialHost 通过主机名特征快速识别是否属于美国家宽节点。
func isUSResidentialHost(hostname string) bool {
	h := strings.ToLower(strings.TrimSpace(hostname))
	for _, suffix := range usResidentialHostSuffixes {
		if strings.HasSuffix(h, suffix) || strings.Contains(h, suffix) {
			return true
		}
	}
	for _, kw := range usResidentialKeywords {
		if strings.Contains(h, kw) {
			return true
		}
	}
	return false
}

// exitIPSources 出口 IP 多源探测列表，避免单点故障导致误判
var exitIPSources = []string{
	"http://api.ipify.org",
	"https://ipv4.icanhazip.com",
	"http://ifconfig.me/ip",
	"http://ip-api.com/line?fields=query",
}

// probeExitIPMulti 在 netns 内逐个尝试多个公共出口探测源，拿到的第一个合法 IPv4 即返回。
func probeExitIPMulti(nsName string) (string, error) {
	for _, url := range exitIPSources {
		out, err := exec.Command("ip", "netns", "exec", nsName,
			"curl", "-s", "--max-time", "5", url).Output()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(out))
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			return ip, nil
		}
	}
	return "", fmt.Errorf("所有出口 IP 探测源均无法访问")
}

// probeExitIPInfo 在 netns 内查询出口 IP 及其地理与网络属性（含美国家宽检测）。
func probeExitIPInfo(nsName string) (*IPInfo, error) {
	// 优先问 ip-api.com 获取完整的 ISP/Hosting/地理信息
	out, err := exec.Command("ip", "netns", "exec", nsName,
		"curl", "-s", "--max-time", "8",
		"http://ip-api.com/json/?fields=status,message,country,countryCode,regionName,city,isp,org,as,mobile,proxy,hosting,query").Output()
	if err == nil && len(out) > 0 {
		var resp struct {
			Status      string `json:"status"`
			Country     string `json:"country"`
			CountryCode string `json:"countryCode"`
			RegionName  string `json:"regionName"`
			City        string `json:"city"`
			ISP         string `json:"isp"`
			Org         string `json:"org"`
			AS          string `json:"as"`
			Mobile      bool   `json:"mobile"`
			Proxy       bool   `json:"proxy"`
			Hosting     bool   `json:"hosting"`
			Query       string `json:"query"`
		}
		if jsonErr := json.Unmarshal(out, &resp); jsonErr == nil && resp.Status == "success" && resp.Query != "" {
			info := &IPInfo{
				IP:          resp.Query,
				CountryCode: strings.ToUpper(resp.CountryCode),
				Country:     resp.Country,
				Region:      resp.RegionName,
				City:        resp.City,
				ISP:         resp.ISP,
				Org:         resp.Org,
				AS:          resp.AS,
				Hosting:     resp.Hosting,
				Proxy:       resp.Proxy,
				Mobile:      resp.Mobile,
			}
			JudgeIPType(info)
			return info, nil
		}
	}

	// 降级：若未获得详细属性，获取基础 IPv4 地址
	ip, err := probeExitIPMulti(nsName)
	if err != nil {
		return nil, err
	}
	info := &IPInfo{
		IP:     ip,
		IPType: "未知",
	}
	return info, nil
}

// isUSResidentialOperator 通过志愿者填写的 Operator 客户端特征识别家庭 PC
func isUSResidentialOperator(op string) bool {
	lower := strings.ToLower(strings.TrimSpace(op))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "desktop-") || strings.Contains(lower, "laptop-") ||
		strings.Contains(lower, "msi's") || strings.Contains(lower, "home") ||
		strings.Contains(lower, "family") || strings.Contains(lower, "owner") {
		return true
	}
	return false
}

// extractISPFromName 从主机名或 PTR 反向解析域名中提取人能读懂的运营商名。
func extractISPFromName(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "comcast") || strings.Contains(lower, "xfinity"):
		return "Comcast (Xfinity)"
	case strings.Contains(lower, "spectrum") || strings.Contains(lower, "charter") || strings.Contains(lower, "rr.com"):
		return "Charter Spectrum"
	case strings.Contains(lower, "frontier") || strings.Contains(lower, "frontiernet"):
		return "Frontier Fiber"
	case strings.Contains(lower, "suddenlink") || strings.Contains(lower, "altice") || strings.Contains(lower, "optimum"):
		return "Altice / Optimum"
	case strings.Contains(lower, "at&t") || strings.Contains(lower, "att") || strings.Contains(lower, "sbcglobal") || strings.Contains(lower, "bellsouth"):
		return "AT&T"
	case strings.Contains(lower, "verizon") || strings.Contains(lower, "fios"):
		return "Verizon Fios"
	case strings.Contains(lower, "cox"):
		return "Cox Communications"
	case strings.Contains(lower, "centurylink") || strings.Contains(lower, "lumen") || strings.Contains(lower, "brightspeed"):
		return "CenturyLink"
	case strings.Contains(lower, "mediacom"):
		return "Mediacom"
	case strings.Contains(lower, "starlink"):
		return "Starlink"
	case strings.Contains(lower, "t-mobile"):
		return "T-Mobile Home"
	default:
		return "Residential Broadband"
	}
}

// enrichUSResidentialNodes 对美国节点并发执行快速 PTR 反向解析，
// 即使 VPN Gate 节点域名被覆盖为 opengw.net，也能通过底层 IP 的 PTR 记录准确识别美国家宽，
// 确保不会漏掉真实的家宽志愿者节点。
func enrichUSResidentialNodes(nodes []Node) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)

	// 使用公共 DNS (8.8.8.8:53) 直接执行 PTR 解析，避免被 VPS 本地内网 DNS 屏蔽或超时
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", "8.8.8.8:53")
		},
	}

	for i := range nodes {
		if !strings.EqualFold(nodes[i].CountryCode, "US") {
			continue
		}
		ip := nodes[i].IP
		if ip == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, targetIP string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			names, err := resolver.LookupAddr(ctx, targetIP)
			if err == nil {
				for _, name := range names {
					if isUSResidentialHost(name) {
						nodes[idx].IsResidential = true
						if nodes[idx].ISP == "" || nodes[idx].ISP == "Residential Broadband" {
							nodes[idx].ISP = extractISPFromName(name)
						}
						return
					}
				}
			}
		}(i, ip)
	}
	wg.Wait()
}

