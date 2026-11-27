package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/gin-gonic/gin"
)

func (h *Handler) Login(c *gin.Context) {
	if !h.limiter.allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "muitas tentativas, aguarde alguns minutos"})
		return
	}
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	var (
		id         uint64
		name, hash string
	)
	err := h.DB.QueryRowContext(c, `SELECT id, name, password_hash FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(in.Email))).Scan(&id, &name, &hash)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !auth.CheckPassword(hash, in.Password)) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "e-mail ou senha incorretos"})
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	token, exp, err := h.JWT.Issue(id, name)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_at": exp,
		"user":       gin.H{"id": id, "name": name, "email": strings.ToLower(in.Email)},
	})
}

func (h *Handler) Me(c *gin.Context) {
	var name, email string
	if err := h.DB.QueryRowContext(c, `SELECT name, email FROM users WHERE id = ?`, userID(c)).Scan(&name, &email); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuário não encontrado"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": userID(c), "name": name, "email": email})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || len(in.New) < 8 {
		badRequest(c, "a nova senha deve ter pelo menos 8 caracteres")
		return
	}
	var hash string
	if err := h.DB.QueryRowContext(c, `SELECT password_hash FROM users WHERE id = ?`, userID(c)).Scan(&hash); err != nil {
		serverError(c, err)
		return
	}
	if !auth.CheckPassword(hash, in.Current) {
		badRequest(c, "senha atual incorreta")
		return
	}
	nh, err := auth.HashPassword(in.New)
	if err != nil {
		serverError(c, err)
		return
	}
	if _, err := h.DB.ExecContext(c, `UPDATE users SET password_hash = ? WHERE id = ?`, nh, userID(c)); err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
