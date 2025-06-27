# qday-abel-bridge-indexer

## 配置说明

本项目使用 `.env` 文件进行配置管理。请复制 `.env.example` 文件为 `.env` 并根据你的环境修改配置：

```bash
cp .env.example .env
```

### 主要配置项

#### Indexer 配置
- `INDEXER_ROOT_DIR`: 配置文件目录
- `INDEXER_LOG_LEVEL`: 日志级别 (info, debug, warn, error)
- `INDEXER_LOG_FORMAT`: 日志格式 (console, json)
- `INDEXER_DATABASE_SOURCE`: 数据库连接字符串
- `INDEXER_DATABASE_MAX_IDLE_CONNS`: 数据库最大空闲连接数
- `INDEXER_DATABASE_MAX_OPEN_CONNS`: 数据库最大打开连接数
- `INDEXER_DATABASE_CONN_MAX_LIFETIME`: 数据库连接最大生命周期

#### Bitcoin 配置
- `BITCOIN_NETWORK_NAME`: Bitcoin 网络名称 (mainnet, testnet3, signet)
- `BITCOIN_RPC_HOST`: Bitcoin RPC 主机地址
- `BITCOIN_RPC_PORT`: Bitcoin RPC 端口
- `BITCOIN_RPC_USER`: Bitcoin RPC 用户名
- `BITCOIN_RPC_PASS`: Bitcoin RPC 密码
- `BITCOIN_DISABLE_TLS`: 是否禁用 TLS
- `BITCOIN_ENABLE_INDEXER`: 是否启用索引器
- `BITCOIN_INDEXER_LISTEN_ADDRESS`: 索引器监听地址
- `BITCOIN_INDEXER_LISTEN_TARGET_CONFIRMATIONS`: 目标确认数

#### Bridge 配置
- `BITCOIN_BRIDGE_ETH_RPC_URL`: Ethereum RPC URL
- `BITCOIN_BRIDGE_CONTRACT_ADDRESS`: Bridge 合约地址
- `BITCOIN_BRIDGE_ETH_PRIV_KEY`: Ethereum 私钥
- `BITCOIN_BRIDGE_ABI`: ABI 文件路径
- `BITCOIN_BRIDGE_AA_B2_API`: AA B2 API 地址

详细配置说明请参考 `docs/ENVS.md`。

## 运行

```bash
go run main.go start
```

## 测试

```bash
go test ./...
```
