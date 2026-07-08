# OpenModelPool P2P 网络发现与 Gateway 架构设计

> 版本：v2.0 | 2026-07-08

## 1. 核心思路

借鉴 BT（BitTorrent）网络的互惠共享理念 + 比特币 P2P 网络的节点发现机制，设计一套**互惠共享、渐进去中心化**的算力共享网络。

### 核心理念：像 BT 一样共享，不收过路费

BT 网络中，每个节点分享自己拥有的数据片段给别人，也从别人那里获取自己缺少的片段。没有"过路费"，激励来自互惠——你分享得越多，能获取的也越多。

OpenModelPool 同理：

```
Node A 有 gpt-4o，Node B 有 claude-3
     │                          │
     └──── 加入共享网络 ────────┘
                │
     双方共享资源池，互相免费使用对方的算力
     转发请求不收过路费，跟 BT 传数据一样
```

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
| **Gateway Node** | 绑定固定域名 + 标记 `is_gateway: true` | 请求代理 + 路由转发（全路由节点） | 随网络增长 |
| **Regular Node** | 加入共享网络，无固定域名 | 提供算力 + 参与 gossip | 不限 |
| **Solo Node** | 独立运行，不加入网络 | 自用 | 不限 |

**关键设计**：
- Seed Node 一定是 Gateway Node，但 Gateway Node 不一定是 Seed Node。Seed 是 Gateway 的一个特殊子集。
- **每个 Gateway 都是全路由节点**：加入网络后，不仅能处理本地模型，还能路由全网所有模型请求。
- 用户直接用 Gateway 的固定域名作为 base URL，就能访问全网资源。

---

## 3. 互惠共享模型（BT 模式）

### 3.1 核心理念

```
传统思路：我贡献算力 → 别人付费使用 → 我赚钱
BT 模式：我贡献算力 → 别人免费使用 → 我也免费使用别人的算力
```

**没有过路费，没有算力交易市场。** 就是一个互惠互助的算力共享网络。

### 3.2 激励来自互惠

| 贡献 | 权益 |
|------|------|
| 不加入网络 | 只能用自己本地的 Provider Key |
| 加入，贡献 30% 额度到共享池 | 访问全网共享资源池 |
| 贡献 50% 额度 | 同上，但能为网络贡献更多，吸引更多节点加入 |

**激励不是"赚回多少"，而是"能用多少"：**
- 你贡献了 gpt-4o 的算力 → 你能用上别人的 claude-3、gemini 等
- 你贡献的算力越多越稳定 → 网络中可用模型越丰富、响应越快
- 类似 BT：做种越多 → 下载速度越快、可获取资源越多

### 3.3 转发成本谁承担？

跟 BT 一样——**谁提供算力谁承担**：

```
用户请求 claude-3，Node A 没有 → 转发到 Node B

Node B（Provider）：
  - 用自己的 Provider Key 调 Claude API
  - 消耗的是自己贡献给共享池的额度
  - 不向 Node A 收费，不向用户收费
  
Node A（Gateway）：
  - 只是帮忙转发，不额外消耗
  - 享受的是 Node B 也可能转发请求到 Node A 的模型
```

### 3.4 额度模型

沿用已有的额度分配模型：

```
节点配置：
  - X% 额度 → 消费者（自己的 API Key 用户）
  - Y% 额度 → 共享网络（全网用户可用）

X + Y = 100%
```

当共享网络中的请求消耗你的 Provider 额度时，消耗的是你分配的 Y% 部分。这就是你为享受全网资源所付出的"成本"——不是钱，是你的算力贡献。

---

## 4. 全路由节点设计

### 4.1 核心概念

每个加入网络的节点，都可以成为全网入口：

```
用户 → https://my-node.chal.cc/v1（自己的固定域名）
         │
         ▼
    my-node 收到请求
         │
         ├── 本地有该模型？ → 直接处理
         │
         └── 本地没有？ → 查路由表，转发到最优节点
                          → 对用户完全透明
```

**用户视角**：
```python
# 用自己的节点地址，也能访问全网模型
client = OpenAI(
    base_url="https://my-node.chal.cc/v1",
    api_key="sk-my-key"
)

# 调 gpt-4o → 本地处理（自己有 Provider Key）
# 调 claude-3 → 自动转发到 node-bob.com，用户无感知
# 调 gemini  → 自动转发到 gpu.dave.io，用户无感知
```

### 4.2 路由表

每个 Gateway 节点维护一份全网路由表，通过 Gossip 同步：

```json
{
  "routes": {
    "gpt-4o": [
      { "node_id": "mmx-aaa", "url": "https://my-node.chal.cc", "local": true, "score": 0.95 },
      { "node_id": "mmx-bbb", "url": "https://node.bob.com", "local": false, "score": 0.82 }
    ],
    "claude-3-opus": [
      { "node_id": "mmx-ccc", "url": "https://gpu.dave.io", "score": 0.91 }
    ]
  }
}
```

### 4.3 路由决策优先级

```
1. 本地有该模型 → 直接处理（成本最低）
2. 本地没有 → 查路由表，选最优远程节点：
   a. 优先选 延迟最低 + 在线率最高的节点
   b. 同级时随机选择（负载均衡）
3. 转发请求，返回结果给用户
```

### 4.4 认证与请求转发

```
用户 (sk-xxxx) → Node A (Gateway) → Node B (Provider) → Provider API
                        │                    │
                        └─ 内部转发认证 ──────┘
                           - 节点间通过 P2P 握手建立的信任
                           - 转发时附加: 来源节点、请求追踪 ID
                           - 不暴露用户的原始 Key
```

**最小信任原则**：Gateway 只做路由和转发，不解析请求内容，不窃取 API Key。

---

## 5. Phase 0：冷启动方案

### 5.1 GitHub 注册表

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

### 5.2 官方 Seed 节点

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

### 5.3 Seed 节点运维规则

- **最低在线率**：99.5%（月度）
- **心跳检查**：官方监控服务每 5 分钟检查 Seed 健康状态
- **DNS 更新**：Seed IP 变化时自动更新 DNS A 记录
- **退役条件**：当网络中 Gateway 节点数量 ≥ 10 且平均在线率 > 95% 时，逐步减少 Seed

---

## 6. 节点发现协议

### 6.1 Gossip 协议设计

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

### 6.2 地址管理器（AddrMan）

每个节点维护一个本地地址管理器，类似比特币的 `addrman`：

```go
type AddrMan struct {
    Known    map[string]*PeerInfo  // 已知节点
    Gateways []*PeerInfo           // Gateway 节点子集（按 score 排序）
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

### 6.3 启动时的发现流程

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

## 7. DNS 与统一入口寻址

### 7.1 两类用户的寻址方式

#### 纯消费者（只用 mk_public_v1）

```
用户 → DNS 解析 api.openmodelpool.com
     → 拿到 Gateway IP 列表（DNS 轮询）
     → 连接任意一个 Gateway
     → 用 mk_public_v1 发请求
     → Gateway 路由到最优 Provider
     → 返回结果
```

用户不需要知道背后有哪些节点，只需要一个 URL。

#### 节点运营者（用自己的固定域名）

```
用户 → https://my-node.chal.cc/v1
     → 自己的节点
     → 本地有的模型：直接处理
     → 本地没有的：转发到全网
     → 用户体验与全球统一入口完全一致
```

### 7.2 DNS 记录设计

```
; Seed 节点（固定，官方管理）
seed1.openmodelpool.com.  A  1.2.3.4
seed2.openmodelpool.com.  A  5.6.7.8
seed3.openmodelpool.com.  A  9.10.11.12

; 统一入口（动态，指向所有活跃 Gateway）
api.openmodelpool.com.    A  1.2.3.4
                          A  5.6.7.8
                          A  9.10.11.12
                          A  203.0.113.1   ; 用户 Gateway C
                          A  198.51.100.5  ; 用户 Gateway D

; TTL 设置
$TTL 300  ; 5 分钟，确保节点下线后快速生效
```

### 7.3 DNS 自动更新服务

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

初期 DNS Manager 运行在 Seed 节点上（中心化依赖），随网络自治逐步由多节点共同维护。

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
- [ ] 鼓励早期用户在 GitHub 注册自己的节点域名

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
- [ ] 实现全路由：Gateway 转发请求到全网节点
- [ ] DNS Manager 自动从 Gateway 列表更新 DNS
- [ ] GitHub 注册表作为 Gossip 的补充 bootstrap

节点发现：GitHub + Gossip 混合
路由：每个 Gateway 独立维护路由表
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
- [ ] 探索纯 P2P 寻址（DHT 替代 DNS）

节点发现：纯 Gossip
路由：分布式路由表
DNS：多节点共同维护 → 最终可能过渡到 DHT
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

### P2（DNS 自动化）

| 模块 | 文件 | 说明 |
|------|------|------|
| DNS Manager | 独立服务 | 自动更新 DNS A 记录 |
| Gateway 健康检查 | DNS Manager | 定期检查 Gateway 可达性 |

---

## 10. 安全考量

### 10.1 恶意节点防护

| 风险 | 防御 |
|------|------|
| 虚假节点注册 | PING-PONG 握手验证可达性 |
| 伪造模型列表 | 请求失败后降低 uptime_score，多次失败后摘除 |
| Gateway 拒绝服务 | 请求超时后自动切换下一个节点 |
| 路由投毒（gossip 广播恶意节点） | 每个节点独立验证，不盲目信任 peer 信息 |
| DDoS 攻击 Gateway | 限流 + Cloudflare 防护 |
| 白嫖（不贡献只想用） | 额度分配模型已限制：共享池额度用尽即止 |

### 10.2 信任模型

```
用户信任 Gateway → Gateway 不篡改请求内容（纯透传）
                  → Gateway 不窃取 API Key（Key 由目标节点验证）
                  
节点信任 peer 的模型声明 → 实际请求时验证
                         → 失败后自动降级
```

**最小信任原则**：Gateway 只做路由和转发，不解析请求内容。

---

## 附录 A：与 BT 网络类比

| 维度 | BT 网络 | OpenModelPool |
|------|--------|---------------|
| 共享什么 | 数据片段 | 算力（模型调用额度） |
| 激励 | 互惠：分享越多，下载越快 | 互惠：贡献越多，可用模型越多 |
| 收费 | 不收费 | 不收过路费 |
| 节点发现 | DHT + Tracker | Gossip + GitHub 注册表 + DNS Seeds |
| 地址解析 | info hash → peer 列表 | 模型名 → Provider 节点列表 |
| 冷启动 | Tracker（中心化） | Seed 节点 + DNS（中心化） |
| 成熟后 | 纯 DHT（去中心化） | 纯 Gossip（去中心化） |
| 地址持久化 | .torrent 文件 + DHT 缓存 | peers.dat |

## 附录 B：用户视角

### 纯消费者

```python
# 使用全球统一入口
client = OpenAI(
    base_url="https://api.openmodelpool.com/v1",
    api_key="mk_public_v1"  # 全球公共 Key，零门槛
)
```

### 节点运营者（用自己的域名）

```python
# 用自己的固定域名，也能访问全网
client = OpenAI(
    base_url="https://my-node.chal.cc/v1",
    api_key="sk-my-key"  # 自己的 Proxy API Key
)
# 调 gpt-4o → 本地处理
# 调 claude-3 → 自动转发，用户无感知
```

### 节点运营者（部署步骤）

```bash
# 1. 部署节点
./openmodelpool

# 2. 绑定域名（假设已购买 ai.example.com）
# 在 Cloudflare/域名商设置 A 记录指向服务器 IP

# 3. 在管理面板开启 Gateway + 加入共享网络
# Settings → 网络设置 → 开启 "作为 Gateway 节点"
# Settings → 共享网络 → 开启，设置额度分配比例

# 4. 完成！节点自动加入网络
# - 自动发现其他节点
# - 自己的域名成为全网入口之一
# - 为网络贡献算力，享受全网资源
```
