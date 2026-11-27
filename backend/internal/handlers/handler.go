package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/agreser-innovations/urna-backend/internal/cloudinary"
	"github.com/agreser-innovations/urna-backend/internal/config"
	"github.com/agreser-innovations/urna-backend/internal/middleware"
	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	DB      *sql.DB
	JWT     *auth.JWT
	Hub     *realtime.Hub
	Cfg     *config.Config
	Cloud   *cloudinary.Client // nil quando o upload não está configurado
	limiter *loginLimiter
}

func New(db *sql.DB, j *auth.JWT, hub *realtime.Hub, cfg *config.Config) *Handler {
	h := &Handler{DB: db, JWT: j, Hub: hub, Cfg: cfg, limiter: newLoginLimiter(10, 5*time.Minute)}
	hub.OnUrnaMessage = h.onUrnaMessage
	if cfg.CloudinaryURL != "" {
		cloud, err := cloudinary.New(cfg.CloudinaryURL, cfg.CloudinaryFolder)
		if err != nil {
			log.Printf("upload desativado: %v", err)
		}
		h.Cloud = cloud
	}
	return h
}

// Routes registra todas as rotas da API.
func (h *Handler) Routes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		if err := h.DB.PingContext(c); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "db_down"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api")
	api.POST("/auth/login", h.Login)

	// Rotas públicas da urna (autenticadas pelo token do link)
	urna := api.Group("/urna/:token")
	urna.GET("/ballot", h.UrnaBallot)
	urna.GET("/state", h.UrnaState)
	urna.POST("/vote", h.UrnaVote)
	urna.GET("/ws", h.UrnaWS)

	// Rotas do mesário
	p := api.Group("", middleware.RequireAuth(h.JWT))
	p.GET("/auth/me", h.Me)
	p.PUT("/auth/password", h.ChangePassword)

	p.GET("/elections", h.ListElections)
	p.POST("/elections", h.CreateElection)
	p.GET("/elections/:id", h.GetElection)
	p.PUT("/elections/:id", h.UpdateElection)
	p.DELETE("/elections/:id", h.DeleteElection)
	p.POST("/elections/:id/duplicate", h.DuplicateElection)
	p.POST("/elections/:id/open", h.OpenElection)
	p.POST("/elections/:id/close", h.CloseElection)
	p.POST("/elections/:id/reopen", h.ReopenElection)
	p.POST("/elections/:id/urna-link", h.GenerateUrnaLink)
	p.GET("/elections/:id/ws", h.AdminWS)

	p.GET("/elections/:id/candidates", h.ListCandidates)
	p.POST("/elections/:id/candidates", h.CreateCandidate)
	p.POST("/elections/:id/candidates/add-voters", h.AddCandidateVoters)
	p.PUT("/elections/:id/candidates/:cid", h.UpdateCandidate)
	p.DELETE("/elections/:id/candidates/:cid", h.DeleteCandidate)

	p.GET("/elections/:id/voters", h.ListVoters)
	p.POST("/elections/:id/voters", h.CreateVoter)
	p.POST("/elections/:id/voters/import", h.ImportVoters)
	p.PUT("/elections/:id/voters/:vid", h.UpdateVoter)
	p.DELETE("/elections/:id/voters/:vid", h.DeleteVoter)
	p.POST("/elections/:id/voters/:vid/undo-vote", h.UndoVote)

	p.POST("/uploads/candidate-photo", h.UploadCandidatePhoto)

	p.GET("/elections/:id/sessions/active", h.ActiveSession)
	p.POST("/elections/:id/sessions", h.ReleaseVoter)
	p.DELETE("/elections/:id/sessions/active", h.CancelSession)

	p.GET("/elections/:id/results", h.Results)
	p.GET("/elections/:id/results/votes", h.ResultVotes)
}

// ── helpers ─────────────────────────────────────────────────────────────────

var errNotFound = errors.New("não encontrado")

func idParam(c *gin.Context, name string) (uint64, bool) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || v == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id inválido"})
		return 0, false
	}
	return v, true
}

func userID(c *gin.Context) uint64 {
	v, _ := c.Get(middleware.CtxUserID)
	id, _ := v.(uint64)
	return id
}

func badRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": msg})
}

func conflict(c *gin.Context, msg string) {
	c.JSON(http.StatusConflict, gin.H{"error": msg})
}

func notFound(c *gin.Context, msg string) {
	c.JSON(http.StatusNotFound, gin.H{"error": msg})
}

func serverError(c *gin.Context, err error) {
	log.Printf("erro interno %s %s: %v", c.Request.Method, c.FullPath(), err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "erro interno"})
}

func nullStr(s sql.NullString) *string {
	if s.Valid {
		return &s.String
	}
	return nil
}

func nullTime(t sql.NullTime) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}

func emptyToNull(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// ── rate limit simples para login (por IP) ──────────────────────────────────

type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.window)
	list := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cut) {
			list = append(list, t)
		}
	}
	if len(list) >= l.max {
		l.hits[key] = list
		return false
	}
	l.hits[key] = append(list, now)
	return true
}
