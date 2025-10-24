# Milestone 2: Active Replication with Fault Tolerance

## 概述

Milestone 2 实现了一个完整的主动复制（Active Replication）系统，具备以下特性：

1. **GFD (Global Fault Detector)**: 维护全局成员列表，跟踪所有活跃的服务器副本
2. **LFD (Local Fault Detector)**: 每个服务器配备一个 LFD，执行心跳检测，支持指数退避重连
3. **服务器副本**: 3 个服务器副本 (S1, S2, S3)，每个副本独立处理客户端请求
4. **客户端**: 3 个客户端 (C1, C2, C3)，每个客户端连接所有 3 个副本，支持：
   - 请求重复检测和去重
   - 本地请求队列（断连时排队，重连后发送）
   - 指数退避重连机制

---

## 快速启动

### 1. 构建所有组件

```bash
make build
```

这将构建以下二进制文件：
- `bin/gfd` - Global Fault Detector
- `bin/server` - Server 副本
- `bin/lfd` - Local Fault Detector
- `bin/client` - Client

### 2. 配置环境变量

```bash
cp .env.example .env
```

根据需要编辑 `.env` 文件中的配置。

### 3. 启动完整演示

```bash
make run-milestone2
```

这将按顺序启动：
1. GFD (端口 8000)
2. 3 个 Server 副本 (端口 9001, 9002, 9003)
3. 3 个 LFD (监控各自的服务器)
4. 3 个 Client (自动模式，持续发送请求)

### 4. 查看日志

在不同的终端窗口中查看实时日志：

```bash
# GFD 日志 (查看成员变更)
tail -f logs/gfd.log

# Server 日志
tail -f logs/server1.log
tail -f logs/server2.log
tail -f logs/server3.log

# LFD 日志 (查看心跳和故障检测)
tail -f logs/lfd1.log
tail -f logs/lfd2.log
tail -f logs/lfd3.log

# Client 日志 (查看请求、响应和重复检测)
tail -f logs/client1.log
tail -f logs/client2.log
tail -f logs/client3.log
```

### 5. 测试故障容错

#### 测试单个故障：

```bash
# 查找 S1 的 PID
cat run/server1.pid

# Kill S1
kill $(cat run/server1.pid)
```

**预期行为：**
1. LFD1 检测到心跳失败
2. LFD1 尝试指数退避重连（最多 5 次）
3. 重连失败后，LFD1 发送 `DELETE S1` 到 GFD
4. GFD 更新成员列表：`GFD: 2 members: S2, S3`
5. 客户端继续使用 S2 和 S3，不中断服务

#### 测试双故障容错：

```bash
# 等待一段时间后，kill S2
kill $(cat run/server2.pid)
```

**预期行为：**
1. LFD2 检测并报告故障
2. GFD 更新为：`GFD: 1 members: S3`
3. 客户端继续使用 S3

### 6. 停止演示

```bash
make stop-milestone2
```

---

## 组件详细说明

### GFD (Global Fault Detector)

**功能：**
- 维护全局成员列表 `membership[]`
- 处理来自 LFD 的 `ADD` 和 `DELETE` 消息
- 打印成员变更：`GFD: N members: S1, S2, S3`

**启动命令：**

```bash
# 使用默认配置
./bin/gfd -addr :8000

# 使用 Make
make run-gfd ARGS="-addr :8000"
```

**协议：**
- LFD → GFD: `ADD S1\n` (首次心跳成功)
- LFD → GFD: `DELETE S1\n` (心跳失败且重试耗尽)

**日志示例：**
```
[GFD] listening on :8000
GFD: 0 members
[GFD] added replica S1
GFD: 1 members: S1
[GFD] added replica S2
GFD: 2 members: S1, S2
[GFD] deleted replica S1
GFD: 1 members: S2
```

---

### Server (服务器副本)

**功能：**
- 处理客户端 JSON 请求
- 维护独立的状态计数器（每个请求递增）
- 响应 LFD 心跳 (PING/PONG)

**启动命令：**

```bash
# S1
./bin/server -addr :9001 -rid S1 -init_state 0

# S2
./bin/server -addr :9002 -rid S2 -init_state 0

# S3
./bin/server -addr :9003 -rid S3 -init_state 0
```

**请求/响应格式：**

```json
// 请求
{
  "type": "REQ",
  "client_id": "C1",
  "request_num": 5,
  "message": "Hello"
}

// 响应
{
  "type": "RESP",
  "server_id": "S1",
  "client_id": "C1",
  "request_num": 5,
  "server_state": 10,
  "message": "Hello"
}
```

**日志示例：**
```
[SERVER][S1] received JSON request from client, clientId: C1, request_num: 5, Message: Hello
[SERVER][S1] server state before: 9
[SERVER][S1] server state after: 10
[SERVER][S1] sent JSON reply to client, clientId: C1, request_num: 5, server state: 10
```

---

### LFD (Local Fault Detector)

**功能：**
- 定期发送心跳到服务器 (PING/PONG)
- 首次成功心跳后，发送 `ADD` 到 GFD
- 心跳失败时，尝试指数退避重连
- 重连失败后，发送 `DELETE` 到 GFD 并退出

**启动命令：**

```bash
# LFD1 (监控 S1)
./bin/lfd \
    -target 127.0.0.1:9001 \
    -id S1 \
    -gfd 127.0.0.1:8000 \
    -hb 1s \
    -timeout 3s \
    -max-retries 5 \
    -base-delay 1s \
    -max-delay 30s

# LFD2 (监控 S2)
./bin/lfd -target 127.0.0.1:9002 -id S2 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s

# LFD3 (监控 S3)
./bin/lfd -target 127.0.0.1:9003 -id S3 -gfd 127.0.0.1:8000 -hb 1s -timeout 3s
```

**参数说明：**
- `-target`: 监控的服务器地址
- `-id`: 副本 ID (与服务器的 `-rid` 对应)
- `-gfd`: GFD 地址
- `-hb`: 心跳间隔 (默认 1s)
- `-timeout`: 心跳超时 (默认 3s)
- `-max-retries`: 最大重连次数 (默认 5)
- `-base-delay`: 基础退避延迟 (默认 1s)
- `-max-delay`: 最大退避延迟 (默认 30s)

**指数退避示例：**
```
Retry 1/5: reconnecting in 1s...
Retry 2/5: reconnecting in 2s...
Retry 3/5: reconnecting in 4s...
Retry 4/5: reconnecting in 8s...
Retry 5/5: reconnecting in 16s...
Failed to connect after 5 attempts
```

**日志示例：**
```
[LFD][S1] connecting to GFD at 127.0.0.1:8000 ...
[LFD][S1] connected to GFD
[LFD][S1] connecting to 127.0.0.1:9001 ...
[LFD][S1] connected to 127.0.0.1:9001
[LFD][S1] [heartbeat_count=1] LFD->S send heartbeat: 'PING'
[LFD][S1] [heartbeat_count=1] S->LFD recv heartbeat reply: 'PONG'
[LFD][S1] sent ADD S1 to GFD
[LFD][S1] [heartbeat_count=2] HEARTBEAT RECV FAILED from 127.0.0.1:9001: EOF
[LFD][S1] Retry 1/5: reconnecting in 1s...
[LFD][S1] sent DELETE S1 to GFD
SERVER 127.0.0.1:9001 DOWN
```

---

### Client (客户端)

**功能：**
- 同时连接 3 个服务器副本
- 发送相同请求到所有副本
- 检测并丢弃重复响应
- 本地请求队列（连接断开时）
- 指数退避重连机制

**启动命令：**

```bash
# C1 (自动模式，每 2 秒发送一次请求)
./bin/client \
    -id C1 \
    -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" \
    -interval 2s \
    -auto

# C2 (手动模式)
./bin/client \
    -id C2 \
    -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003"

# 手动模式下输入 "auto" 可切换到自动模式
```

**参数说明：**
- `-id`: 客户端 ID (例如 C1, C2, C3)
- `-servers`: 服务器地址列表 (格式: `ID1=addr1,ID2=addr2,...`)
- `-interval`: 自动模式下的请求间隔 (默认 2s)
- `-auto`: 启动时进入自动模式

**日志示例：**

```
[C1] Connecting to all replicas...
[C1→S1] Connected successfully
[C1→S2] Connected successfully
[C1→S3] Connected successfully

[C1] Sending request_num=1 to all replicas
[C1→S1] Sending request_num=1
[C1→S2] Sending request_num=1
[C1→S3] Sending request_num=1
[C1←S1] Received reply for request_num=1 (state=1)
[C1←S2] request_num 1: Discarded duplicate reply from S2
[C1←S3] request_num 1: Discarded duplicate reply from S3

# 当 S1 断开连接
[C1→S1] Error sending request: write tcp: broken pipe
[C1→S1] Connection down, queued request_num=5 (queue size: 1)
[C1→S1] Reconnecting in 1s (attempt 1/5)...

# 重连成功后
[C1→S1] Reconnected successfully, flushing 3 queued requests
[C1→S1] Sending queued request_num=5 (queued for 5.2s)
[C1→S1] Sending queued request_num=6 (queued for 3.1s)
```

---

## 演示场景

### 场景 1: 正常运行

1. 启动完整系统：`make run-milestone2`
2. 观察 GFD 显示：`GFD: 3 members: S1, S2, S3`
3. 观察客户端发送请求到所有 3 个副本
4. 观察重复检测：第一个回复被接收，其他两个被丢弃

### 场景 2: 单故障容错

1. Kill S1：`kill $(cat run/server1.pid)`
2. 观察 LFD1 尝试重连（5 次，指数退避）
3. 观察 LFD1 通知 GFD：`DELETE S1`
4. 观察 GFD 更新：`GFD: 2 members: S2, S3`
5. 观察客户端继续使用 S2 和 S3

### 场景 3: 双故障容错

1. 在场景 2 基础上，Kill S2
2. 观察 LFD2 行为
3. 观察 GFD 更新：`GFD: 1 members: S3`
4. 观察客户端继续使用 S3

### 场景 4: 请求队列机制

1. Kill S1
2. 在 LFD1 重连期间，观察客户端继续发送请求
3. 客户端将 S1 的请求加入队列
4. 手动重启 S1：`./bin/server -addr :9001 -rid S1 -init_state 0 > logs/server1_new.log 2>&1 &`
5. 观察客户端重连并刷新队列

---

## 故障检测时间线

```
时间轴：
0s    - Server S1 crash
1s    - LFD1 发送心跳失败，开始重连
1s    - 等待 1s (attempt 1)
2s    - 重连失败
2s    - 等待 2s (attempt 2)
4s    - 重连失败
4s    - 等待 4s (attempt 3)
8s    - 重连失败
8s    - 等待 8s (attempt 4)
16s   - 重连失败
16s   - 等待 16s (attempt 5)
32s   - 重连失败
32s   - LFD1 发送 DELETE S1 到 GFD
32s   - GFD 更新成员列表
```

**总检测延迟：** ~32 秒（可通过调整 `max-retries` 和 `base-delay` 缩短）

---

## 配置建议

### 生产环境

```bash
# LFD 配置（快速故障检测）
LFD_HB_FREQ="500ms"
LFD_TIMEOUT="1s"
LFD_MAX_RETRIES="3"
LFD_BASE_DELAY="500ms"
LFD_MAX_DELAY="5s"

# 客户端配置
CLIENT_INTERVAL="1s"
```

### 测试环境

```bash
# LFD 配置（允许更多重试）
LFD_HB_FREQ="1s"
LFD_TIMEOUT="3s"
LFD_MAX_RETRIES="10"
LFD_BASE_DELAY="1s"
LFD_MAX_DELAY="60s"

# 客户端配置
CLIENT_INTERVAL="3s"
```

---

## 常见问题

### Q1: 如何查看 GFD 的当前成员列表？

查看 GFD 日志：
```bash
tail logs/gfd.log | grep "GFD:"
```

### Q2: 客户端可以容忍几个故障？

理论上可以容忍 2 个故障（f=2, N=3）。但本实现中，至少需要 1 个副本可用。

### Q3: 如何调整故障检测时间？

修改 `.env` 中的以下参数：
- `LFD_MAX_RETRIES`: 减少重试次数
- `LFD_BASE_DELAY`: 减少基础延迟
- `LFD_HB_FREQ`: 增加心跳频率

### Q4: 请求队列满了怎么办？

默认队列大小为 100。满了之后会丢弃最老的请求（FIFO）。可在代码中调整 `maxQueueSize`。

### Q5: 如何单独启动各个组件？

参见上面的"组件详细说明"中的启动命令。

---

## 测试清单

- [ ] GFD 启动显示 "GFD: 0 members"
- [ ] 3 个 Server 启动成功
- [ ] 3 个 LFD 连接到 GFD 并注册服务器
- [ ] GFD 显示 "GFD: 3 members: S1, S2, S3"
- [ ] 3 个 Client 连接到所有服务器
- [ ] 客户端发送请求到所有 3 个副本
- [ ] 客户端正确检测和丢弃重复响应
- [ ] Kill S1 后，LFD1 尝试重连
- [ ] LFD1 重连失败后通知 GFD
- [ ] GFD 更新为 "GFD: 2 members: S2, S3"
- [ ] 客户端继续使用 S2 和 S3
- [ ] Kill S2 后，系统继续运行（只用 S3）
- [ ] 客户端请求队列正常工作

---

## 架构图

```
┌─────────────────────────────────────────────────────────┐
│                         GFD                              │
│                    (Port 8000)                          │
│              membership: [S1, S2, S3]                   │
└─────────────────────────────────────────────────────────┘
         ▲           ▲           ▲
         │ADD/DELETE │ADD/DELETE │ADD/DELETE
         │           │           │
    ┌────┴────┐ ┌────┴────┐ ┌────┴────┐
    │  LFD1   │ │  LFD2   │ │  LFD3   │
    │  (S1)   │ │  (S2)   │ │  (S3)   │
    └────┬────┘ └────┬────┘ └────┬────┘
         │PING/PONG  │PING/PONG  │PING/PONG
         ▼           ▼           ▼
    ┌────────┐  ┌────────┐  ┌────────┐
    │   S1   │  │   S2   │  │   S3   │
    │ :9001  │  │ :9002  │  │ :9003  │
    └────────┘  └────────┘  └────────┘
         ▲           ▲           ▲
         │           │           │
         │  REQ/RESP │  REQ/RESP │  REQ/RESP
         └───────────┴───────────┴───────────
                     │
            ┌────────┼────────┐
            │        │        │
        ┌───┴──┐ ┌───┴──┐ ┌───┴──┐
        │  C1  │ │  C2  │ │  C3  │
        └──────┘ └──────┘ └──────┘
```

---

## 技术细节

### 重复检测算法

客户端维护 `pendingReplies` map，记录已交付的请求号。

```go
// 第一个回复
if !pendingReplies[reqNum] {
    pendingReplies[reqNum] = true
    log("Received reply")
} else {
    log("Discarded duplicate")
}
```

### 请求队列机制

每个副本连接维护独立队列：

```go
type ReplicaConnection struct {
    Queue []QueuedRequest
}

// 连接断开时
queue.append(request)

// 重连成功后
for req in queue {
    send(req)
}
queue.clear()
```

### 指数退避算法

```go
delay = baseDelay * 2^attempt
if delay > maxDelay {
    delay = maxDelay
}
```

示例：baseDelay=1s, maxDelay=30s
- Attempt 0: 1s
- Attempt 1: 2s
- Attempt 2: 4s
- Attempt 3: 8s
- Attempt 4: 16s
- Attempt 5: 30s (capped)

---

## 性能考虑

1. **网络开销**: 客户端请求数 × 3（发送到 3 个副本）
2. **延迟**: 取决于最快响应的副本
3. **吞吐量**: 受单个副本限制（所有副本处理相同请求）
4. **队列大小**: 默认 100，根据内存和业务需求调整

---

## 下一步改进

- [ ] 实现 GFD 持久化（成员列表保存到磁盘）
- [ ] 支持动态副本添加/移除
- [ ] 客户端智能选择最快副本
- [ ] 实现请求去重（服务器端）
- [ ] 添加性能监控和指标
- [ ] 支持配置文件热重载

---

## 联系方式

Team 12: Vincent Guo, Wenyin He, Fan Bu, Qiaoan Shen, Lenghan Zhu

Course: CMU 18-749 Building Reliable Distributed Systems
