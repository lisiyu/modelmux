#!/bin/bash
# run-omp.sh — Codespaces 主进程：以 supervisor 循环守护 openmodelpool。
#
# 作为 devcontainer 的 `command` 运行，即容器的主进程：
#   - 前台运行（不会被 postCreateCommand 退出回收）
#   - 崩溃后 3s 自动重启，保证 codespace 始终可用、可排查
#   - 所有输出落盘到 /tmp/openmodelpool.log（可由 8002 诊断服务读取）
set -u

REPO="/workspaces/openmodelpool"
BIN="/usr/local/bin/openmodelpool"
LOG="/tmp/openmodelpool.log"

cd "$REPO" || { echo "[run-omp] cannot cd to $REPO" >> "$LOG"; exit 1; }

while true; do
  echo "[run-omp] starting openmodelpool at $(date)" >> "$LOG"
  "$BIN" >> "$LOG" 2>&1
  code=$?
  echo "[run-omp] openmodelpool exited ($code) at $(date), restarting in 3s" >> "$LOG"
  sleep 3
done
