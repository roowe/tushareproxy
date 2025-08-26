# tushareproxy

为 tushare.pro 提供代理与缓存服务，提升访问效率并降低请求频率。

## 项目简介

**开发动机**：作为金融数据使用者，通常每两周或在模型验证时才会使用 tushare，本地并未存储 tushare 数据。每次都需要按个股逐一请求，往往涉及上千个请求，效率极低。虽然可以自建数据存储，但容易产生数据处理错误，因此开发了这个代理项目。

**核心价值**：在特定 `end_date` 参数下，代理服务器会自动缓存已请求过的数据，避免重复请求。本地访问不会触发 API 限流，实现快速数据获取。

tushareproxy 是一个高效的 tushare.pro API 代理服务器，主要功能包括：

- **智能缓存**: 基于 BadgerDB 的本地缓存系统，使用 SHA256 对请求 body 生成缓存键
- **请求代理**: 作为中间层代理访问 tushare.pro 接口，统一管理请求
- **性能优化**: 缓存有效期为 100 天，自动过期，显著减少重复请求
- **状态码保持**: 保留并传递原始 API 返回的 HTTP 状态码
- **错误处理**: 只有当 tushare API 返回 code=0 时才进行缓存，并记录 API 错误信息

## 快速开始

### 安装

```bash
go install github.com/roowe/tushareproxy@latest
```

### 部署

1. 创建工作目录:
```bash
mkdir tushareproxy && cd tushareproxy
```

2. 复制配置文件:
```bash
# 将 proxy.toml.example 复制为 proxy.toml
cp proxy.toml.example proxy.toml
```

3. 启动服务:
```bash
~/go/bin/tushareproxy
```

## 客户端使用

### Python 客户端示例

使用以下 Python 代码替换原有的 tushare pro 接口：

```python
"""
Pro数据接口
Created on 2017/07/01
@author: polo,Jimmy
@group : https://waditu.com
"""

import pandas as pd
import json
from functools import partial
import requests


class DataApi:

    __token = ''
    # 原始接口地址
    # __http_url = 'http://api.waditu.com/dataapi'
    # 代理服务器地址
    __http_url = 'http://127.0.0.1:1155/dataapi'

    def __init__(self, token, timeout=30):
        """
        Parameters
        ----------
        token: str
            API接口TOKEN，用于用户认证
        """
        self.__token = token
        self.__timeout = timeout

    def query(self, api_name, fields='', **kwargs):
        req_params = {
            'api_name': api_name,
            'token': self.__token,
            'params': kwargs,
            'fields': fields
        }
        res = requests.post(f"{self.__http_url}", json=req_params, timeout=self.__timeout)
        if res:
            result = json.loads(res.text)
            if result['code'] != 0:
                raise Exception(result['msg'])
            data = result['data']
            columns = data['fields']
            items = data['items']
            return pd.DataFrame(items, columns=columns)
        else:
            return pd.DataFrame()

    def __getattr__(self, name):
        return partial(self.query, name)
```

### 使用方法

1. 初始化 pro 接口:
```python
# 使用你的 tushare token
ts_token = 'your_tushare_token_here'
pro = DataApi(ts_token)
```

2. 调用 API 接口:
```python
# 获取股票基本信息
df = pro.stock_basic(exchange='', list_status='L', fields='ts_code,symbol,name,area,industry,list_date')
print(df.head())

# 获取交易日历
df = pro.trade_cal(exchange='', start_date='20230101', end_date='20231231')
print(df.head())
```

## 工作原理

1. **请求处理**: 客户端发送请求到代理服务器（默认端口 1155）
2. **缓存检查**: 代理服务器根据请求参数的 SHA256 哈希值检查缓存
3. **缓存命中**: 如果缓存存在且未过期，直接返回缓存数据
4. **API转发**: 如果缓存未命中，转发请求到 tushare.pro API
5. **缓存存储**: 当 tushare API 返回 code=0（成功）时，将响应数据存储到缓存
6. **响应返回**: 返回统一格式的 JSON 响应给客户端

## 限制与注意事项

### 功能限制
1. **分页数据处理**: 当前版本未处理 `has_more=true` 的逐一获取逻辑，需要多次手动请求才能获得完整数据集。
2. **缓存过期机制**: 默认缓存有效期较长（100天），如果请求参数相同将直接返回缓存。建议配合 `end_date` 参数使用来获得新的数据。
   - 示例：`pro.stock_basic()` 不传 `end_date` 参数时，只能等待缓存自然过期。

## 性能特性

- **高性能缓存**: 基于 BadgerDB 的本地缓存，读写性能优异
- **智能过期**: 缓存自动过期机制，避免数据过时
- **错误处理**: 完善的错误处理和日志记录
- **并发支持**: 支持高并发请求处理
- **资源优化**: 减少对 tushare.pro 的重复请求，降低tushare.pro服务器的压力，**绿色环保**

### 使用注意事项

1. **Token 配置**: 确保 tushare token 有效且权限充足
2. **端口冲突**: 服务启动前检查端口 1155 是否被占用
3. **存储空间**: 确保有足够的磁盘空间用于缓存存储
4. **日志管理**: 定期清理过期的日志文件，生产环境建议调整日志级别为 `info` 或 `warn`
5. **数据新鲜度**: 对于时效性要求高的数据，建议使用 `end_date` 参数来确保数据的及时更新



## 许可证

本项目采用 MIT 许可证，详见 LICENSE 文件。
