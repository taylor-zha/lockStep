# LockStep 分布式架构设计

## 架构总览

```
┌──────────────────────────────────────────────────────────────────────────────────────┐
│                                        Client                                         │
└──────────────────────────────────────────────────────────────────────────────────────┘
                    │                                        │
                    │ WebSocket (认证/匹配)                   │ KCP (游戏帧同步)
                    ▼                                        │
┌─────────────────────────────────────────────┐            │
│              Gateway 接入层 (可扩展)          │            │
│  ┌─────────────────────────────────────┐    │            │
│  │  • WebSocket 监听                    │    │            │
│  │  • 认证/鉴权                         │    │            │
│  │  • 路由/负载均衡                     │    │            │
│  │  • 限流/熔断                         │    │            │
│  └─────────────────────────────────────┘    │            │
└─────────────────────────────────────────────┘            │
                    │                                    │
                    │ gRPC (负载均衡)                     │
                    ▼                                    │
┌─────────────────────────────────────────────┐            │
│              游戏侧 (Logic)                  │            │
│  ┌───────────────┐  ┌───────────────────┐   │            │
│  │    逻辑服      │  │    匹配服集群      │   │            │
│  │  (有状态)     │  │   (无状态)        │   │            │
│  │              │  │                   │   │            │
│  │ • 玩家数据    │  │  ┌─ Match #1 ─┐  │   │            │
│  │ • 背包/商城   │  │  │ Match #2   │  │   │            │
│  │ • 好友系统    │  │  │ Match #3   │  │   │            │
│  │ • Token验证   │  │  └────────────┘  │   │            │
│  └───────────────┘  └─────────┬─────────┘   │            │
└────────────────────────────────┼────────────┘            │
                                 │                         │
                                 ▼                         │
┌─────────────────────────────────────────────┐            │
│              Redis Cluster                   │            │
│  ┌─────────────────────────────────────┐    │            │
│  │  • Match Queue    (匹配队列)         │    │            │
│  │  • Room Registry  (房间服注册表)     │    │            │
│  │  • Session Store  (Gateway 会话)     │    │            │
│  │  • Match State    (匹配状态)         │    │            │
│  └─────────────────────────────────────┘    │            │
└─────────────────────────────────────────────┘            │
                                 │                         │
                    ┌────────────┴────────────┐            │
                    │ 注册/心跳 (gRPC)         │            │
                    ▼                         ▼            │
┌─────────────────────────────────────────────┐            │
│            房间服集群 (Room Cluster)          │◀──────────┘
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│  │ RoomServer  │  │ RoomServer  │  │ RoomServer  │
│  │    #1       │  │    #2       │  │    #3       │
│  │             │  │             │  │             │
│  │ KCP:8888    │  │ KCP:8888    │  │ KCP:8888    │
│  │ gRPC:9000   │  │ gRPC:9000   │  │ gRPC:9000   │
│  │ Load: 45    │  │ Load: 32 ✓  │  │ Load: 58    │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
│         │                │                │
└─────────│────────────────│────────────────│──────────
          │                │                │
          └────────────────┼────────────────┘
                           │
                      KCP 直连
                           │
                     ┌─────┴─────┐
                     │  Clients  │
                     └───────────┘
```

## 通信流程

### 1. 登录认证流程

```
Client                Gateway              逻辑服
  │                     │                    │
  │──WebSocket Connect─►│                    │
  │                     │                    │
  │──── LoginRequest ──►│                    │
  │     (token)         │                    │
  │                     │──gRPC VerifyToken─►│
  │                     │                    │
  │                     │◄── TokenInfo ──────│
  │                     │     (player_id)    │
  │                     │                    │
  │◄─ LoginResponse ────│                    │
  │   (session_id)      │                    │
```

### 2. 匹配流程

```
Client                Gateway              匹配服              房间服
  │                     │                    │                   │
  │── MatchRequest ────►│                    │                   │
  │                     │──gRPC Match ──────►│                   │
  │                     │                    │                   │
  │                     │                    │──AssignRoom──────►│
  │                     │                    │                   │
  │                     │                    │◄─RoomAddr─────────│
  │                     │                    │  (kcp_host:port)  │
  │                     │                    │                   │
  │                     │◄─ MatchResult ─────│                   │
  │                     │  (room_addr)       │                   │
  │                     │                    │                   │
  │◄─ MatchResponse ────│                    │                   │
  │   (kcp_addr,        │                    │                   │
  │    room_id,         │                    │                   │
  │    player_index)    │                    │                   │
```

### 3. 游戏帧同步流程 (KCP 直连)

```
Client                              房间服
  │                                   │
  │──── KCP Connect ─────────────────►│
  │     (room_id, player_id, token)   │
  │                                   │
  │◄─── ConnectAck ───────────────────│
  │     (success)                     │
  │                                   │
  │══════════ 帧同步循环 ═════════════│
  │                                   │
  │──── PlayerInput ─────────────────►│
  │     (frame, input_data)           │
  │                                   │
  │◄─── FrameData ────────────────────│
  │     (frame, all_inputs[])         │
  │                                   │
  │──── PlayerInput ─────────────────►│
  │◄─── FrameData ────────────────────│
  │            ...                    │
```

## 服务职责

### Gateway (接入层)

| 模块 | 职责 |
|------|------|
| WebSocket Handler | 客户端连接管理、消息编解码 |
| Auth Middleware | Token 验证、Session 管理 |
| Router | 消息路由到对应后端服务 |
| LoadBalancer | 后端服务负载均衡 |
| RateLimiter | 限流、防刷 |

**不处理**: 游戏逻辑、帧同步

### 逻辑服 (Logic Server)

| 模块 | 职责 |
|------|------|
| Player Service | 玩家数据 CRUD |
| Item Service | 背包、道具管理 |
| Shop Service | 商城、充值 |
| Friend Service | 好友、社交 |
| Auth Service | Token 验证、登录 |

### 匹配服 (Match Server)

| 模块 | 职责 |
|------|------|
| Match Queue | 匹配排队、ELO 匹配 |
| Room Allocator | 房间分配、负载均衡 |
| Room Registry | 房间服注册/发现 |
| Match Logic | 匹配规则、队伍组建 |

### 房间服 (Room Server)

| 模块 | 职责 |
|------|------|
| KCP Server | KCP 连接监听 (复用现有) |
| Session Manager | 玩家会话管理 |
| Room Manager | 房间生命周期 |
| Frame Manager | 帧同步控制 (复用现有) |
| Input Buffer | 输入缓冲 (复用现有) |

## 协议定义

### 外部协议 (Client ↔ Gateway)

| 路径 | 协议 | 用途 |
|------|------|------|
| `/ws` | WebSocket | 认证、匹配、业务请求 |
| `/kcp` | KCP | 游戏帧同步 (直连房间服) |

### 内部协议 (服务间)

| 服务对 | 协议 | 用途 |
|--------|------|------|
| Gateway ↔ 逻辑服 | gRPC | 认证、业务请求 |
| Gateway ↔ 匹配服 | gRPC | 匹配请求 |
| 匹配服 ↔ 房间服 | gRPC | 房间分配、状态同步 |

## 项目结构

```
lockStep/
├── cmd/
│   ├── gateway/           # Gateway 服务入口
│   │   └── main.go
│   ├── logic/             # 逻辑服入口
│   │   └── main.go
│   ├── match/             # 匹配服入口
│   │   └── main.go
│   └── room/              # 房间服入口 (原 server)
│       └── main.go
│
├── internal/
│   ├── gateway/
│   │   ├── server.go      # WebSocket 服务器
│   │   ├── handler.go     # 消息处理
│   │   ├── middleware.go  # 认证中间件
│   │   └── router.go      # 路由
│   │
│   ├── logic/
│   │   ├── service/
│   │   │   ├── player.go  # 玩家服务
│   │   │   ├── item.go    # 道具服务
│   │   │   └── auth.go    # 认证服务
│   │   └── repository/    # 数据访问层
│   │
│   ├── match/
│   │   ├── queue.go       # 匹配队列
│   │   ├── allocator.go   # 房间分配
│   │   └── registry.go    # 房间服注册
│   │
│   ├── room/              # 房间服 (复用现有)
│   │   ├── server.go
│   │   ├── manager.go
│   │   └── ...
│   │
│   ├── session/           # 共用
│   ├── handler/           # 共用
│   └── frame/             # 共用
│
├── pkg/
│   ├── protocol/
│   │   ├── message.proto      # 客户端消息 (现有)
│   │   ├── internal.proto     # 服务间消息 (新增)
│   │   └── *.pb.go
│   │
│   └── common/
│       ├── errors.go      # 统一错误码
│       └── config.go      # 配置结构
│
├── config/
│   ├── gateway.yaml
│   ├── logic.yaml
│   ├── match.yaml
│   └── room.yaml
│
└── docs/
    └── architecture.md    # 本文档
```

## 关键设计

### 1. 房间服集群扩展方案（方案三：房间绑定实例）

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          房间服集群架构                                    │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│   匹配服                                                                   │
│   ┌─────────────────────────────────────────────────────────────────┐    │
│   │  Room Registry                                                   │    │
│   │  ┌─────────────────────────────────────────────────────────┐    │    │
│   │  │ RoomServer-1 │ RoomServer-2 │ RoomServer-3 │ ...        │    │    │
│   │  │ load: 45     │ load: 32     │ load: 58     │            │    │    │
│   │  │ kcp: :8888   │ kcp: :8889   │ kcp: :8890   │            │    │    │
│   │  └─────────────────────────────────────────────────────────┘    │    │
│   └─────────────────────────────────────────────────────────────────┘    │
│                              │                                            │
│                              │ 分配房间时选择负载最低的实例               │
│                              ▼                                            │
│   ┌─────────────────────────────────────────────────────────────────┐    │
│   │                        RoomServer 集群                           │    │
│   │                                                                  │    │
│   │   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐          │    │
│   │   │ RoomServer  │   │ RoomServer  │   │ RoomServer  │          │    │
│   │   │     #1      │   │     #2      │   │     #3      │          │    │
│   │   │ 10.0.0.1    │   │ 10.0.0.2    │   │ 10.0.0.3    │          │    │
│   │   │ :8888(KCP)  │   │ :8888(KCP)  │   │ :8888(KCP)  │          │    │
│   │   │ :9000(gRPC) │   │ :9000(gRPC) │   │ :9000(gRPC) │          │    │
│   │   └─────────────┘   └─────────────┘   └─────────────┘          │    │
│   │         ▲                 ▲                 ▲                   │    │
│   └─────────│─────────────────│─────────────────│───────────────────┘    │
│             │                 │                 │                        │
│             └─────────────────┼─────────────────┘                        │
│                               │ KCP 直连                                 │
│                               │                                          │
│                          ┌────┴────┐                                     │
│                          │  Client │                                     │
│                          └─────────┘                                     │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

**核心流程**:
1. RoomServer 启动时向匹配服注册 (IP、端口、最大容量)
2. 匹配服维护所有 RoomServer 的负载状态
3. 分配房间时，选择负载最低的实例
4. 返回该实例的 KCP 地址给客户端
5. 客户端直连该 RoomServer 实例

**优势**:
- ✅ 延迟最低 (直连)
- ✅ 可横向扩展 (新增实例即可)
- ✅ 匹配服做负载均衡
- ✅ 每个实例独立，故障隔离

### 2. 房间服发现与注册机制

```go
// 匹配服维护房间服列表
type RoomRegistry struct {
    servers    map[string]*RoomServerInfo  // server_id -> info
    mu         sync.RWMutex
    healthTick time.Duration               // 健康检查间隔
}

type RoomServerInfo struct {
    ID        string    // 服务唯一标识
    KCPAddr   string    // KCP 监听地址 (给客户端直连)
    GRPCAddr  string    // gRPC 地址 (给匹配服调用)
    RoomCount int       // 当前房间数
    MaxRooms  int       // 最大房间数
    Status    string    // online / offline / draining
    LastHeart time.Time // 最后心跳时间
}

// 房间服启动时注册
func (r *RoomRegistry) Register(info *RoomServerInfo) error

// 房间服定期心跳
func (r *RoomRegistry) Heartbeat(serverID string, roomCount int) error

// 分配时选择负载最低的服务器
func (r *RoomRegistry) PickBest() (*RoomServerInfo, error) {
    // 选择 roomCount/minRooms 最小且 status=online 的实例
}

// 健康检查，踢掉超时的实例
func (r *RoomRegistry) HealthCheck()
```

**注册流程**:
```
RoomServer 启动
     │
     ├──► 监听 KCP (:8888)
     ├──► 监听 gRPC (:9000)
     │
     └──► 调用匹配服 Register()
              │
              ▼
         匹配服 Registry
         ┌─────────────────────────────┐
         │ ID: room-001                │
         │ KCP: 10.0.0.1:8888          │
         │ GRPC: 10.0.0.1:9000         │
         │ Status: online              │
         └─────────────────────────────┘
              │
              └──► 定期心跳 (每 5s)
                   更新 roomCount, LastHeart
```

### 3. Token 校验流程

```
1. Client 携带 token 连接 Gateway
2. Gateway 调用逻辑服 VerifyToken
3. 逻辑服返回 player_id
4. Gateway 创建 session，绑定 player_id
5. 后续请求携带 session_id

房间服直连时:
1. Client 携带 room_token 连接 (由匹配服签发)
2. 房间服验证 room_token (含 room_id, player_id, 过期时间)
3. 验证通过后加入房间
```

### 4. 匹配服高可用

#### 架构设计

```
┌────────────────────────────────────────────────────────────────────────────┐
│                           匹配服集群 (无状态化)                              │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   Gateway                                                                   │
│      │                                                                      │
│      │ gRPC (负载均衡)                                                      │
│      ▼                                                                      │
│   ┌──────────────────────────────────────────────────────────────────┐     │
│   │                        匹配服集群                                  │     │
│   │                                                                   │     │
│   │   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐           │     │
│   │   │  Match #1   │   │  Match #2   │   │  Match #3   │           │     │
│   │   │             │   │             │   │             │           │     │
│   │   │  无状态     │   │  无状态     │   │  无状态     │           │     │
│   │   │  计算匹配   │   │  计算匹配   │   │  计算匹配   │           │     │
│   │   └──────┬──────┘   └──────┬──────┘   └──────┬──────┘           │     │
│   │          │                 │                 │                  │     │
│   └──────────│─────────────────│─────────────────│──────────────────┘     │
│              │                 │                 │                        │
│              └─────────────────┼─────────────────┘                        │
│                                │                                           │
│                                ▼                                           │
│   ┌──────────────────────────────────────────────────────────────────┐    │
│   │                         Redis Cluster                             │    │
│   │                                                                   │    │
│   │   ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │    │
│   │   │  Match Queue    │  │  Room Registry  │  │  Match State    │  │    │
│   │   │                 │  │                 │  │                 │  │    │
│   │   │  • 等待队列     │  │  • RoomServer   │  │  • 进行中匹配   │  │    │
│   │   │  • ELO 分段     │  │  • 心跳状态     │  │  • 匹配结果     │  │    │
│   │   │  • 玩家信息     │  │  • 负载信息     │  │  • 超时检测     │  │    │
│   │   └─────────────────┘  └─────────────────┘  └─────────────────┘  │    │
│   │                                                                   │    │
│   └──────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└────────────────────────────────────────────────────────────────────────────┘
```

#### 核心设计

**1. 无状态化**

```go
// 匹配服不保存任何本地状态
type MatchServer struct {
    redis    *redis.Client    // 所有状态存 Redis
    id       string           // 实例 ID (用于日志追踪)
}

// 匹配请求 -> 计算 -> 结果写 Redis
func (m *MatchServer) Match(ctx context.Context, req *MatchRequest) (*MatchResult, error) {
    // 1. 加入 Redis 队列
    m.redis.LPush(ctx, "match:queue", req.PlayerID)

    // 2. 尝试匹配 (原子操作)
    result, err := m.tryMatch(ctx, req)

    // 3. 匹配成功，分配房间
    if result != nil {
        roomServer := m.pickRoomServer(ctx)
        result.RoomAddr = roomServer.KCPAddr
    }

    return result, nil
}
```

**2. Redis 分布式队列**

```
Redis 数据结构:

┌─────────────────────────────────────────────────────────────────┐
│ match:queue:{mode}:{elo_range}    → List (等待队列)              │
│                                  例: match:queue:rank:1500-1600 │
│                                                                  │
│ match:player:{player_id}          → Hash (玩家匹配信息)          │
│                                  {elo, mode, wait_time, ...}    │
│                                                                  │
│ room:registry                     → Hash (RoomServer 注册表)     │
│                                  {server_id → json_info}        │
│                                                                  │
│ room:heartbeat:{server_id}        → String (心跳 TTL)           │
│                                  5秒过期                        │
└─────────────────────────────────────────────────────────────────┘
```

**3. 分布式匹配算法**

```go
// 基于 Redis 的原子匹配
func (m *MatchServer) tryMatch(ctx context.Context, req *MatchRequest) (*MatchResult, error) {
    queueKey := fmt.Sprintf("match:queue:%s:%s", req.Mode, req.EloRange)

    // Lua 脚本保证原子性
    script := `
        local queue = KEYS[1]
        local player = ARGV[1]
        local teamSize = tonumber(ARGV[2])

        -- 获取队列中的玩家
        local candidates = redis.call('LRANGE', queue, 0, teamSize - 1)

        if #candidates >= teamSize - 1 then
            -- 匹配成功，移除这些玩家
            for i = 1, teamSize - 1 do
                redis.call('LREM', queue, 1, candidates[i])
            end
            return candidates
        end

        -- 加入队列等待
        redis.call('RPUSH', queue, player)
        return nil
    `

    result, _ := m.redis.Eval(ctx, script, []string{queueKey}, req.PlayerID, req.TeamSize).Result()
    // ...
}
```

**4. 心跳与故障检测**

```
RoomServer 心跳机制:

RoomServer                                Redis
    │                                       │
    ├── SETEX room:heartbeat:{id} 5 "1" ───►│  (5秒TTL)
    │                                       │
    │                  每 5 秒               │

MatchServer (任意实例)                    Redis
    │                                       │
    │◄── 定时扫描 room:heartbeat:* ─────────│
    │                                       │
    ├── 过期 → 标记 offline                 │
    └── 在线 → 更新负载信息                 │
```

#### 故障恢复

```
Match Server 故障:
┌─────────────────────────────────────────────────────────────────┐
│                                                                  │
│  Match #1 故障 ──► Gateway 自动重试到 Match #2/#3               │
│                                                                  │
│  Redis 中:                                                       │
│  • 匹配队列完好无损                                               │
│  • 进行中的匹配可通过 match:player:{id} 恢复                      │
│  • 超时未完成的匹配会被定时任务清理                                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘

RoomServer 故障:
┌─────────────────────────────────────────────────────────────────┐
│                                                                  │
│  心跳超时 ──► 从 room:registry 移除                              │
│           ──► 新匹配不分配到该实例                                │
│           ──► 通知受影响玩家重新匹配                              │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

#### 部署建议

| 组件 | 实例数 | 说明 |
|------|--------|------|
| Match Server | 3+ | 无状态，按并发量扩展 |
| Redis Cluster | 3+ | 奇数节点，哨兵模式或 Cluster |
| Room Server | N | 按在线玩家数动态扩展 |

## 部署架构

```
                              ┌──────────────┐
                              │   DNS/LB     │
                              │ (域名入口)    │
                              └──────┬───────┘
                                     │
              ┌──────────────────────┼──────────────────────┐
              ▼                      ▼                      ▼
        ┌──────────┐           ┌──────────┐           ┌──────────┐
        │ Gateway  │           │ Gateway  │           │ Gateway  │
        │   #1     │           │   #2     │           │   #3     │
        │ WS:8080  │           │ WS:8080  │           │ WS:8080  │
        └────┬─────┘           └────┬─────┘           └────┬─────┘
             │                      │                      │
             └──────────────────────┼──────────────────────┘
                                    │ gRPC
                                    │
         ┌──────────────────────────┼──────────────────────────┐
         │                          │                          │
         ▼                          ▼                          ▼
   ┌───────────┐             ┌───────────┐             ┌───────────┐
   │   Logic   │             │   Match   │             │   Redis   │
   │  Server   │◄───────────►│  Server   │◄───────────►│  (共享)   │
   │ gRPC:9001 │             │ gRPC:9002 │             │           │
   └───────────┘             └─────┬─────┘             └───────────┘
                                   │
                                   │ 注册/心跳
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
              ▼                    ▼                    ▼
        ┌──────────┐         ┌──────────┐         ┌──────────┐
        │  Room    │         │  Room    │         │  Room    │
        │ Server#1 │         │ Server#2 │         │ Server#3 │
        │          │         │          │         │          │
        │ KCP:8888 │         │ KCP:8888 │         │ KCP:8888 │
        │ gRPC:9000│         │ gRPC:9000│         │ gRPC:9000│
        │          │         │          │         │          │
        │ Rooms:45 │         │ Rooms:32 │         │ Rooms:58 │
        └────┬─────┘         └────┬─────┘         └────┬─────┘
             │                    │                    │
             ▲                    ▲                    ▲
             │                    │                    │
             └────────────────────┼────────────────────┘
                                  │
                            KCP 直连
                                  │
                            ┌─────┴─────┐
                            │  Clients  │
                            └───────────┘
```

**说明**:
- Gateway: 无状态，可任意扩展，Session 存 Redis
- Logic/Match: 按需扩展，数据存 Redis/DB
- Room Server: 根据在线人数横向扩展，匹配服自动负载均衡

## 后续扩展

| 功能 | 优先级 | 说明 |
|------|--------|------|
| Redis Session 共享 | P0 | Gateway 无状态化 |
| 服务注册发现 (Consul/ETCD) | P1 | 动态扩缩容 |
| 配置中心 | P1 | 统一配置管理 |
| 监控告警 (Prometheus) | P1 | 性能指标 |
| 日志聚合 (ELK) | P2 | 日志分析 |
| 断线重连 | P1 | 游戏体验 |
| 录像回放 | P2 | 存储帧数据 |
