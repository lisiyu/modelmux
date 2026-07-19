#!/bin/bash
# OpenModelPool Codespaces 初始化：装浏览器依赖、构建并以后台常驻方式启动服务。
#
# 注意：postCreateCommand 所在的 shell 退出时，普通 nohup 后台进程会被回收
# （它仍属于 postCreateCommand 的会话/进程组），导致端口 8000 永远不监听。
# 因此这里改用 setsid 把 supervisor 放到独立的会话里，使其脱离 postCreateCommand
# 的生命周期；supervisor 内部用 while true 循环守护 openmodelpool，异常退出后 3s 重启。
# （openmodelpool 自身把 SIGHUP 当作配置热重载，故此处不需要 nohup 保护。）

# 动态计算仓库根目录（setup.sh 位于 .devcontainer/ 下），并导出供 supervisor 使用。
OMP_REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export OMP_REPO

echo "[devcontainer] repo root: $OMP_REPO"
cd "$OMP_REPO"

# 诊断：尽早起一个 HTTP 日志服务（端口 8002），即使后续构建/启动失败也能从外部
# 读取 /tmp 下的启动日志（Codespaces 会自动转发任何监听端口）。
echo "[devcontainer] installing python3 for diagnostics log server ..."
sudo apt-get update -qq 2>/dev/null || true
sudo apt-get install -y -qq python3 >/dev/null 2>&1 || true
if command -v python3 >/dev/null 2>&1; then
  setsid bash -c 'cd /tmp && python3 -m http.server 8002 --bind 0.0.0.0 >/tmp/omp-logserver.log 2>&1' </dev/null >/dev/null 2>&1 &
  echo "[devcontainer] ✅ diagnostics log server started on :8002 (serves /tmp)"
else
  echo "[devcontainer] ⚠️ python3 unavailable, skipping diagnostics log server"
fi

echo "[devcontainer] installing chromium + fonts ..."
sudo apt-get update -qq 2>/dev/null || true
sudo apt-get install -y -qq chromium fonts-liberation fonts-noto-cjk 2>/dev/null || true

echo "[devcontainer] building openmodelpool ... (build log -> /tmp/omp-build.log)"
go build -o openmodelpool . > /tmp/omp-build.log 2>&1
if [ $? -ne 0 ]; then
  echo "[devcontainer] ❌ go build failed, aborting. Last 40 lines of /tmp/omp-build.log:"
  tail -n 40 /tmp/omp-build.log
  exit 1
fi
echo "[devcontainer] ✅ build ok"

# 写 supervisor 脚本到 /tmp。使用 quoted heredoc，使 $(date) 与 $? 在文件内保持字面量；
# OMP_REPO 通过已导出的环境变量传入（setsid 会继承当前环境）。
cat > /tmp/omp-supervise.sh <<'EOF'
#!/bin/bash
cd "$OMP_REPO"
while true; do
  echo "[supervise] starting openmodelpool at $(date)"
  ./openmodelpool >> /tmp/openmodelpool.log 2>&1
  echo "[supervise] openmodelpool exited ($?) at $(date), restarting in 3s"
  sleep 3
done
EOF
chmod +x /tmp/omp-supervise.sh

echo "[devcontainer] launching supervisor (detached via setsid) ..."
setsid bash /tmp/omp-supervise.sh >/dev/null 2>&1 < /dev/null &

echo "[devcontainer] waiting for /health (up to ~40s) ..."
OK=0
for i in $(seq 1 20); do
  code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8000/health 2>/dev/null)
  if [ "$code" = "200" ]; then
    OK=1
    break
  fi
  sleep 2
done

if [ "$OK" = "1" ]; then
  echo "[devcontainer] ✅ openmodelpool is up. First run: open /setup to configure the admin account."
else
  echo "[devcontainer] ⚠️ openmodelpool did not respond on /health within timeout. Check /tmp/openmodelpool.log:"
  tail -n 50 /tmp/openmodelpool.log
fi
