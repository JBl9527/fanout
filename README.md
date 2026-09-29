# fanout (美国家宽增强版)

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> **原项目出处与鸣谢**：  
> 本项目基于原作者 **Joey** 的优秀开源项目 [byJoey/fanout](https://github.com/byJoey/fanout) 进行深度定制与功能增强。感谢原作者的高质量架构设计与无私开源！  
> - 原作者博客：[joeyblog.net](https://joeyblog.net)  
> - 原作者油管：[@joeyblog](https://youtube.com/@joeyblog)  
> - 原作者交流群：[Telegram 群组](https://t.me/+ft-zI76oovgwNmRh)

---

把 VPN Gate 公共节点及自定义住宅节点变成本地 SOCKS5 端口：**一个端口一个出口 IP**。  
再给每个出口挂一个节点链接，客户端连哪个端口就从哪个出口出去。支持同机接管 3x-ui、xray-cf-lite 或 fanout 自建 Xray。

![主界面](https://images.joeyblog.net/2026/7/27/fanout-dashboard.png)

---

### ✨ 本版本独家增强特性

1. **美国家宽（US Residential IP）智能精准识别**：
   - 自动检测 IP 归属地与网络类型（`CountryCode: US`，`hosting: false`，`proxy: false`）。
   - 内置主流美国家宽运营商特征库（Comcast Xfinity, Charter Spectrum, AT&T, Verizon Fios, Cox, Frontier, CenturyLink, Altice, Mediacom 等），严格剔除 AWS、DigitalOcean、Vultr、Cloudflare 等机房 IP。
2. **底层并发 PTR 反向域名解析（防漏判）**：
   - VPN Gate 志愿者的主机名经常被官方域名覆盖为 `vg*.opengw.net`；系统后台并发对底层 IP 执行快速 PTR 反查，**精准揪出隐藏在动态域名背后的真实美国家宽**，最大化挖掘可用住宅节点！
3. **美国家宽专属置顶分类**：
   - 新建出口向导中单列 **【美国家宽 (US-RES)】** 专属标签，显示当前可用空闲数量，支持一键批量开通。
   - 界面出口列表与终端菜单中带专属绿色高亮徽章（如 `美国家宽 · Comcast`），运营商一目了然。
4. **支持添加私有美国家宽 / 自定义节点**：
   - 页面新增「**添加美国家宽节点**」按钮，支持直接录入自建住宅 VPS 或采购的静态住宅 OpenVPN 配置（.ovpn），数据持久化保存在 `/var/lib/fanout/custom_nodes.json`，解决纯公共节点在线率痛点。
5. **系统稳定性与容错修复**：
   - **出口探测与心跳多源化**：彻底解决原版仅依赖单一 `api.ipify.org` 因限流或超时导致隧道频繁被误判断线杀掉的问题。
   - **Netns 拨号超时保护**：设置 15 秒超时控制，避免远程不可达时锁死系统线程。
   - **网络命名空间残留彻底清理**。

---

## 安装与快速开始

需要 root 权限，Linux 系统（依赖 netns 与 `/dev/net/tun`）。

### 全新一键安装

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/JBl9527/fanout/main/install.sh)
```

**Alpine** 默认不带 bash，先装一下：
```bash
apk add bash curl
bash <(curl -fsSL https://raw.githubusercontent.com/JBl9527/fanout/main/install.sh)
```

---

## 升级指南（从原版平滑升级）

整个程序在底层编译为单个二进制可执行文件 `/usr/local/bin/fanout`，所有端口、口令、路径和隧道状态保存在 `/var/lib/fanout/` 中。**升级过程不会丢失任何已生成的端口与配置**。

### 方式 1：原版机器一条命令直接升级

如果你的服务器之前已经安装过原版 fanout，直接在终端执行：

```bash
REPO="JBl9527/fanout" f update
```
或者再次执行一键安装脚本即可完成覆盖升级：
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/JBl9527/fanout/main/install.sh)
```

### 方式 2：升级之后后续更新

升级到本版本后，后续系统已默认指向本仓库：
* 终端敲 `f update` 即自动升级到最新版；
* 在 Web 管理界面的「设置」中点击「检查更新」与「更新到最新版」即可一键热更新。

---

## 运维管理

装完后敲 `f` 打开管理菜单：

```
  状态      运行中
  版本      fanout v0.2.0
  开机自启  enabled

  管理地址  http://1.2.3.4:8899/gwPuWHvaNr/
  访问口令  f81120ac328d11c11b

   1) 启动          2) 停止
   3) 重启          4) 查看日志
   5) 隧道列表      6) 连接信息
   7) 改端口        8) 改口令
   9) 改访问路径   10) 开机自启开关
  11) 更新         12) 卸载
```

常用命令行参数：
```bash
f info       # 连接信息
f list       # 隧道列表（实时标注美国家宽类型）
f restart    # 重启服务
f log        # 跟踪日志
f update     # 检查并更新到本仓库最新版本
```

---

## 许可与声明

[MIT License](LICENSE)。

节点来自 [VPN Gate](https://www.vpngate.net/)（筑波大学学术实验项目）及用户自配置的节点，本工具仅调用接口进行网络聚合与 SOCKS5 转发，请遵守当地法律法规。
