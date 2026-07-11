# OpenModelPool 架构审计报告：需求文档 vs 实现偏差分析

> **完成日期**: 2026-07-09（初始审计） / 2026-07-11（v4.0.1 更新）
> **代码基线**: modelmux-backup (26,166 行 Go 代码, 50+ 源文件)
> **审计基线**: OpenModelPool v4.0.1 设计文档
> **审计范围**: 9 份设计/需求文档 + ARCHITECTURE_CN.md vs 实际代码实现 + v4.0.1 设计变更审计

---

## 执行摘要

对 OpenModelPool 项目的 9 份需求/设计文档与实际代码的逐项比对发现：**核心代理层和 P2P Relay 功能实现完整度较高**，但密钥体系、经济模型、网络发现三大模块存在**设计文档间相互矛盾**和**设计与实现严重偏离**的双重问题。密钥体系 v2.0 设计（01-密钥体系v2.0设计.md）明确提出 4 种 Key 类型并废弃旧类型，但代码中仍保留全部 6 种旧 Key 类型（`mk_trial_`、`mk_open_`、`mk_open_global_` 等）及完整实现；P2P 架构设计（02-P2P架构设计.md）描述了 Kademlia DHT + GossipSub + 洋葱路由，但实际 DHT 仅是 16 位哈希环的简化查找表，洋葱路由和多跳中继完全未实现；网络发现设计（07-网络发现设计.md）提出的 Gateway 全路由节点、AddrMan、`:8001` Seed 端点均未在代码中出现。**代码中存在大量"半成品"结构**（`NodeUnlockState`、`CreditsManager` 与 `BalanceEngine` 重叠），设计文档自身也在 v1/v2.0/Gateway 三个版本间未达成一致，导致实现无所适从。

> **v4.0.1 更新（2026-07-11）**：设计文档已升级至 v4.0.1，核心变更包括：双模式架构（个人版 + 共享版）、助记词身份机制、4 种密钥类型（mk_* 废弃）、贡献积分（Contribution Credit）、传输路径加密、两级开关、路线图重排。这些变更**解决了本审计报告中部分"设计文档间相互矛盾"的问题**，但代码实现尚未同步更新。详见附录 A。

---

## 需求文档 vs 实现偏差

### 一、01-密钥体系v2.0设计 vs 实现

#### 1.1 密钥类型：设计要求 4 种，代码实现 6+ 种

| 设计要求的 Key 类型 | 设计格式 | 代码实现状态 | 代码位置 |
|---|---|---|---|
| **Proxy API Key** | `sk-{random}` | ✅ 已实现 | `config.go` + `withProxyAuth()` |
| **Guest Proxy Key** | `sk-guest-{node_id}-{random}` | ❌ **未实现** — 代码中无 `sk-guest-` 前缀的任何处理 | 全代码搜索结果为空 |
| **全球公共 Key** | `sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1`（固定常量） | ❌ **未实现** — 代码中使用 `mk_open_global_{node_id}_{random}` 格式 | `network_keys.go:861-903` |
| **Provider Key** | `sk-xxx`（各平台原生） | ✅ 已实现 | `provider.go` |

**代码中实际存在但设计已废弃的 Key 类型**：

| 废弃的 Key 类型 | 设计状态 | 代码实际状态 | 代码位置 |
|---|---|---|---|
| `mk_trial_{node_id}_{ts}` | 已移除 | ✅ **仍完整实现** | `network_keys.go:713-750` |
| `mk_open_{random}`（非绑定） | 已移除 | ✅ **仍完整实现** | `network_keys.go:907-948` |
| `mk_open_{node_id}_{random}`（绑定） | 已移除 | ✅ **仍完整实现** | `network_keys.go:966-1016` |
| `mk_open_global_{node_id}_{random}` | 已移除 | ✅ **仍完整实现** | `network_keys.go:861-903` |
| `mk_{consumer_id}.{payload}.{sig}`（标准签名 Key） | 已移除 | ✅ **仍完整实现** | `network_keys.go:133-170` |

**偏差严重性**: 🔴 **高** — 密钥体系是整个网络的核心，v2.0 设计明确废弃了 6 种旧 Key 并简化为 4 种，但代码中旧 Key 全部保留且功能完整，v2.0 新增的 `sk-guest-` 和全球公共 Key（`sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1`）均未实现。`ClassifyKey()` 函数仍识别全部旧类型（`KeyTypeTrial`、`KeyTypeOpenUnbound`、`KeyTypeOpenBound`、`KeyTypeGlobal`、`KeyTypeStandard`），与设计要求的"只识别 3 种"完全不符。

#### 1.2 QuotaAllocation 额度分配模型

| 设计要求 | 实现状态 | 偏差说明 |
|---|---|---|
| X% 给消费者 + Y% 给其他节点 | ⚠️ 部分实现 | `network_quota.go` 中 `OpenKeyQuotaManager` 使用 `openKeyRatio=30%` 但此为 Open Key 专用，非设计中的"节点主人设定 X%/Y%"模型 |
| 简单百分比分配，移除积分系统 | ❌ 未实现 | `credits.go` 中仍有完整积分系统（`creditEarnPer1KTokens`、`creditSpendPer1KTokens`、`creditSpendPerMessage`），与设计"移除积分系统"相矛盾 |
| 移除 daily cap、invite bonus | ❌ 未实现 | `credits.go:139` 仍有 `todaySpending()` 日消费上限逻辑 |
| 全球公共 Key 从免费消费者池分配 | ⚠️ 偏差 | 代码中无"免费消费者池"概念，公共 Key 额度由 `OpenKeyQuotaManager` 基于 `openKeyRatio` 计算，但该计算基于全网资源而非设计中的"节点分配比例" |

#### 1.3 身份验证：设计要求简化，代码仍复杂

| 设计要求 | 实现状态 |
|---|---|
| Proxy API Key = 节点自签名 JWT，包含 NodeID | ❌ 实际为随机字符串，非 JWT |
| Guest Proxy Key = 节点签名，含 NodeID + 签发信息 | ❌ `sk-guest-` 格式未实现 |
| 全球公共 Key = 固定常量 `sk-openmodelpool-com-...`，无身份信息 | ❌ 实际为 `mk_open_global_{node_id}_{random}.{payload}.{signature}`，含签名载荷 |
| 移除签名验证相关代码 | ❌ `network_keys.go` 仍有 Ed25519 签名/验证全流程 |

#### 1.4 代码变更清单执行情况

设计文档第六章列出了具体代码变更清单，**全部未执行**：

| 变更项 | 要求 | 实际 |
|---|---|---|
| 移除 `KeyTypeTrial` 等 | 删除 | 仍存在 (`network_keys.go:683-687`) |
| 新增 `KeyTypeGuest` | 新增 | 不存在 |
| 简化 `ClassifyKey()` | 简化至 3 类 | 仍识别 6 类 (`network_keys.go:1138-1158`) |
| 移除 `mk_trial_` 等解析逻辑 | 删除 | 仍完整 |
| 移除积分系统 | 删除 | `credits.go` 完整保留 |
| 移除共识投票 | 删除 | `network_algorithm.go` 仍保留投票机制 |

---

### 二、02-P2P架构设计 vs 实现

#### 2.1 节点类型

| 设计定义 | 实现状态 |
|---|---|
| **Bootstrap Node** — 硬编码地址，仅提供节点发现 | ⚠️ 部分 — `network_discovery.go:510-553` 有 Bootstrap 注册，但仅作为心跳端点，无 DHT 种子功能 |
| **Ordinary Node** — Consumer + Provider | ✅ 基本实现 |
| **Relay Node** — 高带宽，仅转发加密信元 | ❌ **未实现** — 代码中无独立 Relay 角色概念，所有节点均可中继 |
| **Exit Node** — 拥有海外 AI API，解密最终请求 | ❌ **未实现** — 无 Exit 角色概念，中继为透传式 |

#### 2.2 节点发现：三层机制 vs 单层心跳

| 设计机制 | 设计用途 | 实现状态 |
|---|---|---|
| **Kademlia DHT** | 全局节点路由、能力注册 | ❌ **严重简化** — `dht.go` 仅实现 16 位哈希环（设计要求 256 位），无 k-bucket、无迭代查找、无 DHT 协议消息，只是从 `federation.go` 重建的本地查找表 |
| **Gossip 协议** | 实时状态传播 | ⚠️ 部分 — `gossip.go` 实现了 30 秒周期的状态交换，但非设计中的 Plumtree/Scuttlebutt 变体，也无 GossipSub mesh 拓扑 |
| **mDNS** | 本地网络发现 | ❌ **未实现** |
| **Bootstrap 节点** | 初始引导 | ⚠️ 部分 — 有注册心跳但无 DHT 引导功能 |

**DHT 实现偏差细节**：
- 设计要求：256 位 SHA-256 哈希空间, k=20, α=10, 256 个 k-bucket
- 代码实际：16 位哈希环 (`dht.go:14`)，无 k-bucket 概念，`FindClosest()` 仅遍历全表排序，时间复杂度 O(N log N) 而非设计的 O(log N)

#### 2.3 多跳中继 / 洋葱路由

| 设计要求 | 实现状态 |
|---|---|
| 2-3 跳可配置洋葱路由 | ❌ **完全未实现** |
| 请求级电路（短生命周期） | ❌ 无电路概念 |
| AES-256-GCM 分层加密 | ❌ 仅节点间 TLS，无洋葱加密 |
| DH 密钥协商 + 电路构建 | ❌ 无 |
| SSE 流式响应逐 chunk 加密回传 | ❌ 透传式 relay |
| 信元帧协议（HANDSHAKE/DATA/CONTROL/TEARDOWN/GOSSIP） | ❌ 无自定义帧协议 |
| QUIC 主传输 + WebSocket 备选 | ❌ 使用标准 HTTP |

**实际中继实现**：`network_relay.go` 仅实现**单跳透传** — `handleRelayToLocal()` 使用 `httputil.ReverseProxy` 回环到 localhost，`relayToRemote()` 直接 HTTP 转发。跳数限制 `maxRelayHops = 3` 仅用于防循环，不代表多跳中继。

#### 2.4 信任评分算法

| 设计维度 | 设计权重 | 实现维度 | 实现权重 |
|---|---|---|---|
| 请求成功率 | 25% | 可用性 (Availability) | EWMA α=0.3 |
| 平均响应延迟 | 15% | 延迟 (Latency) | EWMA α=0.3 |
| 累计服务时长 | 20% | 准确性 (Accuracy) | EWMA α=0.3 |
| 邀请链深度 | 15% | 同行共识 (PeerScore) | 手动评分 |
| 异常行为扣分 | 25% | ❌ 无 | — |

**评级体系偏差**：
- 设计：Diamond/Gold/Silver/Bronze/Untrusted 五级（0-1.0 浮点分）
- 实现：S/A/B/C/D 五级（0-100 整数分），`reputation.go:142-167`
- 设计要求"每天衰减 5%（半衰期约 14 天）"，实现中**无衰减机制**

#### 2.5 Sybil 攻击防御

| 设计防御层 | 实现状态 |
|---|---|
| 经济成本（邀请码/身份质押） | ⚠️ 有邀请码系统但无质押 |
| 社交信任图（Web of Trust） | ❌ 未实现 |
| 行为分析（请求模式指纹） | ❌ 未实现 |
| IP 限制（同 /24 子网 ≤3 节点） | ❌ 未实现 |
| 声誉门槛 | ⚠️ 有 `NodeUnlockState` 但为半成品 |

---

### 三、03-域名绑定设计 vs 实现

#### 3.1 Cloudflare API Token 方案

| 设计 API | 实现状态 | 代码位置 |
|---|---|---|
| `POST /api/tunnel/token` — 存储 API Token | ⚠️ 部分 — 通过 `POST /api/domain/bind` 间接传入，存储为 `cf_api_token` | `tunnel.go:547-550` |
| `POST /api/tunnel/create` — 创建命名隧道 | ✅ `createTunnelViaAPI()` | `tunnel.go:318-370` |
| `GET /api/tunnel/status` — 查询隧道状态 | ✅ `handleTunnelStatus()` | `tunnel.go:233-246` |
| `POST /api/tunnel/start` | ⚠️ 通过 `applyTunnelConfig()` 间接实现 | `tunnel.go:247-278` |
| `POST /api/tunnel/stop` | ✅ `TunnelManager.stop()` | `tunnel.go:182-203` |

#### 3.2 前端 UI 流程

| 设计 UI 状态 | 实现状态 |
|---|---|
| 未绑定 → "绑定域名"按钮 + 对话框 | ✅ 基本实现（`admin.html` 中有域名绑定区域） |
| 绑定中 → 进度显示 | ⚠️ 有进度输出但无实时 WebSocket 推送 |
| 已绑定 → 域名显示 + 隧道状态 | ✅ 基本实现 |

#### 3.3 TunnelManager 结构体 vs 设计

| 设计字段 | 实现 | 偏差 |
|---|---|---|
| `apiToken string` | ❌ 不在 TunnelManager 中，在 `DomainBinder` 中 | `tunnel.go:307` |
| `tunnelID string` | ❌ 不在 TunnelManager 中 | 由 DomainBinder 返回 |
| `customDomain string` | ✅ `domain` 字段 | 命名不同 |
| `mode string` ("quick"/"named") | ✅ 实现 | 一致 |

#### 3.4 分阶段实现状态

| 阶段 | 设计内容 | 实现状态 |
|---|---|---|
| **Phase 1** (MVP) | 存储 Token + 创建隧道 + DNS + 前端 UI | ✅ 大部分实现 |
| **Phase 2** | 状态监控 + 解绑 + 域名验证 + 错误提示 | ⚠️ 部分实现 |
| **Phase 3** | 多域名 + 健康检查 + 自动重连 + 统计 | ❌ 未实现 |

#### 3.5 安全考虑

| 设计要求 | 实现状态 |
|---|---|
| Token 加密存储（encryptor） | ⚠️ 存储为 `cf_api_token` 和 `cf_tunnel_id`，未确认是否经 encryptor 加密 |
| Token 权限最小化引导 | ❌ 无引导说明 |
| 不在日志中输出 Token | ⚠️ 未明确验证 |

**域名绑定偏差严重性**: 🟡 **中** — 核心功能已实现但与设计细节有出入，Phase 2/3 未完成。

---

### 四、04-迭代路线图 vs 实现

#### 4.1 Phase 1：联邦基础架构

| 功能 | 路线图状态 | 代码实际状态 | 偏差 |
|---|---|---|---|
| NodeID 生成（Ed25519 派生） | ✅ | ✅ `node.go:283` | 格式差异：设计要求 `mm-` + base58，实际为 `mmx-` + hex |
| GitHub-Native Registry (trust_pool.json) | ✅ | ✅ `federation.go:242` `refreshFromGitHub()` | 一致 |
| Gossip 传播协议 (30s 周期) | ✅ | ✅ `gossip.go:48-64` | 一致 |
| Provider 中继模式 | ✅ | ✅ `network_relay.go` | 中继方式不同：设计为签名验证后调用，实际为 HTTP 透传 |
| Provider 自发现广播 | ✅ | ✅ `gossip.go:broadcastAnnouncement()` | 一致 |

#### 4.2 Phase 2：信誉与治理

| 功能 | 路线图状态 | 代码实际状态 | 偏差 |
|---|---|---|---|
| 信誉评分系统（4 维加权） | ✅ | ⚠️ `reputation.go` 实现 3 维（可用性+延迟+准确性）+ 同行评分 | 缺少设计中的投诉维度 |
| 多节点联合审核 | ✅ | ⚠️ `node_weight.go:ApprovalRequest` 存在但逻辑不完整 | 设计要求 3 种审核通过条件，代码中仅结构定义 |
| GitHub 账户验证 | ✅ | ❌ 未见 OAuth 验证流程实现 | 缺失 |
| 防恶意注册（IP 限制等） | ✅ | ❌ 无 IP 限制、无账户年龄检查 | 完全缺失 |

#### 4.3 Phase 3：积分与经济系统

| 功能 | 路线图状态 | 代码实际状态 | 偏差 |
|---|---|---|---|
| 积分获取（+1/千 token 等） | ✅ | ✅ `credits.go:32-34` 定义了积分率 | 一致 |
| 积分消耗（-1/千 token 等） | ✅ | ✅ `credits.go:84-117` | 一致 |
| 中心化清算 | ✅ | ⚠️ 仅本地记录，无 GitHub Actions 汇总 | 未实现清算流程 |
| 签名对账 | 后期 | ❌ 未实现 | 符合预期 |

#### 4.4 Phase 4-5：前端与裂变

| 功能 | 路线图状态 | 代码实际状态 |
|---|---|---|
| Web 管理面板联邦页面 | ✅ | ⚠️ 部分实现（网络状态面板存在但拓扑可视化缺失） |
| CLI 命令扩展 | ✅ | ❌ 无 `network` 子命令 |
| 网络拓扑可视化 | ✅ | ❌ 未实现 |
| 点对点消息系统 | ✅ | ✅ `message.go` 收件箱/发件箱 |
| 游戏化激励 | ✅ | ❌ 未实现 |
| GitHub 裂变 | ✅ | ❌ 未实现 |

#### 4.5 新增文件规划 vs 实际

| 设计文件 | 实际文件 | 状态 |
|---|---|---|
| `node.go` | ✅ 存在 (355 行) | 一致 |
| `federation.go` | ✅ 存在 (398 行) | 一致 |
| `gossip.go` | ✅ 存在 (572 行) | 一致 |
| `relay.go` | ✅ 存在 (364 行) — 但设计为 `relay.go`，实际中继在 `network_relay.go` (596 行) | 偏差：两个文件功能重叠 |
| `reputation.go` | ✅ 存在 (451 行) | 一致 |
| `credits.go` | ✅ 存在 (250 行) | 一致 |
| `message.go` | ✅ 存在 (596 行) | 一致 |
| `discovery.go` | ✅ 存在但仅 127 行（引导节点交换），设计中为节点发现核心 | 严重不足 |
| `network_cmd.go` | ❌ 不存在 | 缺失 |

---

### 五、05-完整PRD vs 实现

PRD 文档本身已包含对已实现功能的 Gap 分析（第 8-14 章），此处仅补充 PRD 描述 vs 实际代码的关键偏差：

#### 5.1 密钥体系 PRD vs 代码

| PRD 描述 | 代码实际 | 偏差 |
|---|---|---|
| Guest Proxy Key: `sk-{device_id}.{random}` | ❌ 代码中无此格式 | PRD 自身也与 01-密钥体系设计矛盾（PRD 用 `device_id`，设计用 `node_id`） |
| 全球公共 Key: `sk-openmodelpool-com-...` 固定常量 | ❌ 实际为 `mk_open_global_{node_id}_{random}` | PRD 与设计一致但与代码矛盾 |
| 签名 Key 体系 (mk_) 为 2.5 节 | ✅ 代码中完整实现 | PRD 仍列出 mk_ 签名体系但 01-设计已废弃 |

#### 5.2 额度体系 PRD vs 代码

| PRD 描述 | 代码实际 | 偏差 |
|---|---|---|
| 三层额度架构（全网→节点→Key） | ⚠️ Key 级和全网级存在，节点级不完整 | `ContribRecord` 存在但未深度接入 |
| 动态阈值解锁 `avgContrib * 0.3 * scaleFactor` | ✅ `network.go:1010-1088` 实现了公式 | 但 01-设计要求删除此机制 |
| 共识投票（公共 Key 免费额度比例） | ⚠️ `network_algorithm.go` 有投票结构 | 投票逻辑不完整，未与公共 Key 额度关联 |

---

### 六、06-联邦网络说明 vs 实现

#### 6.1 节点权利实现情况

| 说明文档承诺 | 实现状态 |
|---|---|
| 资源发现权 — 浏览全网 Provider 目录 | ✅ Gossip 同步 + 路由表 |
| 信誉参与权 — 对节点评分 | ✅ `reputation.go:AddPeerScore()` |
| 积分经济权 — 赚取/消费积分 | ✅ `credits.go` 完整 |
| 治理参与权 — 提案/投票 | ⚠️ `network_algorithm.go` 有结构，投票逻辑不完整 |
| P2P 通信权 — 端到端加密消息 | ⚠️ `message.go` 有收发功能，但未实现端到端加密 |

#### 6.2 节点义务实现情况

| 说明文档要求 | 实现状态 |
|---|---|
| 保持在线（≥80% 可用率） | ⚠️ 心跳存在但无可用率追踪 |
| 长期离线标记 inactive | ✅ `network_discovery.go:24` `maxMissedHeartbeats=3` |
| 诚实评分（防恶意刷分） | ⚠️ EWMA 有平滑效果，但无专门防刷分机制 |
| 积分账本维护（签名交易链） | ❌ 无交易链，仅本地 JSON |
| D 级 7 天后移出信任池 | ⚠️ `reputation.go:ShouldRemoveNode()` 有逻辑但未见自动执行 |

#### 6.3 数据分布式方案

| 说明文档承诺 | 实现状态 |
|---|---|
| 节点注册表：Gossip 全量同步 | ✅ `gossip.go` 实现交换 |
| Provider 目录：签名发布 + Gossip 传播 | ✅ `gossip.go:broadcastAnnouncement()` |
| 信誉评分：Gossip 交换 + 加权平均 | ⚠️ 评分存在但 Gossip 传播不完整 |
| 积分交易：签名交易链 | ❌ 无交易链，仅本地记录 |
| 治理投票：BFT 多数签名 | ⚠️ 投票结构存在，共识逻辑简化 |
| P2P 消息：端到端加密 + 中继投递 | ❌ 消息有收发但无加密 |

---

### 七、07-网络发现设计 vs 实现

此文档偏差**最为严重**，几乎所有设计均未落地。

#### 7.1 节点角色

| 设计角色 | 实现状态 |
|---|---|
| **Seed Node** — 绑定固定域名 + is_seed 标记 | ⚠️ `types.go:392` 有 `SeedNode bool` 字段，但无 `:8001` Seed 端点、无 `/api/peers` API |
| **Gateway Node** — 绑定固定域名 + is_gateway 标记 | ❌ **完全未实现** — 代码中无 `IsGateway` 字段、无 Gateway 路由逻辑 |
| **Regular Node** | ✅ 隐式实现（所有非 Seed 节点） |
| **Solo Node** | ✅ 个人模式 |

#### 7.2 全路由节点设计

| 设计要求 | 实现状态 |
|---|---|
| 每个加入网络的节点可成为全网入口 | ❌ 未实现 — 仍需指定 NodeID `/network/{node_id}/v1` |
| Gateway 接收请求后自动选最优节点 | ❌ 无 Gateway 路由入口 |
| 用户无需知道 NodeID | ❌ 当前所有路由仍需 NodeID |
| 统一入口 `api.openmodelpool.com/v1` | ❌ 未实现 |

#### 7.3 Gossip 协议设计

| 设计消息类型 | 实现状态 |
|---|---|
| `PING/PONG` | ⚠️ 心跳存在但非独立消息类型 |
| `GET_PEERS` | ❌ 未实现 |
| `PEERS` | ⚠️ 心跳响应中附带 peer 列表，但非设计格式 |
| `ANNOUNCE` | ❌ 未实现为独立消息 |

#### 7.4 AddrMan 地址管理器

| 设计组件 | 实现状态 |
|---|---|
| `AddrMan` 结构体（Known/Gateways/Seeds） | ❌ **完全未实现** |
| `PeerInfo` 含 LatencyMs/UptimeScore/FailCount | ⚠️ 代码中 `PeerInfo` 有 `TrustScore` 但无 `LatencyMs`/`UptimeScore`/`FailCount` |
| 节点 30 分钟无响应 → fail_count++ | ❌ 无 fail_count 概念 |
| `peers.dat` 持久化 | ❌ 无此文件 |

#### 7.5 Seed 复用模型

| 设计要求 | 实现状态 |
|---|---|
| 每个节点暴露 `:8001` 端口做 Seed | ❌ 未实现 |
| `/api/peers` 端点返回已知节点 | ❌ 未实现 |
| 项目域名作为全局入口 | ❌ 未实现 |
| DNS 轮询指向所有 Gateway | ❌ 未实现 |

#### 7.6 冷启动方案

| 设计步骤 | 实现状态 |
|---|---|
| 注册域名 openmodelpool.com | ❌ 未知 |
| 项目域名 A 记录指向创始人节点 | ❌ 未知 |
| 实现 `:8001` Seed 端点 | ❌ 未实现 |
| GitHub 发布节点注册引导 | ⚠️ 有 `refreshFromGitHub()` 但非设计中的 `.nodes/` 注册表格式 |
| `peers.dat` 本地缓存 | ❌ 无 |

**网络发现偏差严重性**: 🔴 **极高** — 这是消费者体验的核心（统一入口、无需知道 NodeID），设计文档给出了完整方案（Seed 复用、Gateway 全路由、AddrMan），但代码中**几乎零实现**。

---

### 八、08-定价模型对比 vs 实现

此文档为对比分析文档，非需求文档，主要价值在于明确了 v1/v2.0/Gateway 三个版本的取舍。其对当前实现的判断与本次审计基本一致：

| 对比文档判断 | 本次审计验证 |
|---|---|
| v2.0 缺少跨节点信任验证 | ✅ 确认 — 依赖隐式信任 |
| v2.0 无法精确追踪贡献与消费平衡 | ✅ 确认 — `ContribRecord` 存在但未深度接入 |
| v2.0 没有防搭便车机制 | ✅ 确认 — 全球公共 Key 的替代品 `mk_open_global_*` 无额度限制 |
| v2.0 Gateway 路由缺少节点质量评估 | ✅ 确认 — `RouteTable` 仅记录地址 |
| v2.0 NodeUnlock 机制半成品 | ✅ 确认 — `NodeUnlockState` 存在但与路由决策未关联 |

#### 8.1 对比文档建议的代码保留/修改 — 执行情况

| 建议动作 | 目标文件 | 实际执行 |
|---|---|---|
| ❌ 删除 `NodeUnlockState` 相关代码（~150 行死代码） | `network.go` | ❌ **未执行** — `NodeUnlockState` 仍完整存在 (line 963-1088) |
| ❌ 删除 `globalKeyStore` 遗留 stub | `network_global_pool.go` | ❌ **未执行** |
| ❌ 删除 `fetchPeerPublicKey()` 遗留 stub | `network_keys.go` | ❌ **未执行** |
| 🔧 修改 `GuestKeyRecord` 增加额度/有效期/模型白名单 | `network_keys.go` | ❌ **未执行** |
| 🔧 增强 `RouteTable` 增加延迟/负载字段 | `network.go` | ❌ **未执行** — `RouteEntry` 无 `LatencyMS`/`LoadScore` |
| ➕ 新增 Gateway 路由入口 | `network_relay.go` | ❌ **未执行** |
| ➕ 新增 `ContributionTracker` | `credits.go` | ❌ **未执行** |

---

### 九、完整PRD评审 (openmodelpool-full-prd-and-review.md) vs 实现

此文档与 05-完整PRD 高度重叠，额外补充的审查发现：

| PRD 审查发现 | 代码验证 |
|---|---|
| `network.go` 过于庞大（~600行）| ✅ 实际 1203 行，**比审查时翻倍** |
| 大量全局变量形成隐式耦合 | ✅ 确认 — `netMgr`, `fed`, `gossip`, `node`, `repMgr`, `credits`, `globalPool`, `balanceEngine`, `regionMgr` 等 |
| `federation.go` 与 `network.go` 功能重叠 | ✅ 确认 — 两者都维护节点列表 |
| `credits.go` 与 `network_balance.go` 功能重叠 | ✅ 确认 — 两套独立积分系统 |
| 投票机制不完整 (`FederationVote`) | ✅ 确认 — `network_algorithm.go` 有 `Vote()` 方法但提案/投票流程不完整 |
| Gossip 额度同步未实现 | ✅ 确认 |
| 跨节点贡献证明缺失 | ✅ 确认 |
| 防女巫攻击缺失 | ✅ 确认 |

---

### 十、ARCHITECTURE_CN.md vs 实现

| 架构文档描述 | 实现状态 | 偏差 |
|---|---|---|
| 256 位 Kademlia DHT (k=20, α=10) | ❌ 实际 16 位简化环 | 严重偏差 |
| GossipSub 网状网络 (fanout=10) | ⚠️ 30 秒 gossip 交换，无 mesh | 简化 |
| 三种密钥（Proxy/Guest/Public） | ❌ 实际 6+ 种密钥 | 与架构文档和 01-设计均不一致 |
| 公钥格式 `sk-openmodelpool-com-...` | ❌ 实际无此格式 | 架构文档自身与 01-设计也不一致 |
| IPFS + IOTA 存储层 | ❌ 完全未实现 | 仅规划 |
| LevelDB 本地存储 | ❌ 使用 JSON 文件 | 简化 |
| 贡献账本三层架构（Gossip/IPFS/IOTA） | ❌ 仅第一层部分实现 | 严重不足 |
| 能力声明系统 (CapabilityClaim) | ⚠️ Provider 有 SharedModels 但无签名声明 | 简化 |
| 虚假能力防御（主动探测） | ❌ 未实现 | 缺失 |
| AutoNAT 角色判定 | ❌ 未实现 | 缺失 |
| 五维负载均衡 | ⚠️ `network_loadbalancer.go` 有多维评分但权重不同 | 偏差 |

---

## 重大设计矛盾汇总

审计发现**设计文档自身存在严重矛盾**，这是实现偏差的根本原因之一：

| 矛盾点 | 文档 A | 文档 B | 影响 |
|---|---|---|---|
| **密钥类型数量** | 01-设计: 4 种 (Proxy/Guest/Public/Provider) | 02-P2P: 6 种 (含签名mk_/trial/open/global) | 代码按旧版实现 |
| **全球公共 Key 格式** | 01-设计: `sk-openmodelpool-com-...` 固定常量 ✅已对齐 | ARCHITECTURE_CN: `sk-openmodelpool-com-...` ✅已对齐 | 代码用 `mk_open_global_` ❌ |
| **DHT 位宽** | 02-P2P: 256 位 Kademlia | 代码: 16 位简化环 | 性能和功能严重不足 |
| **中继方式** | 02-P2P: 多跳洋葱路由 | 08-对比: 单跳足够，放弃多跳 | 代码按单跳实现 |
| **信誉系统** | 02-P2P: 5 维 Diamond-Untrusted (0-1.0) | 04-路线图: 4 维 S-D (0-100+) | 代码按 S-D 实现 |
| **积分系统** | 01-设计: 移除积分系统 | 04-路线图: Phase 3 完整积分系统 | 代码保留旧积分 |
| **网络发现** | 07-发现: Gateway 全路由 + AddrMan | ARCHITECTURE_CN: Kademlia DHT | 代码两者均未完整实现 |
| **消费入口** | 07-发现: 统一入口无需 NodeID | 02-P2P: `/network/{NodeID}/v1` 显式路由 | 代码按显式路由实现 |

---

## 偏差严重性矩阵

| 文档 | 偏差严重性 | 核心未实现项 |
|---|---|---|
| 01-密钥体系v2.0设计 | 🔴 **高** | 4 种 Key 类型未落地，旧 Key 未清理，积分系统未移除 |
| 02-P2P架构设计 | 🔴 **高** | 洋葱路由/多跳中继/完整 DHT/Sybil 防御均未实现 |
| 03-域名绑定设计 | 🟡 **中** | 核心功能已实现，Phase 2/3 未完成 |
| 04-迭代路线图 | 🟡 **中** | Phase 1 基本完成，Phase 2-5 大量未实现 |
| 05-完整PRD | 🔴 **高** | 密钥体系/额度模型/安全机制与代码多处矛盾 |
| 06-联邦网络说明 | 🟡 **中** | 核心流程已实现，端到端加密/交易链/完整治理缺失 |
| 07-网络发现设计 | 🔴 **极高** | Gateway/AddrMan/Seed端点/统一入口几乎零实现 |
| 08-定价模型对比 | 🟢 **低** | 分析文档，建议动作均未执行 |
| ARCHITECTURE_CN | 🔴 **高** | 256 位 DHT/IPFS/IOTA/能力声明等核心架构未实现 |

---

## 优先修复建议

### P0 — 必须立即对齐（架构根基）

1. **统一密钥体系**：要么执行 01-设计的 4 种 Key 简化方案（删除旧 Key、实现 `sk-guest-` 和全球公共 Key `sk-openmodelpool-com-...`），要么更新设计文档承认当前 6+ 种 Key 的现实
2. **决定 DHT 方案**：ARCHITECTURE_CN 描述的 256 位 Kademlia vs 当前 16 位简化环，选择其一并统一文档
3. **清理死代码**：执行 08-对比文档建议的 `NodeUnlockState`/`globalKeyStore`/`fetchPeerPublicKey()` 删除

### P1 — 近期对齐（消费者体验）

4. **实现 Gateway 全路由**：07-网络发现设计的核心价值——统一入口、无需 NodeID——是消费者体验的关键提升
5. **统一积分系统**：合并 `credits.go` 和 `network_balance.go` 为单一系统，消除激励信号冲突
6. **实现全球公共 Key 固定常量**：使用 `sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1` 替代当前的 `mk_open_global_*` 签名密钥，真正实现零门槛试用

### P2 — 中期对齐（网络健壮性）

7. **增强 RouteTable**：增加延迟/负载/模型覆盖等字段，为智能选路奠基
8. **Guest Key 增强**：增加额度/有效期/模型白名单
9. **实现 AddrMan**：替代当前简单的 peer 列表，支持节点质量追踪
10. **端到端消息加密**：实现 06-联邦说明承诺的加密消息

### P3 — 远期对齐（去中心化愿景）

11. **完整 Kademlia DHT**：如 ARCHITECTURE_CN 所述的 256 位 k-bucket 实现
12. **Sybil 防御**：IP 限制 + 社交信任图 + 行为分析
13. **贡献账本三层架构**：Gossip LevelDB → IPFS → IOTA

---

*本报告基于 modelmux-backup 代码库（26,166 行 Go 代码）与 9 份设计/需求文档的逐项比对生成。所有偏差判断均有具体代码行号和设计文档章节作为依据。*


---

## 附录 A: v4.0.1 变更审计

### A.1 v4.0 → v4.0.1 主要变更

| 变更项 | v3.0/v4.0 | v4.0.1 | 影响评估 |
|--------|-----------|--------|---------|
| 架构模式 | 纯 P2P 网络 | 双模式（个人版 + 共享版） | ✅ 降低入门门槛 |
| 身份系统 | `omsk_*` 密钥层级 | BIP39 助记词 → Ed25519 → Node ID | ✅ 简化身份管理 |
| 密钥类型 | 5+ 种（含 mk_* 系列） | 4 种（mk_* 废弃） | ✅ 收敛复杂度 |
| 加密模型 | E2EE | 传输路径加密（中继不可见，资源节点解密） | ⚠️ 需评估安全影响 |
| 积分系统 | 贡献积分 | Contribution Credit（不可提现/交易） | ✅ 明确非金融属性 |
| 开关设计 | 单一开关 | 两级开关（network_enabled + share_to_pool） | ✅ 更精细控制 |
| 公共体验 | 无 | Public Global Key（四重限速） | ✅ 降低试用摩擦 |
| 路线图 | 3 阶段 | 4 阶段（Phase 0/1/2/3） | ✅ 更务实渐进 |
| 防共谋 | 基础 | 公证人去中心化演进 | ✅ 增强抗审查 |
| request-id | 可选 | 缺失 = 不计入积分 | ✅ 防伪造贡献 |

### A.2 新增审计关注点

1. **双模式隔离**：确保个人版模式完全不产生网络流量
2. **助记词安全**：评估助记词生成、存储、恢复的全链路安全
3. **Public Global Key 滥用**：评估四重限速是否足够防滥用
4. **贡献积分非金融化**：确保积分系统设计不构成金融资产
5. **传输路径加密**：评估资源节点解密的安全边界

### A.3 审计结论（v4.0.1）

v4.0.1 的核心改进——双模式架构和助记词身份——显著降低了用户门槛和系统复杂度。主要风险点在于传输路径加密的安全模型需要清晰文档化，以及助记词备份的用户体验需要充分验证。整体架构设计方向正确，建议按计划推进 Phase 1 实现。
