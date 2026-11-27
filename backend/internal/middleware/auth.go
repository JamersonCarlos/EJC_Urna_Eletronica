package middleware

import (
	"net/http"
	"strings"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/gin-gonic/gin"
)

const CtxUserID = "uid"
const CtxUserName = "uname"

// RequireAuth valida o JWT do mesário. Aceita "Authorization: Bearer" ou
// ?access_token= (necessário para WebSocket no navegador).
func RequireAuth(j *auth.JWT) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ""
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if q := c.Query("access_token"); q != "" {
			token = q
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "não autenticado"})
			return
		}
		claims, err := j.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "sessão expirada, faça login novamente"})
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUserName, claims.Name)
		c.Next()
	}
}

// SecurityHeaders aplica cabeçalhos básicos (OWASP) às respostas da API.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}
