# RProxy

> Debian 13 透明代理网关系统
> 客户端 + 服务端配套使用，VMess + WebSocket + TLS + 真网站架构

---

## 项目简介

RProxy 是一个完整的透明代理解决方案，包含两个独立部署的部分：

| 部分 | 部署位置 | 作用 |
|---|---|---|
| **客户端 (`client/`)** | 内网 Debian 13 旁路由 | DNS 分流、透明代理、WireGuard 入站、WebUI 管理 |
| **服务端 (`server/`)** | 境外 Debian 13 VPS | TLS 终结、代理出口、真网站伪装 |


## 安装

需要分别在旁路由和 VPS 上执行：

### 客户端（旁路由）

```bash
wget -O- https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/client/install.sh | bash
```

要求：
- Debian 13 (Trixie) x86_64
- root 权限
- 局域网内可访问的 IP
- **首次安装时旁路由能访问 GitHub**（国内用户请先挂代理）

### 服务端（VPS）

```bash
wget -O- https://raw.githubusercontent.com/Fr33raNg3r/RProxy/main/server/install.sh | bash
```

要求：
- Debian 13 (Trixie) x86_64
- root 权限
- 公网 IP
- 一个域名（DNS A 记录指向 VPS IP）
- 80 / 443 端口未被占用

## 部署顺序

1. **先装服务端**——安装完成后会打印一组客户端配置（地址、UUID、WS 路径等）
2. **再装客户端**——把服务端打印的配置填到客户端 WebUI 的【节点管理】里
3. **配置局域网**——主路由 DHCP 把网关 + DNS 改成旁路由 IP


## 协议（License）

本项目采用 **GNU General Public License v3.0** (GPLv3) 协议发布。

> 完整协议文本请参见 [LICENSE](LICENSE) 文件，
> 或访问 https://www.gnu.org/licenses/gpl-3.0.html

简要说明（**法律效力以 LICENSE 文件全文为准**）：

- ✅ 你**可以**自由使用、修改、分发本项目
- ✅ 你**可以**用于商业用途
- ✅ 你**可以**基于本项目创建衍生作品
- ⚠️ 衍生作品**必须**以同样的 GPLv3 协议开源
- ⚠️ 分发时**必须**保留原版权声明和协议文本
- ⚠️ 必须**附上完整源代码**或提供获取源码的途径
- ❌ **不提供**任何明示或暗示的担保
- ❌ 作者**不对**因使用本软件造成的任何损失负责

## 致谢

本项目设计与实现参考了以下开源项目和资料：

- [Xray-core](https://github.com/XTLS/Xray-core) — 核心代理引擎
- [mosdns](https://github.com/IrineSistiana/mosdns) — DNS 分流转发
- [acme.sh](https://github.com/acmesh-official/acme.sh) — 证书自动管理
- [Loyalsoldier/v2ray-rules-dat](https://github.com/Loyalsoldier/v2ray-rules-dat) — GeoIP/GeoSite 规则数据


## 不提供售后支持

本项目按现状提供，作者**没有义务**解答个人使用问题或修复非通用 bug。
如果你能提交清晰可复现的 issue 或 PR，欢迎贡献。

## 项目状态

| 组件 | 当前版本 | 状态 |
|---|---|---|
| Client | 1.0.4 | 稳定 |
| Server | 1.0.0 | 稳定 |

