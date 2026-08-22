#!/usr/bin/env python3
# Freedom 进程后端示例：Python 实现（纯标准库）。
#
# 协议与 Go/Node 后端完全一致：stdin 接收请求、stdout 返回响应/推送事件。
# 壳通过 `python backends/py_backend.py` 启动本进程。
import json
import sys
import threading
import time


def emit_tick():
    """后台线程定时推送 tick 事件。"""
    count = 0
    while True:
        count += 1
        line = json.dumps({
            "event": "tick",
            "data": {
                "time": time.strftime("%H:%M:%S"),
                "count": count,
                "lang": "Python",
            },
        })
        sys.stdout.write(line + "\n")
        sys.stdout.flush()
        time.sleep(2)


threading.Thread(target=emit_tick, daemon=True).start()


def handle(req):
    method = req.get("method")
    params = req.get("params") or []
    if method == "Greet":
        name = params[0] if params else ""
        if not name:
            return {"id": req["id"], "error": "名字不能为空"}
        return {"id": req["id"], "result": f"Hello, {name}! (Python backend)"}
    if method == "Add":
        if len(params) < 2:
            return {"id": req["id"], "error": "Add 需要两个数字参数"}
        return {"id": req["id"], "result": params[0] + params[1]}
    if method == "WhoAmI":
        return {"id": req["id"], "result": "Python"}
    return {"id": req["id"], "error": f"unknown method {method}"}


for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
    except json.JSONDecodeError:
        continue
    resp = handle(req)
    sys.stdout.write(json.dumps(resp) + "\n")
    sys.stdout.flush()
