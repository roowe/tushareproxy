# tushareproxy

为 `tushare.pro` 提供本地代理和缓存。

## 开发动机

我之前会时不时回测一些模型，所以最早的缓存设计是围绕特定 `end_date` 参数展开的。现在我每天都会使用 `tushare.pro` 的数据做复盘，原来的缓存机制就不够顺手了。

现在这版重新设计了缓存机制，允许调用方按不同 API 设计刷新时间，也可以通过 `namespace` 做项目隔离。一个可直接参考的客户端实现见 [example/tushare_api.py](example/tushare_api.py)。

## 核心能力

- 代理 `tushare.pro` 的 `/dataapi`
- 基于 BadgerDB 做本地缓存
- 缓存键为 `namespace + 规范化请求体`
- 支持请求级 `_cache`：`namespace`、`ttl`、`expires_at`、`no_cache`
- 只有当 tushare 返回 `code=0` 时才写缓存

## 快速开始

安装：

```bash
go install github.com/roowe/tushareproxy@latest
```

运行：

```bash
mkdir tushareproxy && cd tushareproxy
cp proxy.toml.example proxy.toml
~/go/bin/tushareproxy
```

## Python 客户端

推荐直接使用 [example/tushare_api.py](example/tushare_api.py) 替换原有的 tushare pro 接口。

初始化：

```python
ts_token = "your_tushare_token_here"
pro = DataApi(ts_token)
```

如果要显式指定代理地址：

```python
pro = DataApi(ts_token, http_url="http://127.0.0.1:1155/dataapi")
```

或者：

```bash
export TUSHARE_DATAAPI_URL=http://127.0.0.1:1155/dataapi
```

调用示例：

```python
df = pro.stock_basic(
    exchange="",
    list_status="L",
    fields="ts_code,symbol,name,area,industry,list_date",
)
print(df.head())

df = pro.trade_cal(
    exchange="",
    start_date="20230101",
    end_date="20231231",
)
print(df.head())
```

### 示例客户端的缓存行为

[example/tushare_api.py](example/tushare_api.py) 会自动给每次请求附带 `_cache`，你不需要在 `pro.xxx(...)` 里手动传。

默认策略：

- `namespace` 按 `PROJECT_CACHE_NAMESPACE:api_name` 生成
- `expires_at` 自动算到下一个刷新时间点
- `ttl` 自动算成 `expires_at - now`
- `no_cache = False`

如果你要调整刷新策略，直接改 [example/tushare_api.py](example/tushare_api.py) 里的常量：

```python
PROJECT_CACHE_NAMESPACE = "myproject"
DEFAULT_REFRESH_HOUR = 20
SPECIAL_REFRESH_HOURS = {
    "margin_detail": 9,
}
```

## `_cache` 协议

如果你不是用 [example/tushare_api.py](example/tushare_api.py)，而是直接调 `myproxy` 的 HTTP 接口，可以手动传顶层 `_cache`：

```python
payload = {
    "api_name": "daily",
    "token": "your_tushare_token_here",
    "params": {
        "ts_code": "000001.SZ",
        "start_date": "20240101",
        "end_date": "20240131",
    },
    "fields": "ts_code,trade_date,open,high,low,close,vol",
    "_cache": {
        "namespace": "myproject:daily",
        "ttl": 300,
        "expires_at": 1773200400,
        "no_cache": False,
    },
}
```

字段说明：

- `namespace`: 缓存命名空间
- `ttl`: 相对过期时间，单位秒
- `expires_at`: 绝对过期时间，Unix timestamp，单位秒
- `no_cache`: 直接回源，不读缓存，也不写缓存

优先级：

1. `no_cache=true` 时，直接回源，不读也不写缓存
2. 同时传 `ttl` 和 `expires_at` 时，取更早过期的那个
3. 都不传时，使用服务端默认 TTL

## 注意事项

- 对于不传 `end_date` 或包含当前交易日的数据，建议按 API 设计刷新时间
- 使用示例客户端时，优先改 [example/tushare_api.py](example/tushare_api.py) 的刷新常量
- 手写 HTTP 请求时，再显式设置 `_cache.ttl` 或 `_cache.expires_at`

## 许可证

MIT，见 `LICENSE`。
