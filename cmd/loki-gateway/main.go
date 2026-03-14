package main

import (
	"log/slog"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	bflog "butterfly.orx.me/core/log"
	redisstore "butterfly.orx.me/core/store/redis"
	"github.com/gin-gonic/gin"
	grpcserver "github.com/orvice/loki-gateway/internal/grpc"
)

const serviceName = "loki-gateway"

type AppConfig struct {
	HTTP struct {
		Greeting string `yaml:"greeting"`
	} `yaml:"http"`
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
				logger := bflog.FromContext(nil).With("service", serviceName)
				logger.Info("service init hook running")
				_ = redisstore.GetClient("default")
				return nil
			},
		},
	})
	slog.Info("starting butterfly service", "service", serviceName)
	svc.Run()
}

func registerRoutes(r *gin.Engine, cfg *AppConfig) {
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": cfg.HTTP.Greeting,
		})
	})
}
