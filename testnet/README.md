# AIB 2.0 ZKML 3-Node Testnet

本目录包含 AIB 2.0 ZKML 共识机制的 3 节点测试网络部署配置。

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                    3-Node Testnet                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌──────────┐   ┌──────────┐   ┌──────────┐                  │
│   │  Node 1  │   │  Node 2  │   │  Node 3  │                  │
│   │ :51210   │   │ :51211   │   │ :51212   │                  │
│   └────┬─────┘   └────┬─────┘   └────┬─────┘                  │
│        │              │              │                          │
│        └──────────────┼──────────────┘                          │
│                       │                                         │
│                       ▼                                         │
│              ┌─────────────────┐                               │
│              │     Ollama      │                               │
│              │  (localhost)    │                               │
│              └─────────────────┘                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 前置条件

1. **Go 1.22+**: 已安装 Go 编译器
2. **Ollama**: 运行 Ollama 服务
3. **llama2 模型**: 已下载 llama2 模型

### 安装 Ollama 和模型

```bash
# 安装 Ollama (Linux)
curl -fsSL https://ollama.com/install.sh | sh

# 启动 Ollama 服务
ollama serve &

# 下载 llama2 模型
ollama pull llama2
```

## 快速开始

### 1. 构建并启动

```bash
cd /home/temple/aib/testnet
chmod +x deploy.sh stop.sh
./deploy.sh start
```

### 2. 检查状态

```bash
./deploy.sh status
```

### 3. 查看日志

```bash
tail -f logs/node1.log
tail -f logs/node2.log
tail -f logs/node3.log
```

### 4. 停止测试网

```bash
./deploy.sh stop
# 或
./stop.sh
```

## 配置文件

每个节点使用独立的配置文件：

| 节点 | 配置文件 | 端口 | 数据目录 |
|------|---------|------|---------|
| Node 1 | `config/node1.json` | 51210 | `data/node1` |
| Node 2 | `config/node2.json` | 51211 | `data/node2` |
| Node 3 | `config/node3.json` | 51212 | `data/node3` |

### 配置示例

```json
{
  "node_id": "testnet-node-1",
  "ollama_url": "http://localhost:11434",
  "model": "llama2",
  "stake_amount": 1000.0,
  "listen_addr": "127.0.0.1:51210",
  "data_dir": "./data/node1",
  "log_level": "info"
}
```

## 测试

### 运行单元测试

```bash
cd /home/temple/aib
go test ./zkml/testnet/... -v
```

### 运行集成测试

```bash
cd /home/temple/aib
go test ./zkml/... -v -count=1
```

## 目录结构

```
testnet/
├── config/
│   ├── node1.json
│   ├── node2.json
│   └── node3.json
├── data/
│   ├── node1/
│   ├── node2/
│   └── node3/
├── logs/
│   ├── node1.log
│   ├── node1.pid
│   ├── node2.log
│   ├── node2.pid
│   ├── node3.log
│   └── node3.pid
├── deploy.sh
├── stop.sh
└── README.md
```

## 故障排除

### Ollama 连接失败

```bash
# 检查 Ollama 是否运行
curl http://localhost:11434/api/tags

# 重启 Ollama
pkill ollama
ollama serve &
```

### 节点启动失败

```bash
# 查看详细日志
cat logs/node1.log

# 检查端口占用
lsof -i :51210
lsof -i :51211
lsof -i :51212
```

### 清理数据

```bash
rm -rf data/node*/*
rm -rf logs/*.log logs/*.pid
```

## 访问链接

- **Ollama API**: http://localhost:11434
- **Node 1**: http://127.0.0.1:51210
- **Node 2**: http://127.0.0.1:51211
- **Node 3**: http://127.0.0.1:51212

---

*更多文档: https://84.247.155.30:51200/plans/zkml-testnet.md*
