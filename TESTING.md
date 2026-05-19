# yuelaiengine-gateway 测试手册

本文档用于在本地完整验证网关当前能力，覆盖模块能力测试与综合并发测试。

## 1. 测试范围与当前约束

- 已覆盖：
  - 配置加载与启动
  - 路由匹配与方法校验
  - 健康检查聚合
  - 反向代理与路径重写
  - 负载均衡（service-a 轮询）
  - 限流插件（ratelimit）
  - 熔断插件（circuitbreaker）
  - 认证链路（auth/apikey/rbac + auth-service）
  - 管理面（`/admin/*`）动态治理接口
  - 指标暴露（`/metrics`，Prometheus exposition）
  - 结构化监控接口（`/admin/metrics/summary`）
  - Swagger 文档（`/swagger/index.html` + `/swagger/openapi.json`）
  - 配置版本化与回滚
  - Web 控制台（`/web`）可视化配置与监控
  - 并发压测（ab）
  - JSON<->gRPC 端到端链路（gateway 转码 + gRPC 上游）
- 暂未覆盖：
  - 外部监控告警系统联动（Prometheus/Grafana/Alertmanager）

## 2. 环境准备

要求：

- Go `1.25.x`
- Python3（用于 `run.py`）
- Node.js 18+（用于构建 `web/dist`）
- npm
- `curl`
- `ab`（ApacheBench）
- macOS/Linux 终端

可选检查：

```bash
go version
python3 --version
which curl
which ab
node -v
npm -v
```

## 3. 启动与停止

在项目根目录执行：

```bash
python3 run.py
```

说明：

- 会启动 `api-gateway`、`auth-service(8085/8086)`、`service-a(8081/8082/8087)`、`service-b(8083/8084)`。
- `service-b` 已改造为 h2c gRPC 上游，并在启动时自动写出 descriptor：`./config/proto/service-b-echo.pb`。

如需验证 Web 控制台，先构建前端静态资源：

```bash
cd web
npm install
npm run build
cd ..
```

停止服务：

```bash
Ctrl + C
```

## 4. 基础可运行性测试

### 4.1 编译与单元测试

```bash
go test ./...
go build ./...
```

通过标准：

- `go test` 无失败
- `go build` 无编译错误

### 4.2 网关基础路由

```bash
curl -i http://127.0.0.1:9000/healthz
curl -i http://127.0.0.1:9000/not-found
curl -i -X POST http://127.0.0.1:9000/healthz
```

预期：

- `/healthz` 返回 `200`
- `/not-found` 返回 `404`
- `POST /healthz` 返回 `405`

## 5. 模块能力测试

### 5.1 代理转发 + 路径重写（service-a）

```bash
curl -i http://127.0.0.1:9000/service-a/hello
```

预期：

- 返回 `200`
- 响应体包含 `Hello from Service A`
- 路径被重写到后端（可在日志看到 `/service-a/hello -> /hello`）

### 5.2 负载均衡行为

```bash
for i in $(seq 1 20); do
  curl -s http://127.0.0.1:9000/service-a/hello | grep -o ':808[12]'
done | sort | uniq -c
```

预期：

- 能看到 `:8081` 与 `:8082` 都被命中（通常接近均衡）

### 5.3 限流能力（ratelimit）

```bash
seq 1 600 | xargs -I{} -P 120 sh -c \
"curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9000/service-a/rl-burst" \
| sort | uniq -c
```

预期：

- 同时出现 `200` 与 `429`
- `429` 代表限流生效

### 5.4 熔断能力（circuitbreaker）

先停止 service-a 实例（模拟故障）：

```bash
for p in 8081 8082 8087; do
  pid=$(lsof -ti tcp:$p || true)
  [ -n "$pid" ] && kill -TERM $pid
done
```

再发请求观察状态演化：

```bash
for i in $(seq 1 12); do
  code=$(curl -s -o /tmp/cb_resp.txt -w '%{http_code}' \
    http://127.0.0.1:9000/service-a/cb-check)
  body=$(tr '\n' ' ' </tmp/cb_resp.txt)
  echo "$i -> $code | $body"
  sleep 0.15
done
```

当前版本预期：

- 前几次可能是 `502`（代理到不可达实例）
- 达到失败阈值后，进入熔断打开状态
- 熔断打开后应返回 `503`（服务暂时不可用）

### 5.5 认证链路验证（auth/apikey/rbac）

```bash
# 1) 先登录拿 token
curl -s -X POST http://127.0.0.1:9000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}'

# 2) 携带 token + API Key 访问受保护路由
curl -i http://127.0.0.1:9000/secure/hello \
  -H 'Authorization: Bearer <TOKEN>' \
  -H 'X-API-Key: gateway-demo-key'
```

预期：

- 登录成功返回 token
- 访问 `/secure/*` 在 token/API Key/role 均满足时返回 `200`
- 缺少 token 返回 `401`，缺少或错误 API Key 返回 `401`，角色不匹配返回 `403`

### 5.6 管理面接口 + 在线变更 + 回滚

> 说明：如 `config.yml` 配置了 `admin.token`，需携带 `X-Admin-Token`。下文示例使用 `admin-secret`。

```bash
# 1) 查看配置版本
curl -s http://127.0.0.1:9000/admin/config/versions \
  -H 'X-Admin-Token: admin-secret'

# 2) 在线新增路由
curl -s -X POST http://127.0.0.1:9000/admin/routes/upsert \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{"route":{"path_prefix":"/dynamic","service_name":"service-a","plugins":[]}}'

# 3) 验证新路由生效
curl -i http://127.0.0.1:9000/dynamic/hello

# 4) 删除该路由
curl -s -X POST http://127.0.0.1:9000/admin/routes/delete \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{"path_prefix":"/dynamic"}'

# 5) 回滚到前一个版本（将 v3 替换为实际版本号）
curl -s -X POST http://127.0.0.1:9000/admin/config/rollback \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{"version":"v3"}'

# 6) 查看当前配置快照
curl -s http://127.0.0.1:9000/admin/config/current \
  -H 'X-Admin-Token: admin-secret'

# 7) 查看健康状态
curl -s http://127.0.0.1:9000/admin/health/status \
  -H 'X-Admin-Token: admin-secret'

# 8) 查看结构化指标
curl -s http://127.0.0.1:9000/admin/metrics/summary \
  -H 'X-Admin-Token: admin-secret'
```

预期：

- `routes/upsert` 之后可访问 `/dynamic/*`
- 删除后 `/dynamic/*` 返回 `404`
- 回滚到包含该路由的版本后恢复 `200`
- `config/current` 返回 `version + config`
- `health/status` 返回服务实例健康矩阵
- `metrics/summary` 返回 JSON 指标（无需解析 Prometheus 文本）
- 响应外层结构统一为 `status/message/data`
- 路由与限流字段使用 snake_case（如 `path_prefix`、`token_bucket.refill_rate`）

### 5.7 配置灰度发布前校验（dry run）

```bash
curl -s -X POST http://127.0.0.1:9000/admin/config/apply \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
 -d '{"dry_run":true,"source":"precheck","config":{"server":{"port":":9000"}}}'
```

预期：

- 上述示例会返回 `400`（故意提供不完整配置，用于验证校验器生效）
- 提交完整合法配置时返回 `200` 与“配置校验通过”

### 5.7.1 配置发布模式验证（仅内存 / 落盘）

`/admin/config/apply` 新增 `persist` 开关：

- `persist=false`：仅内存发布，立即生效，不改 `config/config.yml`
- `persist=true`：发布并落盘（默认写入网关配置路径，可通过 `persist_path` 指定）

示例（仅内存）：

```bash
curl -s -X POST http://127.0.0.1:9000/admin/config/apply \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{"dry_run":false,"persist":false,"source":"web:memory-only","config":<完整配置JSON>}'
```

示例（落盘）：

```bash
curl -s -X POST http://127.0.0.1:9000/admin/config/apply \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{"dry_run":false,"persist":true,"source":"web:persist","config":<完整配置JSON>}'
```

验证建议：

- 执行仅内存发布后，确认业务行为已变化；重启网关后若未落盘，行为应回到文件配置
- 执行落盘发布后，检查 `config/config.yml` 已更新，并在重启后仍保留新配置

### 5.8 灰度能力测试指南（Canary）

本项目灰度核心能力：

- 稳定分流：`traffic_weights + hash_on`（同一 key 命中固定服务）
- 强制分流：`ab_header + ab_variants`（按请求头值直接指定目标服务）

#### 5.8.1 前置确认

`config/config.yml` 默认已包含：

- `service-a`（8081/8082）
- `service-a-canary`（8087）
- `/service-a` 路由：`traffic_weights: {service-a:90, service-a-canary:10}`

先确认 canary 实例可用：

```bash
curl -i http://127.0.0.1:8087/healthz
```

预期：返回 `200`。

#### 5.8.2 稳定分流验证（同 key 粘性）

> 注意：默认 `hash_on: ip` 时，本地压测来源 IP 常相同，可能全部命中同一服务。建议临时改成 `hash_on: header:X-Gray-Key` 来验证稳定分流。

```bash
curl -s -X POST http://127.0.0.1:9000/admin/routes/upsert \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{
    "route": {
      "path_prefix": "/service-a",
      "service_name": "service-a",
      "hash_on": "header:X-Gray-Key",
      "traffic_weights": {
        "service-a": 90,
        "service-a-canary": 10
      },
      "plugins": [
        {"name":"ratelimit","rule":"service-a-path-limit","strategy":"path"},
        {"name":"circuitbreaker","service":"service-a"}
      ]
    }
  }'
```

使用同一个 key 连续请求，应固定命中同一后端端口：

```bash
for i in $(seq 1 10); do
  curl -s http://127.0.0.1:9000/service-a/hello -H 'X-Gray-Key: user-1001' \
  | grep -o ':808[127]'
done | sort | uniq -c
```

预期：

- 仅出现一个端口（例如全是 `:8081/:8082` 或全是 `:8087`）
- 说明同 key 稳定分流生效

#### 5.8.3 权重生效验证（多 key 采样）

用不同 key 采样，观察 canary 命中比例接近配置权重：

```bash
rm -f /tmp/gray_ports.txt
for i in $(seq 1 300); do
  key="user-$i"
  curl -s http://127.0.0.1:9000/service-a/hello -H "X-Gray-Key: $key" \
  | grep -o ':808[127]' >> /tmp/gray_ports.txt
done
sort /tmp/gray_ports.txt | uniq -c
```

预期：

- `:8087`（canary）有稳定命中（通常约 10%，允许波动）
- `:8081/:8082` 合计约 90%

#### 5.8.4 A/B Header 强制分流验证

新增一条专用灰度路由（不影响主链路）：

```bash
curl -s -X POST http://127.0.0.1:9000/admin/routes/upsert \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{
    "route": {
      "path_prefix": "/ab-a",
      "service_name": "service-a",
      "ab_header": "X-Canary",
      "ab_variants": {
        "canary": "service-a-canary",
        "stable": "service-a"
      },
      "plugins": []
    }
  }'
```

验证：

```bash
# 命中 canary（8087）
curl -s http://127.0.0.1:9000/ab-a/hello -H 'X-Canary: canary'

# 命中 stable（8081/8082）
curl -s http://127.0.0.1:9000/ab-a/hello -H 'X-Canary: stable'
```

预期：

- `X-Canary: canary` 返回端口 `:8087`
- `X-Canary: stable` 返回端口 `:8081` 或 `:8082`

#### 5.8.5 回滚与清理

删除临时 A/B 路由：

```bash
curl -s -X POST http://127.0.0.1:9000/admin/routes/delete \
  -H 'X-Admin-Token: admin-secret' \
  -H 'Content-Type: application/json' \
  -d '{"path_prefix":"/ab-a"}'
```

如需恢复 `/service-a` 到原配置，可通过：

- `/admin/config/versions` 查询版本
- `/admin/config/rollback` 回滚到变更前版本

#### 5.8.6 常见失败排查

- 全部落在 stable，没有 canary：
  - 检查 `service-a-canary` 是否启动（`8087/healthz`）
  - 检查 `traffic_weights` 是否包含 `service-a-canary` 且权重 > 0
  - 检查 `hash_on` 是否导致 key 全相同（本地常见）
- A/B 头无效：
  - 检查 `ab_header` 大小写和请求头一致
  - 检查 `ab_variants` 的服务名是否存在于 `services`
- 管理接口更新不生效：
  - 检查是否携带 `X-Admin-Token`
  - 查看 `/admin/config/current` 确认路由已写入当前生效配置

### 5.9 JSON<->gRPC 端到端验证（非单元测试）

目标：验证完整链路 `Client(JSON) -> Gateway 转码 -> service-b(gRPC) -> Gateway 反转码 -> Client(JSON)`。

#### 5.9.1 前置检查

```bash
# 1) service-b 健康检查
curl -i http://127.0.0.1:8083/healthz

# 2) 查看 service-b 暴露的 gRPC 信息
curl -s http://127.0.0.1:8083/_grpc/info

# 3) 确认 descriptor 文件已生成
ls -l ./config/proto/service-b-echo.pb
```

预期：

- `8083/healthz` 返回 `200`
- `/_grpc/info` 返回 `grpc_method=/gateway.serviceb.v1.EchoService/Echo`
- descriptor 文件存在且非空

#### 5.9.2 通过网关发起 JSON 请求，验证 gRPC 上游响应

```bash
curl -i -X POST http://127.0.0.1:9000/service-b/echo \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice","userId":"u-1","age":18}'
```

预期：

- 返回 `200`
- `Content-Type: application/json`
- 响应体包含：
  - `message`（形如 `Hello alice from Service B gRPC...`）
  - `ok=true`
  - `server`（命中实例端口，如 `:8083` 或 `:8084`）

#### 5.9.3 验证 unknown field 策略（discard_unknown=true）

```bash
curl -i -X POST http://127.0.0.1:9000/service-b/echo \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice","userId":"u-1","age":18,"unknownField":"ignored"}'
```

预期：

- 返回 `200`（未知字段被丢弃，不报错）

#### 5.9.4 错误输入验证（类型不匹配）

```bash
curl -i -X POST http://127.0.0.1:9000/service-b/echo \
  -H 'Content-Type: application/json' \
  -d '{"name":"alice","age":"not-an-int"}'
```

预期：

- 返回 `400`
- 响应体提示请求体与 proto 描述不匹配

## 6. 指标验证（Prometheus）

```bash
curl -s http://127.0.0.1:9000/metrics | head -n 80
```

重点关注指标：

- `gateway_requests_total`
- `gateway_qps_10s`
- `gateway_qps_1m`
- `gateway_latency_p99_ms`
- `gateway_responses_429_total`
- `gateway_responses_5xx_total`
- `gateway_circuit_open_total`

### 6.1 指标验证（结构化 JSON）

```bash
curl -s http://127.0.0.1:9000/admin/metrics/summary \
  -H 'X-Admin-Token: admin-secret'
```

重点关注字段：

- `total_requests`
- `qps_10s`
- `qps_1m`
- `latency_p99_ms`
- `total_429`
- `total_5xx`
- `circuit_open_total`

## 6.2 Swagger 文档验证

```bash
curl -i http://127.0.0.1:9000/swagger/openapi.json
curl -i http://127.0.0.1:9000/swagger/index.html
```

预期：

- `openapi.json` 返回 `200` 且包含 `openapi` 字段
- `index.html` 返回 `200`，浏览器可打开 Swagger UI
- `/admin/*` schema 与真实响应保持一致（`status/message/data` + snake_case + `data.circuits` 等关键字段）

## 7. Web 控制台端到端验证

1. 访问 `http://127.0.0.1:9000/web`，输入 `admin-secret` 登录。
2. 在“路由配置”新增 `/dynamic` 路由，验证 `/dynamic/hello` 返回 `200`。
3. 在“限流规则”新增规则并刷新列表，确认规则出现。
4. 在“配置版本”页面执行 Dry Run，然后执行 Apply。
5. 在“配置版本”页面点击某历史版本回滚，确认版本号变化。
6. 在“监控总览”查看 QPS/P99、健康矩阵、熔断状态可正常刷新。

## 8. 综合并发压测

### 7.1 网关健康接口吞吐

```bash
ab -n 3000 -c 100 http://127.0.0.1:9000/healthz
```

关注指标：

- `Requests per second`
- `Failed requests`
- `Time per request`

### 7.2 后端单实例基线

```bash
ab -n 3000 -c 100 http://127.0.0.1:8081/healthz
```

用途：

- 作为后端裸服务性能基线，对比网关层开销

### 7.3 网关业务路径并发（含插件链）

```bash
ab -n 3000 -c 100 http://127.0.0.1:9000/service-a/perf
```

解读建议：

- 在限流开启时，`Non-2xx responses` 增加是正常现象
- `Failed requests` 可能包含响应长度差异（例如 `200` 与 `429` body 长度不同）

## 9. 热更新能力测试

网关已开启热更新（默认 3 秒轮询），可按以下流程验证：

1. 启动后持续请求某路由（例如 `/service-a/hello`）。
2. 修改 `config/config.yml` 中对应路由插件参数（例如限流阈值）。
3. 观察网关日志中的“配置热更新成功”。
4. 再次压测，确认行为变化（例如 `429` 比例变化）。

## 10. 一次性回归脚本

可按以下顺序执行回归：

1. `go test ./...`
2. `go build ./...`
3. `python3 run.py`
4. 执行第 4、5、6、7 章命令
5. `Ctrl + C` 停机
