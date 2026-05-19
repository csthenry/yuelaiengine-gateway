package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"yuelaiengine/gateway/internal/core"
	"yuelaiengine/gateway/pkg/logger"
)

func wrapHTTP(fn func(http.ResponseWriter, *http.Request)) app.HandlerFunc {
	return adaptor.HertzHandler(http.HandlerFunc(fn))
}

// NewHertzServer builds a native Hertz router and reuses existing gateway handlers.
func NewHertzServer(gw *core.Gateway, log logger.Logger, addr string) *server.Hertz {
	h := server.Default(server.WithHostPorts(addr))

	forward := wrapHTTP(gw.ServeHTTP)
	adminNotFound := wrapHTTP(gw.AdminNotFoundHTTP)
	metrics := wrapHTTP(gw.HandleMetricsHTTP)
	web := wrapHTTP(gw.HandleWebUIHTTP)

	// OpenAPI / Swagger
	swaggerUI := adaptor.HertzHandler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/openapi.json"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DefaultModelsExpandDepth(1),
	))
	h.GET("/swagger", func(_ context.Context, c *app.RequestContext) {
		c.Redirect(consts.StatusPermanentRedirect, []byte("/swagger/index.html"))
	})
	h.GET("/swagger/openapi.json", func(_ context.Context, c *app.RequestContext) {
		c.SetStatusCode(consts.StatusOK)
		c.SetContentType("application/json; charset=utf-8")
		c.SetBodyString(openAPISpec)
	})
	h.GET("/swagger/*any", swaggerUI)

	// 管理 API
	admin := h.Group("/admin")
	admin.GET("/circuit/status", wrapHTTP(gw.AdminCircuitStatusHTTP))
	admin.POST("/circuit/reset", wrapHTTP(gw.AdminCircuitResetHTTP))

	admin.GET("/ratelimit/rules", wrapHTTP(gw.AdminRateLimitRuleListHTTP))
	admin.POST("/ratelimit/rules/upsert", wrapHTTP(gw.AdminRateLimitRuleUpsertHTTP))
	admin.POST("/ratelimit/rules/delete", wrapHTTP(gw.AdminRateLimitRuleDeleteHTTP))

	admin.GET("/routes", wrapHTTP(gw.AdminRouteListHTTP))
	admin.POST("/routes/upsert", wrapHTTP(gw.AdminRouteUpsertHTTP))
	admin.POST("/routes/delete", wrapHTTP(gw.AdminRouteDeleteHTTP))

	admin.GET("/services", wrapHTTP(gw.AdminServiceListHTTP))
	admin.POST("/services/upsert", wrapHTTP(gw.AdminServiceUpsertHTTP))
	admin.POST("/services/delete", wrapHTTP(gw.AdminServiceDeleteHTTP))

	admin.GET("/config/versions", wrapHTTP(gw.AdminConfigVersionsHTTP))
	admin.GET("/config/current", wrapHTTP(gw.AdminConfigCurrentHTTP))
	admin.POST("/config/rollback", wrapHTTP(gw.AdminConfigRollbackHTTP))
	admin.POST("/config/apply", wrapHTTP(gw.AdminConfigApplyHTTP))

	admin.GET("/health/status", wrapHTTP(gw.AdminHealthStatusHTTP))
	admin.GET("/metrics/summary", wrapHTTP(gw.AdminMetricsSummaryHTTP))
	admin.GET("/metrics/history", wrapHTTP(gw.AdminMetricsHistoryHTTP))

	// /admin 根路径不存在具体资源，统一返回管理面未找到。
	h.Any("/admin", adminNotFound)

	// 指标
	h.GET("/metrics", metrics)

	// Web Console 静态资源与 SPA fallback
	h.Any("/web", web)
	h.Any("/web/*any", web)

	// 业务网关转发
	h.NoRoute(func(ctx context.Context, c *app.RequestContext) {
		if strings.HasPrefix(string(c.Path()), "/admin/") {
			adminNotFound(ctx, c)
			return
		}
		forward(ctx, c)
	})

	log.Info(context.Background(), "Hertz Router 初始化完成", "routes", len(h.Routes()))
	return h
}
