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
  - 并发压测（ab）
- 暂不覆盖：
  - `auth/apikey/rbac` 插件能力
  - 依赖 `auth-service` 的完整认证链路

## 2. 环境准备

要求：

- Go `1.25.x`
- Python3（用于 `run.py`）
- `curl`
- `ab`（ApacheBench）
- macOS/Linux 终端

可选检查：

```bash
go version
python3 --version
which curl
which ab
```

## 3. 启动与停止

在项目根目录执行：

```bash
python3 run.py
```

说明：

- 会启动 `api-gateway`、`service-a(8081/8082/8087)`、`service-b(8083/8084)`。
- 由于当前没有 `cmd/auth-service`，日志会提示 `auth-service` 目录不存在，这在当前阶段是预期现象。

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

### 5.5 认证链路说明(TODO)

```bash
curl -i http://127.0.0.1:9000/service-b/hello
```

## 6. 综合并发压测

### 6.1 网关健康接口吞吐

```bash
ab -n 3000 -c 100 http://127.0.0.1:9000/healthz
```

关注指标：

- `Requests per second`
- `Failed requests`
- `Time per request`

### 6.2 后端单实例基线

```bash
ab -n 3000 -c 100 http://127.0.0.1:8081/healthz
```

用途：

- 作为后端裸服务性能基线，对比网关层开销

### 6.3 网关业务路径并发（含插件链）

```bash
ab -n 3000 -c 100 http://127.0.0.1:9000/service-a/perf
```

解读建议：

- 在限流开启时，`Non-2xx responses` 增加是正常现象
- `Failed requests` 可能包含响应长度差异（例如 `200` 与 `429` body 长度不同）

## 7. 热更新能力测试

网关已开启热更新（默认 3 秒轮询），可按以下流程验证：

1. 启动后持续请求某路由（例如 `/service-a/hello`）。
2. 修改 `config/config.yml` 中对应路由插件参数（例如限流阈值）。
3. 观察网关日志中的“配置热更新成功”。
4. 再次压测，确认行为变化（例如 `429` 比例变化）。

## 8. 一次性回归脚本

可按以下顺序执行回归：

1. `go test ./...`
2. `go build ./...`
3. `python3 run.py`
4. 执行第 4、5、6 章命令
5. `Ctrl + C` 停机
