package api

const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Yuelaiengine Gateway API",
    "description": "API Gateway 管理与监控接口（Gin 路由层）",
    "version": "1.1.0"
  },
  "servers": [
    {
      "url": "http://127.0.0.1:9000"
    }
  ],
  "tags": [
    { "name": "Admin", "description": "网关管理接口" },
    { "name": "Metrics", "description": "网关监控接口" },
    { "name": "Health", "description": "健康检查接口" },
    { "name": "Proxy", "description": "业务代理路由入口" }
  ],
  "paths": {
    "/healthz": {
      "get": {
        "tags": ["Health"],
        "summary": "网关健康检查聚合结果",
        "responses": {
          "200": {
            "description": "健康状态",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/HealthStatusMap" }
              }
            }
          }
        }
      }
    },
    "/metrics": {
      "get": {
        "tags": ["Metrics"],
        "summary": "Prometheus 文本指标",
        "responses": {
          "200": {
            "description": "Prometheus exposition 文本",
            "content": {
              "text/plain": {
                "schema": {
                  "type": "string"
                }
              }
            }
          }
        }
      }
    },
    "/admin/config/versions": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询配置版本历史",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "版本列表",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminConfigVersionsResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/config/current": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询当前生效配置",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "当前配置与版本信息",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminConfigCurrentResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/config/apply": {
      "post": {
        "tags": ["Admin"],
        "summary": "应用配置（支持 dry_run / 内存发布 / 落盘发布）",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/ConfigApplyRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "应用成功或 dry_run 校验通过",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/config/rollback": {
      "post": {
        "tags": ["Admin"],
        "summary": "按版本回滚配置",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/ConfigRollbackRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "回滚成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/routes": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询路由列表",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "路由列表",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminRouteListResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/routes/upsert": {
      "post": {
        "tags": ["Admin"],
        "summary": "新增或更新路由",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/RouteUpsertRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "更新成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/routes/delete": {
      "post": {
        "tags": ["Admin"],
        "summary": "删除路由",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/RouteDeleteRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "删除成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/services": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询服务配置列表",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "服务列表",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminServiceListResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/services/upsert": {
      "post": {
        "tags": ["Admin"],
        "summary": "新增或更新服务配置",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/ServiceUpsertRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "更新成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/services/delete": {
      "post": {
        "tags": ["Admin"],
        "summary": "删除服务配置",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/ServiceDeleteRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "删除成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/ratelimit/rules": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询限流规则",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "规则列表",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminRateLimitRuleListResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/ratelimit/rules/upsert": {
      "post": {
        "tags": ["Admin"],
        "summary": "新增或更新限流规则",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/RateLimitRuleUpsertRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "更新成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/ratelimit/rules/delete": {
      "post": {
        "tags": ["Admin"],
        "summary": "删除限流规则",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/RateLimitRuleDeleteRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "删除成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMessageResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/circuit/status": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询熔断状态",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "熔断状态",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminCircuitStatusResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/circuit/reset": {
      "post": {
        "tags": ["Admin"],
        "summary": "重置单服务熔断状态",
        "parameters": [
          { "$ref": "#/components/parameters/AdminToken" },
          {
            "name": "service",
            "in": "query",
            "required": true,
            "schema": { "type": "string", "example": "service-a" }
          }
        ],
        "responses": {
          "200": {
            "description": "重置成功",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminCircuitResetResponse" }
              }
            }
          },
          "400": { "$ref": "#/components/responses/BadRequest" },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/health/status": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询实例健康状态",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "健康状态矩阵",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminHealthStatusResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    },
    "/admin/metrics/summary": {
      "get": {
        "tags": ["Admin"],
        "summary": "查询结构化监控指标",
        "parameters": [{ "$ref": "#/components/parameters/AdminToken" }],
        "responses": {
          "200": {
            "description": "结构化指标",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/AdminMetricsSummaryResponse" }
              }
            }
          },
          "401": { "$ref": "#/components/responses/AdminUnauthorized" }
        }
      }
    }
  },
  "components": {
    "parameters": {
      "AdminToken": {
        "name": "X-Admin-Token",
        "in": "header",
        "required": false,
        "description": "管理接口鉴权 Token；当服务端配置 admin.token 时必填",
        "schema": { "type": "string", "example": "admin-secret" }
      }
    },
    "responses": {
      "AdminUnauthorized": {
        "description": "管理接口鉴权失败",
        "content": {
          "application/json": {
            "schema": { "$ref": "#/components/schemas/ErrorResponse" }
          }
        }
      },
      "BadRequest": {
        "description": "请求参数错误或配置校验失败",
        "content": {
          "application/json": {
            "schema": { "$ref": "#/components/schemas/ErrorResponse" }
          }
        }
      }
    },
    "schemas": {
      "ErrorResponse": {
        "type": "object",
        "required": ["code", "message"],
        "properties": {
          "code": { "type": "string", "example": "BAD_REQUEST" },
          "message": { "type": "string", "example": "请求体格式错误" }
        }
      },
      "AdminVersionMeta": {
        "type": "object",
        "required": ["version", "source", "created_at"],
        "properties": {
          "version": { "type": "string", "example": "v3" },
          "source": { "type": "string", "example": "admin:routes:upsert" },
          "created_at": { "type": "string", "format": "date-time", "example": "2026-05-13T17:28:00+08:00" }
        }
      },
      "AdminTokenBucket": {
        "type": "object",
        "required": ["capacity", "refill_rate"],
        "properties": {
          "capacity": { "type": "integer", "example": 100 },
          "refill_rate": { "type": "integer", "example": 50 }
        }
      },
      "AdminRateLimitRule": {
        "type": "object",
        "required": ["name", "type", "token_bucket"],
        "properties": {
          "name": { "type": "string", "example": "default-ip-limit" },
          "type": { "type": "string", "example": "memory_token_bucket" },
          "token_bucket": { "$ref": "#/components/schemas/AdminTokenBucket" }
        }
      },
      "AdminRoute": {
        "type": "object",
        "required": ["service_name"],
        "properties": {
          "path_prefix": { "type": "string", "example": "/service-a" },
          "path": { "type": "string", "example": "/healthz" },
          "service_name": { "type": "string", "example": "service-a" },
          "plugins": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": true
            }
          },
          "methods": {
            "type": "array",
            "items": { "type": "string", "example": "GET" }
          },
          "requires_auth": { "type": "boolean", "example": true },
          "health_check_scope": { "type": "string" },
          "upstream_protocol": { "type": "string", "example": "http" },
          "protocol_convert": { "type": "string", "example": "none" },
          "grpc_method": { "type": "string" },
          "proto_descriptor_path": { "type": "string" },
          "emit_unpopulated": { "type": "boolean" },
          "use_proto_names": { "type": "boolean" },
          "discard_unknown": { "type": "boolean" },
          "hash_on": { "type": "string" },
          "ab_header": { "type": "string" },
          "ab_variants": {
            "type": "object",
            "additionalProperties": { "type": "string" }
          },
          "traffic_weights": {
            "type": "object",
            "additionalProperties": { "type": "integer" }
          }
        }
      },
      "AdminServiceInstance": {
        "type": "object",
        "required": ["url", "weight"],
        "properties": {
          "url": { "type": "string", "example": "http://127.0.0.1:8081" },
          "weight": { "type": "integer", "example": 1 }
        }
      },
      "AdminService": {
        "type": "object",
        "required": ["name", "instances"],
        "properties": {
          "name": { "type": "string", "example": "service-a" },
          "instances": {
            "type": "array",
            "items": { "$ref": "#/components/schemas/AdminServiceInstance" }
          },
          "health_check_path": { "type": "string", "example": "/healthz" },
          "load_balancer": { "type": "string", "example": "round_robin" }
        }
      },
      "CircuitState": {
        "type": "object",
        "required": ["service_name", "state", "failure_count", "success_count", "failure_threshold", "success_threshold", "reset_timeout"],
        "properties": {
          "service_name": { "type": "string", "example": "service-a" },
          "state": { "type": "string", "example": "closed" },
          "failure_count": { "type": "integer", "example": 0 },
          "success_count": { "type": "integer", "example": 0 },
          "last_open_time": { "type": "string", "format": "date-time" },
          "failure_threshold": { "type": "integer", "example": 5 },
          "success_threshold": { "type": "integer", "example": 2 },
          "reset_timeout": { "type": "string", "example": "1m0s" }
        }
      },
      "MetricsSummary": {
        "type": "object",
        "required": ["timestamp", "total_requests", "total_4xx", "total_5xx", "total_429", "qps_10s", "qps_1m", "latency_p99_ms", "latency_count", "latency_sum_ms", "latency_histogram", "circuit_open_total", "uptime_seconds"],
        "properties": {
          "timestamp": { "type": "string", "format": "date-time" },
          "total_requests": { "type": "integer", "format": "uint64" },
          "total_4xx": { "type": "integer", "format": "uint64" },
          "total_5xx": { "type": "integer", "format": "uint64" },
          "total_429": { "type": "integer", "format": "uint64" },
          "qps_10s": { "type": "number" },
          "qps_1m": { "type": "number" },
          "latency_p99_ms": { "type": "number" },
          "latency_count": { "type": "integer", "format": "uint64" },
          "latency_sum_ms": { "type": "number" },
          "latency_histogram": {
            "type": "array",
            "items": {
              "type": "object",
              "required": ["le", "count"],
              "properties": {
                "le": { "type": "string", "example": "100" },
                "count": { "type": "integer", "format": "uint64" }
              }
            }
          },
          "circuit_open_total": { "type": "integer", "format": "uint64" },
          "uptime_seconds": { "type": "number" }
        }
      },
      "HealthStatusMap": {
        "type": "object",
        "additionalProperties": {
          "type": "object",
          "additionalProperties": { "type": "boolean" }
        }
      },
      "ConfigApplyRequest": {
        "type": "object",
        "required": ["config"],
        "properties": {
          "dry_run": { "type": "boolean", "example": true },
          "source": { "type": "string", "example": "web:config:apply" },
          "persist": { "type": "boolean", "example": false, "description": "true=应用后落盘；false=仅内存生效" },
          "persist_path": { "type": "string", "example": "./config/config.yml", "description": "可选。为空时使用服务默认配置路径" },
          "config": {
            "type": "object",
            "description": "完整 GatewayConfig JSON（与 /admin/config/current 的 config 字段同结构）",
            "additionalProperties": true
          }
        }
      },
      "ConfigRollbackRequest": {
        "type": "object",
        "required": ["version"],
        "properties": {
          "version": { "type": "string", "example": "v3" }
        }
      },
      "RouteUpsertRequest": {
        "type": "object",
        "required": ["route"],
        "properties": {
          "route": { "$ref": "#/components/schemas/AdminRoute" }
        }
      },
      "RouteDeleteRequest": {
        "type": "object",
        "properties": {
          "path_prefix": { "type": "string", "example": "/dynamic" },
          "path": { "type": "string", "example": "/healthz" }
        }
      },
      "ServiceUpsertRequest": {
        "type": "object",
        "required": ["service"],
        "properties": {
          "service": { "$ref": "#/components/schemas/AdminService" }
        }
      },
      "ServiceDeleteRequest": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": { "type": "string", "example": "service-canary" }
        }
      },
      "RateLimitRuleUpsertRequest": {
        "type": "object",
        "required": ["name", "capacity", "refill_rate"],
        "properties": {
          "name": { "type": "string", "example": "default-ip-limit" },
          "type": { "type": "string", "example": "memory_token_bucket" },
          "capacity": { "type": "integer", "example": 100 },
          "refill_rate": { "type": "integer", "example": 50 }
        }
      },
      "RateLimitRuleDeleteRequest": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": { "type": "string", "example": "default-ip-limit" }
        }
      },
      "AdminMessageResponse": {
        "type": "object",
        "required": ["status"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "message": { "type": "string", "example": "配置已应用" },
          "data": {
            "type": "object",
            "additionalProperties": true
          }
        }
      },
      "AdminRouteListResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": {
            "type": "array",
            "items": { "$ref": "#/components/schemas/AdminRoute" }
          }
        }
      },
      "AdminRateLimitRuleListResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": {
            "type": "array",
            "items": { "$ref": "#/components/schemas/AdminRateLimitRule" }
          }
        }
      },
      "AdminServiceListResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": {
            "type": "array",
            "items": { "$ref": "#/components/schemas/AdminService" }
          }
        }
      },
      "AdminConfigVersionsResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": {
            "type": "object",
            "required": ["history"],
            "properties": {
              "current": { "$ref": "#/components/schemas/AdminVersionMeta" },
              "history": {
                "type": "array",
                "items": { "$ref": "#/components/schemas/AdminVersionMeta" }
              }
            }
          }
        }
      },
      "AdminConfigCurrentResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": {
            "type": "object",
            "required": ["config"],
            "properties": {
              "version": { "$ref": "#/components/schemas/AdminVersionMeta" },
              "config": {
                "type": "object",
                "additionalProperties": true
              }
            }
          }
        }
      },
      "AdminCircuitStatusResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": {
            "type": "object",
            "required": ["circuits"],
            "properties": {
              "circuits": {
                "type": "object",
                "additionalProperties": { "$ref": "#/components/schemas/CircuitState" }
              }
            }
          }
        }
      },
      "AdminCircuitResetResponse": {
        "type": "object",
        "required": ["status", "message", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "message": { "type": "string", "example": "熔断器重置成功" },
          "data": {
            "type": "object",
            "required": ["service"],
            "properties": {
              "service": { "type": "string", "example": "service-a" }
            }
          }
        }
      },
      "AdminHealthStatusResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": { "$ref": "#/components/schemas/HealthStatusMap" }
        }
      },
      "AdminMetricsSummaryResponse": {
        "type": "object",
        "required": ["status", "data"],
        "properties": {
          "status": { "type": "string", "example": "ok" },
          "data": { "$ref": "#/components/schemas/MetricsSummary" }
        }
      }
    }
  }
}`
