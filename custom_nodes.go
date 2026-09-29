package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CustomNode 是用户添加的自定义节点（例如自建美国家宽节点、静态住宅代理或私有 OpenVPN 节点）。
type CustomNode struct {
	Name          string `json:"name"`           // 节点名称/备注，例如 "US-Home-Comcast-1"
	IP            string `json:"ip,omitempty"`   // 节点 IP（选填）
	CountryCode   string `json:"country_code"`   // 国家代码，如 "US"
	Country       string `json:"country"`        // 国家名称，如 "United States"
	ISP           string `json:"isp,omitempty"`  // 运营商，例如 "Comcast", "Spectrum", "AT&T"
	IsResidential bool   `json:"is_residential"` // 是否为住宅家宽
	Config        string `json:"config"`         // 完整的 .ovpn 配置文件内容
}

var customMu sync.Mutex

func customNodesPath(dir string) string {
	return filepath.Join(dir, "custom_nodes.json")
}

// loadCustomNodes 从磁盘载入所有自定义节点。
func loadCustomNodes(dir string) ([]CustomNode, error) {
	customMu.Lock()
	defer customMu.Unlock()

	path := customNodesPath(dir)
	blob, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []CustomNode
	if err := json.Unmarshal(blob, &list); err != nil {
		return nil, fmt.Errorf("解析自定义节点列表失败: %w", err)
	}
	return list, nil
}

// saveCustomNodes 将自定义节点写入磁盘。
func saveCustomNodes(dir string, list []CustomNode) error {
	path := customNodesPath(dir)
	blob, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// addCustomNode 添加或更新一个自定义节点。
func addCustomNode(dir string, node CustomNode) error {
	node.Name = strings.TrimSpace(node.Name)
	if node.Name == "" {
		return fmt.Errorf("节点名称不能为空")
	}
	node.Config = strings.TrimSpace(node.Config)
	if node.Config == "" {
		return fmt.Errorf("OpenVPN 配置内容不能为空")
	}
	if node.CountryCode == "" {
		node.CountryCode = "US"
	}
	node.CountryCode = strings.ToUpper(strings.TrimSpace(node.CountryCode))
	if node.Country == "" {
		if node.CountryCode == "US" {
			node.Country = "United States"
		} else {
			node.Country = node.CountryCode
		}
	}

	customMu.Lock()
	defer customMu.Unlock()

	path := customNodesPath(dir)
	var list []CustomNode
	blob, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(blob, &list)
	}

	// 若存在同名节点则更新，否则追加
	updated := false
	for i, item := range list {
		if item.Name == node.Name {
			list[i] = node
			updated = true
			break
		}
	}
	if !updated {
		list = append([]CustomNode{node}, list...)
	}

	return saveCustomNodes(dir, list)
}

// deleteCustomNode 移除指定名称的自定义节点。
func deleteCustomNode(dir string, name string) error {
	customMu.Lock()
	defer customMu.Unlock()

	path := customNodesPath(dir)
	var list []CustomNode
	blob, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(blob, &list); err != nil {
		return err
	}

	kept := make([]CustomNode, 0, len(list))
	for _, item := range list {
		if item.Name != name {
			kept = append(kept, item)
		}
	}
	return saveCustomNodes(dir, kept)
}

// ToNode 将 CustomNode 转化为系统通用的 Node 结构体。
func (c CustomNode) ToNode() Node {
	cc := strings.ToUpper(strings.TrimSpace(c.CountryCode))
	if cc == "" {
		cc = "US"
	}
	country := c.Country
	if country == "" {
		country = "United States"
	}
	return Node{
		HostName:      c.Name,
		IP:            c.IP,
		Country:       country,
		CountryCode:   cc,
		Ping:          1,
		SpeedMbps:     100.0,
		Sessions:      0,
		Config:        c.Config,
		IsResidential: c.IsResidential,
		ISP:           c.ISP,
		Source:        "custom",
	}
}
