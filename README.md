# yuelaiengine-gateway

一个基于 Go 的可配置 API Gateway 示例项目，提供路由、负载均衡、健康检查、插件链治理、配置热更新，以及 JSON <-> gRPC 协议转换能力。

## 快速开始

环境要求：

- Go `1.25+`
- Python3
- 建议工具：`curl`、`ab`

安装 `run.py` 依赖：

```bash
pip3 install pyyaml
```

启动全部本地服务：

```bash
python3 run.py
```

基础验证：

```bash
curl -i http://127.0.0.1:9000/healthz
```

## 文档导航

- 测试手册：`TESTING.md`
- 配置示例：`config/config.yml.example`
