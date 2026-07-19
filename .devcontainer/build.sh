#!/bin/bash
# build.sh — Codespaces postCreateCommand：
#   1. 装 chromium + 字体（browser-login 特性用，best-effort，不阻塞启动）
#   2. 编译 openmodelpool 到 /usr/local/bin/openmodelpool
#   3. 起一个 setsid 常驻的诊断日志 HTTP 服务（8002），暴露 /tmp 下的启动日志
set -u

REPO="/workspaces/openmodelpool"
LOG="/tmp/openmodelpool.log"

echo "[build] repo: $REPO"

# 1) 浏览器依赖（best-effort）
echo "[build] installing chromium + fonts (best-effort) ..."
sudo apt-get update -qq >/dev/null 2>&1 || true
sudo apt-get install -y -qq chromium fonts-liberation fonts-noto-cjk >/dev/null 2>&1 || \
  echo "[build] ⚠️ chromium install skipped/failed (browser-login may be unavailable)"

# 2) 编译
echo "[build] go build -> /usr/local/bin/openmodelpool (log: /tmp/omp-build.log)"
cd "$REPO" || { echo "[build] ❌ cannot cd $REPO"; exit 1; }
go build -o /usr/local/bin/openmodelpool . > /tmp/omp-build.log 2>&1
if [ $? -ne 0 ]; then
  echo "[build] ❌ go build FAILED. Last 50 lines of /tmp/omp-build.log:"
  tail -n 50 /tmp/omp-build.log
  exit 1
fi
echo "[build] ✅ build ok"

# 3) 诊断日志服务（setsid 常驻，脱离 postCreateCommand 生命周期）
echo "[build] starting diagnostics log server on :8002 ..."
sudo apt-get install -y -qq python3 >/dev/null 2>&1 || true
if command -v python3 >/dev/null 2>&1; then
  setsid bash -c 'cd /tmp && python3 -m http.server 8002 --bind 0.0.0.0 >/tmp/omp-logserver.log 2>&1' </dev/null >/dev/null 2>&1 &
  echo "[build] ✅ diagnostics log server up on :8002 (serves /tmp)"
else
  echo "[build] ⚠️ python3 unavailable, diagnostics log server skipped"
fi

echo "[build] done. openmodelpool will be launched as the container main process."
