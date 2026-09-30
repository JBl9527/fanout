package main

import (
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const vpngateAPI = "https://www.vpngate.net/api/iphone/"

// vpngateMirror 是直连拿不到节点列表时的兜底（Cloudflare Worker 反代）。
// 用 FANOUT_VPNGATE_MIRROR 可以换成自己的地址，设成空字符串就只走直连。
const vpngateMirror = "https://p.xy.kg/vpngate"

// mirrorKey 只是让反代不被爬虫和端口扫描白嫖，不是安全边界。
const mirrorKey = "8rhIFzFKRJMFAe-xP5OQPclDEvSjKlHo"

func mirrorURL() string {
	if v, ok := os.LookupEnv("FANOUT_VPNGATE_MIRROR"); ok {
		return strings.TrimSpace(v)
	}
	return vpngateMirror
}

func mirrorAccessKey() string {
	if v, ok := os.LookupEnv("FANOUT_VPNGATE_MIRROR_KEY"); ok {
		return strings.TrimSpace(v)
	}
	return mirrorKey
}

// Node 是一个 VPN Gate 节点。
type Node struct {
	HostName      string  `json:"hostname"`
	IP            string  `json:"ip"`
	Country       string  `json:"country"`
	CountryCode   string  `json:"country_code"`
	Ping          int     `json:"ping"`
	SpeedMbps     float64 `json:"speed_mbps"`
	Sessions      int     `json:"sessions"`
	Config        string  `json:"-"` // 解码后的 .ovpn 内容
	IsResidential bool    `json:"is_residential"`
	ISP           string  `json:"isp,omitempty"`
	Source        string  `json:"source,omitempty"` // "vpngate" | "custom"
	Operator      string  `json:"operator,omitempty"`
}

// fetchNodes 拉取并解析 VPN Gate 的节点列表。
// 先直连；连不上或者拿回来的内容不对（被拦截、返回门户页）就换反代再试一次。
// 返回的列表已按速度降序排列。
func fetchNodes(timeout time.Duration) ([]Node, error) {
	return fetchNodesWith(vpngateAPI, timeout)
}

// fetchNodesWith 并发拉取官方直连源与反代镜像源，并将节点去重聚合，
// 最大化节点池覆盖面（特别是稀缺的美国家宽节点）。
func fetchNodesWith(direct string, timeout time.Duration) ([]Node, error) {
	type fetchRes struct {
		nodes []Node
		err   error
	}

	sources := []struct {
		url string
		key string
	}{
		{url: direct, key: ""},
	}
	if mirror := mirrorURL(); mirror != "" {
		sources = append(sources, struct {
			url string
			key string
		}{url: mirror, key: mirrorAccessKey()})
	}

	resCh := make(chan fetchRes, len(sources))
	for _, s := range sources {
		go func(targetURL, targetKey string) {
			list, err := fetchNodesFrom(targetURL, targetKey, timeout)
			resCh <- fetchRes{nodes: list, err: err}
		}(s.url, s.key)
	}

	var all []Node
	var lastErr error
	for i := 0; i < len(sources); i++ {
		res := <-resCh
		if res.err != nil {
			lastErr = res.err
			continue
		}
		all = append(all, res.nodes...)
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("所有节点源拉取失败: %w", lastErr)
	}

	// 按 IP（或 HostName）合并去重，并保留家宽属性与最优网速
	seen := make(map[string]int) // key -> index in merged
	var merged []Node
	for _, n := range all {
		key := n.IP
		if key == "" {
			key = n.HostName
		}
		if idx, exists := seen[key]; exists {
			if n.IsResidential && !merged[idx].IsResidential {
				merged[idx].IsResidential = true
				if merged[idx].ISP == "" {
					merged[idx].ISP = n.ISP
				}
			}
			if n.SpeedMbps > merged[idx].SpeedMbps {
				merged[idx].SpeedMbps = n.SpeedMbps
			}
			continue
		}
		seen[key] = len(merged)
		merged = append(merged, n)
	}

	sort.Slice(merged, func(i, j int) bool { return merged[i].SpeedMbps > merged[j].SpeedMbps })
	return merged, nil
}

func fetchNodesFrom(url, key string, timeout time.Duration) ([]Node, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("拉取节点列表失败: %w", err)
	}
	if key != "" {
		req.Header.Set("X-Fanout-Key", key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("拉取节点列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("拉取节点列表失败: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取节点列表失败: %w", err)
	}
	return parseNodeCSV(string(raw))
}

// parseNodeCSV 解析 VPN Gate 的 CSV。首行是 "*vpn_servers"，
// 第二行是以 '#' 开头的表头，末行是 "*"。
func parseNodeCSV(body string) ([]Node, error) {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		kept = append(kept, strings.TrimPrefix(line, "#"))
	}
	if len(kept) < 2 {
		return nil, fmt.Errorf("节点列表格式异常: 有效行不足")
	}

	r := csv.NewReader(strings.NewReader(strings.Join(kept, "\n")))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析节点 CSV 失败: %w", err)
	}

	header := records[0]
	idx := map[string]int{}
	for i, name := range header {
		idx[strings.TrimSpace(name)] = i
	}
	need := []string{"HostName", "IP", "CountryLong", "CountryShort", "Ping", "Speed", "OpenVPN_ConfigData_Base64"}
	for _, k := range need {
		if _, ok := idx[k]; !ok {
			return nil, fmt.Errorf("节点列表缺少字段 %s", k)
		}
	}

	var nodes []Node
	for _, rec := range records[1:] {
		get := func(k string) string {
			i := idx[k]
			if i >= len(rec) {
				return ""
			}
			return rec[i]
		}
		cfgB64 := get("OpenVPN_ConfigData_Base64")
		if cfgB64 == "" || get("HostName") == "" {
			continue
		}
		cfg, err := base64.StdEncoding.DecodeString(cfgB64)
		if err != nil {
			continue
		}
		ping, _ := strconv.Atoi(get("Ping"))
		speed, _ := strconv.ParseFloat(get("Speed"), 64)
		sessions, _ := strconv.Atoi(get("NumVpnSessions"))
		cc := strings.ToUpper(strings.TrimSpace(get("CountryShort")))
		hostName := get("HostName")
		operator := get("Operator")
		isRes := false
		isp := ""
		if cc == "US" {
			if isUSResidentialHost(hostName) || isUSResidentialOperator(operator) {
				isRes = true
				isp = "Residential Broadband"
			}
		}
		nodes = append(nodes, Node{
			HostName:      hostName,
			IP:            get("IP"),
			Country:       get("CountryLong"),
			CountryCode:   cc,
			Ping:          ping,
			SpeedMbps:     speed / 1e6,
			Sessions:      sessions,
			Config:        string(cfg),
			IsResidential: isRes,
			ISP:           isp,
			Source:        "vpngate",
			Operator:      operator,
		})
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("节点列表为空")
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].SpeedMbps > nodes[j].SpeedMbps })
	return nodes, nil
}
