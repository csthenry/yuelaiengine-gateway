package api

import (
	"context"
	"net/http"
	"strings"

	"yuelaiengine/gateway/internal/core"
	"yuelaiengine/gateway/pkg/logger"

	"github.com/gin-gonic/gin"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// NewRouter 构建 Gin 路由入口。
// Gin 负责统一 HTTP 路由编排，Gateway 负责核心转发与治理逻辑。
func NewRouter(gw *core.Gateway, log logger.Logger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	forward := func(c *gin.Context) {
		gw.ServeHTTP(c.Writer, c.Request)
	}

	// OpenAPI 文档
	swaggerUIHandler := gin.WrapH(httpSwagger.Handler(
		httpSwagger.URL("/swagger/openapi.json"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DefaultModelsExpandDepth(1),
	))
	r.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusPermanentRedirect, "/swagger/index.html")
	})
	r.GET("/swagger/*any", func(c *gin.Context) {
		if c.Param("any") == "/openapi.json" {
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(openAPISpec))
			return
		}
		swaggerUIHandler(c)
	})

	// 管理 API
	admin := r.Group("/admin")
	{
		admin.GET("/circuit/status", func(c *gin.Context) { gw.AdminCircuitStatusHTTP(c.Writer, c.Request) })
		admin.POST("/circuit/reset", func(c *gin.Context) { gw.AdminCircuitResetHTTP(c.Writer, c.Request) })

		admin.GET("/ratelimit/rules", func(c *gin.Context) { gw.AdminRateLimitRuleListHTTP(c.Writer, c.Request) })
		admin.POST("/ratelimit/rules/upsert", func(c *gin.Context) { gw.AdminRateLimitRuleUpsertHTTP(c.Writer, c.Request) })
		admin.POST("/ratelimit/rules/delete", func(c *gin.Context) { gw.AdminRateLimitRuleDeleteHTTP(c.Writer, c.Request) })

		admin.GET("/routes", func(c *gin.Context) { gw.AdminRouteListHTTP(c.Writer, c.Request) })
		admin.POST("/routes/upsert", func(c *gin.Context) { gw.AdminRouteUpsertHTTP(c.Writer, c.Request) })
		admin.POST("/routes/delete", func(c *gin.Context) { gw.AdminRouteDeleteHTTP(c.Writer, c.Request) })

		admin.GET("/services", func(c *gin.Context) { gw.AdminServiceListHTTP(c.Writer, c.Request) })
		admin.POST("/services/upsert", func(c *gin.Context) { gw.AdminServiceUpsertHTTP(c.Writer, c.Request) })
		admin.POST("/services/delete", func(c *gin.Context) { gw.AdminServiceDeleteHTTP(c.Writer, c.Request) })

		admin.GET("/config/versions", func(c *gin.Context) { gw.AdminConfigVersionsHTTP(c.Writer, c.Request) })
		admin.GET("/config/current", func(c *gin.Context) { gw.AdminConfigCurrentHTTP(c.Writer, c.Request) })
		admin.POST("/config/rollback", func(c *gin.Context) { gw.AdminConfigRollbackHTTP(c.Writer, c.Request) })
		admin.POST("/config/apply", func(c *gin.Context) { gw.AdminConfigApplyHTTP(c.Writer, c.Request) })

		admin.GET("/health/status", func(c *gin.Context) { gw.AdminHealthStatusHTTP(c.Writer, c.Request) })
		admin.GET("/metrics/summary", func(c *gin.Context) { gw.AdminMetricsSummaryHTTP(c.Writer, c.Request) })
		admin.GET("/metrics/history", func(c *gin.Context) { gw.AdminMetricsHistoryHTTP(c.Writer, c.Request) })
	}

	// /admin 根路径不存在具体资源，统一返回管理面未找到。
	r.Any("/admin", func(c *gin.Context) {
		gw.AdminNotFoundHTTP(c.Writer, c.Request)
	})

	// 指标
	r.GET("/metrics", func(c *gin.Context) {
		gw.HandleMetricsHTTP(c.Writer, c.Request)
	})

	// Web Console 静态资源与 SPA fallback
	r.Any("/web", func(c *gin.Context) {
		gw.HandleWebUIHTTP(c.Writer, c.Request)
	})
	r.Any("/web/*any", func(c *gin.Context) {
		gw.HandleWebUIHTTP(c.Writer, c.Request)
	})

	// 业务网关转发
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/admin/") {
			gw.AdminNotFoundHTTP(c.Writer, c.Request)
			return
		}
		forward(c)
	})

	log.Info(context.Background(), "Gin Router 初始化完成", "routes", len(r.Routes()))
	return r
}
