package main

import (
	"context"
	"log/slog"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	bflog "butterfly.orx.me/core/log"
	redisstore "butterfly.orx.me/core/store/redis"
	"github.com/gin-gonic/gin"
	internalconfig "github.com/orvice/loki-gateway/internal/config"
	"github.com/orvice/loki-gateway/internal/forwarder"
	grpcserver "github.com/orvice/loki-gateway/internal/grpc"
	"github.com/orvice/loki-gateway/internal/httpapi"
	"github.com/orvice/loki-gateway/internal/service"
)

const serviceName = "loki-gateway"

type AppConfig struct {
	HTTP struct {
		Greeting string `yaml:"greeting"`
	} `yaml:"http"`
	Loki internalconfig.LokiConfig `yaml:"loki"`
}

func (c *AppConfig) Print() {}

func main() {
	cfg := new(AppConfig)
	svc := core.New(&app.Config{
		Service:      serviceName,
		Config:       cfg,
		GRPCRegister: grpcserver.Register,
		Router: func(r *gin.Engine) {
			registerRoutes(r, cfg)
		},
		InitFunc: []func() error{
			func() error {
				logger := bflog.FromContext(context.Background()).With("service", serviceName)
				logger.Info("service init hook running")
				if err := cfg.Loki.Validate(); err != nil {
					return err
				}
				_ = redisstore.GetClient("default")
				return nil
			},
		},
	})
	slog.Info("starting butterfly service", "service", serviceName)
	svc.Run()
}

func registerRoutes(r *gin.Engine, cfg *AppConfig) {
	logger := bflog.FromContext(context.Background()).With("service", serviceName, "component", "loki-gateway")
	fwd := forwarder.NewHTTPClient()
	pushSvc := service.NewPushService(cfg.Loki, fwd, logger)
	querySvc := service.NewQueryService(cfg.Loki, fwd)

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": cfg.HTTP.Greeting,
		})
	})
	httpapi.RegisterPushRoutes(r, pushSvc)
	httpapi.RegisterQueryRoutes(r, querySvc)
}
