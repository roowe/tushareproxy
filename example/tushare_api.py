from __future__ import annotations

"""
Pro数据接口
Created on 2017/07/01
@author: polo,Jimmy
@group : https://waditu.com
"""

import datetime
import json
import os
from functools import partial
from typing import Any

import pandas as pd
import requests

BEIJING_TZ = datetime.timezone(datetime.timedelta(hours=8))
PROJECT_CACHE_NAMESPACE = "myproject"
# 默认每天20点刷新数据，特殊接口可以单独配置
DEFAULT_REFRESH_HOUR = 20
# 配置不同api的刷新策略
SPECIAL_REFRESH_HOURS: dict[str, int] = {
    "margin_detail": 9,
}


class DataApi:
    __token = ""
    # __http_url = 'http://api.waditu.com/dataapi'
    __http_url = "http://127.0.0.1:1155/dataapi"

    def __init__(self, token: str, timeout: int = 30, http_url: str | None = None):
        """
        Parameters
        ----------
        token: str
            API接口TOKEN，用于用户认证
        """
        self.__token = token
        self.__timeout = timeout
        http_url = (http_url or os.getenv("TUSHARE_DATAAPI_URL") or "").strip()
        if http_url:
            self.__http_url = http_url

    def query(
        self,
        api_name: str,
        fields: str = "",
        **kwargs: Any,
    ) -> pd.DataFrame:
        req_params = {
            "api_name": api_name,
            "token": self.__token,
            "params": kwargs,
            "fields": fields,
            "_cache": self._build_cache(api_name),
        }
        res = requests.post(f"{self.__http_url}", json=req_params, timeout=self.__timeout)
        if res:
            result = json.loads(res.text)
            if result["code"] != 0:
                raise Exception(result["msg"])
            data = result["data"]
            columns = data["fields"]
            items = data["items"]
            return pd.DataFrame(items, columns=columns)
        return pd.DataFrame()

    def __getattr__(self, name: str):
        return partial(self.query, name)

    def _build_cache(self, api_name: str) -> dict[str, Any]:
        now_ts = int(self._now().timestamp())
        expires_at = int(self._next_refresh_dt(api_name).timestamp())
        return {
            "namespace": self._qualify_namespace(api_name),
            "ttl": max(expires_at - now_ts, 0),
            "expires_at": expires_at,
            "no_cache": False,
        }

    def _next_refresh_dt(self, api_name: str) -> datetime.datetime:
        now = self._now()
        refresh_hour = SPECIAL_REFRESH_HOURS.get(api_name.lower(), DEFAULT_REFRESH_HOUR)
        candidate_date = now.date()
        while True:
            if candidate_date.weekday() >= 5:
                candidate_date = self._next_trade_day(candidate_date)
                continue

            candidate_dt = datetime.datetime.combine(
                candidate_date,
                datetime.time(hour=refresh_hour, minute=0),
                tzinfo=BEIJING_TZ,
            )
            if candidate_dt > now:
                return candidate_dt
            candidate_date = self._next_trade_day(candidate_date)

    def _next_trade_day(self, day: datetime.date) -> datetime.date:
        candidate = day + datetime.timedelta(days=1)
        while candidate.weekday() >= 5:
            candidate += datetime.timedelta(days=1)
        return candidate

    def _now(self) -> datetime.datetime:
        return datetime.datetime.now(tz=BEIJING_TZ)

    def _qualify_namespace(self, namespace: str) -> str:
        prefix = f"{PROJECT_CACHE_NAMESPACE}:"
        return namespace if namespace.startswith(prefix) else f"{prefix}{namespace}"
