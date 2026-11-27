package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/agreser-innovations/urna-backend/internal/config"
	"github.com/agreser-innovations/urna-backend/internal/database"
	"github.com/agreser-innovations/urna-backend/internal/handlers"
	"github.com/agreser-innovations/urna-backend/internal/middleware"
	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Connect(cfg.DSN())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(db); err != nil {
		log.Fatal(err)
	}

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.SecurityHeaders())
	_ = r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins(),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	h := handlers.New(db, auth.NewJWT(cfg.JWTSecret, cfg.JWTTTL), realtime.NewHub(), cfg)
	h.Routes(r)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("urna-backend ouvindo em :%s (%s)", cfg.Port, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
