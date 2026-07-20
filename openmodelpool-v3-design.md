# OpenModelPool v4.0.5 统一设计文档

> **版本**: v4.0.5 | **日期**: 2026-07-20 | **状态**: 设计定稿 + 实现进展同步（v4.0.5 工程迭代更新）
>
> **核心叙事**：OpenModelPool 是一个**临时 Token 银行** + **极客共享网络**。用户把本月用不完、过期会浪费的模型额度存入网络；当别人实际使用这些额度后，系统记录贡献积分；未来用户需要自己没有的模型时，可以用贡献积分取回等价模型资源。如果用户永远不取回，则这些贡献自然成为对社区的极客共享。
>
> **产品本质**：OpenModelPool **默认是个人模型代理**。只有当用户配置了 Provider Token、开启额度管理，并且检测到本月存在闲置额度时，系统才在管理员界面提示用户可以**主动加入共享网络**。加入共享网络后才生成助记词和 Node ID，贡献积分绑定 Node ID，Provider Token 始终只保存在本地。
>
> **红线原则**：不追求商业收益，不发行任何金融资产（**绝不发币**），不建立商业化交易体系。
>
> **安全底线**：资源提供方的 API Key 永远保存在本地，绝不上传服务器。请求方的 Prompt 经**传输路径加密**——对中继节点不可见，但资源提供节点需解密以调用上游模型。

---

## 目录

1. [项目概述与愿景](#1-项目概述与愿景)（含 1.5 产品运行模式 + 1.6 核心角色与使用链路）
2. [核心概念与术语表](#2-核心概念与术语表)
2A. [核心技术栈选型（Go 语言生态）](#2a-核心技术栈选型go-语言生态)
2B. [API 协议转换层（Protocol Translation Layer）](#2b-api-协议转换层protocol-translation-layer)
3. [密钥体系 v4.0](#3-密钥体系-v40)
4. [双模式节点模型（Personal / Network）](#4-双模式节点模型personal--network)
5. [P2P 网络架构](#5-p2p-网络架构)（含 5.1A 混合穿透 + 5.4.1A DNS 备用信标）
6. [额度分配模型](#6-额度分配模型)（含 6.1A 数据流/传输路径加密/Token 精准估算）
7. [贡献账本系统](#7-贡献账本系统)（含 7.1A 贡献积分 + 7.1B Ticket + 防共谋）
8. [信任与声誉系统](#8-信任与声誉系统)
9. [请求路由与负载均衡](#9-请求路由与负载均衡)
10. [身份认证与安全](#10-身份认证与安全)（含助记词机制）
10A. [本地安全风控底座 (WAF)](#10a-本地安全风控底座-waf)（含分级阻断 + 申诉机制）
10B. [重新部署与恢复策略](#10b-重新部署与恢复策略)
11. [域名绑定与隧道](#11-域名绑定与隧道)
12. [联邦治理与共识](#12-联邦治理与共识)
13. [P2P 消息系统](#13-p2p-消息系统)
14. [虚假能力防御](#14-虚假能力防御)
15. [迭代路线图](#15-迭代路线图)
16. [完整 BT vs OpenModelPool 对照表](#16-完整-bt-vs-openmodelpool-对照表)
17. [去中心化数据矩阵](#17-去中心化数据矩阵)

---

## 1. 项目概述与愿景

### 1.1 项目定位

OpenModelPool 是一个**创新的去中心化 AI 资源公益共享网络**。它同时也是一个**临时 Token 银行**：用户把本月用不完、过期会浪费的模型额度"存入"网络，当别人实际使用后系统记录贡献积分，未来用户可以用积分"取回"等价模型资源。

**双重叙事**：

```
临时 Token 银行叙事：
  你的 GPT-4o 额度本月只用 60% → 剩余 40% 过期浪费
  → 存入 OpenModelPool 网络 → 别人使用后你获得贡献积分
  → 未来你需要 Claude-3 → 用贡献积分取回等价资源
  → 如果你永远不取回 → 这些贡献自然成为极客共享

BT 类比叙事：
  BitTorrent:   你有文件块 A，我有文件块 B → 互换 → 双方获得完整文件
  OpenModelPool: 你有 GPT-4o 额度，我有 Claude-3 额度 → 互换 → 双方获得全网模型
```

这不是一个"API Key 交易市场"——没有买卖，没有收费站。这是一个**互惠互助的算力共享池**：你贡献闲置的 AI 算力，换取使用他人闲置算力的权利。贡献越多，网络中可用模型越丰富，你的路由优先级越高。

**核心目标**：盘活全球闲置的 AI 模型额度与 API 资源，帮助资源不足的用户获得 AI 服务。系统对普通用户隐藏所有底层 P2P 与加密细节，追求极简体验。

**关键区分**：
- 配置 Token ≠ 加入共享网络
- 有剩余额度 ≠ 自动共享
- 加入共享网络 ≠ 共享全部额度

### 1.2 愿景演进

| 阶段 | 定位 | 网络模型 | 类比 |
|------|------|---------|------|
| v1–v2 | API 代理网关 | 星型（Client → Gateway → Provider） | 单一下载工具 |
| v3.0 | 去中心化 AI 算力共享网络 | P2P Overlay（DHT + Gossip，无中心依赖） | BitTorrent Swarm |
| **v4.0** | **个人版优先 + 共享网络按需加入** | **默认离线个人代理 → 主动升级为共享节点** | **先做本地工具，再做网络公民** |
| **v4.0.5** | **个人版全功能交付 + Phase 1 共享基础已就绪** | **37 预设平台 + Anthropic API + web_session 模板 + 多周期额度 + 自动发现** | **工具成熟，准备开放共享网络** |

v4.0 的本质变化：**默认启动永远是个人版；只有配置 Provider Token、开启额度管理且本月有剩余额度时，才温和提示加入共享网络。** 加入共享网络后才生成助记词和 Node ID。

v4.0.5 工程进展：**Phase 0 个人版 MVP 全量交付，Phase 1 共享版基础架构（密钥/Seed/Gossip/Relay/Reputation/Message/Federation）已实现。** 新增 Anthropic Messages API 兼容层、web_session 通用模板系统、多 Key 健康检查、多周期额度系统、平台自动发现、SOCKS5 连接池优化。Admin UI 模块化重构。脚本精简为 2 个全功能管理脚本。

### 1.3 核心差异化价值

1. **能力共享而非数据共享**：BT 共享静态文件，OpenModelPool 共享实时、有状态、有配额的 API 调用能力。这使得激励模型和风控机制完全不同——需要主动探测验证能力真实性，而非 SHA 哈希验证数据完整性。
2. **社区信任而非密码学信任**：BT 用 info hash 验证文件完整性，OpenModelPool 用 Ed25519 签名验证身份和声明真实性，用声誉系统保证服务质量。
3. **互惠而非盈利**：没有过路费，没有算力交易市场。激励来自"贡献越多 → 可用模型越多 → 路由优先级越高"的正循环。

### 1.4 设计原则

| 原则 | 描述 | BT 对应 |
|------|------|---------|
| **极简密钥** | 仅 4 种 Key，兼容 OpenAI SDK | BT 客户端只需一个端口号 |
| **去中心化** | 无中心认证服务器，身份自包含 | 无中心 Tracker 亦可运行 |
| **贡献即权益** | 贡献越多 Provider 算力，获得越多网络访问额度 | 做种越多下载越快 |
| **算力交换** | 共享闲置算力，换取他人闲置算力 | 互换文件片段 |
| **渐进去中心化** | GitHub 注册表引导 → Gossip 自治 | Tracker → DHT |

### 1.5 产品运行模式

OpenModelPool 有两种运行模式。**默认启动永远是个人版**，只有用户主动选择后才进入共享网络。

| 模式 | 默认状态 | 是否联网 | 是否生成 Node ID | 是否生成助记词 | 是否贡献积分 |
|------|---------|---------|-----------------|---------------|-------------|
| **个人版 Personal Mode** | ✅ 默认 | ❌ 否 | ❌ 否 | ❌ 否 | ❌ 否 |
| **共享版 Network Mode** | 用户主动加入 | ✅ 是 | ✅ 是 | ✅ 是 | ✅ 是 |

#### 1.5.1 个人版（Personal Mode）

个人版是一个**纯本地的 AI 模型代理**，不加入 P2P 网络：

**只做**：
- 本地 Provider Token 管理（AES-256-GCM 加密存储，密钥文件 `data/.enc_key`）
- 本地 OpenAI-compatible API 代理（`http://127.0.0.1:8000/v1`）
- 本地额度管理与调用统计
- 管理员界面

**不做**：
- P2P 发现（DHT / Gossip / Seed 端点）
- 贡献账本
- 共享额度
- Guest Proxy Key / Public Global Key
- Node ID / 助记词

#### 1.5.2 加入共享网络的触发条件

只有**同时满足**以下三个条件时，管理员界面才提示用户加入共享网络：

```
触发条件（全部满足）：
  ✅ 已配置至少一个 Provider Token
  ✅ 该 Token 已开启额度管理
  ✅ 本月 remaining_quota > 0（存在闲置额度）
```

**提示文案**：
```
发现你本月还有闲置模型额度。
你可以选择将部分闲置额度共享到 OpenModelPool 网络。
别人实际使用后，你会获得贡献积分。
未来你需要自己没有的模型时，可以用贡献积分调用其他节点模型。
这是可选操作，不会自动共享你的 Token。
[了解并加入共享网络]
```

**三条原则**：
- 配置 Token ≠ 加入共享网络
- 有剩余额度 ≠ 自动共享
- 加入共享网络 ≠ 共享全部额度

#### 1.5.3 共享版（Network Mode）—— 加入流程

用户点击"了解并加入共享网络"后：

```
用户点击加入共享网络
  ↓
展示项目说明（公益共享、绝不发币、Key 不上传）
  ↓
用户确认理解共享机制
  ↓
生成 BIP39 助记词（12/24 词）
  ↓
由助记词派生 Ed25519 私钥
  ↓
生成公钥 → Node ID = hash(public_key)
  ↓
强制用户备份助记词（抄写/加密导出）
  ↓
配置共享额度边界
  ↓
进入共享网络
```

#### 1.5.4 共享版功能

共享版在个人版基础上**额外启用**：
- Node ID + 助记词
- P2P 网络（Gossip / DHT / Seed 端点）
- 贡献账本与贡献积分
- Guest Proxy Key
- Public Global Key
- 能力声明（CapabilityClaim）
- 路由调度
- 共享额度配置（`join_shared_network` + `share_to_pool` 两级开关）

### 1.6 核心角色与使用链路

#### 1.6.1 资源提供方（共享节点）

| 步骤 | 操作 | 说明 |
|------|------|------|
| **下载启动** | 下载软件，默认进入个人版 | 本地 OpenAI-compatible API 代理 |
| **配置 Token** | 输入大模型厂商 API Token | AES-256-GCM 加密存储（`data/.enc_key`） |
| **额度管理** | 开启额度管理，设定上限 | "每日最大共享 Token 数"等 |
| **收到提示** | 系统检测到闲置额度，温和提示 | 非强制，用户自主决定 |
| **加入共享** | 点击"了解并加入共享网络" | 生成助记词 + Node ID |
| **配置共享** | 设定共享边界 | 模型白名单、额度上限、时间窗口 |
| **一键启动** | 开启共享 | 软件穿透局域网接入网络待命 |

#### 1.6.2 资源消费方（请求节点）

| 步骤 | 操作 | 说明 |
|------|------|------|
| **下载启动** | 下载软件，默认进入个人版 | 本地暴露 API 端口 |
| **体验模式** | 无 Provider Token 时可用 Public Global Key | 极小额度体验，不保证可用/稳定 |
| **配置 Token** | 配置自己的 Provider Token 后 | 完整使用本地模型能力 |
| **加入共享**（可选） | 加入共享网络获取更多模型 | 贡献积分换取等价资源 |

**设计哲学**：默认个人版保护用户隐私和 autonomy。共享网络是**可选的升级路径**，而非默认行为。消费方无需了解 P2P 网络、密钥体系、路由算法——只需知道"下载什么"。

---

## 2. 核心概念与术语表

### 2.1 术语映射：BT → OpenModelPool

| BT 术语 | OpenModelPool 术语 | 含义 |
|---------|-------------------|------|
| Swarm | 共享池 (Pool) | 所有参与共享的节点集合 |
| Peer / Client | 节点 (Node) | 网络中的对等参与者 |
| Seeder | 贡献节点 (Contributor) | 开启 `share_to_pool` 的节点 |
| Leecher | 消费节点 (Consumer) | 仅消费不贡献的节点 |
| .torrent 文件 | GitHub 注册表 | 初始种子信息源 |
| Tracker | Seed 节点 (:8001) | 节点发现引导 |
| DHT | Kademlia DHT (256-bit) | 去中心化节点路由 |
| PEX (Peer Exchange) | Gossip 协议 | 节点信息扩散 |
| info hash | 节点 NodeID | 唯一标识符 |
| Bitfield | CapabilityClaim | 能力声明（持有哪些模型） |
| Have 消息 | Provider 公告 | 广播新能力 |
| Choking | 额度限制 | 限制低贡献节点的访问 |
| Unchoke | 开放共享 | 允许节点访问共享资源 |
| 分享率 (Ratio) | 贡献权重 | 上传/下载比 → 贡献/消费比 |
| 做种 (Seeding) | 共享 (Sharing) | 将 Provider 贡献到共享池 |
| 下载 (Downloading) | 消费 (Consuming) | 使用共享池中的模型服务 |
| Piece | 请求/响应单元 | 单次 API 调用 |

### 2.2 核心概念定义

| 概念 | 定义 |
|------|------|
| **个人版 (Personal Mode)** | 默认运行模式，纯本地代理，不加入 P2P 网络 |
| **共享版 (Network Mode)** | 用户主动加入的运行模式，启用 P2P 网络、Node ID、贡献账本 |
| **节点 (Node)** | 运行 OpenModelPool 实例的实体。个人版是本地代理；共享版同时扮演消费者和提供者 |
| **共享池 (Pool)** | 所有开启 `share_to_pool` 的节点贡献的 Provider 算力总和 |
| **能力声明 (CapabilityClaim)** | 节点广播其可提供的 AI 模型服务列表，类比 BT 的 Bitfield |
| **贡献积分 (Contribution Credit)** | 节点为网络提供的有效服务量的记账单位，不可交易/提现，用于未来取回等价资源 |
| **贡献权重 (Weight)** | 基于贡献积分计算的调度优先级，类比 BT 的分享率 |
| **Provider Token** | 上游 AI 平台的服务凭证（原称 Provider Key），本地加密存储 |
| **Node Proxy Key** | 节点代理凭证（原称 Proxy API Key），用于 API 认证 |
| **Public Global Key** | 全网统一免费 Key（原称全球公共Key），低额度体验入口 |
| **Seed 端点** | 节点暴露的 `:8001` 端点，提供节点发现服务 |
| **AddrMan** | 本地地址管理器，持久化已知节点列表（类比 BT 的 peers.dat） |
| **助记词 (Mnemonic)** | BIP39 助记词，用于生成/恢复 Node ID 和贡献积分，不包含 Provider Token |

---

## 2A. 核心技术栈选型（Go 语言生态）

客户端将被编译为**无依赖的纯净单文件二进制程序**，极致轻量，跨平台运行。当前基线 **Go 1.26+**。

| 模块名称 | 推荐 Go 技术选型 | 选型优势与说明 | 实现状态 |
|---|---|---|---|
| **底层网络协议** | `go-libp2p` | 顶级 Web3 P2P 网络库，内置发现、加密、打洞及中继服务 | 🔲 Phase 2 引入 |
| **HTTP 代理分发** | Go 原生 `net/http/httputil` | 原生 ReverseProxy，完美支持 LLM 流式输出 (SSE) 转发 | ✅ 已实现 |
| **本地轻量级数据库** | `go.etcd.io/bbolt` (BoltDB) | 纯 Go 编写的 K-V 数据库，无需 CGO 编译，用于存储账单与配置 | 🔲 计划中 |
| **内存限流 (防刷)** | `x/time/rate` + `golang-lru` | 内存级令牌桶，高性能维护各请求节点 ID 的频率限制 | ✅ 已实现（自定义令牌桶） |
| **合规审查 (敏感词)** | `cloudflare/ahocorasick` | 纯 Go 高性能 AC 自动机，纳秒级正则/敏感词过滤，极低内存消耗 | 🔲 计划中 |
| **密钥安全存储** | AES-256-GCM 本地加密 | 加密密钥文件 `data/.enc_key`（32字节），加密前缀 `omp:e:` | ✅ 已实现 |
| **BIP39 助记词** | `tyler-smith/go-bip39` | 生成/恢复 12/24 词助记词，派生 Ed25519 私钥 | ✅ 已实现 |
| **Ed25519 签名** | Go 原生 `crypto/ed25519` | 节点身份、消息签名、贡献账本验签 | ✅ 已实现 |
| **Anthropic API 兼容** | 原生 HTTP 适配层 | `/v1/messages` 端点，双向格式转换（Anthropic ↔ OpenAI） | ✅ 已实现 |
| **web_session 模板** | 原生 HTTP + Chrome 自动化 | 抽象无 API 的网页登录平台（Sider 为首个模板） | ✅ 已实现 |
| **SOCKS5 连接池** | `sync.Map` 缓存 Transport | 代理 Transport 复用，延迟从 5-7s 降至 0.3s | ✅ 已实现 |
| **VMess 代理** | 自定义 VMess 协议实现 | 支持通过 VMess 代理访问被墙平台 | ✅ 已实现 |
| **平台自动发现** | 后台异步扫描 + 管理面板审核 | 自动发现免费 AI 平台，一键导入为预设 | ✅ 已实现 |
| **Token 估算** | 优先上游 usage + tiktoken 降级 | 精准 Token 计数，支持额度管控 | ✅ 已实现 |

**选型原则**：
- **零外部依赖**：编译产物为单二进制文件，用户无需安装 runtime 或依赖
- **纯 Go 优先**：避免 CGO 依赖，确保交叉编译简单可靠
- **流式优先**：所有 HTTP 代理原生支持 SSE (Server-Sent Events) 流式转发
- **跨平台编译**：支持 linux-amd64 / linux-arm64 / linux-armv7 / windows-amd64 四平台交叉编译

---

## 2B. API 协议转换层（Protocol Translation Layer）

系统对外暴露统一的 **OpenAI 兼容 API**，但上游 AI 平台（Provider）的请求格式各异。协议转换层负责将各种厂商的底层接口标准化转换为 OpenAI 格式，使消费方无需感知异构差异。

> **实现状态（v4.0.5）**：OpenAI/DeepSeek/通义千问等兼容平台直接透传；Anthropic Messages API 通过原生 `/v1/messages` 端点提供双向兼容（消费方用 Anthropic SDK 可直接调用）；Gemini 通过 OpenRouter/SiliconFlow 等聚合平台间接支持。37 个预设平台已覆盖主流 AI 服务商。web_session 类型抽象了无 API 的网页登录平台（如 Sider），通过 Chrome 自动化实现。

### 2B.1 核心转换矩阵

| 上游平台 | 请求格式差异 | 角色映射差异 | 流式 Chunk 差异 | 转换复杂度 |
|---------|------------|------------|---------------|----------|
| **OpenAI** | 原生格式，无需转换 | 原生 | 原生 `data: {...}` | ★☆☆☆☆ |
| **Anthropic (Claude)** | `messages` 结构相似但 `system` 参数位置不同 | `user`/`assistant` 一致，`system` 需单独提取 | `event` + `delta` 双字段结构 | ★★☆☆☆ |
| **Google (Gemini)** | `contents` + `parts` 嵌套结构 | `user`/`model`（非 assistant），`function` 角色映射不同 | `candidates[0].content.parts[0].text` 增量拼接 | ★★★★☆ |
| **DeepSeek** | 兼容 OpenAI 格式 | 原生兼容 | 原生兼容 | ★☆☆☆☆ |
| **其他兼容 OpenAI 的平台** | 无需转换 | 无需转换 | 无需转换 | ★☆☆☆☆ |

### 2B.2 转换流程架构

```
消费方请求（OpenAI 格式）
  │
  ▼
┌─────────────────────────────────────────────────────────────────┐
│  Protocol Translator（协议转换器）                                │
│                                                                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Step 1: 请求反序列化                                       │  │
│  │  └── 解析 OpenAI 格式请求体 → 内部统一中间表示 (IR)          │  │
│  └─────────────────────────────┬─────────────────────────────┘  │
│                                │                                 │
│  ┌─────────────────────────────▼─────────────────────────────┐  │
│  │  Step 2: 角色映射 (Role Mapping)                            │  │
│  │  ├── OpenAI: system/user/assistant → 直接使用               │  │
│  │  ├── Gemini: system→user(with prefix), assistant→model      │  │
│  │  └── Anthropic: system 提取到顶层参数                        │  │
│  └─────────────────────────────┬─────────────────────────────┘  │
│                                │                                 │
│  ┌─────────────────────────────▼─────────────────────────────┐  │
│  │  Step 3: 载荷重写 (Payload Rewrite)                         │  │
│  │  ├── 按目标平台格式重组请求体                                │  │
│  │  ├── 注入 Provider Key（Authorization / x-api-key 等）       │  │
│  │  └── 适配平台特有参数（temperature、max_tokens 等映射）      │  │
│  └─────────────────────────────┬─────────────────────────────┘  │
│                                │                                 │
│  └── 发送至上游 AI 平台 API ──▶│                                 │
└─────────────────────────────────────────────────────────────────┘
                                │
                    上游平台返回响应
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  Response Translator（响应转换器）                                │
│                                                                  │
│  ┌─ 非流式：将平台响应格式 → OpenAI choices[0].message 格式      │
│  └─ 流式：逐 chunk 转换                                          │
│      ├── Gemini: candidates[0].content.parts[0].text             │
│      │          → data: {"choices":[{"delta":{"content":"..."}}]}│
│      ├── Anthropic: event=content_block_delta                    │
│      │          → data: {"choices":[{"delta":{"content":"..."}}]}│
│      └── 最终 chunk: 提取 usage 统计 → 写入 OpenAI usage 字段    │
└─────────────────────────────────────────────────────────────────┘
```

### 2B.3 内部统一中间表示 (IR)

所有请求在进入转换层后，先统一转换为内部中间表示（Intermediate Representation），再由 IR 转换为目标平台格式。这避免了 N×M 的转换矩阵（N 个消费端格式 × M 个上游平台格式），简化为 N+M。

```go
// 内部统一中间表示
type RequestIR struct {
    Model       string        `json:"model"`
    Messages    []MessageIR   `json:"messages"`
    SystemPrompt string       `json:"system_prompt,omitempty"`
    Temperature *float64      `json:"temperature,omitempty"`
    MaxTokens   *int          `json:"max_tokens,omitempty"`
    Stream      bool          `json:"stream"`
    Tools       []ToolIR      `json:"tools,omitempty"`
    // ... 其他通用参数
}

type MessageIR struct {
    Role    string `json:"role"`    // "system" | "user" | "assistant" | "tool"
    Content string `json:"content"`
}

// 转换接口
type PlatformAdapter interface {
    // IR → 平台原生请求格式
    TranslateRequest(ir *RequestIR) ([]byte, error)
    // 平台原生响应 → OpenAI 兼容格式
    TranslateResponse(raw []byte) (*OpenAIResponse, error)
    // 流式 chunk 转换
    TranslateStreamChunk(raw []byte) (*OpenAIStreamChunk, error)
    // 提取 usage 统计
    ExtractUsage(raw []byte) (*TokenUsage, error)
}
```

### 2B.4 新增 Provider 平台的接入流程

当需要支持新的 AI 平台时，只需实现 `PlatformAdapter` 接口：

| 步骤 | 操作 | 工作量 |
|------|------|--------|
| 1 | 实现 `TranslateRequest()` | 理解目标平台请求格式，编写映射逻辑 |
| 2 | 实现 `TranslateResponse()` + `TranslateStreamChunk()` | 理解响应格式，编写反向映射 |
| 3 | 实现 `ExtractUsage()` | 从响应中提取 token 消耗统计 |
| 4 | 在管理面板注册新 Platform Adapter | 用户选择平台后自动加载对应适配器 |

**BT 类比**：协议转换层类似 BT 客户端支持的多种 Peer 协议（uTP / TCP / HTTP Seeds）——底层传输各异，但上层客户端看到的接口统一。

---

## 3. 密钥体系 v4.0

### 3.1 设计哲学

BT 客户端只需一个端口号即可加入网络——密钥体系同样追求极简。v3.0 将原先 6+ 种 Key 精简为 4 种，**全部兼容 OpenAI SDK**（`sk-` 前缀），用户无需理解复杂概念即可使用。

**BT 类比**：BT 客户端连接 Tracker 时使用 peer_id 标识自己；OpenModelPool 的 Proxy API Key 就是节点的 peer_id，全球公共 Key 就是所有人的匿名通行证。

### 3.2 四种密钥完整定义

| # | Key 类型 | 格式 | 归属 | 说明 |
|---|---------|------|------|------|
| 1 | **Proxy API Key** | `sk-{48位random}` | 节点运营者 | 节点主 Key，加入网络后可路由全网资源 |
| 2 | **Guest Proxy Key** | `sk-guest-{node_id}-{random}` | 节点签发给他人 | 派生 Key，兼容 OpenAI SDK，受签发节点额度约束 |
| 3 | **全球公共 Key** | `sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1` | 全局固定常量 | 全网统一免费 Key，零门槛但有额度上限 |
| 4 | **Provider Key** | 各平台原始格式 | AI 平台 | 上游服务凭证，不参与网络通信 |

#### 3.2.1 Proxy API Key — 节点保密凭证

```
格式: sk-a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4
      └──────────────── 48位随机字符 ────────────────────────┘
```

- **归属**：节点运营者，每节点一个
- **保密性**：**必须严格保密，不可泄露**。传播中一旦泄露可能被恶意使用，节点运营者应随时可更换（轮换/重新签发）
- **泄露后 Guest Key 处理**：Proxy API Key 泄露并被节点停用后，**已签发的 Guest Key 不自动失效**。设计理由：Guest Key 的有效期和额度由节点主人手工控制，节点主人可通过管理面板逐个禁用或批量撤销特定 Guest Key，无需依赖自动级联失效机制。手工控制的优势：(1) 避免误伤正常使用中的 Guest Key；(2) 节点主人可逐条审计后选择性保留；(3) 简化系统逻辑，减少隐式行为
- **与 NodeID 的关系**：Proxy API Key 与节点的 peer_id（NodeID）**完全不同**——NodeID 是公开的 Ed25519 公钥编码（`mmx-abc123...`），用于网络身份标识；Proxy API Key 是私有凭证，用于 API 认证。二者分离设计，确保身份公开性与凭证保密性独立
- **权限**：完全访问自己节点的所有 Provider；加入网络后路由范围扩展到全网资源池
- **额度消耗**：与 Guest Key 行为一致——优先消耗签发节点（即自身）的 Provider 额度，不足时回退到 Guest Key Pool 消耗均分额度。无需单独设置额度
- **管理权限**：Proxy API Key 拥有节点最高管理权限，可用于**重置管理员登录密码**
- **个人使用建议**：节点运营者日常自用**优先推荐 Guest Proxy Key**——由自己签发给自己，避免 Proxy API Key 在日常使用中意外泄露
- **核心价值**：算力交换的入场券。你有 GPT-4o 闲置额度但缺 Claude，另一个节点正好相反——加入网络后双方各取所需
- **BT 类比**：类似 BT 客户端的**连接密码**（而非 peer_id）——拥有它意味着你可加入 Swarm，但它不是你的公开身份，泄露后应立即更换

#### 3.2.2 Guest Proxy Key — 节点签发的邀请函

```
格式: sk-guest-mmx-abc123def456-x9y8z7w6v5u4
            └──── node_id ────┘ └─ random ─┘
```

- **归属**：由节点主人签发，分发给指定用户
- **数量**：每节点可签发多个
- **权限范围**：
  - **API 调用**：使用模型资源（额度消耗逻辑与 Proxy API Key 一致，优先用签发节点 Provider，不足时回退 Guest Key Pool）
  - **协作账号密码重置**：可重置被该 Guest Key 邀请进入协作的账号密码
  - **不能**：重置管理员登录密码（仅 Proxy API Key 有此权限）
- **额度**：全网均分制，无需逐 Key 设定（详见 §3.2.2.1）
- **BT 类比**：类似 BT 中你告诉朋友"用我的客户端下载"——朋友通过你的通道获取资源，但受你带宽限制

#### Guest Key 智能路由：公共池访问权限

Guest Key 签发时可选择是否授予 **Public Pool 访问权限**（`access_public_pool` 字段）。

| 权限 | 行为 | 类比 |
|------|------|------|
| 无公共池权限（默认） | 只能访问签发节点本地资源，请求不跨节点 | 普通访客通行证 |
| 有公共池权限 | 可在任意共享节点访问全网资源，行为等同 Public Key | VIP 全网通行证 |

**路由决策树**：
```
请求到达任意节点 → 检查 Key 类型
  ├─ Proxy API Key → public 路由（全网池）
  ├─ Guest Key + access_public_pool=true → public 路由（全网池）
  ├─ Guest Key + access_public_pool=false → guest 路由（签发节点本地资源）
  └─ Public Key → public 路由（全网池）
```

**约束**：
- 未加入共享网络的节点签发的 Guest Key，无论是否有公共池权限，都只能通过该节点自身的 URL 访问（因为没有路由转发能力）
- 节点接入共享网络后，其 Guest Key 路由信息自动通过 Gossip 广播全网，其他节点可转发请求


#### 3.2.2.1 Guest Key 额度模型 — 全网均分制

节点在共享网络中设置自己愿意贡献的总额度，以及 **Guest Key / Public Key 的分配比例**。

**核心原则**：Guest Key 和 Public Key 在消耗公共资源时，机制完全相同——都是从所有人共享出来的资源池中均分。每个节点设置的比例决定往哪个池子注入多少，但消耗方式没有区别。

**节点共享设置**：

每个节点设置自己的总共享额度，以及 Guest Key 与 Public Key 之间的分配比例：

```
节点 A：总共享额度 10,000 tokens/天 → Guest Key 60% / Public Key 40%
  → 贡献给 Guest Key Pool：6,000
  → 贡献给 Public Key Pool：4,000

节点 B：总共享额度 5,000 tokens/天 → Guest Key 30% / Public Key 70%
  → 贡献给 Guest Key Pool：1,500
  → 贡献给 Public Key Pool：3,500

节点 C：总共享额度 20,000 tokens/天 → Guest Key 50% / Public Key 50%
  → 贡献给 Guest Key Pool：10,000
  → 贡献给 Public Key Pool：10,000

═══════════════════════════════════════════════════
  Guest Key Pool 合计：17,500 tokens/天
  Public Key Pool 合计：17,500 tokens/天
```

**额度分配规则**：

```
┌───────────────────────────────────────────────────────────┐
│  Guest Key Pool（Guest Key 共享池）                        │
│  来源：全网各节点设置的 Guest Key 份额汇总                  │
│  分配：全网签发的 Guest Key 总数 均分                       │
│  公式：每个 Guest Key = Pool ÷ 全网 Guest Key 总数          │
└───────────────────────────────────────────────────────────┘

┌───────────────────────────────────────────────────────────┐
│  Public Key Pool（Public Key 共享池）                       │
│  来源：全网各节点设置的 Public Key 份额汇总                  │
│  分配：有效 Public Key 数量 均分                            │
│  公式：每个 Public Key = Pool ÷ 有效 Public Key 数量        │
│  有效定义：24 小时内有实际调用记录的 Public Key               │
└───────────────────────────────────────────────────────────┘
```

**逐 Key 本地额度**：

节点管理员可以为自身签发的每个 Guest Key 设置**本地资源访问额度上限**。这个额度控制的是该 Key 能从签发节点自身 Provider 中消耗多少资源，与公共池无关。

```
节点 A 设置了 3 个 Guest Key：
  Key-1：本地额度 5,000 tokens/天
  Key-2：本地额度 3,000 tokens/天
  Key-3：未设限（默认不限 / 使用节点总额度上限）
```

- 本地额度由节点管理员在签发或管理 Guest Key 时设定
- 只约束访问签发节点自身资源的消耗，不影响公共池均分
- 未设限时，该 Key 可消耗签发节点的全部可用额度（受节点总共享额度约束）

**Guest Key 的消耗优先级**：

Guest Key 在使用时遵循"先本地、后公共"的原则，本地消耗受逐 Key 额度约束：

```
Guest Key 调用请求到达
  │
  ├─ 签发节点自身有该模型的 Provider？
  │   ├─ 是 → 优先消耗签发节点自身的 Provider 额度（受该 Key 的本地额度上限约束）
  │   │       ├─ 本地额度未用完 → 从签发节点 Provider 扣减
  │   │       └─ 本地额度已用完 → 回退到 Guest Key Pool（全网共享池），消耗均分额度
  │   └─ 否 → 回退到 Guest Key Pool，消耗均分额度
  │
  └─ 公共池均分额度也已耗尽？
      └─ 返回 429 限流
```

即：Guest Key 持有者通过签发节点的 Provider 访问模型时，**先消耗本地额度（受逐 Key 上限约束），不消耗公共池额度**。只有当签发节点没有所需的模型、或该 Key 的本地额度已用完时，才会回退到 Guest Key Pool 消耗均分份额。

**Public Key 的消耗方式**：

Public Key 与 Guest Key 消耗公共资源的逻辑一致——从 Public Key Pool 中均分额度。每个有效 Public Key（24h 内有调用）获得相同份额，不经过任何单一节点。

**动态调整**：

- 额度每 24 小时重新计算一次
- Guest Key 签发/撤销 → Guest Key 总数变化 → 每个 Key 的均分量变化
- Public Key 活跃度变化 → 有效 Key 数量变化 → 每个 Key 的均分量变化
- 节点调整共享比例 → Pool 总量变化 → 所有 Key 的均分量变化

**示例**：

```
场景：节点 A 签发 Guest Key-1，本地额度设为 5,000 tokens/天
  Guest Key Pool = 17,500 tokens/天，全网有 7 个 Guest Key
  → Key-1 公共池均分额度：17,500 ÷ 7 = 2,500 tokens/天

  调用 GPT-4o：
  → 签发节点有 GPT-4o Provider → 先扣本地额度（上限 5,000）
  → 本地额度用完 5,000 后 → 回退公共池，消耗 2,500 均分额度
  → 公共池也耗尽 → 429 限流

  调用 Claude-3（签发节点无此模型）：
  → 直接走公共池均分额度 2,500 tokens/天

场景：Public Key Pool = 17,500 tokens/天，24h 内有效的 Public Key 有 5 个
  → 每个有效 Public Key 可用：17,500 ÷ 5 = 3,500 tokens/天
```

**设计优势**：

- **简单 + 灵活**：公共池均分保持简单，逐 Key 本地额度让节点精细控制自己的资源消耗
- **公平**：贡献越多 Pool 越大，但分配均分，多劳者扩大池子惠及所有人
- **本地优先**：Guest Key 先用签发节点自身资源（受本地额度约束），减少对公共池的不必要消耗
- **可控**：节点管理员可为每个 Guest Key 设定本地额度上限，防止单个 Key 耗尽节点资源
- **自适应**：用的人多自动摊薄，类似 BT 带宽分配
- **透明**：管理面板可实时展示 Pool 总量、Key 数量、每个 Key 当前均分量和本地额度使用情况


#### 3.2.3 全球公共 Key — 全网统一的免费通行证

```
格式: sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1
      └────────── 品牌信息嵌入 ──────────────┘
```

- **定位**：**低额度体验入口**，全网固定常量。严格限流，不保证可用/稳定
- **品牌信息**：Key 中嵌入项目主站 `openmodelpool.com` 和 GitHub 仓库 `github.com/lisiyu/openmodelpool`，如同品牌名片
- **与节点完全无关**：全球公共 Key 不绑定任何节点
- **零门槛**：无需注册、无需搭建节点，下载即用
- **权限**：访问全网已加入共享网络的节点贡献的共享资源
- **额度约束**：额度由公共池动态分配，但**必须受以下多重上限约束**：
  - 全局总量上限（全网公共 Key 共享池总额）
  - 单 IP 上限（防止单用户滥用）
  - 单时间窗口上限（如每小时 1000 tokens）
  - 单模型上限（每种模型分配固定额度）
- **不参与贡献权益**：使用公共 Key 不产生贡献积分，不享受路由优先
- **不保证可用**：公共池额度耗尽时返回 429/503，不提供 SLA
- **无 BT 类比关系**：全球公共 Key 的访问模式与 BT 协议的做种/下载机制本质不同——它是全网统一的匿名访问凭证，不依赖任何 BT 对等交换逻辑

#### 3.2.4 Provider Key — 上游服务的凭证

```
格式: 各平台原始格式（如 sk-proj-xxxxx）
```

- **归属**：各 AI 平台
- **数量**：每个 Provider 可配多条
- **管理**：支持别名、独立额度、优先级、启用/禁用
- **不参与网络通信**：Provider Key 仅在本地使用，通过加密器加密存储
- **BT 类比**：类似 BT 客户端连接的源站——数据来自源站，但客户端之间的共享不需要暴露源站信息

### 3.3 已废弃 Key 类型

| 原类型 | 前缀 | 废弃原因 | 替代方案 |
|--------|------|---------|---------|
| ~~试用 Key~~ | `mk_trial_` | 被 Guest Proxy Key 完全覆盖 | Guest Proxy Key |
| ~~开放 Key (未绑定)~~ | `mk_open_` | 格式不兼容 OpenAI SDK | Guest Proxy Key |
| ~~开放 Key (已绑定)~~ | `mk_open_{node}_` | 格式不兼容 OpenAI SDK | Guest Proxy Key |
| ~~Global Key~~ | `mk_open_global_` | 含 node_id 和签名，非固定常量 | 全球公共 Key |
| ~~消费者 Key~~ | `mk_{consumer_id}` | 过于复杂 | Guest Proxy Key |
| ~~标准签名 Key~~ | `mk_{consumer_id}.{payload}.{sig}` | Ed25519 签名 Key，用户不友好 | Guest Proxy Key |

**代码清理已完成（✅ v4.0.5）**：上述 6 种 mk_* 密钥的签发/验证/解析逻辑已全部删除（约 530 行），`ClassifyKey()` 现仅保留 3 个分支（Public / Guest / Proxy）+ 1 个 Unknown 拒绝分支。当前实现严格 4 种 Key 类型（Proxy API Key / Guest Proxy Key / 全球公共 Key / Provider Key）。

### 3.4 密钥分类逻辑

```go
func ClassifyKey(key string) KeyType {
    switch {
    case key == GlobalPublicKey:  // "sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1"
        return KeyTypePublic
    case strings.HasPrefix(key, "sk-guest-"):
        return KeyTypeGuest
    case strings.HasPrefix(key, "sk-"):
        return KeyTypeProxy   // Proxy API Key 或 Provider Key
    default:
        return KeyTypeUnknown  // 拒绝所有旧格式 mk_*
    }
}
```

### 3.5 路由规则

**Key 类型 + 节点是否加入共享网络 = 路由范围**

| Key 类型 | 节点未加入网络 | 节点已加入网络 |
|---------|--------------|--------------|
| **Proxy API Key** | 自己的 Provider（保密使用，泄露可随时更换） | 自己 Provider + 全网资源池 |
| **Guest Proxy Key** | 签发节点的 Provider | 签发节点 Provider + 网络（受额度约束） |
| **全球公共 Key** | 任意可达网络池 base_url + 公共 Key 访问全网共享资源 | 同上——与节点是否加入网络无关 |
| **Provider Key** | 不适用（不是用户 Key） | 不适用 |

**BT 类比**：

```
Proxy API Key = 你的 BT 连接密码（保密，泄露可更换）→ 既能下载也能上传
  └── 泄露后：更换连接密码 ≠ 自动吊销已签发给朋友的临时通行证，需手工逐个禁用
Guest Proxy Key = 朋友借用你的连接密码 → 受你带宽限制
全球公共 Key = 全网统一免费通行证（与 BT 无类比关系）→ 任意入口均可访问全网资源
```

### 3.6 代码变更清单

| 变更项 | 操作 | 影响文件 |
|--------|------|---------|
| 移除 `KeyTypeTrial`、`KeyTypeOpenUnbound`、`KeyTypeOpenBound`、`KeyTypeGlobal`、`KeyTypeStandard` | 删除 | `network_keys.go` |
| 新增 `KeyTypeGuest` | 新增 | `network_keys.go` |
| 简化 `ClassifyKey()` 至 3 类 | 重写 | `network_keys.go` |
| 移除 `mk_trial_`、`mk_open_`、`mk_open_global_` 解析逻辑 | 删除 | `network_keys.go` |
| 全球公共 Key 常量替换 | 替换 | `network_keys.go` |
| 移除 Ed25519 签名验证相关代码 | 删除 | `network_keys.go` |
| Guest Proxy Key 签发/验证/撤销 | 新增 | `network_keys.go` |

---

## 4. 双模式节点模型（Personal / Network）

### 4.1 设计哲学：默认个人版，主动升级为共享节点

v4.0 采用**双模式架构**：软件默认以个人版（Personal Mode）启动，不加入任何 P2P 网络。只有用户主动选择加入共享网络后，才升级为共享版（Network Mode），生成 Node ID 和助记词。

**设计理念**：

```
默认 Personal Node（个人版）
  → 配置 Token + 有闲置额度 → 温和提示
  → 用户主动选择加入 → 生成助记词 + Node ID
  → 升级为 Shared Peer（共享版）
```

**BT 类比**：BT 客户端默认只是本地下载工具；只有当你开始做种时才成为 Swarm 的一部分。OpenModelPool 同理——个人版是本地工具，共享版才是网络公民。

### 4.2 节点状态模型

```
                    ┌──────────────────────────┐
                    │    Personal Mode（默认）    │
                    │  network_enabled = false    │
                    │  仅本地 Provider 代理       │
                    │  无 Node ID / 无 P2P       │
                    └────────────┬───────────────┘
                                 │ 满足条件 + 用户主动加入
                                 ▼
                    ┌──────────────────────────┐
                    │    Network Mode（共享版）   │
                    │  network_enabled = true     │
                    │  已生成助记词 + Node ID     │
                    │  可配置 share_to_pool       │
                    └────────────┬───────────────┘
                                 │ 开启 share_to_pool
                                 ▼
                    ┌──────────────────────────┐
                    │    Shared Peer（共享节点）  │
                    │  share_to_pool = true       │
                    │  贡献 Provider 到共享池     │
                    │  可消费全网资源             │
                    └──────────────────────────┘
```

| 状态 | network_enabled | share_to_pool | 行为 | BT 类比 |
|------|----------------|--------------|------|---------|
| **Personal Mode** | `false` | N/A | 纯本地代理，不联网 | 未加入 Swarm 的本地客户端 |
| **Network Mode（不共享）** | `true` | `false` | 可消费全网，不贡献 | 只下载不做种的 peer |
| **Shared Peer** | `true` | `true` | 贡献 Provider + 消费全网 | 同时做种和下载 |

**关键设计**：
- **两级开关**：`network_enabled`（是否加入共享网络）与 `share_to_pool`（是否共享额度）分离
- 加入共享网络 ≠ 自动共享额度
- 用户可加入网络但不共享（纯消费），也可共享部分额度
- 角色可动态切换：Personal → Network → Shared Peer，或反向回退
- 助记词一旦生成，即使回退到 Personal Mode 仍保留（供未来恢复）

### 4.3 节点信息结构

```json
{
  "node_id": "mmx-abc123def456",
  "endpoint": "https://ai.example.com",
  "pub_key": "ed25519:Ak3x...",
  "share_to_pool": true,
  "shared_models": ["gpt-4o", "claude-3.5-sonnet"],
  "shared_providers": [
    {"platform": "openai", "models": ["gpt-4o"], "capacity": 85}
  ],
  "is_gateway": true,
  "joined_at": "2026-07-01T00:00:00Z",
  "last_seen": "2026-07-09T13:45:00Z",
  "status": "active",
  "version": "3.0.0"
}
```

### 4.4 能力声明（CapabilityClaim）

节点广播其可提供的 AI 模型服务——类比 BT 的 `bitfield` 消息声明对等方持有哪些数据块。

```go
type CapabilityClaim struct {
    NodeID      string            `json:"node_id"`
    Timestamp   time.Time         `json:"timestamp"`
    ExpiresAt   time.Time         `json:"expires_at"`
    Models      []ModelCapability `json:"models"`
    MaxConcurrent int             `json:"max_concurrent"`
    RateLimit     int             `json:"rate_limit"`
    Signature   []byte            `json:"signature"`  // Ed25519 签名
}

type ModelCapability struct {
    Provider    string  `json:"provider"`
    Model       string  `json:"model"`
    Available   bool    `json:"available"`
    AvgLatency  int64   `json:"avg_latency_ms"`
    SuccessRate float64 `json:"success_rate"`
}
```

**BT 类比**：

```
BitTorrent:  "我持有数据块 [0,2,3,6,7]"          → bitfield 消息
OpenModelPool: "我能提供 [gpt-4o, claude-3, deepseek]" → CapabilityClaim
```

### 4.5 与旧模型的对比

| 旧模型 (v2/v3) | 新模型 (v4.0) | 变更原因 |
|------------|-------------|---------|
| Bootstrap Node — 硬编码引导节点 | 所有节点都是 peer，:8001 Seed 端点 | 去中心化，无特殊节点 |
| Ordinary Node — 普通节点 | 统一 peer 模型 | 消除类型歧视 |
| 默认加入网络 | 默认 Personal Mode，主动升级为 Network Mode | 保护用户隐私和自主权 |
| 首次启动静默生成 Node ID | 加入共享网络时才生成（助记词机制） | 避免用户不知要备份导致贡献丢失 |
| Solo / Seed 二分 | Personal / Network / Shared Peer 三级 | 更清晰的产品模式分层 |
| 单一 share_to_pool | network_enabled + share_to_pool 两级开关 | 加入网络 ≠ 自动共享额度 |

---

## 5. P2P 网络架构

### 5.1 架构总览

```
┌──────────────────────────────────────────────────────────────────┐
│                        应用层                                      │
│  ┌──────────┐  ┌──────────────┐  ┌──────────┐  ┌─────────────┐ │
│  │ API      │  │ Model Relay  │  │ 能力     │  │ 负载均衡    │ │
│  │ Gateway  │  │ Handler      │  │ Manager  │  │ (五维评分)  │ │
│  └──────────┘  └──────────────┘  └──────────┘  └─────────────┘ │
├──────────────────────────────────────────────────────────────────┤
│                    路由与发现层                                    │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐│
│  │ Kademlia DHT │  │ Gossip 协议  │  │ Seed 端点 (:8001)     ││
│  │ (256-bit)    │  │ (节点扩散)   │  │ + GitHub 注册表       ││
│  │ k=20, α=10   │  │              │  │ + AddrMan (peers.dat) ││
│  └──────────────┘  └──────────────┘  └────────────────────────┘│
├──────────────────────────────────────────────────────────────────┤
│                    贡献账本层                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐│
│  │ Layer 1:     │  │ Layer 2: IPFS（可选持久化）                                ││
│  │ Gossip Ledger│  │  └──────────────┘  └────────────────────────────────────────────┘│
├──────────────────────────────────────────────────────────────────┤
│                        传输层                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐│
│  │ HTTP/HTTPS   │  │ TLS 1.3      │  │ WebSocket (备选)      ││
│  │ 连接池       │  │              │  │ (未来: QUIC)          ││
│  └──────────────┘  └──────────────┘  └────────────────────────┘│
└──────────────────────────────────────────────────────────────────┘
```

### 5.1A P2P 网络穿透架构（方案 B：混合降级模式）

针对个人电脑复杂的局域网/NAT 环境，采用 **"直连优先 + 官方 Relay 中继降级"** 的混合网络架构。

#### 官方引导与中继节点 (Bootstrap & Relay Node)

| 属性 | 说明 |
|------|------|
| **部署位置** | 官方主服务器，绑定域名 `seed1.openmodelpool.com` |
| **职责 1 — 寻址路由** | 利用 GitHub 存放的静态 JSON 列表或直接通过 DNS TXT 记录（`dnsaddr`），让新节点找到引导节点并获取全网路由表 |
| **职责 2 — Circuit Relay v2 中继** | 当两个内网节点（NAT 防火墙严苛）无法直接建立连接时，充当流量中继站 |
| **带宽保护机制** | 强制限制单次中继连接时长上限为 **2 分钟**，仅限纯文本数据流，保护官方服务器带宽不被恶意刷爆 |

#### 客户端底层连接策略 (Fallback 机制)

```
连接策略流程：

1. 直连探测
   └── 请求节点优先向目标节点发起直连（UDP/TCP 打洞）

2. 直连建立（理想路径）
   └── 若 5 秒内打洞成功 → 双方直接端到端通信
       ├── 不消耗官方服务器流量
       └── 延迟最低

3. 中继降级（Fallback）
   └── 若 5 秒内直连失败 → 自动向 seed1.openmodelpool.com 申请 Relay 隧道
       ├── 通过 Circuit Relay v2 完成借道通信
       └── 单次连接上限 2 分钟，纯文本数据流
```

**设计意图**：大多数节点间可直连（消耗零服务器带宽），仅 NAT 严苛的少数场景走中继。官方服务器仅承担引导和降级中继职责，不承担常规流量转发。

#### 5.1A.1 流式传输抗抖动与防丢包机制

在实际物理网络中，硬件故障（如交换机丢包 Bug）或跨国链路常导致偶发性数据传输卡顿和丢包。大模型输出为纯文本 SSE 数据流，UDP 穿透成功后的直连极易受网络抖动影响。

**应用层抗丢包重传机制**：

```
SSE 流式传输的可靠性保障：

┌─────────────────────────────────────────────────────────────────┐
│  动态抖动缓冲 (Jitter Buffer)                                    │
│  ├── 接收端维护自适应缓冲区（初始 200ms，动态调整至 50ms-2s）      │
│  ├── 检测 chunk 到达间隔的标准差（σ），缓冲时间 = μ + 2σ          │
│  ├── 网络平稳时缓冲缩小（低延迟），抖动剧烈时缓冲增大（抗丢包）    │
│  └── 缓冲区满后按序输出 chunk，用户感知为连续流式文本              │
├─────────────────────────────────────────────────────────────────┤
│  应用层选择性重传 (Selective Retransmission)                      │
│  ├── 每个 SSE chunk 附带递增序号 (chunk_seq)                     │
│  ├── 接收端检测序号空洞 → 请求发送端重传丢失 chunk                │
│  ├── 重传超时：500ms 无响应则跳过（SSE 容忍少量丢失）             │
│  └── 仅对关键 chunk（usage 统计、final chunk）强制重传            │
└─────────────────────────────────────────────────────────────────┘
```

**Relay 超时动态延长机制**：

对于超过 2 分钟的长上下文或复杂代码生成场景，强制断开 Relay 会导致生成截断。改进方案：

| 条件 | Relay 超时策略 |
|------|--------------|
| 常规请求（< 2 分钟） | 保持 2 分钟上限 |
| 长上下文生成 | 实时检测 Token 生成状态，若上游 API 持续返回 SSE chunk，每收到新 chunk 重置 30s 倒计时 |
| 复杂代码生成 | 同上，但额外限制最大延长至 **10 分钟**，防止无限占用中继带宽 |
| 检测到上游 API 无响应 > 60s | 主动断开，返回已生成部分 + 超时提示 |

```go
// Relay 超时动态延长伪代码
func relayTimeoutHandler(stream <-chan SSEChunk) time.Duration {
    baseTimeout := 2 * time.Minute
    maxTimeout := 10 * time.Minute
    idleTimeout := 30 * time.Second
    
    lastChunkTime := time.Now()
    for chunk := range stream {
        lastChunkTime = time.Now()
        // 每收到新 chunk，重置 idle 倒计时
    }
    // 超过 idleTimeout 无新 chunk → 判定为上游卡死，断开
    // 总时长超过 maxTimeout → 强制截断
    return min(baseTimeout + elapsed_idle_resets, maxTimeout)
}
```

#### HTTPS-only 通信策略

所有节点间通信（Relay 转发、Gossip 交换、Bootstrap 查询、Discovery 刷新）**强制 HTTPS**，不允许 HTTP。

**HTTPS 来源优先级**：
1. 固定域名 + Let's Encrypt/Cloudflare 证书 → 最稳定
2. Cloudflare Quick Tunnel → 无固定 IP 的节点，自动获得 `https://xxx.trycloudflare.com`
3. 固定公网 IP + 自签证书 → 技术能力强的节点

**信任池内自签证书互信**：
- 信任池（Trust Pool）内的节点间通信跳过 TLS 证书校验（`InsecureSkipVerify: true`）
- 原理：节点已通过 GitHub 注册表 + Ed25519 签名验证身份，HTTPS 仅保证传输加密
- 所有节点间 HTTP client 统一使用信任池 Transport

**collectAddresses() 地址收集**：
- Cloudflare 隧道 URL（`tunnel_url`）
- 自定义域名（`tunnel_domain` → `https://`）
- 公网 IP 检测（`detectPublicIP()` → `https://IP:port`）
- 本地地址（`https://localhost:port`）

### 5.2 BT 概念完整映射

| BT 概念 | OpenModelPool 对应 | 说明 |
|---------|-------------------|------|
| .torrent 文件 / Tracker | GitHub 注册表（初始种子源） | 冷启动时提供初始节点列表，类比 .torrent 文件包含 Tracker 地址 |
| 初始做种者 | 官方 Seed 节点 | 创始人节点作为首个 Seed，类比 BT 的初始做种者 |
| DHT 网络 | Gossip 协议 + Kademlia DHT（256 位） | 节点发现与路由，类比 BT 的 DHT 实现去中心化发现 |
| Peer 互换 (PEX) | 节点互换 Gateway 地址 | 通过 Gossip 交换已知节点信息，类比 BT 的 PEX 消息 |
| 只要一人做种全网可下载 | 只要一个 Gateway 活着全网可达 | 共享池中任一可用节点即可服务请求 |
| 上传量 / 分享率 | 节点贡献值 / 权重 | 衡量节点的"慷慨程度"，影响路由优先级 |
| leech 最低速 | 公共 Key 基础额度 | 不贡献也能使用，但速度/额度受限 |
| seeder 优先 | 高权重用户优先调度 | 贡献多的节点路由优先级更高 |
| Choking 算法 | 额度限制 + 优先队列 | 限制低贡献节点的访问，优先服务高贡献节点 |
| Optimistic Unchoke | 新节点基础额度 | 给新节点机会体验网络，类比 BT 的乐观取消阻塞 |

### 5.3 DHT 配置

```
哈希空间:     256 位 (SHA-256)
距离度量:     XOR 度量
K-Bucket 大小: k = 20
桶数量:       256（每比特一个）
查找 α:       10（P50 延迟约 0.3s）
查找 β:       3（终止条件）
刷新间隔:     10 分钟
查询超时:     10 秒
记录 TTL:     48 小时
```

**为什么选择 256 位而非 BT 的 160 位？**
- BT 使用 SHA-1（160 位），足够支撑约 800 万节点
- 256 位提供 2^96 倍的更大地址空间，支持更大规模网络
- 与 IPFS/Kademlia 标准匹配，为未来互操作预留空间

### 5.4 网络发现三层机制

节点发现采用三层机制，从中心化引导逐步过渡到完全去中心化——与 BT 从 Tracker 到 DHT 的演进路径完全一致：

```
第一层: GitHub 注册表（类比 .torrent 文件）
  ↓ 启动时拉取
第二层: :8001 Seed 端点（类比 BT 的 addr 消息）
  ↓ 节点互换
第三层: Gossip 协议扩散（类比 BT 的 DHT）
  ↓ 持续同步
AddrMan 地址管理器（类比 BT 的地址簿 / peers.dat）
```

#### 5.4.1 第一层：GitHub 注册表（Bootstrap）

类比 .torrent 文件——包含初始 Tracker 地址和文件元信息。

```json
// GitHub: lisiyu/openmodelpool/federation/trust_pool.json
{
  "version": 3,
  "nodes": [
    {
      "node_id": "mmx-xxx",
      "url": "https://ai.chal.cc",
      "models": ["gpt-4o", "claude-3-opus"],
      "is_gateway": true,
      "last_heartbeat": "2026-07-09T16:05:00Z"
    }
  ]
}
```

**客户端三层获取策略（自动降级）**：
1. GitHub Raw：`raw.githubusercontent.com/lisiyu/openmodelpool/main/federation/trust_pool.json`
2. GitHub Pages：`lisiyu.github.io/openmodelpool/federation/trust_pool.json`
3. 已知活跃节点 P2P 互备
4. 本地缓存兜底（AddrMan / peers.dat）

**更新频率**：5 分钟，带 ETag 增量更新，无变化时 304。

#### 三级信任源降级

节点发现（Discovery）采用三级降级策略，避免单点故障：
1. **GitHub Registry**（主源）→ 从 `trust_pool.json` 获取信任池
2. **Bootstrap/Seed 节点**（备用）→ 向已知 seed 节点请求 `/federation/pool`
3. **Active Peers**（兜底）→ 从当前活跃的 P2P peer 列表构建

任何一级成功即停止降级。

#### 5.4.1A 备用信标：DNS TXT 记录寻址

在极端网络情况下（如特定区域对 GitHub 的网络阻断），整个网络的新节点冷启动将面临瘫痪。增加 DNS TXT 记录作为比 GitHub 更底层的兜底寻址方案：

```
DNS TXT 备用信标设计：

域名: _openmodelpool._tcp.openmodelpool.com
记录类型: TXT
记录值: "v=omp1;nodes=mmx-aaa|https://node-a.com:8001,mmx-bbb|https://node-b.com:8001"

解析流程：
  1. 客户端尝试 GitHub Raw → 失败
  2. 客户端尝试 GitHub Pages → 失败
  3. 客户端执行 DNS TXT 查询:
     dig TXT _openmodelpool._tcp.openmodelpool.com
     → 获取初始节点列表（紧凑格式）
  4. DNS 解析几乎不受 GFW 或网络审查影响
     （DNS 可走 DoH/DoT 加密通道）
```

**DNS TXT 记录约束**：

| 约束 | 说明 |
|------|------|
| **单条记录上限** | DNS TXT 单条最大 255 字符，需拆分为多条 |
| **记录格式** | `v=omp1;nodes={node_id}|{url},{node_id}|{url}...` |
| **多条记录** | `_omp1._tcp.openmodelpool.com`（第一条）、`_omp2._tcp...`（第二条）... |
| **TTL** | 300 秒（5 分钟），确保节点下线后快速生效 |
| **更新机制** | 官方 Seed 节点定期更新 DNS 记录（通过 DNS Provider API） |

**完整冷启动降级链**：

```
节点冷启动寻址优先级：

1. 本地 AddrMan 缓存 (peers.dat)         → 毫秒级，最近使用过的节点
2. GitHub Raw (trust_pool.json)           → 秒级，权威节点列表
3. GitHub Pages (静态站点镜像)             → 秒级，CDN 加速
4. DNS TXT 记录 (_omp._tcp 域名查询)      → 秒级，底层兜底
5. DoH/DoT 加密 DNS 查询                  → 当常规 DNS 被劫持时启用
6. 硬编码的创始节点 IP (fallback)          → 最后手段，写死在二进制中
```

**BT 类比**：BT 客户端内置 Tracker 地址硬编码作为最终兜底；OpenModelPool 同理，将创始节点信息硬编码在二进制中，确保任何情况下都能找到至少一个入口。

#### 5.4.2 第二层：:8001 Seed 端点

每个节点暴露 `:8001` 端口做 Seed——类比 BT 的 `addr` 消息，节点互相交换已知节点列表。

```
节点端口分配：
  :8000 — API 服务（处理请求）
  :8001 — Seed 服务（节点发现）
```

```go
// :8001/api/peers — 每个节点都跑的 Seed 端点
func handlePeers(w http.ResponseWriter, r *http.Request) {
    nodes := addrMan.GetKnownNodes()
    json.NewEncoder(w).Encode(nodes)
}
```

**BT 类比**：BT 节点通过 `addr` 消息交换已知节点地址；OpenModelPool 节点通过 `/api/peers` 端点交换已知节点信息。零额外部署成本——Seed 跟 API 服务跑在同一台机器上。

#### 5.4.3 第三层：Gossip 协议扩散

类比 BT 的 DHT——节点通过 Gossip 协议自动扩散节点信息，无需中心化注册表。

**Gossip 消息类型**：

| 消息 | 方向 | 内容 | 频率 | BT 对应 |
|------|------|------|------|---------|
| `PING` | 双向 | 节点 ID + 版本 + 时间戳 | 每 30s | keepalive |
| `PONG` | 回复 | 确认 + 时间戳 | 收到 PING 即回 | keepalive ack |
| `GET_PEERS` | 请求方 | 已知模型列表（可选过滤） | 每 5min | get_peers |
| `PEERS` | 响应方 | 已知节点列表（最多 50 个） | 收到请求即回 | addr 消息 |
| `ANNOUNCE` | 广播 | 自身信息（ID, URL, models） | 加入时 + 每 10min | announce_peer |

#### 多地址 Gossip 通信

Gossip 协议不再依赖单个 `Endpoint` URL，改为遍历节点的 `Addresses` 列表逐个尝试：
- 优先使用 `peer.Addresses`（可能包含隧道 URL + 公网 IP + localhost）
- 回退到 `peer.Endpoint`（向后兼容）
- 一个地址失败自动尝试下一个，提升网络健壮性

#### 5.4.4 AddrMan 地址管理器

每个节点维护一个本地地址管理器，类比比特币的 `addrman`，持久化到 `peers.dat`。

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
    Models      []string `json:"models"`
    LastSeen    int64    `json:"last_seen"`
    LatencyMs   int      `json:"latency_ms"`
    UptimeScore float64  `json:"uptime_score"` // 0.0 ~ 1.0
    FailCount   int      `json:"fail_count"`   // 连续失败次数
}
```

**维护规则**：
- 节点 30 分钟无响应 → `fail_count++`
- `fail_count >= 3` → 标记为不可达，不参与路由
- 每 5 分钟 Gossip 同步 → 从 peer 获取新节点
- 每 30 分钟清理 → 移除 7 天未见的节点
- 持久化到 `peers.dat`，启动时优先读取本地缓存

**BT 类比**：BT 客户端将已知节点持久化到本地，下次启动时优先连接缓存节点而非重新查询 Tracker。OpenModelPool 的 `peers.dat` 起到同样作用。

### 5.5 启动发现流程

```
节点启动
  │
  ├── 1. 读取本地 peers.dat（AddrMan 缓存）
  │
  ├── 2. 缓存不足？查 GitHub 注册表
  │      GET raw.githubusercontent.com/.../trust_pool.json
  │      → 获取初始节点列表
  │
  ├── 3. 连接已发现的节点
  │      → 逐个 PING，验证可达性
  │
  ├── 4. 请求 /api/peers（:8001 端点）
  │      → 获取更多 peer → 连接 → 继续扩散
  │
  ├── 5. 发送 ANNOUNCE 广播自身信息
  │      → 告知邻居"我来了"
  │
  └── 6. 加入 DHT 环，填充 k-buckets
         → 迭代查找（α=10 并行）
         → 全网路由建立完成
```

### 5.6 请求中继机制

```
用户请求 → 本地节点 → 选择路由：
  ├─ 本地有该模型？ → 直接处理 → 返回结果
  └─ 本地没有？ → 查路由表，转发到最优节点
                    → 对用户完全透明
```

**中继端点**：
```
POST /federation/relay
Headers:
  X-Node-ID: mmx-xxx
  X-Signature: ed25519:xxx
  X-Timestamp: 2026-07-09T14:00:00Z
  X-OpenModelPool-Agent-Hop: 1

Body: { 标准 OpenAI 请求格式 }
```

**防环机制**：`X-OpenModelPool-Agent-Hop` 跳数计数器，最大 3 跳。

**BT 类比**：BT 中数据块直接在 peer 之间传输；OpenModelPool 中请求在节点之间中继转发。关键区别是 BT 传输静态数据（可哈希验证），OpenModelPool 转发动态请求（需声誉系统保证质量）。

---

## 6. 额度分配模型

### 6.1 设计哲学

v4.0 移除旧版复杂 Credit 系统和金融化积分系统，但**保留轻量 Contribution Credit（贡献积分）**作为资源互助的记账单位。采用**节点主人自主决定的精细管控分配模型**。核心围绕两级开关（`network_enabled` + `share_to_pool`），在添加每个 Provider Key 时精细化设定共享额度、模型分配比例、有效期和限流策略。

**贡献积分定义**：
- ✅ 不是货币，不可提现，不可交易，不承诺收益
- ✅ 只用于节点未来取回等价模型资源
- ✅ 绑定 Node ID，重新部署需助记词恢复
- ✅ 如果用户永远不用，则视为极客共享贡献

**BT 类比**：BT 客户端的带宽管理——你可以为每个做种任务设定上传限速、时间段限制和优先级。OpenModelPool 同理：为每个 Provider Key 设定可共享的 token 额度、分配比例和使用条件。

### 6.1A 核心数据流与交互协议

#### 传输路径加密请求报文（中继不可见加密）

为了防止**中继节点**或其他中间人窥探用户隐私，业务载荷必须加密。

> **精确表述**：这是**传输路径加密**（Relay-Not-Visible Encryption），而非完整端到端隐私。请求内容对中继节点不可见，但**资源提供节点需要解密请求体**以调用上游模型。真正看不到 Prompt 的只有：中继节点、Gossip 节点、传输路径上的第三方。

```json
{
  "encrypted_payload": "<AES-256-GCM 加密后的 OpenAI 请求体>",
  "sender_pub_key": "<请求方 Ed25519 公钥>",
  "receiver_node_id": "<目标节点 NodeID>",
  "timestamp": "2026-07-11T10:00:00Z",
  "nonce": "<随机 nonce>"
}
```

- 请求方用接收方公钥加密请求体，中继节点只能看到密文
- 资源节点用私钥解密后，注入本地 API Key 转发给上游平台
- 响应数据同样加密回传，确保传输路径隐私

#### 数据流转 Goroutine Pipeline（资源端处理逻辑）

```
传入请求
  │
  ▼
┌─────────────────────────────────────────────────────────────────┐
│  Goroutine 1: 拦截校验                                           │
│  ├── 提取 Node ID 进行 WAF 四层检验（见 Section 10A）             │
│  ├── 频率检查 → Token 额度检查 → 内容合规检查 → 行为检查          │
│  └── 任一检查失败 → 立即拒绝，不进入后续流程                       │
└──────────────────────────────────┬──────────────────────────────┘
                                   │ 校验通过
                                   ▼
┌─────────────────────────────────────────────────────────────────┐
│  Goroutine 2: 重写注入                                           │
│  ├── 解密 E2EE 载荷                                              │
│  └── 注入本地加密存放的 Authorization: Bearer <Provider Key>      │
└──────────────────────────────────┬──────────────────────────────┘
                                   │ 注入完成
                                   ▼
┌─────────────────────────────────────────────────────────────────┐
│  Goroutine 3: 流式转发                                           │
│  ├── 通过 ReverseProxy 呼叫上游 AI 平台 API                       │
│  ├── 将 SSE Stream 结果无损回传给请求节点                          │
│  └── 响应体同样 E2EE 加密回传                                     │
└──────────────────────────────────┬──────────────────────────────┘
                                   │ 响应完成
                                   ▼
┌─────────────────────────────────────────────────────────────────┐
│  Goroutine 4: 异步记账（不阻塞响应）                               │
│  ├── 优先读取上游平台最终 chunk 的 usage 字段（真实 Token 统计）    │
│  ├── 降级方案：轻量级多模型 Tokenizer 本地估算                     │
│  ├── 生成 Usage Ticket（调用凭证）                                 │
│  └── 批量异步上报贡献账本                                         │
└─────────────────────────────────────────────────────────────────┘
```

**关键设计**：
- **拦截与转发分离**：校验 Goroutine 独立运行，不阻塞流式转发
- **异步记账**：Token 消耗估算在后台 Goroutine 完成，不影响响应延迟
- **流式无损**：ReverseProxy 直接透传 SSE 数据流，不缓冲完整响应

#### Token 消耗精准估算机制

不同语言和模型的分词器（Tokenizer）差异巨大，纯粹按字节估算会产生严重偏差。采用**优先真实值、降级本地估算**的双层策略：

```
Token 消耗统计优先级：

优先级 1：上游平台真实 usage 字段（唯一标准）
  ├── OpenAI/DeepSeek: 最终 chunk 的 usage.prompt_tokens + usage.completion_tokens
  ├── Anthropic: response.usage.input_tokens + output_tokens
  └── Gemini: usageMetadata.promptTokenCount + candidatesTokenCount

优先级 2：本地轻量级 Tokenizer 估算（降级方案）
  ├── 当上游平台不返回 usage 字段时启用
  ├── 集成 tiktoken_go（OpenAI 分词器 Go 实现）
  ├── 其他模型按字符数 × 经验系数估算：
  │   ├── 英文: ~4 字符 ≈ 1 token
  │   ├── 中文: ~1.5 汉字 ≈ 1 token
  │   └── 代码: ~3 字符 ≈ 1 token
  └── 估算结果标记为 estimated: true（与真实值区分）

优先级 3：Ticket 记账时的处理
  ├── 有真实 usage → 直接使用，标记 source: "upstream"
  ├── 仅有估算值 → 使用估算值，标记 source: "estimated"
  └── 公证人审计时：estimated 类型的 Ticket 不参与防共谋指纹比对
  └── 无 request-id 的调用不生成 Ticket，不计入贡献积分
```

```go
type TokenUsage struct {
    PromptTokens     int    `json:"prompt_tokens"`
    CompletionTokens int    `json:"completion_tokens"`
    TotalTokens      int    `json:"total_tokens"`
    Source           string `json:"source"` // "upstream" | "estimated"
    Estimated        bool   `json:"estimated"`
}
```

### 6.2 Provider Key 共享额度精细化管控

节点在 `share_to_pool` 时，需要对每个上游 Provider Key 做精细化配置：

```
添加 Provider Key 时的共享配置流程：

1. 检查上游平台每个 Provider Key 预分配了多少额度可以共享
   ├── Token 管控模式：直接填写可共享 token 数量
   └── 积分管控模式：填写共享积分数量 → 自动推荐换算比例 → 统计 token 额度

2. 自动汇总所有 Provider Key 的共享 token 总额度
   └── 用于后续模型路由算法优先级计算（优先路由快到期的额度）

3. 设定总额度分配比例：
   ├── A% → 给全球公共 Key（免费消费者）
   └── B% → 给有贡献的节点 Key 调用（100 - A = B）

4. 模型级分配（可选）：
   ├── 默认：按天级模型需求热度（获取全网公开的 token 调用排名）比例分配
   └── 自定义：用户手动指定每种模型可共享的额度
```

#### 6.2.1 Token 管控模式

上游平台按 token 数量管控额度时，节点运营者直接设定可共享的 token 数量。

```json
{
  "provider_key_id": "pk_openai_001",
  "platform": "openai",
  "total_quota": 1000000,
  "shared_quota": 300000,
  "quota_type": "token",
  "period": "monthly",
  "expires_at": "2026-08-01T00:00:00Z"
}
```

#### 6.2.2 积分管控模式

上游平台按照积分管控而非 token 数管控时：

```json
{
  "provider_key_id": "pk_other_002",
  "platform": "some-platform",
  "shared_points": 5000,
  "token_conversion_ratio": 100,
  "estimated_shared_tokens": 500000,
  "quota_type": "points"
}
```

- 用户在添加平台配置时勾选共享积分数量
- 系统**自动推荐一个初始换算比例**（基于平台历史数据或默认值）
- 无论上游用积分还是 token 管控，最终统一统计 token 额度到共享总额

#### 6.2.3 额度有效期与计算周期

| 额度类型 | 设置方式 | 说明 |
|---------|---------|------|
| **截止期日** | 设定 `expires_at` | 额度在指定日期失效，适合一次性额度 |
| **周期额度** | 选择计算周期（每天/每月/每年） | 周期性重置的额度，如每日 10 万 tokens |

```json
{
  "period_type": "monthly",
  "period_value": 1000000,
  "reset_day": 1,
  "timezone": "Asia/Shanghai"
}
```

#### 6.2.4 模型级额度分配

如果一个 Provider Key 支持多种模型，分配规则如下：

**默认规则**：按天级模型需求热度（获取全网公开的 token 调用排名）进行比例分配。

```
示例：Provider Key 总共享额度 = 100 万 tokens/月

全网模型热度排名：
  gpt-4o:     40% → 40 万 tokens
  gpt-4o-mini: 25% → 25 万 tokens
  claude-3:    20% → 20 万 tokens
  deepseek:    15% → 15 万 tokens
```

**自定义规则**：用户可以手动指定每一种共享出来的模型单独的额度。

```json
{
  "model_quotas": {
    "gpt-4o": {"quota": 500000, "enabled": true},
    "gpt-4o-mini": {"quota": 200000, "enabled": true},
    "claude-3-sonnet": {"quota": 100000, "enabled": true},
    "deepseek-chat": {"quota": 0, "enabled": false}
  }
}
```

#### 6.2.5 限流选项

如果模型有调用频率和会话次数限制，在添加 Provider Key 时可以勾选限流选项：

```json
{
  "rate_limit": {
    "requests_per_minute": 60,
    "requests_per_day": 1000,
    "max_concurrent_sessions": 10,
    "max_tokens_per_request": 4096
  }
}
```

### 6.3 全局共享总额度池的两部分结构

全球共享的总额度池分为两部分：

```
全球共享总额度池
├── 第一部分：全球公共 Key 共享池
│   └── 总额度 = 所有节点设定的共享给免费公共 Key 的额度之和
│       └── 按当前公共 Key 使用人数均分
│
└── 第二部分：节点分发 Key 共享池
    └── 总额度 = 所有节点设定的共享给其他节点 Key 的额度之和
        └── 节点分发 Key 的可消耗额度 = 该节点贡献度叠加决定
```

**关键规则**：

1. **全球公共 Key 总额度 = 所有节点设定的共享给免费公共 Key 的额度之和**，而不是从免费消费者池中分配一个基础额度
2. 公共 Key 使用者按人数均分第一部分额度
3. 节点分发的 Key（Guest Proxy Key 等）的可消耗额度与该节点贡献度叠加决定

**BT 类比**：

```
BT Swarm 的总上传带宽 = 所有做种者上传速度之和
OpenModelPool 全球公共池 = 所有节点共享给公共 Key 的额度之和

BT 做种者的带宽分配：
  ├── 一部分 → 匿名下载者（leech）均分
  └── 一部分 → 有贡献的 peer 优先获取

OpenModelPool 节点额度分配：
  ├── A% → 全球公共 Key（免费用户均分）
  └── B% → 有贡献的节点 Key（按贡献度分配）
```

### 6.4 节点签发 Key 的路由优先级规则

**核心规则**：用节点签发的 Key 调用模型时：

```
请求到达 → 判断签发节点是否有该模型：
  ├─ 自己节点有这个模型 → 始终优先用自己的（本地优先）
  └─ 自己没有的模型 → 才路由到其他节点提供的模型服务
```

**设计意图**：避免不必要的网络中转——如果签发节点自身就能处理请求，绝不浪费全网资源。只有本地不具备的能力才走 P2P 路由。

**BT 类比**：BT 客户端优先从本地缓存读取数据块，本地没有的才从网络 peer 下载。

### 6.5 共享额度汇总与路由优先级

节点汇总所有 Provider Key 的共享 token 总额度后，为路由算法提供优先级依据：

```
路由优先级因素：
1. 额度剩余量：优先路由到额度充足的节点
2. 额度到期时间：优先路由快到期的额度（避免浪费）
3. 模型匹配度：优先路由到精确匹配模型的节点
4. 节点贡献度：贡献越高的节点路由权重越大
5. 延迟和可用性：综合评分选择最优节点
```

### 6.6 完整示例

节点 A 配置了 2 个 Provider：

```
Provider 1 (OpenAI):
  - 总 token 额度: 100 万/月
  - 共享额度: 30 万/月（30%）
  - 公共 Key 分配: 10 万/月
  - 节点 Key 分配: 20 万/月
  - 模型分配（按热度）:
      gpt-4o: 12 万, gpt-4o-mini: 9 万, gpt-3.5-turbo: 9 万
  - 限流: 60 req/min, 1000 req/day

Provider 2 (Anthropic):
  - 总积分额度: 5000 积分/月
  - 换算比例: 1 积分 = 100 tokens
  - 共享额度: 2500 积分 = 25 万 tokens/月
  - 公共 Key 分配: 5 万/月
  - 节点 Key 分配: 20 万/月
  - 模型分配（自定义）:
      claude-3-opus: 15 万, claude-3-sonnet: 10 万
  - 有效期: 2026-08-01 截止
```

节点 A 汇总：
- **共享给公共 Key 总额**: 15 万 tokens/月
- **共享给节点 Key 总额**: 40 万 tokens/月
- **全网公共池贡献**: 15 万（将加入全球公共池第一部分）

### 6.7 与旧模型的对比

| 旧机制 (v2) | 新机制 (v3.0) | 变更原因 |
|------------|-------------|---------|
| 积分系统（creditEarn/creditSpend） | Provider Key 级精细化管控 | 过于复杂，难以维护 |
| 动态阈值解锁（NodeUnlockState） | 贡献权重驱动路由 + 额度到期优先 | 阻碍网络增长 |
| daily cap / invite bonus | 无 | 机制复杂收益低 |
| 共识投票（公共 Key 额度占比） | 节点自主设定 A%/B% | 简化治理 |
| 全局贡献积分池 | 无全局状态，各节点独立设定 | 分布式一致性成本过高 |
| 固定基础额度 | 全网节点共享额度动态汇总 | 更公平，按需分配 |

---

## 7. 贡献账本系统

### 7.1 设计哲学

贡献账本追踪节点的服务量——类比 BT 的分享率统计。BT 客户端记录上传/下载量计算分享率；OpenModelPool 记录贡献/消费量计算权重。

**贡献积分（Contribution Credit）定义**：

| 属性 | 说明 |
|------|------|
| 是货币？ | ❌ 不是货币，不可提现 |
| 可交易？ | ❌ 不可交易，不可转让 |
| 承诺收益？ | ❌ 不承诺任何收益 |
| 用途 | 节点未来取回等价模型资源 |
| 绑定 | 绑定 Node ID，需助记词恢复 |
| 永不取回 | 视为极客共享贡献 |

移除的是旧版复杂 Credit 系统（动态阈值解锁、信誉评级、金融化积分）；保留的是轻量 Contribution Credit 作为资源互助的记账单位。

### 7.1A 贡献值结算模型

*综合了实际消耗算力、模型稀缺度、节点在线时长及服务成功率*

```
贡献值 = f(实际消耗 Token 数, 模型稀缺度系数, 在线时长, 服务成功率)

各因子说明：
├── 实际消耗 Token 数：通过 Usage Ticket 双向签名确认的真实消耗量
├── 模型稀缺度系数：稀缺模型（如 Claude-3-Opus）系数 > 常见模型（如 GPT-3.5）
├── 在线时长：节点持续在线提供服务的时间累积
└── 服务成功率：成功响应 / 总请求数，反映节点可靠性
```

**与路由权重的关系**：贡献值越高 → 路由权重越大 → 可消耗的共享池额度越多。这是一种"软激励"而非"硬交易"——不涉及任何代币或金融结算。

### 7.1B 半中心化防双花记账 (Ticket 系统)

为防止节点互刷贡献值（女巫攻击），引入 Usage Ticket 双向签名机制：

```
Ticket 生命周期：

1. 请求完成
   └── AI 对话结束后，请求方与资源方必须对本次消耗 Token 数进行**双向签名**

2. 生成 Usage Ticket（调用凭证）
   └── 包含：请求方 NodeID、资源方 NodeID、消耗 Token 数、模型名、时间戳、双方签名

3. 本地暂存
   └── 凭证暂存本地 BoltDB 中

4. 批量公证
   └── 每小时打包异步上报给公证人节点集合
       ├── Phase 1：seed1.openmodelpool.com（唯一公证人）
       └── Phase 2+：多公证人轮转（Gossip 选举 / 信誉 Top-N 节点）

5. 清洗落账
   └── 公证人进行防重放与关联分析，剔除同源互刷行为
   └── 更新全网信誉排行榜

6. 权益兑换
   └── 贡献值高（信誉好）的节点 → 最高路由调度优先级
   └── 可优先调用网络内的稀缺高级模型
```

**防双花核心逻辑**：
- **双向签名**：每次消耗必须双方确认，单方面伪造无效
- **批量公证**：公证人节点集合定期收集所有 Ticket，进行交叉验证（Phase 1 为 seed1，Phase 2+ 多节点轮转）
- **关联分析**：检测同源 IP、同设备指纹、互刷模式（A→B 和 B→A 对称消耗）
- **防重放**：Ticket ID 唯一性校验，重复 Ticket 直接拒绝

**BT 类比**：BT 用 piece 哈希验证数据真实性；OpenModelPool 用 Usage Ticket 验证贡献真实性。两者都是"先验证，后记账"的逻辑。

#### 7.1B.1 防共谋机制（Anti-Collusion）

仅依赖 IP 检测和关联分析容易被廉价代理池绕过。如果节点 A 和节点 B 属于同一实际控制人，可在不向真实大模型平台发起请求的情况下伪造双向签名 Ticket。

**增强防线**：

```
防共谋三层机制：

┌─────────────────────────────────────────────────────────────────┐
│  第一层：上游平台响应指纹 (Response Fingerprint)                  │
│                                                                  │
│  ├── 在 Ticket 中附带上游 AI 平台返回的响应特征摘要               │
│  │   ├── 响应头中的 request-id / x-request-id（平台唯一标识）     │
│  │   ├── 最终 chunk 中的 usage 字段（platform_token_count）       │
│  │   └── 响应内容的 SHA-256 摘要（不泄露 Prompt 隐私）            │
│  ├── 公证人验证：同一请求是否对应真实的上游 request-id             │
│  └── 成本：伪造者必须实际调用上游 API 才能获得有效的 request-id    │
├─────────────────────────────────────────────────────────────────┤
│  第二层：随机抽样验证 (Random Auditing)                           │
│                                                                  │
│  ├── 公证人随机抽取 5% 的 Ticket 进行深度验证                     │
│  ├── 验证方式：用相同 Prompt 重新请求，比对响应一致性              │
│  │   └── 不要求完全一致（LLM 有随机性），但检查模型名、格式等      │
│  ├── 被抽中的节点必须在 5 分钟内提供原始响应的完整内容              │
│  └── 无法提供 → Ticket 作废 + 信誉扣分                           │
├─────────────────────────────────────────────────────────────────┤
│  第三层：统计异常检测 (Statistical Anomaly Detection)              │
│                                                                  │
│  ├── 节点间对称消耗检测：A→B 和 B→A 的消耗量高度对称 → 可疑       │
│  ├── 时间规律检测：固定时间间隔的精确消耗 → 自动化脚本嫌疑          │
│  ├── 消耗/贡献比异常：某节点消耗量远超其 Provider 容量 → 可疑      │
│  └── 触发阈值 → 标记为"待审计"，限制其路由优先级直到审计通过       │
└─────────────────────────────────────────────────────────────────┘
```

**Response Fingerprint 数据结构**：

```go
type TicketFingerprint struct {
    UpstreamRequestID string `json:"upstream_request_id"` // 上游平台返回的唯一请求 ID
    ModelName         string `json:"model_name"`           // 实际调用的模型名
    ResponseHash      string `json:"response_hash"`        // 响应内容 SHA-256（前 1024 字节）
    TokenUsage        *struct {
        PromptTokens     int `json:"prompt_tokens"`
        CompletionTokens int `json:"completion_tokens"`
        TotalTokens      int `json:"total_tokens"`
    } `json:"token_usage,omitempty"`
    Timestamp         int64  `json:"timestamp"`            // 上游平台响应时间戳
}
```

**隐私保护**：Response Fingerprint 仅包含响应元数据和摘要哈希，不包含完整 Prompt 或响应内容，确保不会泄露用户隐私。

#### 7.1B.2 公证人去中心化演进（降低 seed1 单点依赖）

防共谋整条防线锚定在 `request-id` 和公证人上——一旦上游平台不提供 `request-id`（如部分 API 不返回），或 seed1 公证人被攻陷，经济激励层直接崩。因此必须在 Phase 2 之前完成公证人去中心化，消除单点风险。

```
公证人演进路线：

Phase 1（短期）：seed1 单公证人 + 降级策略
  ├── seed1.openmodelpool.com 充当唯一公证人
  ├── 降级策略：若 seed1 不可达
  │   ├── 节点本地缓存 Ticket，等待恢复后批量补报
  │   ├── 连续 3 小时不可达 → 切换到 GitHub Raw 静态公证快照
  │   └── 所有 Ticket 设有效期上限（默认 24h），超时未公证自动作废
  └── 风险缓解：seed1 使用独立密钥对，与 Relay 节点密钥分离

Phase 2（中期）：多公证人轮转
  ├── 信誉 Top-N 节点自动获得公证人资格
  ├── 每个 Ticket 需 ≥3 个公证人中 ≥2 个确认（多数签名）
  ├── 公证人定期轮换（每 7 天重选一次）
  └── 任何单公证人被攻陷不影响整体一致性

Phase 3（长期）：Gossip Ledger 完全去中心化
  ├── 所有节点均可验证 Ticket 合法性
  ├── 不再有"公证人"角色，改为"验证者"
  └── 共识机制：BFT-style 多数决
```

**request-id 缺失的处理策略**：
- 上游 API 未返回 `request-id` 的调用，视为**不可验证调用**，**不计入贡献积分体系**
- 节点本地正常转发响应给用户（不影响使用体验），但该次调用不生成有效 Ticket
- 理由：无 `request-id` 意味着公证人无法通过 Response Fingerprint 验证调用真实性，纳入积分体系会产生无法审计的漏洞
- 长期目标：优先选择返回 `request-id` 的 Provider（如 OpenAI、Anthropic），对不返回的 Provider 标记为"低信誉贡献源"，其产生的调用不参与贡献计算

### 7.2 两层架构

```
──────────────────────────────────────────────────────
┌──────────────────────────┴──────────────────────────┐
│  Layer 2: IPFS（可选持久化）                          │
│  - 关键事件存证（>$100 贡献）                         │
│  - 争议解决存证                                       │
│  - 声誉变更永久记录                                    │
└──────────────────────────┬──────────────────────────┘
                           ↕ (同步接口)
┌──────────────────────────┴──────────────────────────┐
│  Layer 1: Gossip Ledger（当前实现）                   │
│  - 日常贡献记录（本地 + Gossip P2P 同步）             │
│  - Ed25519 签名验证                                  │
│  - 交叉验证防欺诈                                     │
└─────────────────────────────────────────────────────┘
```

### 7.3 Layer 1: Gossip Ledger（当前实现）

#### 7.3.1 数据结构

```go
type ContributionRecord struct {
    ID             string    `json:"id"`
    PeerID         string    `json:"peer_id"`
    Timestamp      time.Time `json:"timestamp"`
    
    RequestsServed int64     `json:"requests_served"`
    TokensProvided int64     `json:"tokens_provided"`
    CostUSD        float64   `json:"cost_usd"`
    
    ModelBreakdown map[string]ModelContribution `json:"model_breakdown"`
    
    Signature      []byte    `json:"signature"`     // Ed25519 签名
    Version        int       `json:"version"`       // 冲突解决用
}

type ConsumptionRecord struct {
    ID             string    `json:"id"`
    PeerID         string    `json:"peer_id"`
    Timestamp      time.Time `json:"timestamp"`
    
    RequestsMade   int64     `json:"requests_made"`
    TokensConsumed int64     `json:"tokens_consumed"`
    CostUSD        float64   `json:"cost_usd"`
    
    ModelBreakdown map[string]ModelConsumption `json:"model_breakdown"`
    Signature      []byte    `json:"signature"`
}
```

#### 7.3.2 核心接口

```go
type ContributionTracker interface {
    RecordContribution(peerID string, req *Request, resp *Response) error
    RecordConsumption(peerID string, req *Request, resp *Response) error
    GetContribution(peerID string) (*ContributionRecord, error)
    GetShareRatio(peerID string) (float64, error)
    BroadcastContribution(record *ContributionRecord) error
    VerifyContribution(record *ContributionRecord) bool
}
```

#### 7.3.3 交叉验证机制

类比 BT 的数据校验——BT 用 SHA 哈希验证文件块完整性，OpenModelPool 用交叉验证验证贡献记录真实性。

```
Node A 报告: 服务了 50 请求, 1万 tokens
Node B 报告: Node A 服务了 48 请求, 9.8K tokens
Node C 报告: Node A 服务了 52 请求, 10.2K tokens
                    ↓
            偏差检查（最大 20%）
            通过 → 接受
            失败 → 标记可疑
```

**BT 类比**：BT 用 SHA-1 哈希确定性地验证每个数据块；OpenModelPool 无法确定性验证服务量（API 响应非确定性），因此用多节点交叉验证作为替代——从不同来源收集同一节点的贡献数据，偏差过大则标记可疑。

### 7.4 Layer 2: IPFS（可选）

| 组件 | 用途 | 成本 |
|------|------|------|
| **IPFS** | 存储关键贡献记录、能力声明 | $0（公共网关） |

**存入 Layer 2 的事件**：
- 贡献金额 > $100
- 节点封禁决定
- 争议解决记录
- 声誉里程碑变更



### 7.5 层间同步

```
Layer 1 → Layer 2 同步触发条件：
  - 贡献记录 > $100
  - 节点被封禁
  - 争议需要存证
```

---

## 8. 信任与声誉系统

### 8.1 设计哲学

BT 用分享率决定谁优先获得上传；OpenModelPool 用声誉分数决定谁优先获得路由。但与 v2 的复杂 4 维加权评分不同，v3.0 的信任系统聚焦于**验证能力声明的真实性**，而非精细的信用评分。

**BT 类比**：BT 客户端通过实际传输验证 peer 真实性——能传数据就是真的 peer，传不了就是假的。OpenModelPool 通过主动探测验证节点真实性——能服务请求就是真的节点，不能就是虚假声明。

### 8.2 主动探测

类比 BT 的 piece 验证——BT 下载完一个数据块后用 SHA 哈希验证完整性；OpenModelPool 发送 1-token 测试请求验证能力声明真实性。

```
探测流程：

Prober Node                          Target Node
     │                                    │
     │  1. 发送 1-token 测试请求           │
     │  （最低成本验证）                    │
     │───────────────────────────────────▶│
     │                                    │
     │  2. 响应（成功/失败）               │
     │◀───────────────────────────────────│
     │                                    │
     │  3. 记录结果                        │
     │     - 延迟                          │
     │     - 成功/失败                     │
     │     - 响应有效性                    │
     │                                    │
     │  4. 更新声誉分数                    │
     │     - EWMA 评分                     │
     │     - 信任等级调整                  │
```

**探测调度**：
- 新节点：每 5 分钟（首小时）
- 常规节点：每 30 分钟
- 高声誉节点：每 2 小时
- 可疑节点：每 1 分钟

**成本分析**：1-token 探测成本约 $0.00001，检测时间 < 5 分钟。

### 8.3 交叉验证

多节点独立探测同一目标——类比 BT 的多个 peer 同时验证同一数据块的哈希。

- 至少需要 3 个独立验证
- 验证结果偏差 > 20% 触发调查
- 验证数据通过 Gossip 同步

### 8.4 声誉分数

```go
type NodeReputation struct {
    NodeID         string  `json:"node_id"`
    Availability   float64 `json:"availability"`   // 0-100, EWMA
    Latency        float64 `json:"latency"`        // 0-100, EWMA
    Accuracy       float64 `json:"accuracy"`       // 0-100, EWMA
    OverallScore   float64 `json:"overall_score"`  // 加权综合分
    Grade          string  `json:"grade"`          // S/A/B/C/D
    TotalRequests  int64   `json:"total_requests"`
    FailedRequests int64   `json:"failed_requests"`
}

// EWMA 更新（alpha = 0.3）
// 综合分数 = 可用性×40% + 延迟×30% + 准确率×20% + 同行评分×10%
```

**评级标准**：

| 等级 | 分数范围 | 描述 | 路由权重 |
|------|----------|------|---------|
| S | 95-100 | 精英 — 已验证，高吞吐量 | 40% |
| A | 80-94 | 优秀 — 可靠，低延迟 | 30% |
| B | 60-79 | 良好 — 正常运行 | 20% |
| C | 40-59 | 一般 — 偶有问题 | 8% |
| D | 0-39 | 差 — 可能被隔离 | 2% |

**BT 类比**：BT 的 Choking 算法根据 peer 的上传速率决定是否 unchoke；OpenModelPool 的路由算法根据声誉分数决定调度优先级。高声誉节点类似 BT 中的高速上传者——优先获得对方的 unchoke。

### 8.5 信任数据存储

- 所有信任数据存入 Gossip Ledger（Layer 1）
- 节点封禁等关键事件同步到 IPFS（Layer 2）
- 声誉分数通过 Gossip 协议全网传播
- 每个节点独立计算，加权平均后广播

### 8.6 与旧模型的对比

| 旧模型 (v2) | 新模型 (v3.0) | 变更 |
|------------|-------------|------|
| 5 维加权（含投诉维度） | 4 维（可用性+延迟+准确率+同行评分） | 简化 |
| Diamond/Gold/Silver/Bronze/Untrusted | S/A/B/C/D | 统一评级 |
| 浮点分 0-1.0 | 整数分 0-100 | 更直观 |
| 无衰减机制 | EWMA α=0.3 自然衰减 | 增加衰减 |
| 积分系统挂钩 | 与额度分配解耦 | 简化 |

---

## 9. 请求路由与负载均衡

### 9.1 设计哲学

BT 的 piece 选择策略决定从哪个 peer 下载哪个数据块；OpenModelPool 的路由算法决定将请求转发到哪个节点。两者核心目标一致：**最大化吞吐量，最小化延迟**。

### 9.2 五维评分引擎

```
Score = w₁×Trust + w₂×Reputation + w₃×Latency + w₄×Availability + w₅×Contribution

维度               │ 权重   │ 来源
───────────────────┼────────┼─────────────────
信任等级           │  25%   │ 渐进式信任
声誉分数           │  25%   │ EWMA (0-100)
延迟分数           │  20%   │ 近期平均延迟
可用性             │  15%   │ 在线率 EWMA
贡献度             │  15%   │ 共享比率（贡献/消费）
```

**BT 类比**：BT 的 piece 选择综合考虑 peer 上传速率、延迟、已有 piece 稀有度；OpenModelPool 的路由评分综合考虑节点信任、声誉、延迟、可用性、贡献度。

### 9.3 路由决策流程

```
请求到达
  │
  ├── 识别 Key 类型（ClassifyKey）
  │
  ├── 确定路由范围
  │   ├── Proxy API Key → 本地 + 全网
  │   ├── Guest Proxy Key → 签发节点（本地优先）+ 网络（受额度约束）
  │   ── 全球公共 Key → 任意可达网络池 + 全网共享资源（动态算法分配额度）
  │
  ├── 查找候选节点
  │   ├── 能力匹配（模型名匹配）
  │   ├── 信任 ≥ 最低阈值
  │   └── 在线状态确认
  │
  ├── 对候选节点评分（四维评分）
  │
  ├── 选择最优节点
  │   ├── 最高分数优先
  │   └── Fallback 链（最多 3 个备选）
  │
  └── 转发请求
      ├── 成功 → 记录贡献 + 返回响应
      └── 失败 → 尝试下一节点
```

### 9.4 区域感知路由

```
优先同区域路由：
  同区域加分: +0.15
  跨区域减分: -0.10

仅在以下情况跨区域：
  - 同区域节点不可用
  - 同区域节点过载
  - 请求需要该区域没有的特定模型
```

**BT 类比**：BT 的 locality-aware peer 选择——优先连接物理距离近的 peer 减少延迟；OpenModelPool 优先路由到同区域节点。

### 9.5 Provider 路由模式

| 模式 | 策略 | BT 类比 |
|------|------|---------|
| `priority` | 按优先级排序 | 按已知速率排序 peer |
| `cheapest` | 按价格排序 | 按带宽成本排序 |
| `fastest` | 按延迟 EWMA 排序 | 按延迟排序 peer |
| `auto` | 加权综合（优先级 40% + 成本 25% + 延迟 20% + token 余量 15%） | 综合评分选择 |

### 9.6 路由表结构

每个节点维护一份全网路由表，通过 Gossip 同步：

```json
{
  "routes": {
    "gpt-4o": [
      {"node_id": "mmx-aaa", "url": "https://node-a.com", "local": true, "score": 0.95},
      {"node_id": "mmx-bbb", "url": "https://node-b.com", "local": false, "score": 0.82}
    ],
    "claude-3-opus": [
      {"node_id": "mmx-ccc", "url": "https://node-c.com", "score": 0.91}
    ]
  }
}
```

---

## 10. 身份认证与安全

### 10.1 设计哲学

BT 用 peer_id 标识每个客户端；OpenModelPool 用 Ed25519 公钥派生的 node_id 标识每个节点。身份自包含——不需要中心 CA 签发证书，公钥即身份。

### 10.2 节点身份

```
node_id = "mmx-" + hex(Ed25519 公钥)

示例: mmx-abc123def456
```

**关键原则**：
- **个人版不生成节点身份**：默认启动无 Node ID、无助记词、无 P2P
- **节点身份只在加入共享网络时生成**

#### 10.2.1 生成流程（加入共享网络时）

```
用户点击"加入共享网络"
  ↓
展示项目说明（公益共享、绝不发币、Key 不上传）
  ↓
用户确认理解共享机制
  ↓
生成 BIP39 助记词（12/24 词）
  ↓
由助记词派生 Ed25519 私钥（BIP32 派生路径 m/44'/2024'/0'）
  ↓
生成公钥 → Node ID = "mmx-" + hex(public_key)
  ↓
强制用户备份助记词（抄写/加密导出）
  ↓
私钥 AES-256-GCM 加密存储于 data/node.key
  ↓
公钥广播到全网用于验证
  ↓
进入共享网络
```

#### 10.2.2 恢复流程（重新部署后）

```
重新部署 → 默认进入个人版
  ↓
用户选择"恢复共享节点身份"
  ↓
输入助记词
  ↓
恢复私钥与 Node ID
  ↓
同步贡献账本
  ↓
重新配置 Provider Token（助记词不包含 Provider Token）
  ↓
恢复共享版
```

#### 10.2.3 关键特性

- **身份自包含**：node_id = Ed25519 公钥编码，无需注册
- **不可伪造**：没有私钥签名过不了验证
- **贡献绑定**：贡献积分通过 node_id 绑定，需助记词恢复
- **DHT 定位**：SHA-256(node_id) → 256 位 DHT 位置
- **助记词不包含 Provider Token**：重新部署后 Provider Token 需重新配置
- **丢失助记词 = 丢失 Node ID 和贡献积分**：只能创建新共享身份

**BT 类比**：BT 的 peer_id 是 20 字节标识符；OpenModelPool 的 node_id 是 Ed25519 公钥编码，通过助记词实现跨部署恢复。

### 10.3 授权链（故障关闭）

```
请求授权流程（Fail-Close 模型）：

  传入请求
     │
     ▼
  密钥有效？ ──── 否 ──→ 拒绝 (401)
     │ 是
     ▼
  分类 Key 类型
     │
     ├── Proxy Key → 路由到所有者 Provider + 全网
     ├── Guest Key → 路由到签发节点
     └── 公共 Key → 路由到共享池
     │
     ▼
  Provider 存在且有配额？ ──── 否 ──→ 拒绝 (503)
     │ 是
     ▼
  转发请求
```

**任何验证环节的错误都导致拒绝**——不尝试"容错继续"，确保安全性优先。

### 10.4 签名验证

| 场景 | 签名内容 | 验证方式 |
|------|---------|---------|
| 节点心跳 | node_id + 时间戳 | Ed25519 + 节点公钥 |
| Gossip 消息 | JSON(message) | Ed25519 + 发送者公钥 |
| Provider 公告 | JSON(announcement) | Ed25519 + 节点公钥 |
| 能力声明 | JSON(CapabilityClaim) | Ed25519 + 节点公钥 |
| 贡献记录 | JSON(ContributionRecord) | Ed25519 + 节点公钥 |

### 10.5 防重放保护

- 所有签名消息包含时间戳 + nonce
- 声明 TTL 默认 48 小时
- 通过记录 ID 进行重复检测
- 中继跳数限制（maxRelayHops = 3）

### 10.6 中继安全

```go
const (
    headerRelayHop  = "X-OpenModelPool-Agent-Hop"
    headerRelayFrom = "X-OpenModelPool-Agent-Relay-From"
    maxRelayHops    = 3
)

// 循环防护：每次中继检查跳数
if hopCount >= maxRelayHops {
    writeError(w, 508, "max relay hops exceeded")
    return
}
```

### 10.7 私钥与 API Key 安全

- **助记词**：用户加入共享网络时生成 BIP39 助记词，用于派生 Ed25519 私钥和恢复 Node ID
- **节点私钥**：由助记词派生，AES-256-GCM 加密存储于 `data/node.key`，按需解密签名后立即清零内存
- **Provider Key 存储**：使用 **AES-256-GCM** 加密存储于本地文件（密钥文件 `data/.enc_key`，32 字节，加密前缀 `omp:e:`）
- **Keyring 可选扩展**：未来可扩展为优先调用操作系统底层 Keyring（macOS Keychain / Windows Credential Manager / Linux Secret Service），当前使用文件加密方案已满足安全要求
- **安全原则**：Provider Key **永远保存在本地，绝不上传服务器**，即使在 P2P 通信中也仅本地使用
- **助记词不包含 Provider Token**：重新部署后 Provider Token 需重新配置
- 私钥和 Provider Key 都绝不离开本机，不参与任何网络通信

---

## 10A. 本地安全风控底座 (WAF)

为了防止资源节点的 API Key 被"薅羊毛"超额扣费，或因违规内容被大模型厂商封号，客户端内置**四层极轻量级防护**：

| 层级 | 防护名称 | 实现方式 | 触发后果 |
|------|---------|---------|---------|
| **第一层** | 高频并发拦截 (Rate Limit) | 限制单 IP/单 Node ID 每分钟请求数（例如 10 RPM） | 违规直接**封禁 Node ID 2 小时** |
| **第二层** | 超长上下文拦截 (Token Limit) | 在请求到达大模型 API 前，拦截预估上下文超出安全阈值（如 4096 Tokens）的调用 | 拒绝请求 |
| **第三层** | 内容合规拦截 (Content Safety) | 使用 AC 自动机加载本地敏感词 Hash 库，极速扫描 Prompt | 命中暴恐/涉黄指令立即**阻断请求并拉黑对方节点** |
| **第四层** | 恶意白嫖拦截 (Behavioral) | 检测高频断连、不提交"调用确认凭证"的恶意请求方 | 降低信誉 + 限制访问 |

**技术实现**：
- **Rate Limit**：基于 `x/time/rate` 令牌桶 + `golang-lru` 维护每个 Node ID 的频率计数
- **Token Limit**：请求头中预估 token 数，超出 `max_tokens_per_request` 直接 429
- **Content Safety**：`cloudflare/ahocorasick` AC 自动机，纳秒级匹配，极低内存消耗
- **Behavioral**：结合 Ticket 系统（见第 7 节），检测不提交 Usage Ticket 的异常行为

**设计原则**：所有防护在本地完成，不依赖中心化服务。即使全网断连，节点自身的安全防护依然有效。

### 10A.1 分级阻断与误杀防护

大模型的 Prompt 常包含复杂长文本或代码块，纯正则/关键词匹配极易产生误杀。一旦误杀直接封禁节点，惩罚过重。

**分级阻断策略**：

| 违规次数 | 触发动作 | 恢复机制 |
|---------|---------|---------|
| **第 1 次** | 拒绝单次请求 + 返回警告信息（含命中的敏感词类别） | 自动恢复，无需操作 |
| **第 2 次**（1 小时内） | 拒绝请求 + 警告 + 记录违规到本地日志 | 1 小时后自动重置计数器 |
| **第 3 次**（24 小时内） | **临时封禁 Node ID 2 小时** + 通知对方节点运营者 | 2 小时后自动解封 |
| **第 5 次+**（7 天内） | **长期封禁** + 通过 Gossip 广播封禁信息 | 需手动解封（见申诉机制） |

**敏感词分级**：

| 敏感级别 | 示例类别 | 处理策略 |
|---------|---------|---------|
| **L1 — 硬拦截** | 暴恐、儿童安全 | 立即阻断 + 直接拉黑（不分级） |
| **L2 — 软拦截** | 政治敏感、色情暗示 | 分级阻断（上述策略） |
| **L3 — 记录** | 争议性话题、边缘内容 | 仅记录日志，不阻断 |

### 10A.2 解封与申诉机制

防止高质量请求节点因一次无意的提问而被永久拒绝服务：

```
申诉流程：

1. 被封节点运营者发现无法访问
   └── 在本地管理面板查看封禁原因和封禁时长

2. 自动解封
   └── 临时封禁（2 小时）到期后自动恢复
   └── 无需任何操作

3. 手动申诉（针对长期封禁）
   ├── 节点运营者在管理面板提交申诉
   ├── 系统自动生成申诉请求（含违规记录摘要）
   ├── 申诉通过 Gossip 协议传播至全网
   └── 由管理端（seed1.openmodelpool.com）或社区仲裁审核

4. 审核结果
   ├── 通过 → 解除封禁 + 重置违规计数
   └── 驳回 → 维持封禁，30 天后可再次申诉
```

**管理面板展示**：
- 封禁列表（Node ID、封禁原因、封禁时间、剩余时长）
- 违规日志（时间、命中的敏感词类别、Prompt 摘要）
- 一键解封按钮（本地管理员权限）

---

## 10B. 重新部署与恢复策略

### 10B.1 个人版重新部署

```
重新安装
  ↓
默认个人版
  ↓
重新配置 Provider Token
```

无需助记词。个人版数据（Token、额度配置、调用统计）如需迁移，用户可自行导出/导入加密配置备份。

### 10B.2 共享版重新部署

```
重新安装
  ↓
默认进入个人版
  ↓
点击"恢复共享节点身份"
  ↓
输入助记词
  ↓
恢复 Node ID（由助记词派生 Ed25519 私钥 → 公钥 → Node ID）
  ↓
同步贡献积分（从 Gossip Ledger 恢复）
  ↓
重新配置 Provider Token（助记词不包含 Provider Token）
  ↓
恢复共享版
```

### 10B.3 丢失助记词

```
❌ 无法恢复原 Node ID
❌ 无法恢复贡献积分
✅ 只能创建新共享身份（重新生成助记词）
```

**重要提醒**：助记词是恢复 Node ID 和贡献积分的唯一凭证。用户加入共享网络时必须备份助记词。

---

## 11. 域名绑定与隧道

### 11.1 设计哲学

BT 客户端需要公网 IP 或端口转发才能被其他 peer 连接；OpenModelPool 节点需要公网域名才能充当 Gateway 为全网服务。Cloudflare Tunnel 是实现这一目标的核心工具。

### 11.2 域名角色

| 域名类型 | 角色 | BT 类比 |
|---------|------|---------|
| **项目域名** (openmodelpool.com) | 全球统一入口，指向 Seed 节点 | Tracker 地址 |
| **用户域名** (my-node.chal.cc) | 节点专属入口，成为 Gateway | 公网 peer 地址 |

### 11.3 两类用户的寻址方式

#### 纯消费者（使用全球公共 Key）

```
用户 → DNS 解析 api.openmodelpool.com
     → 拿到 Gateway IP 列表（DNS 轮询）
     → 连接任意一个 Gateway
     → 用公共 Key 发请求
     → Gateway 路由到最优 Provider
     → 返回结果
```

**BT 类比**：类似 BT 新手连接 Tracker → 获取 peer 列表 → 开始下载。

#### 节点运营者（使用自己的固定域名）

```
用户 → https://my-node.chal.cc/v1
     → 自己的节点
     ├── 本地有该模型？ → 直接处理
     └── 本地没有？ → 转发到全网
     → 用户体验与全球统一入口完全一致
```

**BT 类比**：类似有公网 IP 的 BT 客户端——既能直连下载，也能被其他 peer 连接获取数据。

### 11.4 隧道集成（多方案支持）

系统支持多种隧道方案，用户根据自身环境选择：

| 方案 | 说明 | 适用场景 | 实现状态 |
|------|------|---------|---------|
| **Cloudflare Tunnel** | 自定义域名，需 Cloudflare 托管域名 | 生产环境 Gateway | ✅ 已实现 |
| **ngrok** | 固定 HTTPS 域名，零配置快速穿透 | 测试/临时使用/无域名场景 | ✅ 已实现 |
| **FRP** | 固定 IP，自建中继服务器 | 有公网服务器的用户 | ✅ 已实现 |
| **公网 IP 直连** | 直接暴露端口 | 有公网 IP 的服务器 | ✅ 已实现 |

**Cloudflare Tunnel 一键绑定流程**：
1. 用户在管理面板输入 Cloudflare API Token + 自定义域名
2. 后端自动创建命名隧道 + 配置 DNS
3. 启动 `cloudflared tunnel run`
4. 节点成为 Gateway，自动注册到网络

**ngrok 快速穿透**：
1. 用户输入 ngrok authtoken（可选，免费版随机域名）
2. 系统自动启动 ngrok 隧道，获取 HTTPS 域名
3. 支持固定域名（付费版），域名复用时自动检测已有隧道

**FRP 自建中继**：
1. 用户输入 FRP 服务端地址 + 端口 + token
2. 系统自动配置 FRP 客户端，建立固定 IP 隧道
3. 支持 systemd 开机自启

### 11.5 DNS 轮询与负载均衡

**Phase 0（单 Seed）**：
```
openmodelpool.com.       A  创始节点 IP
api.openmodelpool.com.   A  创始节点 IP
```

**Phase 1+（多 Gateway，DNS 轮询）**：
```
api.openmodelpool.com.   A  创始节点 IP
                         A  Bob 的节点 IP
                         A  Alice 的节点 IP
$TTL 300  ; 5 分钟，确保节点下线后快速生效
```

**BT 类比**：多 Tracker 地址轮询——BT 客户端连接多个 Tracker 获取更多 peer；OpenModelPool 的 DNS 轮询指向多个 Gateway 实现天然负载均衡。

---

## 12. 联邦治理与共识

### 12.1 设计哲学

BT 没有中心治理——协议就是法律，客户端实现遵循协议即可互操作。OpenModelPool 同样追求最小治理：GitHub 注册表作为"宪法层"，日常运行靠 Gossip 自治，仅在必要时（争议裁决）引入社区治理。

### 12.2 三层治理模型

```
┌──────────────────────────────────────────────────┐
│  宪法层：GitHub 仓库 (trust_pool.json)            │
│  - 节点准入审核                                    │
│  - 争议最终裁决                                    │
└──────────────────────┬───────────────────────────┘
                       │
┌──────────────────────┴───────────────────────────┐
│  运行层：Gossip 协议                               │
│  - 实时状态同步                                    │
│  - 声誉评分交换                                    │
│  - Provider 公告广播                               │
└──────────────────────┬───────────────────────────┘
                       │
┌──────────────────────┴───────────────────────────┐
│  数据层：贡献账本                                   │
│  - 贡献/消费记录                                   │
│  - 争议存证                                       │
│  - 关键事件上链                                    │
└──────────────────────────────────────────────────┘
```

**BT 类比**：

```
GitHub 注册表 = Tracker（权威源）
Gossip 协议 = DHT（日常运行）
贡献账本 = 下载统计（数据记录）
```

### 12.3 节点准入

| 阶段 | 准入方式 | BT 类比 |
|------|---------|---------|
| 冷启动 | 管理员审核（一票制） | 私有 Tracker 邀请制 |
| 网络形成 | 种子节点 approve | 半公开 Tracker |
| 自治网络 | Gossip 自治 + GitHub 注册表仅作备用 | 公开 Tracker + DHT |

### 12.4 共识机制

- **日常运行**：Gossip 最终一致，GitHub 注册表为最终权威
- **冲突解决**：GitHub trust_pool.json 为最终权威，Gossip 为加速层
- **争议裁决**：多节点签名确认，关键事件上链存证

---

## 13. P2P 消息系统

### 13.1 设计哲学

BT 客户端之间通过扩展协议（如 uTP、PEX）交换控制消息；OpenModelPool 节点之间通过 P2P 消息系统进行管理通信——节点交流、请求协助、合作提议等。

### 13.2 消息架构

```
发送方                          中继节点                         接收方
  │                               │                               │
  │  1. 用接收方公钥加密消息       │                               │
  │  2. 附加发送方签名             │                               │
  │  3. Gossip 中继投递 ─────────▶│  4. 无法解密，仅转发 ────────▶│
  │                               │                               │
  │                               │                  5. 用私钥解密 │
  │                               │                  6. 验证签名   │
  │◀────────────────────────────────────────────── 7. 回复 ──────│
```

**关键特性**：
- **端到端加密**：用接收方 Ed25519 公钥加密，中继节点无法解密
- **签名验证**：发送方 Ed25519 签名，接收方可验证来源
- **Gossip 中继**：无需直连，消息通过 Gossip 网络投递
- **额度消耗**：发送消息消耗签发节点的免费额度

### 13.3 消息类型

| 类型 | 用途 | BT 类比 |
|------|------|---------|
| 节点交流 | 管理员之间的日常沟通 | BT 扩展协议消息 |
| 请求协助 | 请求特定模型服务 | piece request |
| 合作提议 | 节点间合作协议 | PEX 交换 |
| 系统通知 | 网络参数变更通知 | Tracker 通知 |

### 13.4 消息安全

- 中继节点只能看到加密密文，无法解密内容
- 发送方身份通过签名验证
- 消息有 TTL，过期自动清理
- 每人每天免费消息额度由网络参数决定

---

## 14. 虚假能力防御

### 14.1 问题描述

节点可能声称可以提供某个模型的服务，但实际上无法提供——类比 BT 中的虚假 peer（声称拥有数据块但实际没有）。v3.0 采用**纵深防御**策略。

### 14.2 四层防御体系

```
┌─────────────────────────────────────────────────────────────────┐
│  第一层：主动探测                                                │
│  - 定期发送 1-token 测试请求验证能力声明                          │
│  - 成本：每次约 $0.00001                                        │
│  - 检测时间：< 5 分钟                                            │
│  - BT 类比：下载一个 piece 后用 SHA 哈希验证完整性               │
├─────────────────────────────────────────────────────────────────┤
│  第二层：交叉验证                                                │
│  - 多个节点独立验证同一声明                                      │
│  - 至少 3 个独立验证                                             │
│  - 偏差 > 20% 触发调查                                          │
│  - BT 类比：多个 peer 同时验证同一 piece 的哈希                  │
├─────────────────────────────────────────────────────────────────┤
│  第三层：惩罚机制                                                │
│  - 成功率 < 70%：警告                                            │
│  - 成功率 < 50%：降低路由优先级                                  │
│  - 成功率 < 30%：隔离（从池中移除）                              │
│  - 成功率 < 10%：封禁（全局广播）                                │
│  - BT 类比：Choking 慢速 peer → Snubbing 忽略 → Ban 封禁       │
├─────────────────────────────────────────────────────────────────┤
│  第四层：全局广播                                                │
│  - 封禁信息通过 Gossip 传播到所有节点                            │
│  - 立即更新路由表                                                │
│  - 被封禁节点无法服务任何请求                                    │
│  - 解除封禁需要多节点共识                                        │
│  - BT 类比：peer ban 列表全网同步                                │
└─────────────────────────────────────────────────────────────────┘
```

### 14.3 成本收益分析

```
虚假声明的成本：
  • 1-token 探测成本：约 $0.00001
  • 检测时间：< 5 分钟
  • 惩罚：声誉永久损失 + 可能被封禁
  • 恢复：需要 30 天以上的诚实服务

虚假声明的收益：
  • 临时的路由优先级（5 分钟内即被检测移除）
  • 无实际收益（无法服务真实请求）

结论：成本 >> 收益 → 自我执行的诚实机制
```

---

## 15. 迭代路线图

> **更新日期**：2026-07-20 | **当前版本**：v4.0.5（GitHub Release） | **代码 AppVersion**：4.0.4（待同步）

### 15.1 Phase 0：个人版 MVP（第 1-2 个月）— ✅ 全量交付

**目标**：先做好本地工具，再考虑网络

| 任务 | 状态 | 说明 |
|------|------|------|
| 本地 OpenAI-compatible proxy | ✅ | `:8000/v1/chat/completions`，支持 SSE 流式 |
| Provider Token 本地存储 | ✅ | AES-256-GCM 加密（`data/.enc_key`，前缀 `omp:e:`） |
| 本地额度管理 | ✅ | 多周期额度（daily/monthly/total），Key 级 + Provider 级双层 |
| 管理员界面 | ✅ | 模块化 Admin UI（admin.html + 3 子页面 + 5 JS 模块） |
| 剩余额度检测 | ✅ | 实时剩余额度计算与展示 |
| 加入共享网络提示 | ✅ | 满足条件时温和提示 |
| API 协议转换层 | ✅ | OpenAI 原生 + Anthropic `/v1/messages` + web_session 模板 |
| 本地 WAF 四层防护 | ✅ | Rate Limit / Token Limit / Content Safety / Behavioral |
| 注册项目域名 | ✅ | openmodelpool.com（Cloudflare 隧道绑定） |
| 预设平台覆盖 | ✅ | 37 个预设平台（含 iFlytek MaaS） |
| 平台自动发现 | ✅ | 后台异步扫描免费 AI 平台，管理面板审核导入 |
| 多 Key 管理 | ✅ | 单 Provider 多 API Key，逐个健康检查 + 合并模型 |
| SOCKS5/VMess 代理 | ✅ | 连接池复用（5-7s→0.3s），VMess 协议支持 |
| 统一管理脚本 | ✅ | 每平台一个 omp-manager 脚本 + `--auto-update` 无人值守模式 |

**交付物**：✅ 可独立使用的本地 AI 模型代理（个人版）— v4.0.5 已交付
**节点发现**：无（不联网）
**成本**：域名注册费（~$10-50/年）

### 15.2 Phase 1：共享版最小闭环（第 3-4 个月）— ✅ 基础架构已就绪

**目标**：用户主动加入共享网络，完成最小共享闭环

| 任务 | 状态 | 说明 |
|------|------|------|
| 用户主动加入共享网络流程 | ✅ | 助记词生成 + 确认 + 备份 |
| 助记词 + Node ID 生成 | ✅ | BIP39 → Ed25519 派生（`node.go`） |
| 共享额度配置 | ✅ | `network_enabled` + `share_to_pool` 两级开关 |
| Guest Proxy Key 签发 | ✅ | 含公共池访问权限选项、额度/限流配置 |
| mk_* 全系密钥清理 | ✅ | ClassifyKey 仅识别 3 类（Public/Guest/Proxy），mk_* 全部移除 |
| 旧积分系统清理 | ✅ | credits.go 重构为 QuotaAllocation，无 CreditTransaction/NodeUnlockState |
| 单官方 Seed 节点 | ✅ | :8001 端点（`network_seed.go`） |
| Gossip 协议 | ✅ | PING/PONG/GET_PEERS/PEERS/ANNOUNCE（`gossip.go`） |
| Relay 中继 | ✅ | 请求转发 + 签名验证 + 流式透传（`relay.go` + `network_relay.go`） |
| 声誉系统 | ✅ | 主动探测 + EWMA + 等级计算（`reputation.go`） |
| P2P 消息系统 | ✅ | 传输路径加密 + Ed25519 签名（`message.go`） |
| 联邦治理 | ✅ | 联邦节点认证 + 共享密钥（`federation.go`） |
| 传输路径加密 | ✅ | 中继不可见加密 |
| GitHub 注册表引导 | ✅ | 含三级降级策略 |
| HTTPS-only 通信 | ✅ | 自签证书互信 |
| 公网 IP 广播 | ✅ | 多地址 Gossip |
| 三级信任源降级 | ✅ | GitHub Registry → Seed 节点 → Active Peers |
| 轻量贡献积分 | 🔲 | Contribution Credit 记账（非金融化） |
| Provider Key 不上传承诺 | 🔲 | 代码审计 + 开源验证文档 |
| 贡献账本 Layer 1 | 🔲 | Gossip Ledger + Ed25519 签名验证 |
| Ticket 防双花系统 | 🔲 | 双向签名 + 批量公证 + 防共谋 |

**节点发现**：✅ GitHub 注册表 + 单 Seed 节点
**路由**：✅ Seed 节点直接转发 + Gateway 全路由模式
**DNS**：A 记录指向单 Seed 节点

### 15.3 Phase 2：P2P 网络增强（第 5-8 个月）— 🟡 部分完成

**目标**：去中心化网络基础设施

| 任务 | 状态 | 说明 |
|------|------|------|
| 实现 Gossip 协议 | ✅ | `gossip.go`，完整 PING/PONG/GET_PEERS/PEERS/ANNOUNCE |
| 实现 AddrMan | 🟡 | RouteTable 已实现（简化版），peers.dat 持久化待完善 |
| 实现 Kademlia DHT (256-bit) | 🔲 | 当前为 16 位简化版 RouteTable，Kademlia 待重构 |
| 混合穿透架构 | ✅ | 直连优先 + Relay 降级 |
| 节点绑定域名后自动注册为 Gateway | ✅ | Gateway 全路由模式（`network_relay.go`） |
| Gateway 转发请求到全网节点 | ✅ | `handleGatewayRequest` 实现模型路由 |
| DNS Manager 自动从 Gateway 列表更新 DNS | 🔲 | 半自动化 |
| 贡献账本 Layer 1 | 🔲 | Gossip Ledger + Ed25519 签名验证 |
| 信任系统完善 | ✅ | `reputation.go` 主动探测 + 交叉验证 + 声誉评分 |
| Ticket 防双花系统 | 🔲 | 双向签名 + 批量公证 + 防共谋 |
| HTTPS-only 通信 + 自签证书互信 | ✅ | 已实现 |
| 公网 IP 广播 + 多地址 Gossip | ✅ | 已实现 |
| 三级信任源降级 | ✅ | 已实现 |
| go-libp2p 集成 | 🔲 | P2P 底层从自实现迁移到 libp2p |

**节点发现**：✅ GitHub + Gossip + Seed（DHT 待实现）
**路由**：✅ 每个 Gateway 独立维护 RouteTable
**DNS**：半自动（DNS Manager 服务管理）

### 15.4 Phase 3：自治网络与生态（10 个月+）— 🔲 未开始

**目标**：移除中心化依赖，网络完全自治；裂变增长

| 任务 | 状态 | 说明 |
|------|------|------|
| Seed 节点降级为普通 Gateway | 🔲 | 不再特殊对待 |
| DNS 由多个 Gateway 共同维护 | 🔲 | 去中心化 DNS |
| 节点发现完全依赖 Gossip | 🔲 | GitHub 注册表仅作备用 |
| 贡献账本 Layer 2 | 🔲 | IPFS 可选持久化 |
| P2P 消息系统增强 | 🔲 | 传输路径加密 + Gossip 中继（基础已实现，待完善） |
| GitHub 裂变 | 🔲 | README "Join Federation" 按钮 + Issue 模板 |
| 游戏化激励 | 🔲 | 成就系统（Seed Node / Power Node / Connector 等） |
| 社交媒体裂变 | 🔲 | 分享卡片 + 传播渠道 |
| Web 前端完善 | 🔲 | 联邦网络页 + 审核面板 + 拓扑可视化 |
| AC 自动机敏感词过滤 | 🔲 | cloudflare/ahocorasick 集成 |
| BoltDB 持久化 | 🔲 | 账单与配置持久化存储 |

**节点发现**：纯 Gossip + DHT
**路由**：分布式路由表
**DNS**：多节点共同维护

### 15.5 文件实现状态

| 文件 | 职责 | 状态 |
|------|------|------|
| `node.go` | 节点身份、Ed25519 密钥对、BIP39 助记词、签名验签 | ✅ 已实现 |
| `gossip.go` | Gossip 协议：消息交换、传播、防循环 | ✅ 已实现 |
| `relay.go` | Per-node 限流 + Provider 中继 | ✅ 已实现 |
| `network_relay.go` | Gateway 全路由模式、请求转发 | ✅ 已实现 |
| `reputation.go` | 声誉评分：主动探测、EWMA、等级计算 | ✅ 已实现 |
| `network_seed.go` | :8001 Seed 端点、节点发现服务 | ✅ 已实现 |
| `network_keys.go` | 4 种 Key 类型（Proxy/Guest/Public/Provider），ClassifyKey | ✅ 已实现 |
| `credits.go` | 重构为 QuotaAllocation（GuestKeyPercent/PublicKeyPercent） | ✅ 已重构 |
| `message.go` | 点对点消息：加密、签名、Gossip 中继 | ✅ 已实现 |
| `federation.go` | 联邦节点认证、共享密钥 | ✅ 已实现 |
| `anthropic_api.go` | Anthropic Messages API 兼容层（`/v1/messages`） | ✅ 已实现 (v4.0.4) |
| `platform_discovery.go` | 平台自动发现、异步扫描、审核导入 | ✅ 已实现 (v4.0.3) |
| `vmess.go` | VMess 代理协议实现 | ✅ 已实现 |
| `sider.go` | Sider web_session 模板（首个无 API 平台适配） | ✅ 已实现 |
| `token_estimator.go` | Token 精准估算（上游 usage + tiktoken 降级） | ✅ 已实现 |
| `addrman.go` | 地址管理器、peers.dat 持久化 | 🔲 待实现（当前用 RouteTable 替代） |
| `dht.go` (重构) | Kademlia DHT：256 位、k-buckets、迭代查找 | 🔲 待实现 |
| `eventbus.go` | 事件总线 | ✅ 已实现 |
| `tracker.go` | 节点追踪 | ✅ 已实现 |
| `health.go` | 健康检查 | ✅ 已实现 |
| `pricing_impl.go` | 定价数据 | ✅ 已实现（部分数据待填充） |

### 15.6 v4.0.2 ~ v4.0.5 Changelog 摘要

| 版本 | 日期 | 主要变更 |
|------|------|---------|
| **v3.3.0** | 2026-07-18 | web_session 通用模板系统（Sider 迁移为首个模板，20 模型）；Admin UI 模块化重构 |
| **v3.4.1** | 2026-07-18 | Admin UI 模块化：admin.html 5063→2457行，JS 拆分为 4 个独立模块，子页面改为 iframe 模态弹窗 |
| **v4.0.2** | 2026-07-20 | ngrok 隧道域名复用修复；FRP 固定 IP + ngrok 固定 HTTPS 域名双隧道 systemd 开机自启 |
| **v4.0.3** | 2026-07-20 | 多 Key 健康检查（逐个测试所有 Key）；多周期额度系统（daily/monthly/total × private/shared）；Guest 池拆分（GuestPoolPercent）；平台自动发现 |
| **v4.0.4** | 2026-07-20 | Anthropic Messages API 兼容层（`/v1/messages`）；SOCKS5 Transport 连接池（5-7s→0.3s）；ChatMessage 数组 content 修复；FindCandidates 使用 GetAllRaw 修复 Safe() 掩码 bug |
| **v4.0.5** | 2026-07-20 | 11 脚本精简为 2（omp-manager.sh + omp-manager.ps1）；`--auto-update` 无人值守模式；动态 Release 资产检测（自动适配裸二进制/压缩包）；README 全面修复（28 项） |

---

## 16. 完整 BT vs OpenModelPool 对照表

| 维度 | BitTorrent | OpenModelPool v4.0.5 |
|------|-----------|-------------------|
| **共享资源** | 文件分片（静态、可哈希验证） | AI 模型调用能力（动态、有状态、有配额） |
| **核心隐喻** | Swarm 下载 | 能力隧道（Capability Tunnel） |
| **节点类型** | 所有客户端对等，同时做种和下载 | 默认 Personal Mode；主动升级为 Network Mode（两级开关） |
| **种子源** | .torrent 文件 / Tracker | GitHub 注册表 / Seed 端点 (:8001) |
| **初始做种者** | 第一个上传完整文件的人 | 官方 Seed 节点（创始人节点） |
| **节点发现** | Tracker → DHT (Kademlia 160-bit) | GitHub 注册表 → :8001 Seed → Gossip + DHT (256-bit) |
| **地址交换** | PEX (Peer Exchange) | Gossip 协议（GET_PEERS/PEERS） |
| **地址持久化** | peer 缓存 | AddrMan + peers.dat |
| **能力声明** | bitfield 消息（持有数据块） | CapabilityClaim（持有模型服务） |
| **能力更新** | Have 消息 | Provider 公告（Gossip 广播） |
| **激励模式** | Tit-for-Tat（以牙还牙） | 贡献权重驱动路由优先级 |
| **分享率** | 上传量 / 下载量 | 贡献量 / 消费量 |
| **Choking** | 限制低速率 peer 的下载 | 限制低贡献节点的访问额度 |
| **Unchoke** | 允许高速率 peer 下载 | 高权重用户优先调度 |
| **Rarest-first** | 优先下载稀有 piece | 路由到负载最低的可用节点 |
| **Snubbing** | 忽略慢速 peer | 慢节点检测 + 替换 |
| **内容验证** | SHA-1/SHA-256 哈希 | Ed25519 签名 + 主动探测 |
| **匿名性** | 无 | 中等（请求者与提供者互相不知身份） |
| **延迟敏感度** | 分钟级可容忍 | 毫秒级敏感 |
| **验证确定性** | 确定性（哈希匹配） | 概率性（API 响应非确定性） |
| **网络成熟度** | Tracker → DHT | GitHub 注册表 → Gossip + DHT |
| **只要一人做种** | 全网可下载完整文件 | 只要一个 Gateway 活着全网可达 |
| **不贡献行为** | Leech（被 Choking 限制） | 仅消费节点（公共 Key 动态算法分配额度） |

---

## 17. 去中心化数据矩阵

### 17.1 数据分布策略

所有数据按照"需要全网共识 → P2P 分布式存储；仅本地使用 → 本地存储"的原则分类。

| 数据类型 | 存储方式 | 共识要求 | BT 类比 | 说明 |
|----------|----------|---------|---------|------|
| **节点注册表** | ✅ Gossip 全量同步 + GitHub 注册表 | 全网一致 | Tracker peer 列表 | 每个节点持有完整副本 |
| **Provider 目录** | ✅ 签名发布 + Gossip 传播 | 最终一致 | bitfield + Have | 由发布节点签名，可验证来源 |
| **模型路由表** | ✅ 从 Provider 目录推导 | 无需单独存储 | piece 选择策略 | 无需共识，本地计算 |
| **能力声明** | ✅ 签名发布 + Gossip 传播 + TTL | 最终一致 | bitfield | 带过期时间，自动失效 |
| **声誉评分** | ✅ Gossip 交换 + 加权平均 | 最终一致 | peer 速率统计 | 每个节点独立计算后广播 |
| **贡献记录** | ✅ Gossip Ledger (Layer 1) | 交叉验证 | 上传/下载统计 | Ed25519 签名 + 3 节点确认 |
| **关键事件** | ✅ IPFS (Layer 2) | 不可变存证 | 无直接对应 | 争议裁决、大额贡献 |
| **P2P 消息** | ⚠️ 端到端加密 + Gossip 中继 | 无 | BT 扩展协议 | 中继节点无法解密内容 |
| **网络参数** | ✅ GitHub 注册表为权威 | 权威决定 | BT BEP 提案 | 变更由 GitHub 注册表决定 |
| **AddrMan / peers.dat** | 🔒 本地持久化 | 无 | peer 缓存 | 每个节点独立维护 |
| **Node Proxy Key** | 🔒 本地存储 | 无 | peer_id | 仅本地验证（原 Proxy API Key） |
| **Guest Key Store** | 🔒 本地存储 | 无 | 无 | 签发节点独立管理 |
| **Provider Token** | 🔒 本地 AES-256-GCM 加密存储 | 无 | 无 | 原 Provider Key，绝不上传服务器 |
| **助记词 (Mnemonic)** | 🔒 用户自行保管（抄写/加密导出） | 无 | 无 | 恢复 Node ID 的唯一凭证，不含 Provider Token |
| **Ed25519 私钥** | 🔒 本地加密存储（由助记词派生） | 无 | 无 | AES-256-GCM 加密，由助记词恢复 |
| **额度分配 (Provider Key 级)** | 🔒 本地配置 | 无 | 带宽限制设置 | 节点主人对每个 Provider Key 精细化设定 |
| **使用量统计** | 🔒 本地存储 | 无 | 下载统计 | 仅用于本地额度控制 |

### 17.2 数据一致性模型

| 层级 | 一致性要求 | 机制 | 延迟 |
|------|-----------|------|------|
| GitHub 注册表 | 强一致（权威源） | git commit + push | 分钟级 |
| Gossip 协议 | 最终一致 | 30 秒周期交换 | 秒级 |
| DHT 路由 | 最终一致 | k-bucket 刷新（10 分钟） | 分钟级 |
| 本地数据 | 强一致 | 本地文件读写 | 毫秒级 |

### 17.3 数据冲突解决

| 冲突类型 | 解决策略 | 依据 |
|---------|---------|------|
| Gossip 数据与 GitHub 注册表冲突 | **GitHub 注册表为最终权威** | 类比 Tracker 数据优先于 DHT |
| 多节点声誉评分不一致 | **加权平均** | EWMA 平滑 + 交叉验证 |
| 贡献记录偏差 > 20% | **标记可疑，触发调查** | 3 节点交叉验证 |
| 路由表与实际能力不符 | **主动探测验证，以探测结果为准** | 类比 BT 哈希验证 |

---

## 附录 A：已确认的设计决策记录

本附录记录 v3.0 设计过程中所有已确认的决策，用于解决历史文档间的矛盾。

### A.1 密钥体系

| 决策 | 确认内容 | 废弃内容 |
|------|---------|---------|
| Key 类型数量 | 4 种 | mk_trial_, mk_open_, mk_open_global_, mk_{consumer_id} 全部移除 |
| Guest Key 格式 | `sk-guest-{node_id}-{random}` | `sk-{device_id}.{random}` |
| 公共 Key 格式 | `sk-openmodelpool-com-github-lisiyu-openmodelpool-public-key-v1` | `mk_public_v1`, `mk_open_global_*` |
| Ed25519 签名 Key | 不再用于用户 Key | `mk_{consumer_id}.{payload}.{sig}` 全部移除 |

### A.2 节点模型

| 决策 | 确认内容 | 废弃内容 |
|------|---------|---------|
| 节点类型 | 统一 Peer，所有节点对等 | Bootstrap/Ordinary/Relay/Exit 预设类型 |
| 运行模式 | 双模式：Personal Mode（默认）/ Network Mode（主动加入） | 默认加入网络 |
| 两级开关 | `network_enabled`（加入网络）+ `share_to_pool`（共享额度）分离 | 单一 `share_to_pool` 开关 |
| 共享机制 | 加入共享网络 ≠ 自动共享额度 | 加入网络 = 共享资源 |
| 角色决定 | 能力声明动态决定 | 静态类型绑定 |
| 节点身份生成 | 只在加入共享网络时生成（助记词 → Ed25519） | 首次启动静默生成 |

### A.3 额度模型

| 决策 | 确认内容 | 废弃内容 |
|------|---------|---------|
| 分配模型 | Provider Key 级精细化管控 | 积分系统、动态阈值解锁 |
| 免费额度 | 全网节点共享给公共 Key 额度之和（动态汇总） | 固定基础额度、从消费者池分配 |
| 贡献激励 | 路由优先级（软约束） | 动态阈值解锁（硬约束） |
| 公共 Key 总额度 | = 所有节点设定的共享给免费公共 Key 的额度之和 | 从免费消费者池中分配基础额度 |
| 节点 Key 路由 | 本地优先（自己有模型始终优先用自己的） | 无本地优先规则 |
| 额度管控 | Token/积分双模式、有效期/周期、模型级分配、限流 | 简单 X%/Y% 百分比 |

### A.4 网络架构

| 决策 | 确认内容 | 废弃内容 |
|------|---------|---------|
| DHT | 256 位 Kademlia | 16 位简化哈希环 |
| 节点发现 | GitHub + :8001 Seed + Gossip + AddrMan | 仅 Bootstrap 节点查询 |
| 中继方式 | 单跳透传 | 多跳洋葱路由 |
| 身份格式 | `mmx-{hex}` (Ed25519 公钥编码) | `mm-{base58}` |
| 身份格式 | `mmx-{hex}` (Ed25519 公钥编码) | `mm-{base58}` |

### A.5 移除的机制

| 移除项 | 移除原因 |
|--------|---------|
| IOTA Token Economy (Layer 3) | 代币相关机制不需要 |
| 防恶意注册机制（邀请码/邀请链/速率限制） | 防恶意注册相关不需要 |
| 提案+投票共识 | 不需要社区投票共识 |
| Sybil 攻击防御（经济成本/社交信任图） | 与代币/邀请机制相关，一并移除 |

### A.6 产品定位与技术选型（v3.0 修订版新增）

| 决策 | 确认内容 |
|------|---------|
| **产品定位** | 去中心化 AI 资源**公益**共享网络，盘活闲置 AI 模型额度 |
| **红线原则** | 不追求商业收益，不发行任何金融资产（绝不发币），不建立商业化交易体系 |
| **安全底线** | API Key 永远保存在本地绝不上传服务器；Prompt 端到端加密 |
| **消费端体验** | 零配置启动，本地暴露 OpenAI 兼容 API 端口，即插即用 |
| **提供方存储** | AES-256-GCM 加密存储 Provider Key（密钥文件 `data/.enc_key`） |
| **技术栈** | 纯 Go 生态，单二进制文件，go-libp2p + BoltDB + AC 自动机 |
| **穿透模式** | 直连优先 + Relay 降级（5秒超时，2分钟中继上限） |
| **WAF 防护** | 本地四层防护（Rate Limit / Token Limit / Content Safety / Behavioral） |
| **防双花** | Usage Ticket 双向签名 + 批量公证 + 关联分析 |

### A.7 工程健壮性优化（v3.0 第二次修订新增）

| 决策 | 确认内容 |
|------|---------|
| **抗抖动缓冲** | 应用层 Jitter Buffer（自适应 50ms-2s）+ SSE chunk 序号选择性重传 |
| **Relay 动态超时** | 长上下文生成场景按 chunk 到达重置 30s 倒计时，最大延长至 10 分钟 |
| **API 协议转换** | 统一中间表示 (IR) + PlatformAdapter 接口，支持 Gemini/Claude 等异构平台 |
| **防共谋增强** | 上游响应指纹 (request-id) + 随机抽样验证 + 统计异常检测 + 公证人去中心化演进 |
| **WAF 分级阻断** | 违规分级（1次警告→3次临时封禁→5次+长期封禁），L1 硬拦截除外 |
| **申诉机制** | 临时封禁自动解封，长期封禁通过管理面板申诉 |
| **Token 精准估算** | 优先读取上游 usage 字段，降级用 tiktoken_go 本地估算 |
| **DNS 备用信标** | DNS TXT 记录作为 GitHub 阻断时的兜底寻址方案 |
| **冷启动降级链** | AddrMan → GitHub Raw → GitHub Pages → DNS TXT → DoH → 硬编码 IP |

### A.8 产品模式与身份机制（v4.0 重大修订新增）

| 决策 | 确认内容 |
|------|---------|
| **默认模式** | 默认启动为个人版 Personal Mode，不加入 P2P 网络 |
| **加入条件** | 配置 Provider Token + 开启额度管理 + 本月有剩余额度 → 温和提示 |
| **节点身份生成时机** | 只在用户主动加入共享网络时生成（非首次启动静默生成） |
| **助记词机制** | BIP39 助记词 → 派生 Ed25519 私钥 → Node ID。加入时强制备份 |
| **助记词不包含 Provider Token** | 重新部署后 Provider Token 需重新配置 |
| **两级开关** | `network_enabled`（加入网络）+ `share_to_pool`（共享额度）分离 |
| **贡献积分保留** | 移除旧版复杂 Credit 系统，保留轻量 Contribution Credit 作为记账单位 |
| **Public Global Key 定位** | 低额度体验入口，严格限流（全局/IP/时间窗口/模型四重上限），不保证可用 |
| **加密表述修正** | "端到端加密" → "传输路径加密（中继不可见）"，资源节点需解密 |
| **Token 银行叙事** | 临时 Token 银行 + 极客共享网络，替代纯 BT 隐喻 |
| **路线图重排** | Phase 0 个人版MVP → Phase 1 共享版最小闭环 → Phase 2 P2P增强 → Phase 3 自治网络 |

### A.9 Guest Key 智能路由与 P2P 健壮性（v4.0.2 新增）

| 决策 | 确认内容 |
|------|---------|
| **Guest Key 公共池权限** | 签发时可选择授予 Public Pool 访问权限，有此权限可在任意节点访问全网资源 |
| **Guest Key 无公共池权限** | 只能访问签发节点本地资源，不跨节点 |
| **未入网节点 Guest Key** | 只能通过节点自身 URL 访问，无路由转发能力 |
| **HTTPS-only** | 所有节点间通信强制 HTTPS，不允许 HTTP |
| **自签证书互信** | 信任池内节点跳过 TLS 证书校验，HTTPS 仅保证传输加密 |
| **HTTPS 来源** | 固定域名 > Cloudflare 隧道 > 公网 IP 自签证书 |
| **公网 IP 广播** | collectAddresses() 合并公网 IP 检测，以 https://IP:port 格式广播 |
| **多地址 Gossip** | 遍历 peer.Addresses 逐个尝试，回退到 peer.Endpoint |
| **三级信任源** | GitHub Registry → Bootstrap/Seed 节点 → Active Peers |
| **中继独立于共享网络** | 中继可单独开启，不依赖是否加入共享网络 |
| **统一分享中心** | Guest Key 签发 + 协作者邀请 + 多渠道分享（QR码+文案）合并为统一界面 |
| **贡献仪表盘** | 网络面板展示 🏆我的贡献 / 📊网络概览 / 🌐全局指标 |

### A.10 工程迭代与功能增强（v4.0.3 ~ v4.0.5 新增）

| 决策 | 确认内容 |
|------|---------|
| **Anthropic Messages API** | 原生 `/v1/messages` 端点，消费方用 Anthropic SDK 直接调用，内部转换为 OpenAI 格式路由 |
| **web_session 模板系统** | 新增 `web_session` 类型抽象无 API 的网页登录平台，WebSessionConfig + 7 个通用函数，Sider 为首个模板 |
| **多 Key 管理** | 单 Provider 支持多个 API Key（`APIKeys []APIKeyConfig`），健康检查遍历所有 Key 任一成功即绿灯，模型合并所有 Key 的模型 |
| **多周期额度系统** | Key 级（daily/monthly/total）+ Provider 级（private/shared × daily/monthly）双额度控制，有效额度 = min(非零周期) |
| **Guest 池拆分** | shared key 的 quota 按 guest_pool_percent 比例拆分到 Public 池和 Guest 池，node_stats 按加权平均计算 |
| **SOCKS5 连接池** | `proxiedTransportCache`（sync.Map）缓存代理 Transport，连接复用延迟从 5-7s 降至 0.3s |
| **Safe() 掩码修复** | `FindCandidates` 使用 `GetAllRaw()` 获取真实 proxy/API key，避免 `Safe()` 方法掩码敏感字段导致路由失败 |
| **平台自动发现** | 后台异步扫描免费 AI 平台，管理面板审核，一键导入为预设 |
| **VMess 代理支持** | 自定义 VMess 协议实现，支持通过 VMess 代理访问被墙平台 |
| **Admin UI 模块化** | admin.html 从 5063 行精简至 2457 行，JS 按模块拆分为 4 个独立文件，子页面改为 iframe 模态弹窗 |
| **脚本精简** | 11 个脚本合并为 2 个全功能管理脚本（Linux omp-manager.sh + Windows omp-manager.ps1），支持 `--auto-update` 无人值守模式 |
| **动态 Release 资产检测** | 脚本自动检测 GitHub Release 资产格式（裸二进制/压缩包），自动选择并解压 |
| **跨平台编译** | 支持 linux-amd64 / linux-arm64 / linux-armv7 / windows-amd64 四平台交叉编译 |
| **37 预设平台** | 新增 iFlytek MaaS（讯飞星火），覆盖 openai/deepseek/qwen/zhipu/moonshot/minimax/siliconflow/groq/xai/together/mistral/doubao/spark/gemini/openrouter/poe/sider/ollama/coze/nvidia/anthropic/qianfan/novita/fireworks/cohere/cerebras/stepfun/baichuan/agnes/aihubmix/lmstudio/xfyun-maas |

---

## 附录 B：与当前实现的偏差清单

> **更新日期**：2026-07-20 | **基线版本**：v4.0.5 | 原始基线 v3.3.0 的大部分偏差已修复

### B.1 密钥体系偏差（原严重性：🔴 高 → ✅ 已修复）

| 偏差项 | 原始实现 | v4.0 要求 | 当前状态 |
|--------|---------|----------|---------|
| Key 类型 | 6+ 种旧 Key 完整保留 | 4 种 | ✅ 已修复：mk_* 全部移除 |
| ClassifyKey() | 识别 6 类 | 仅识别 3 类 | ✅ 已修复：仅识别 Public/Guest/Proxy |
| 公共 Key | `mk_open_global_{node_id}_{random}` | 固定常量 | ✅ 已修复：固定常量 `sk-openmodelpool-com-...` |
| Guest Key | `sk-guest-` 格式未实现 | 完整实现 | ✅ 已修复：`sk-guest-{node_id}-{random}` |
| Ed25519 签名 Key | 完整保留 | 全部移除 | ✅ 已修复：用户 Key 不再使用 Ed25519 签名 |

### B.2 额度模型偏差（原严重性：🔴 高 → ✅ 大部分已修复）

| 偏差项 | 原始实现 | v4.0 要求 | 当前状态 |
|--------|---------|----------|---------|
| 积分系统 | credits.go 完整保留 | 移除 | ✅ 已修复：credits.go 重构为 QuotaAllocation |
| NodeUnlockState | ~150 行死代码 | 移除 | ✅ 已修复：全代码库 0 引用 |
| 动态阈值解锁 | network.go 实现 | 移除 | ✅ 已修复 |
| Provider Key 级额度管控 | 未实现 | 实现 | ✅ 已实现：Key 级 daily/monthly/total |
| 额度有效期/周期 | 未实现 | 支持截止期日和周期额度 | ✅ 已实现：daily/monthly/total + 自动重置 |
| 积分管控模式 | 未实现 | 支持积分→token换算 | 🔲 待实现：当前用 Token 直接管控 |
| 模型级额度分配 | 未实现 | 按热度或自定义分配 | 🔲 待实现 |
| 限流选项 | 未实现 | 支持频率/会话次数限制 | ✅ 已实现：RPM/DailyRequestLimit |
| 公共 Key 总额度计算 | 未实现 | = 所有节点公共 Key 共享额度之和 | 🔲 待实现（需共享网络激活后） |
| 节点 Key 本地优先路由 | 未实现 | 本地模型始终优先 | ✅ 已实现：FindCandidates 本地优先 |
| 多 Key 管理 | 未实现 | 单 Provider 多 Key | ✅ 已实现：APIKeys []APIKeyConfig |
| Guest 池拆分 | 未实现 | GuestPoolPercent 按比例拆分 | ✅ 已实现 |
| Provider 级额度 | 未实现 | private/shared × daily/monthly | ✅ 已实现 |

### B.3 网络发现偏差（原严重性：🔴 极高 → ✅ 大部分已修复）

| 偏差项 | 原始实现 | v4.0 要求 | 当前状态 |
|--------|---------|----------|---------|
| :8001 Seed 端点 | 未实现 | 实现 | ✅ 已实现：`network_seed.go` |
| /api/peers 端点 | 未实现 | 实现 | ✅ 已实现 |
| AddrMan + peers.dat | 未实现 | 实现 | 🟡 部分实现：RouteTable（简化版），peers.dat 持久化待完善 |
| Gossip 协议 | 30 秒心跳，非标准协议 | 完整协议 | ✅ 已实现：`gossip.go` PING/PONG/GET_PEERS/PEERS/ANNOUNCE |
| DHT | 16 位简化版 | 256 位 Kademlia | 🔲 待实现：当前仍为简化 RouteTable |
| Gateway 全路由 | 未实现 | 实现 | ✅ 已实现：`network_relay.go` handleGatewayRequest |

### B.4 代码清理清单（原严重性：🔴 高 → ✅ 全部完成）

| 删除目标 | 文件 | 原行数估计 | 当前状态 |
|---------|------|---------|---------|
| NodeUnlockState 相关代码 | network.go | ~150 行 | ✅ 已清理 |
| fetchPeerPublicKey() 遗留 stub | network_keys.go | ~30 行 | ✅ 已清理 |
| CreditTransaction / NodeCredits 类型 | types.go | ~40 行 | ✅ 已清理 |
| mk_trial_ 签发/验证逻辑 | network_keys.go | ~100 行 | ✅ 已清理 |
| mk_open_ 签发/验证逻辑 | network_keys.go | ~150 行 | ✅ 已清理 |
| mk_open_global_ 签发/验证逻辑 | network_keys.go | ~80 行 | ✅ 已清理 |
| mk_{consumer_id} 签名 Key 逻辑 | network_keys.go | ~200 行 | ✅ 已清理 |
| credits.go 积分系统 | credits.go | ~250 行 | ✅ 已重构为 QuotaAllocation |

### B.5 v4.0.5 新增待修复项

| 待修复项 | 说明 | 优先级 |
|---------|------|--------|
| main.go AppVersion 同步 | 代码中 AppVersion = "4.0.4"，GitHub Release 最新为 v4.0.5 | 低 |
| providers.go 注释更新 | 注释写 "35 platforms" 但实际有 37 个 | 低 |
| 3 个预存测试失败 | TestDetectRegion_IPv4 / TestVerifyHeartbeatAuthInvalidBase64 / TestVerifyHeartbeatAuthWrongLength | 中 |
| success_rate 追踪 | /api/health 返回 null，需对接 | 中 |
| quota 追踪完善 | token 返回 0 需对接上游 usage | 中 |
| 本地 scripts/ 目录残留 | 本地仍有 11 个旧脚本（GitHub 已删 9 个） | 低 |
| Proxy API Key 未设置 | config 中 proxy_api_key 为 NOT SET | 中 |
| 请求日志持久化 | 内存存储重启丢失 | 中 |

---

*文档结束。本设计文档基于 OpenModelPool v2.0 设计文档（8 份）、v3.0 设计文档（2 次修订）、完整 PRD 评审、贡献账本设计、中文架构文档、架构审计报告、产品改进意见（14 点），以及最新确认的设计决策综合编写。所有矛盾以最新确认决策为准。v4.0 核心变化：默认个人版 + 助记词机制 + 两级开关 + Token 银行叙事 + 路线图重排。v4.0.1 补充：防共谋公证人去中心化演进、mk_* 全系密钥 Phase 1 优先清理排期、Proxy API Key 泄露后 Guest Key 手工控制。v4.0.5 更新：Phase 0 全量交付确认、Phase 1 基础架构已就绪、附录 B 偏差清单同步修复状态、新增 A.10 工程迭代决策记录、新增 15.5 文件实现状态表、新增 15.6 Changelog 摘要、技术栈更新为 Go 1.26+ + 实现状态标注。*
