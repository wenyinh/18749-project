# Milestone 2 快速启动指南

## ✅ 已完成的实现

所有 Milestone 2 功能已成功实现并通过编译！

---

## 🚀 一键启动（推荐）

```bash
# 1. 启动完整演示
make run-milestone2

# 2. 在新终端查看 GFD 日志
tail -f logs/gfd.log

# 3. 在新终端查看客户端日志
tail -f logs/client1.log

# 4. 测试故障容错（Kill 服务器）
kill $(cat run/server1.pid)

# 5. 停止演示
make stop-milestone2
```

---

## 📋 各组件单独启动命令

### 1. GFD (Global Fault Detector)

```bash
./bin/gfd -addr :8000
```

**作用：** 维护全局成员列表，显示 "GFD: N members: ..."

---

### 2. 服务器副本 (3 个)

```bash
# Server S1
./bin/server -addr :9001 -rid S1 -init_state 0 > logs/s1.log 2>&1 &

# Server S2
./bin/server -addr :9002 -rid S2 -init_state 0 > logs/s2.log 2>&1 &

# Server S3
./bin/server -addr :9003 -rid S3 -init_state 0 > logs/s3.log 2>&1 &
```

**作用：** 处理客户端请求，维护状态计数器

---

### 3. LFD (Local Fault Detector) - 每个服务器一个

```bash
# LFD1 监控 S1
./bin/lfd \
    -target 127.0.0.1:9001 \
    -id S1 \
    -gfd 127.0.0.1:8000 \
    -hb 1s \
    -timeout 3s \
    -max-retries 5 \
    -base-delay 1s \
    -max-delay 30s \
    > logs/lfd1.log 2>&1 &

# LFD2 监控 S2
./bin/lfd \
    -target 127.0.0.1:9002 \
    -id S2 \
    -gfd 127.0.0.1:8000 \
    -hb 1s \
    -timeout 3s \
    > logs/lfd2.log 2>&1 &

# LFD3 监控 S3
./bin/lfd \
    -target 127.0.0.1:9003 \
    -id S3 \
    -gfd 127.0.0.1:8000 \
    -hb 1s \
    -timeout 3s \
    > logs/lfd3.log 2>&1 &
```

**作用：** 发送心跳，检测故障，支持指数退避重连

---

### 4. 客户端 (3 个，自动模式)

```bash
# Client C1
./bin/client \
    -id C1 \
    -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" \
    -interval 2s \
    -auto \
    > logs/c1.log 2>&1 &

# Client C2
./bin/client \
    -id C2 \
    -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" \
    -interval 2s \
    -auto \
    > logs/c2.log 2>&1 &

# Client C3
./bin/client \
    -id C3 \
    -servers "S1=127.0.0.1:9001,S2=127.0.0.1:9002,S3=127.0.0.1:9003" \
    -interval 2s \
    -auto \
    > logs/c3.log 2>&1 &
```

**作用：**
- 连接所有 3 个副本
- 发送相同请求到所有副本
- 检测并丢弃重复响应
- 断连时请求排队，重连后发送

---

## 🧪 故障容错测试

### 测试 1: 单故障

```bash
# 启动系统
make run-milestone2

# 等待 5 秒让系统稳定
sleep 5

# Kill S1
kill $(cat run/server1.pid)

# 观察 LFD1 重连尝试
tail -f logs/lfd1.log

# 观察 GFD 成员变更
tail -f logs/gfd.log
# 应该看到: GFD: 2 members: S2, S3

# 观察客户端继续工作
tail -f logs/client1.log
# 应该看到继续收到 S2 和 S3 的响应
```

### 测试 2: 双故障

```bash
# 在测试 1 基础上，Kill S2
kill $(cat run/server2.pid)

# 观察 GFD
# 应该看到: GFD: 1 members: S3

# 客户端继续使用 S3
```

### 测试 3: 请求队列

```bash
# Kill S1
kill $(cat run/server1.pid)

# 客户端继续发送，S1 的请求被排队
# 查看客户端日志
tail -f logs/client1.log
# 应该看到: "Connection down, queued request_num=X"

# 重启 S1
./bin/server -addr :9001 -rid S1 -init_state 0 > logs/s1_new.log 2>&1 &

# 观察客户端重连并刷新队列
# 应该看到: "Reconnected successfully, flushing N queued requests"
```

---

## 📊 预期输出示例

### GFD 输出
```
GFD: 0 members
[GFD] listening on :8000
[GFD] added replica S1
GFD: 1 members: S1
[GFD] added replica S2
GFD: 2 members: S1, S2
[GFD] added replica S3
GFD: 3 members: S1, S2, S3
```

### LFD 输出（正常）
```
[LFD][S1] connecting to GFD at 127.0.0.1:8000 ...
[LFD][S1] connected to GFD
[LFD][S1] connected to 127.0.0.1:9001
[LFD][S1] [heartbeat_count=1] LFD->S send heartbeat: 'PING'
[LFD][S1] [heartbeat_count=1] S->LFD recv heartbeat reply: 'PONG'
[LFD][S1] sent ADD S1 to GFD
```

### LFD 输出（故障检测）
```
[LFD][S1] [heartbeat_count=5] HEARTBEAT RECV FAILED from 127.0.0.1:9001: EOF
[LFD][S1] Retry 1/5: reconnecting in 1s...
[LFD][S1] Retry 2/5: reconnecting in 2s...
[LFD][S1] Retry 3/5: reconnecting in 4s...
[LFD][S1] Failed to connect after 5 attempts
[LFD][S1] sent DELETE S1 to GFD
SERVER 127.0.0.1:9001 DOWN
```

### Client 输出（正常）
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
```

### Client 输出（故障恢复）
```
[C1→S1] Error sending request: write tcp: broken pipe
[C1→S1] Connection down, queued request_num=5 (queue size: 1)
[C1→S1] Reconnecting in 1s (attempt 1/5)...
[C1→S1] Reconnected successfully, flushing 3 queued requests
[C1→S1] Sending queued request_num=5 (queued for 5.234s)
```

### Server 输出
```
[SERVER][S1] listening on :9001
[SERVER][S1] connected to 127.0.0.1:xxxxx
[SERVER][S1] received JSON request from client, clientId: C1, request_num: 1, Message: Auto request 1 from C1
[SERVER][S1] server state before: 0
[SERVER][S1] server state after: 1
[SERVER][S1] sent JSON reply to client, clientId: C1, request_num: 1, server state: 1
```

---

## 🔧 常用命令

```bash
# 构建所有组件
make build

# 清理构建产物和日志
make clean

# 查看帮助
make help

# 单独运行组件（使用 .env 配置）
make run-gfd
make run-server ARGS="-addr :9001 -rid S1 -init_state 0"
make run-lfd ARGS="-target :9001 -id S1 -gfd :8000"
make run-client ARGS="-id C1 -servers 'S1=:9001,S2=:9002,S3=:9003' -auto"
```

---

## 📁 重要文件

- **MILESTONE2_README.md**: 详细技术文档和架构说明
- **QUICK_START.md**: 本文件，快速启动指南
- **.env**: 环境变量配置
- **scripts/run_milestone2.sh**: 自动启动脚本
- **scripts/stop_milestone2.sh**: 自动停止脚本
- **logs/**: 所有组件的日志文件

---

## ✨ 实现的关键特性

✅ **GFD**: 全局故障检测器，维护成员列表
✅ **LFD 指数退避**: 心跳失败后自动重连（1s → 2s → 4s → 8s → 16s）
✅ **客户端主动复制**: 同时连接 3 个副本
✅ **重复检测**: 接收第一个回复，丢弃后续重复
✅ **请求队列**: 断连时排队，重连后自动发送
✅ **容错**: 支持 1-2 个服务器故障
✅ **详细日志**: 所有操作清晰可见

---

## 🎯 下一步

1. 启动系统：`make run-milestone2`
2. 查看日志验证功能正常
3. Kill 服务器测试故障容错
4. 阅读 MILESTONE2_README.md 了解技术细节

**祝你测试顺利！** 🎉
