package handlers

import (
	"net/http"
	"strings"

	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func (h *Handler) upgrader() *websocket.Upgrader {
	allowed := h.Cfg.AllowedOrigins()
	return &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin == "" {
				return true // clientes não-browser
			}
			for _, o := range allowed {
				if o == origin {
					return true
				}
			}
			return h.Cfg.AppEnv != "production"
		},
	}
}

// AdminWS: painel do mesário (JWT via ?access_token=).
func (h *Handler) AdminWS(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	conn, err := h.upgrader().Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.Hub.Register(conn, e.ID, realtime.RoleAdmin)
}

// UrnaWS: celular da urna (autenticado pelo token do link).
func (h *Handler) UrnaWS(c *gin.Context) {
	e, ok := h.urnaFromToken(c)
	if !ok {
		return
	}
	conn, err := h.upgrader().Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.Hub.Register(conn, e.ID, realtime.RoleUrna)
}
