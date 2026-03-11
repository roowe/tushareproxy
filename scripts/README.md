# Tushare curl test scripts

## Script

- `tushare_curl_test.sh`: call `myproxy` with fixed `_cache` scenarios

## Quick start

```bash
cd myproxy
chmod +x scripts/tushare_curl_test.sh
TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh all
```

脚本默认请求本地代理：

```text
http://127.0.0.1:1155/dataapi
```

所以运行前请先启动 `myproxy`。

## Design

为了减少使用成本，脚本只要求配置一个环境变量：

- `TUSHARE_TOKEN`

其它测试内容都写死在脚本 payload 里。你通过位置参数选择要执行的场景。

## Scenarios

可选场景：

- `all`
- `ttl`
- `no-cache`
- `expires-at`

固定使用的 `_cache` 能力包括：

- `namespace`
- `ttl`
- `expires_at`
- `no_cache`

其中：

- `ttl` 场景使用 `namespace = script.stock_basic.ttl`
- `expires_at` 场景使用 `namespace = script.stock_basic.expires_at`
- `expires_at` 会在脚本启动时自动计算成“当前时间 + 1800 秒”

示例：

```bash
TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh ttl
TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh ttl ttl
TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh no-cache
TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh expires-at
TUSHARE_TOKEN=your_token ./scripts/tushare_curl_test.sh expires-at expires-at
```

## How to observe

脚本本身会打印：

- 请求 payload
- 响应 body

缓存是否命中、是否绕过缓存，建议直接看 `myproxy` 服务日志中的 `cache_status`。
