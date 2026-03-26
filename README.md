# LockStep Server

基于 Go + KCP 的帧同步游戏服务器

## 特性

- **KCP 协议** - 基于 UDP 的低延迟可靠传输，比 TCP 延迟降低 30-40%
- **帧同步** - 确定性游戏逻辑同步，支持多玩家实时对战
- **房间管理** - 创建/加入/自动匹配房间
- **Protobuf** - 高效二进制消息序列化
- **高并发** - 基于 Goroutine 的轻量级并发模型

## 技术选型

### 为什么选择 Go + KCP？

#### Go 语言优势

| 特性 | 说明 |
|------|------|
| **Goroutine** | 轻量级协程，一个房间一个协程，内存占用极低 (~2KB/协程) |
| **Channel** | 天然适合生产者-消费者模式，如 Session 的异步发送 |
| **GC 停顿短** | 现代 Go 的 GC 停顿在微秒级，不影响帧率稳定性 |
| **开发效率** | 语法简洁，标准库丰富，从原型到生产周期短 |
| **部署简单** | 编译成单二进制，无需运行时环境 |

#### KCP 协议优势

| 对比 | TCP | KCP | UDP |
|------|-----|-----|-----|
| **延迟** | 高 (拥塞控制激进) | 低 (可配置) | 最低 |
| **可靠性** | ✓ | ✓ | ✗ |
| **丢包恢复** | 慢 (等待 RTO) | 快 (选择性重传) | 无 |
| **顺序保证** | ✓ | ✓ | ✗ |
| **带宽成本** | 低 | 中 (+10-20%) | 最低 |

**KCP 核心优势**：牺牲少量带宽，换取比 TCP 低 30-40% 的传输延迟。

#### 组合起来的效果

```
┌─────────────────────────────────────────────────────────┐
│                    Go + KCP 组合                         │
├─────────────────────────────────────────────────────────┤
│                                                          │
│   Go Goroutine        每个房间独立协程，互不阻塞          │
│        +                                                │
│   KCP 低延迟          玩家输入快速到达，帧同步更流畅      │
│        +                                                │
│   Protobuf            消息体小，序列化快                 │
│        =                                                │
│   高性能帧同步服务器  支持 5000+ 并发，延迟 < 50ms       │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### 与其他方案对比

| 方案 | 开发效率 | 性能 | 延迟稳定性 | 适合场景 |
|------|---------|------|-----------|---------|
| **Go + KCP** | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 实时对战，平衡首选 |
| Rust + KCP | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 极致性能，确定性物理 |
| TypeScript + WebSocket | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | Web 客户端，快速原型 |
| Java + TCP | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ | 企业团队，生态复用 |
| C++ + KCP | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | 传统游戏服务器 |

### 为什么不用 WebSocket？

| 场景 | WebSocket | KCP |
|------|-----------|-----|
| Web 浏览器 | ✓ 原生支持 | ✗ 需要降级 |
| 游戏客户端 | 延迟较高 (~100ms) | 延迟较低 (~50ms) |
| 弱网环境 | 表现一般 | 可调优参数适应 |
| 带宽占用 | 较大 (HTTP 头) | 较小 |

**结论**：纯游戏场景选 KCP，需要 Web 支持则双协议。

### 为什么不用 Rust？

| 对比 | Go | Rust |
|------|-----|------|
| 学习曲线 | 低，1-2 周上手 | 高，1-3 月熟练 |
| 开发速度 | 快 | 慢 (与编译器斗争) |
| 性能 | 足够好 | 最优 |
| 确定性 | 依赖实现 | 编译期保证 |

**结论**：Go 性能足够，开发效率更高，适合中小团队快速迭代。

## 项目结构

```
lockStep/
├── cmd/
│   └── server/
│       └── main.go           # 服务入口
├── internal/
│   ├── server/
│   │   └── server.go         # KCP 服务器、连接管理
│   ├── handler/
│   │   └── handler.go        # 消息解析和路由
│   ├── session/
│   │   ├── session.go        # 玩家会话
│   │   └── manager.go        # 会话管理器
│   ├── room/
│   │   ├── room.go           # 房间逻辑
│   │   └── manager.go        # 房间管理器
│   ├── frame/
│   │   └── manager.go        # 帧同步管理
│   ├── input/
│   │   └── buffer.go         # 输入缓冲区
│   └── player/
│       └── player.go         # 玩家实体
├── pkg/
│   └── protocol/
│       ├── message.proto     # Protobuf 消息定义
│       └── message.pb.go     # 生成的 Go 代码
├── config/
│   └── config.yaml           # 配置文件
├── go.mod
├── go.sum
└── README.md
```

## 快速开始

### 环境要求

- Go 1.21+
- Protoc 3.0+ (可选，用于重新生成 protobuf)

### 安装依赖

```bash
go mod download
```

### 运行服务器

```bash
go run cmd/server/main.go
```

输出：
```
2024-01-01T10:00:00.000+0800	INFO	KCP server started	{"addr": "0.0.0.0:8888", "frame_rate": 60, "max_rooms": 1000}
```

### 构建

```bash
go build -o lockstep-server cmd/server/main.go
```

## 配置说明

编辑 `config/config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8888

kcp:
  # KCP 快速模式配置 (适合动作/格斗游戏)
  nodelay: 1      # 0: 关闭, 1: 开启
  interval: 10    # 内部更新间隔 (ms)
  resend: 2       # 快速重传阈值
  nc: 1           # 1: 关闭拥塞控制
  sndwnd: 1024    # 发送窗口大小
  rcvwnd: 1024    # 接收窗口大小

game:
  frame_rate: 60              # 帧率 (fps)
  frame_time: 16              # 每帧时间 (ms)
  max_rooms: 1000             # 最大房间数
  max_players_per_room: 8     # 每房间最大玩家数
  input_timeout_frames: 30    # 输入超时 (帧数)

log:
  level: "debug"              # debug/info/warn/error
  format: "console"           # console/json
```

### KCP 模式说明

| 模式 | nodelay | interval | resend | nc | 适用场景 |
|------|---------|----------|--------|-----|---------|
| 普通 | 0 | 40 | 0 | 0 | 回合制、低频操作 |
| 快速 | 1 | 10 | 2 | 1 | 动作、格斗、MOBA |
| 极速 | 1 | 10 | 1 | 1 | 极低延迟要求 |

## 消息协议

### 消息格式

所有消息使用 Protobuf 序列化，结构如下：

```protobuf
// 客户端 -> 服务器
message ClientMessage {
  oneof payload {
    LoginRequest login = 1;
    JoinRoomRequest join_room = 2;
    PlayerInput input = 3;
    Heartbeat heartbeat = 4;
  }
}

// 服务器 -> 客户端
message ServerMessage {
  oneof payload {
    LoginResponse login = 1;
    JoinRoomResponse join_room = 2;
    FrameData frame = 3;
    RoomState room_state = 4;
    ErrorResponse error = 5;
    Heartbeat heartbeat = 6;
  }
}
```

### 客户端 -> 服务器

| 消息 | 字段 | 说明 |
|------|------|------|
| **LoginRequest** | player_id, token | 登录请求 |
| **JoinRoomRequest** | room_id (可选) | 加入指定房间，空则自动匹配 |
| **PlayerInput** | frame, player_index, input_data | 玩家输入数据 |
| **Heartbeat** | timestamp | 心跳保活 |

### 服务器 -> 客户端

| 消息 | 字段 | 说明 |
|------|------|------|
| **LoginResponse** | success, session_id, message | 登录结果 |
| **JoinRoomResponse** | success, room_id, player_index, players[] | 加入房间结果 |
| **FrameData** | frame, inputs[] | 帧数据广播 (所有玩家输入) |
| **RoomState** | state, current_frame | 房间状态变化 |
| **ErrorResponse** | code, message | 错误信息 |

### 错误码

| Code | 说明 |
|------|------|
| 1001 | 未登录 |
| 1002 | 加入房间失败 |
| 1003 | 不在房间中 |
| 1004 | 房间不存在 |

## 通信流程

```
┌──────────┐                    ┌──────────┐
│ Client A │                    │  Server  │
└────┬─────┘                    └────┬─────┘
     │                               │
     │──── LoginRequest ────────────▶│
     │◀─── LoginResponse ───────────│
     │                               │
     │──── JoinRoomRequest ────────▶│
     │◀─── JoinRoomResponse ───────│
     │                               │
     │         (等待其他玩家)         │
     │                               │
     │◀─────── FrameData ───────────│ ◀─ 帧同步开始
     │──── PlayerInput ────────────▶│
     │◀─────── FrameData ───────────│
     │──── PlayerInput ────────────▶│
     │◀─────── FrameData ───────────│
     │            ...                │
```

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                        Game Server                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌─────────────┐                                            │
│  │ KCP Listener│  ◀──── UDP:8888 (KCP Protocol)            │
│  └──────┬──────┘                                            │
│         │                                                    │
│         ▼                                                    │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐   │
│  │   Session   │────▶│   Handler   │────▶│    Room     │   │
│  │   Manager   │     │   (Router)  │     │   Manager   │   │
│  └─────────────┘     └─────────────┘     └──────┬──────┘   │
│                                                 │           │
│         ┌───────────────────────────────────────┘           │
│         ▼                                                    │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐   │
│  │    Room     │────▶│   Frame     │────▶│   Input     │   │
│  │  (State)    │     │   Manager   │     │   Buffer    │   │
│  └─────────────┘     └─────────────┘     └─────────────┘   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## 核心模块

### Server

负责 KCP 连接的监听和生命周期管理。

```go
s, _ := server.New("config/config.yaml")
s.Start()  // 阻塞运行
```

### Session

管理单个客户端连接，采用**异步发送**模式。

```go
type Session struct {
    ID       string              // 会话唯一标识
    Player   *player.Player      // 关联的玩家
    RoomID   string              // 当前房间ID
    conn     *kcp.UDPSession     // KCP 连接
    sendCh   chan []byte         // 发送缓冲 (256条)
    closeCh  chan struct{}       // 关闭信号
}
```

**工作流程**：
```
主协程 ──Send()──▶ sendCh ──▶ writeLoop ──Write()──▶ 网络
   │                                           │
   └── 非阻塞，继续处理其他逻辑 ──────────────────┘
```

**关键方法**：
- `Start()` - 启动 writeLoop 协程
- `Send(data)` - 非阻塞发送，缓冲区满则丢弃
- `Close()` - 安全关闭，只执行一次

### Room Manager

管理所有游戏房间，支持创建、加入、自动匹配。

```go
// 加入指定房间
room, index, _ := roomMgr.JoinRoom("abc123", player)

// 自动匹配或创建
room, index, _ := roomMgr.JoinOrCreate(player)
```

### Frame Manager

控制帧同步的节奏，收集输入并广播。

```go
fm := frame.NewManager(60, 30, logger)  // 60fps, 30帧超时
fm.Start(room, func(frame uint32, inputs map[int][]byte) {
    // 广播帧数据给所有玩家
})
```

## 开发指南

### 添加新消息类型

1. 编辑 `pkg/protocol/message.proto`
2. 运行 protoc 生成代码：

```bash
protoc --go_out=. pkg/protocol/message.proto
```

3. 在 `internal/handler/handler.go` 中添加处理逻辑

### 自定义帧同步逻辑

修改 `internal/frame/manager.go` 中的 `Start` 方法。

## 客户端接入

### Go 客户端示例

```go
package main

import (
    "log"
    "time"

    "github.com/xtaci/kcp-go/v5"
    "google.golang.org/protobuf/proto"
)

func main() {
    // 1. 连接服务器
    conn, err := kcp.DialWithOptions("127.0.0.1:8888", nil, 10, 3)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // 配置 KCP
    conn.SetNoDelay(1, 10, 2, 1)

    // 2. 登录
    loginReq := &ClientMessage{
        Payload: &ClientMessage_Login{
            Login: &LoginRequest{
                PlayerId: "player001",
                Token:    "test-token",
            },
        },
    }
    sendData, _ := proto.Marshal(loginReq)
    conn.Write(sendData)

    // 3. 接收响应
    buf := make([]byte, 4096)
    n, _ := conn.Read(buf)
    var resp ServerMessage
    proto.Unmarshal(buf[:n], &resp)
    log.Printf("Login response: %+v", resp.GetLogin())

    // 4. 加入房间
    joinReq := &ClientMessage{
        Payload: &ClientMessage_JoinRoom{
            JoinRoom: &JoinRoomRequest{},
        },
    }
    sendData, _ = proto.Marshal(joinReq)
    conn.Write(sendData)

    // 5. 游戏循环：发送输入，接收帧数据
    for i := 0; i < 100; i++ {
        // 发送输入
        input := &ClientMessage{
            Payload: &ClientMessage_Input{
                Input: &PlayerInput{
                    Frame:       uint32(i),
                    PlayerIndex: 0,
                    InputData:   []byte{0x01, 0x02}, // 自定义输入数据
                },
            },
        }
        sendData, _ = proto.Marshal(input)
        conn.Write(sendData)

        // 接收帧数据
        n, _ := conn.Read(buf)
        var frameData ServerMessage
        proto.Unmarshal(buf[:n], &frameData)
        if f := frameData.GetFrame(); f != nil {
            log.Printf("Frame %d: %d inputs", f.Frame, len(f.Inputs))
        }

        time.Sleep(16 * time.Millisecond) // ~60fps
    }
}
```

### Unity 客户端

推荐使用 [KCP for Unity](https://github.com/RevenantX/LiteNetLib) 或自行封装 KCP。

**消息序列化**：
- Protobuf: 使用 [protobuf-unity](https://github.com/CloudCityIdeas/protobuf-unity)
- 或使用 FlatBuffers、MessagePack 等替代

## 性能优化建议

1. **KCP 参数调优** - 根据游戏类型调整 nodelay/interval
2. **输入压缩** - 对输入数据进行位压缩
3. **帧缓冲** - 客户端预渲染，隐藏网络抖动
4. **乐观帧同步** - 客户端本地预测，服务端校验回滚

## 部署

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o lockstep-server cmd/server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/lockstep-server .
COPY config/config.yaml ./config/
EXPOSE 8888/udp
CMD ["./lockstep-server"]
```

```bash
docker build -t lockstep-server .
docker run -d -p 8888:8888/udp lockstep-server
```

### Docker Compose

```yaml
version: '3'
services:
  lockstep:
    build: .
    ports:
      - "8888:8888/udp"
    volumes:
      - ./config:/app/config
    restart: unless-stopped
```

### 生产环境建议

| 配置项 | 建议值 |
|--------|--------|
| 日志级别 | info 或 warn |
| 最大房间数 | 根据内存调整 (约 1MB/房间) |
| KCP 模式 | 快速模式 (nodelay=1) |
| 监控 | 接入 Prometheus |

## 常见问题

### Q: UDP 被防火墙拦截怎么办？

A: 两种方案：
1. 同时监听 TCP/WebSocket 作为降级通道
2. 使用 STUN/TURN 进行 NAT 穿透

### Q: 如何处理断线重连？

A: 当前版本会丢失房间状态。计划中功能：
- 保存玩家最后 N 帧输入
- 重连后同步房间当前状态
- 客户端回放追赶

### Q: 如何防止作弊？

A: 建议方案：
- 服务端运行一份游戏逻辑，校验结果
- 输入数据签名验证
- 异常输入检测

### Q: 支持多少人同时在线？

A: 单服务器理论值：
- 4 核 8G: ~5000 并发连接，~500 房间
- 实际取决于：帧率、房间人数、输入大小

## 状态

| 功能 | 状态 |
|------|------|
| KCP 服务器 | ✅ 已完成 |
| 消息解析路由 | ✅ 已完成 |
| 登录流程 | ✅ 已完成 |
| 房间管理 | ✅ 已完成 |
| 帧同步广播 | ✅ 已完成 |
| 断线重连 | 📝 计划中 |
| WebSocket 降级 | 📝 计划中 |
| 输入预测回滚 | 📝 计划中 |
| 监控指标 | 📝 计划中 |
| 测试用例 | 📝 计划中 |

## License

MIT
