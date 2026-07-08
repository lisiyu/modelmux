# OpenModelPool P2P 网络发现与 Gateway 架构设计

> 版本：v1.0 | 2026-07-08

## 1. 核心思路

借鉴比特币 P2P 网络的发现机制，设计一套**渐进去中心化**的节点发现与统一入口体系。

### 整体演进路径

```
Phase 0（冷启动）     Phase 1（网络形成）       Phase 2（自治网络）
━━━━━━━━━━━━━━━━     ━━━━━━━━━━━━━━━━━━      ━━━━━━━━━━━━━━━━━
官方固定 Seed 节点    Seed 节点 + Gossip 发现    完全自治
GitHub 注册表引导     新节点自动加入 Gateway     固定 Seed 逐步退役
用户手动注册域名      Gateway 池自然扩大         DNS 记录由网络维护

用户 → 固定 Seed URL   用户 → Seed + Gateway     用户 → 任一 Gateway
```

---

## 2. 节点角色定义

| 角色 | 条件 | 职责 | 数量预期 |
|------|------|------|---------|
| **Seed Node** | 绑定固定域名 + 标记 `is_seed: true` | 冷启动入口 + 节点发现 + 请求路由 | 初始 3-5 个 |
| **Gateway Node** | 绑定固定域名 + 标记 `is_gateway: true` | 请求代理 + 路由转发 | 随网络增长 |
| **Regular Node** | 加入共享网络，无固定域名 | 提供算力 + 参与 gossip | 不限 |
| **Solo Node** | 独立运行，不加入网络 | 自用 | 不限 |

**关键设计**：Seed Node 一定是 Gateway Node，但 Gateway Node 不一定是 Seed Node。Seed 是 Gateway 的一个特殊子集。

---

## 3. Phase 0：冷启动方案

### 3.1 GitHub 注册表

保留并扩展现有的 `.nodes` 注册表机制，增加 gateway/seed 标记：

```json
// .nodes/{node_id}.json
{
  "node_id": "mmx-xxxx",
  "name": "Chal's Node",
  "url": "https://ai.chal.cc",
  "models": ["gpt-4o", "claude-3-opus", "qwen-72b"],
  "region": "us-east",
  "is_gateway": true,
  "is_seed": true,
  "registered_at": "2026-07-08T16:00:00Z",
  "last_heartbeat": "2026-07-08T16:05:00Z",
  "version": "4.0.0"
}
```

### 3.2 官方 Seed 节点

冷启动期间，官方运营 3 个 Seed 节点：

```
seed1.openmodelpool.com  →  官方节点 A（如 AWS us-east）
seed2.openmodelpool.com  →  官方节点 B（如 GCP eu-west）
seed3.openmodelpool.com  →  官方节点 C（如 阿里云 ap-east）
```

**统一入口域名**（DNS 指向所有 Seed + Gateway）：

```
api.openmodelpool.com  →  [seed1 IP, seed2 IP, seed3 IP, gateway1 IP, ...]
```

用户只需配置：
```
base_url = https://api.openmodelpool.com
api_key  = sk-xxxx（Proxy API Key）
```

### 3.3 Seed 节点运维规则

- **最低在线率**：99.5%（月度）
- **心跳检查**：官方监控服务每 5 分钟检查 Seed 健康状态
- **DNS 更新**：Seed IP 变化时自动更新 DNS A 记录
- **退役条件**：当网络中 Gateway 节点数量 ≥ 10 且平均在线率 > 95% 时，可逐步减少 Seed

---

## 4. 节点发现协议

### 4.1 Gossip 协议设计

借鉴比特币的 `addr` 消息机制：

```
节点A ──── PING ──────────── 节点B
节点A ──── get_peers ──────→ 节点B
节点A ←── peers (N 个节点信息) ── 节点B
```

#### 协议消息类型

| 消息 | 方向 | 内容 | 频率 |
|------|------|------|------|
| `PING` | 双向 | 节点 ID + 版本 + 时间戳 | 每 30s |
| `PONG` | 回复 | 确认 + 时间戳 | 收到 PING 即回 |
| `GET_PEERS` | 请求方 | 已知模型列表（可选过滤） | 每 5min |
| `PEERS` | 响应方 | 已知节点列表（最多 50 个） | 收到请求即回 |
| `ANNOUNCE` | 广播 | 自身信息（ID, URL, models, is_gateway） | 加入时 + 每 10min |

#### PEERS 消息体

```json
{
  "peers": [
    {
      "node_id": "mmx-xxxx",
      "url": "https://ai.chal.cc",
      "is_gateway": true,
      "models": ["gpt-4o", "claude-3-opus"],
      "last_seen": 1720454400,
      "latency_ms": 45,
      "uptime_score": 0.98
    }
  ]
}
```

### 4.2 地址管理器（AddrMan）

每个节点维护一个本地地址管理器，类似比特币的 `addrman`：

```go
type AddrMan struct {
    Known    map[string]*PeerInfo  // 已知节点
    Gateways []*PeerInfo           // Gateway 节点子集
    Seeds    []*PeerInfo           // Seed 节点子集
    LastSync time.Time             // 上次 gossip 同步时间
}

type PeerInfo struct {
    NodeID      string   `json:"node_id"`
    URL         string   `json:"url"`
    IsGateway   bool     `json:"is_gateway"`
    IsSeed      bool     `json:"is_seed"`
    Models      []string `json:"models"`
    LastSeen    int64    `json:"last_seen"`    // Unix timestamp
    LatencyMs   int      `json:"latency_ms"`
    UptimeScore float64  `json:"uptime_score"` // 0.0 ~ 1.0
    FailCount   int      `json:"fail_count"`    // 连续失败次数
}
```

**维护规则**：
- 节点 30 分钟无响应 → `fail_count++`
- `fail_count >= 3` → 标记为不可达，不参与路由
- 每 5 分钟 gossip 同步 → 从 peer 获取新节点
- 每 30 分钟清理 → 移除 7 天未见的节点

### 4.3 启动时的发现流程

```
节点启动
  │
  ├── 1. 读取本地 peers.dat（上次缓存的节点列表）
  │
  ├── 2. 缓存不足？查 GitHub 注册表
  │      GET https://raw.githubusercontent.com/.../.nodes/index.json
  │      → 获取所有已注册节点
  │
  ├── 3. 连接已发现的节点
  │      → 逐个 PING，验证可达性
  │
  ├── 4. 发送 GET_PEERS 给可达节点
  │      → 获取更多 peer → 连接 → 继续扩散
  │
  └── 5. 发送 ANNOUNCE 广播自身信息
         → 告知邻居"我来了"
```

---

## 5. Gateway 路由机制

### 5.1 路由表

Gateway 节点维护一份**全网路由表**，用于决定请求转发目标：

```json
{
  "routes": {
    "gpt-4o": [
      { "node_id": "mmx-aaa", "url": "https://ai.chal.cc", "score": 0.95 },
      { "node_id": "mmx-bbb", "url": "https://node.bob.com", "score": 0.82 }
    ],
    "claude-3-opus": [
      { "node_id": "mmx-ccc", "url": "https://gpu.dave.io", "score": 0.91 }
    ],
    "qwen-72b": [
      { "node_id": "mmx-aaa", "url": "https://ai.chal.cc", "score": 0.95 },
      { "node_id": "mmx-ddd", "url": "https://llm.eve.net", "score": 0.78 }
    ]
  }
}
```

**score 计算**：
```
score = uptime_score × (1 / avg_latency) × availability_weight
```

### 5.2 请求转发流程

```
用户请求（到任意 Gateway）
  │
  ├── 1. 认证：验证 API Key（Proxy API Key / Guest Key / mk_public_v1）
  │
  ├── 2. 查路由表：该模型有哪些节点提供？
  │
  ├── 3. 选节点：score 最高的节点
  │      → 如果该 Gateway 自己就有？直接处理
  │      → 否则转发
  │
  ├── 4. 转发请求
  │      → 添加 X-Forwarded-For 等追踪头
  │      → 记录请求日志（用于额度结算）
  │
  └── 5. 返回结果给用户
```

### 5.3 Gateway 间协作

当多个 Gateway 同时存在时：

```
                    ┌─── Gateway A (seed1.openmodelpool.com)
                    │
用户 ── DNS ────────┼─── Gateway B (seed2.openmodelpool.com)
  ↑                 │
  │                 ├─── Gateway C (ai.chal.cc, 用户节点)
  │                 │
  │                 └─── Gateway D (node.bob.com, 用户节点)
  │
  └── api.openmodelpool.com
      (DNS A 记录指向所有 Gateway IP)
```

- 每个 Gateway 独立维护路由表（通过 gossip 同步）
- DNS 轮询分配流量
- 某个 Gateway 挂了 → DNS TTL 过期后自动摘除
- Gateway 之间**不互相转发**（避免无限循环），只转发给 Regular Node

---

## 6. DNS 管理

### 6.1 DNS 记录设计

```
; Seed 节点（固定，官方管理）
seed1.openmodelpool.com.  A  1.2.3.4
seed2.openmodelpool.com.  A  5.6.7.8
seed3.openmodelpool.com.  A  9.10.11.12

; 统一入口（动态，指向所有活跃 Gateway）
api.openmodelpool.com.    A  1.2.3.4
                          A  5.6.7.8
                          A  9.10.11.12
                          A  203.0.113.1   ; Gateway C
                          A  198.51.100.5  ; Gateway D

; TTL 设置
$TTL 300  ; 5 分钟，确保节点下线后快速生效
```

### 6.2 DNS 自动更新服务

```
┌─────────────────────────────────────┐
│         DNS Manager Service          │
│                                      │
│  1. 定期扫描 Gateway 列表            │
│  2. 检查每个 Gateway 健康状态        │
│  3. 可达 → 加入 DNS A 记录          │
│  4. 不可达 → 从 DNS 摘除            │
│  5. 通过 DNS API (Cloudflare 等)    │
│     自动更新记录                      │
└─────────────────────────────────────┘
```

这个 DNS Manager 本身可以运行在 Seed 节点上，是一个**中心化依赖**，但随着网络自治，可以逐步由多节点共同维护。

---

## 7. 额度激励：Gateway 过路费

### 7.1 动机问题

Gateway 节点承担了流量转发成本（带宽 + 计算），需要有激励：

```
用户请求 → Gateway 转发 → 目标节点处理
              │                    │
           消耗带宽            消耗算力
           应该获得激励       应该获得额度
```

### 7.2 激励模型

```
每次请求的额度分配：

  原始额度：100 units
  
  Gateway 过路费：10%（即 10 units）→ 给 Gateway 节点
  提供者收益：  90%（即 90 units）→ 给实际提供算力的节点
  
  如果 Gateway == Provider（请求直接在本节点处理）：
  全部 100 units → 本节点
```

### 7.3 过路费追踪

```json
{
  "request_id": "req-xxxx",
  "gateway_node": "mmx-seed1",
  "provider_node": "mmx-aaa",
  "quota_total": 100,
  "quota_gateway": 10,
  "quota_provider": 90,
  "timestamp": "2026-07-08T16:30:00Z"
}
```

每个 Gateway 本地记录过路费流水，定期与其他节点对账（可选）。

---

## 8. 渐进去中心化路线图

### Phase 0：冷启动（第 1-3 个月）

```
目标：建立初始网络，验证核心流程

- [ ] 注册域名（openmodelpool.com）
- [ ] 部署 3 个官方 Seed 节点
- [ ] 配置 DNS 记录（seed1/2/3 + api）
- [ ] 搭建 DNS Manager 服务
- [ ] 在 GitHub 发布节点注册引导
- [ ] 鼓励早期用户注册节点域名

节点发现：GitHub 注册表（中心化）
路由：Seed 节点直接转发
DNS：官方手动/半自动管理
```

### Phase 1：网络形成（第 3-6 个月）

```
目标：实现 Gossip 节点发现，Gateway 自动加入

- [ ] 实现 Gossip 协议（PEERS/ANNOUNCE）
- [ ] 实现 AddrMan（本地节点管理）
- [ ] 节点绑定域名后自动注册为 Gateway
- [ ] DNS Manager 自动从 Gateway 列表更新 DNS
- [ ] 实现 Gateway 请求转发 + 过路费
- [ ] GitHub 注册表作为 Gossip 的补充 bootstrap

节点发现：GitHub + Gossip 混合
路由：Gateway 间独立路由表
DNS：半自动（DNS Manager 服务管理）
```

### Phase 2：自治网络（6 个月+）

```
目标：移除中心化依赖，网络完全自治

- [ ] Seed 节点降级为普通 Gateway（不再特殊对待）
- [ ] DNS 由多个 Gateway 共同维护（去中心化 DNS）
- [ ] 节点发现完全依赖 Gossip（GitHub 注册表仅作备用）
- [ ] Gateway 选举：uptime + models + latency 综合评分
- [ ] 新 Gateway 自动加入 DNS 池

节点发现：纯 Gossip
路由：分布式路由表
DNS：多节点共同维护
```

---

## 9. 代码实现优先级

### P0（冷启动必需）

| 模块 | 文件 | 说明 |
|------|------|------|
| 节点注册表扩展 | `.nodes/*.json` | 增加 `is_gateway`/`is_seed` 字段 |
| Gateway 标记 | `admin.html` | 节点设置中增加 Gateway 开关 |
| 路由表 | `network_relay.go` | 按模型查询可用节点 |
| 请求转发 | `network_relay.go` | Gateway 将请求转发给目标节点 |

### P1（网络发现）

| 模块 | 文件 | 说明 |
|------|------|------|
| Gossip 协议 | `network_discovery.go`（新文件） | PING/PONG/GET_PEERS/PEERS/ANNOUNCE |
| AddrMan | `network_discovery.go` | 本地节点列表管理 |
| peers.dat | `network_discovery.go` | 节点列表持久化 |
| GitHub bootstrap | `network_discovery.go` | 启动时从注册表拉取初始节点 |

### P2（DNS 与激励）

| 模块 | 文件 | 说明 |
|------|------|------|
| DNS Manager | 独立服务 | 自动更新 DNS A 记录 |
| 过路费计算 | `credits.go` | Gateway 转发时的额度分配 |
| 过路费追踪 | `credits.go` | 记录流水 |

---

## 10. 安全考量

### 10.1 恶意节点防护

| 风险 | 防御 |
|------|------|
| 虚假节点注册 | 需要 PING-PONG 握手验证可达性 |
| 伪造模型列表 | 请求失败后降低 uptime_score，多次失败后摘除 |
| Gateway 拒绝服务 | 请求超时后自动切换下一个节点 |
| 路由投毒（gossip 中广播恶意节点） | 每个节点独立验证，不盲目信任 peer 传来的信息 |
| DDoS 攻击 Gateway | 限流 + Cloudflare 防护 |

### 10.2 信任模型

```
用户信任 Gateway → Gateway 不篡改请求内容（纯透传）
                  → Gateway 不窃取 API Key（Key 由目标节点验证）
                  
节点信任 peer 的模型声明 → 实际请求时验证
                         → 失败后自动降级
```

**最小信任原则**：Gateway 只做路由和转发，不解析请求内容（和当前代理模式一致）。

---

## 附录 A：与比特币对比

| 维度 | 比特币 | OpenModelPool |
|------|--------|---------------|
| 发现方式 | DNS Seeds + Gossip | GitHub 注册表 + Gossip |
| 节点验证 | PoW（工作量证明） | 实际请求验证（可用性证明） |
| 路由 | 无（广播所有交易） | 按模型路由（内容感知） |
| 激励 | 区块奖励 + 手续费 | 算力额度 + Gateway 过路费 |
| 去中心化程度 | 完全 | 渐进式（Seed → Gateway → 自治） |
| 地址持久化 | peers.dat | peers.dat（相同设计） |

## 附录 B：用户视角

### 节点运营者

```bash
# 1. 部署节点
./openmodelpool

# 2. 绑定域名（假设已购买 ai.example.com）
# 在 Cloudflare/域名商设置 A 记录指向服务器 IP

# 3. 在管理面板开启 Gateway
# Settings → 网络设置 → 开启 "作为 Gateway 节点"

# 4. 完成！节点自动加入网络
# - 自动发现其他节点
# - 自动出现在 DNS 轮询池中
# - 获得 Gateway 过路费激励
```

### API 使用者

```python
# 只需配置一个 URL
client = OpenAI(
    base_url="https://api.openmodelpool.com/v1",
    api_key="sk-xxxx"  # Proxy API Key
)

# 自动路由到最优节点
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello"}]
)
```
