# Natter-go

基于 [MikeWang000000/Natter](https://github.com/MikeWang000000/Natter) 的 Go 重写版本，实现了 STUN 打洞、NAT 穿透与端口转发功能。

---

## 功能特性

* **STUN NAT 类型检测**：UDP/TCP NAT 映射检查，兼容 RFC 3489/5389。
* **端口映射监测**：定时检测本地端口在公网的映射地址（Inner → Outer）。
* **TCP/UDP 转发**：将外部连接转发到本地服务。
* **Keep-Alive 保活**：支持 TCP/UDP 保活，避免 NAT 连接超时。
* **HTTP 测试服务器**：`-t` 模式下启动简单 HTTP 服务，快速验证端口可达性。
* **状态报告**：JSON 文件记录当前映射，支持自定义 Hook 在映射更新时触发命令。

---

## 环境依赖

* Go 1.18+
* Windows/Linux/macOS 均可编译运行
* 网络环境需能访问公共 STUN 服务器。

---

## 快速开始

### 1. 克隆代码

```bash
git clone https://github.com/H1W0XXX/natter_go
cd natter-go
```




### 2. 编译可执行文件

```bash
go build -v -a -o natter.exe -ldflags "-s -w" -asmflags "all=-trimpath=$(Get-Location)" -gcflags "all=-trimpath=$(Get-Location)" ./cmd/natter
```

```MINGW64
#MINGW64编译linux amd64可执行二进制文件
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -gcflags="all=-trimpath=${PWD}" -asmflags="all=-trimpath=${PWD}"  -o natter-linux-amd64 ./cmd/natter
```

### 3. 准备配置文件 `config.json`

示例：

```json
{
  "stun_server": {
    "tcp": [
      "stun.l.google.com",
      "stun1.l.google.com"
    ],
    "udp": [
      "stun.l.google.com:3478",
      "stun1.l.google.com:3478"
    ]
  },
  "keep_alive": "www.qq.com",
  "interval": 10,
  "open_port": {
    "tcp": ["0.0.0.0:34567"],
    "udp": []
  },
  "forward_port": {
    "tcp": ["192.168.1.1:80"],
    "udp": []
  },
  "status_report": {
    "hook": "echo mapped from {inner} to {outer}",
    "status_file": "status.json"
  },
  "logging": {
    "level": "info",
    "log_file": ""
  }
}
```

* `stun_server`: STUN 服务列表（TCP/UDP）
* `keep_alive`: 保活域名或 IP
* `interval`: 周期（秒），控制检测与保活间隔
* `open_port`: 本地待检测端口列表
* `forward_port`: 转发目标地址列表
* `status_report`: 映射更新后写入文件 & 执行 Hook
* `logging`: 日志级别 & 文件路径

### 4. 启动程序

```powershell
./natter.exe -c config.json  
#路由器需要开启upnp,config中设置 "enable_upnp":true
#windows不支持端口复用
```

```bash
./natter -c config.json
#linux支持端口复用
```




### 5. HTTP 测试模式

直接开放本机端口，跳过 STUN：

```powershell
./natter.exe -t 2888
```

访问 `http://<你的公网IP>:2888`，应看到：

```html
It works!
```

---

## 参数说明

| 参数   | 类型     | 说明                |
| ---- | ------ | ----------------- |
| `-c` | string | 配置文件路径（JSON）      |
| `-v` | bool   | Debug 模式，输出更多日志   |
| `-t` | bool   | HTTP 测试服务器（仅端口模式） |

---

## 参考

* 原始项目：[MikeWang000000/Natter](https://github.com/MikeWang000000/Natter)
* STUN 库：[pion/stun](https://github.com/pion/stun)

---
