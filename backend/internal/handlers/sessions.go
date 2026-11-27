package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-gonic/gin"
)

type Session struct {
	ID         uint64    `json:"id"`
	VoterID    uint64    `json:"voter_id"`
	VoterName  string    `json:"voter_name"`
	Document   string    `json:"voter_document"`
	ReleasedAt time.Time `json:"released_at"`
}

func activeSession(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, electionID uint64) (*Session, error) {
	var s Session
	err := q.QueryRowContext(ctx, `
		SELECT s.id, s.voter_id, v.name, v.document, s.released_at
		FROM vote_sessions s JOIN voters v ON v.id = s.voter_id
		WHERE s.election_id=? AND s.status='active' ORDER BY s.id DESC LIMIT 1`, electionID).
		Scan(&s.ID, &s.VoterID, &s.VoterName, &s.Document, &s.ReleasedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &s, err
}

func (h *Handler) ActiveSession(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	s, err := activeSession(c, h.DB, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": s})
}

// ReleaseVoter libera a urna para um eleitor. Apenas uma liberação ativa por eleição.
func (h *Handler) ReleaseVoter(c *gin.Context) {
	eid, ok := idParam(c, "id")
	if !ok {
		return
	}
	var in struct {
		VoterID uint64 `json:"voter_id"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.VoterID == 0 {
		badRequest(c, "informe o eleitor")
		return
	}
	tx, err := h.DB.BeginTx(c, nil)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(c, `SELECT status FROM elections WHERE id=? FOR UPDATE`, eid).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "eleição não encontrada")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if status != "open" {
		conflict(c, "a votação não está aberta")
		return
	}
	if cur, err := activeSession(c, tx, eid); err != nil {
		serverError(c, err)
		return
	} else if cur != nil {
		conflict(c, "a urna já está liberada para "+cur.VoterName+"; aguarde o voto ou cancele a liberação")
		return
	}
	var (
		name  string
		voted sql.NullTime
	)
	err = tx.QueryRowContext(c, `SELECT name, voted_at FROM voters WHERE id=? AND election_id=? FOR UPDATE`, in.VoterID, eid).Scan(&name, &voted)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "eleitor não encontrado nesta eleição")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if voted.Valid {
		conflict(c, name+" já votou")
		return
	}
	if _, err := tx.ExecContext(c, `INSERT INTO vote_sessions (election_id, voter_id, released_by, released_at) VALUES (?,?,?,UTC_TIMESTAMP())`,
		eid, in.VoterID, userID(c)); err != nil {
		serverError(c, err)
		return
	}
	s, err := activeSession(c, tx, eid)
	if err != nil || s == nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}
	h.Hub.ClearTyping(eid)
	h.Hub.SendUrna(eid, realtime.Message{Type: "session_started", Payload: gin.H{"session_id": s.ID, "voter_name": s.VoterName}})
	h.Hub.BroadcastAdmins(eid, realtime.Message{Type: "session_started", Payload: s})
	c.JSON(http.StatusCreated, gin.H{"session": s, "urna_online": h.Hub.UrnaOnline(eid)})
}

func (h *Handler) CancelSession(c *gin.Context) {
	eid, ok := idParam(c, "id")
	if !ok {
		return
	}
	res, err := h.DB.ExecContext(c, `UPDATE vote_sessions SET status='cancelled', finished_at=UTC_TIMESTAMP() WHERE election_id=? AND status='active'`, eid)
	if err != nil {
		serverError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(c, "não há liberação ativa")
		return
	}
	h.Hub.ClearTyping(eid)
	h.Hub.SendUrna(eid, realtime.Message{Type: "session_cancelled"})
	h.Hub.BroadcastAdmins(eid, realtime.Message{Type: "session_cancelled"})
	c.Status(http.StatusNoContent)
}
