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

// ReopenElection volta uma votação encerrada para aberta. Os votos já registrados
// são mantidos e a reabertura fica gravada para aparecer na apuração. O link da
// urna foi invalidado no encerramento, então é preciso gerar um novo.
func (h *Handler) ReopenElection(c *gin.Context) {
	eid, ok := idParam(c, "id")
	if !ok {
		return
	}
	tx, err := h.DB.BeginTx(c, nil)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback()

	var (
		status   string
		closedAt sql.NullTime
	)
	err = tx.QueryRowContext(c, `SELECT status, closed_at FROM elections WHERE id=? FOR UPDATE`, eid).Scan(&status, &closedAt)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "eleição não encontrada")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if status != "closed" {
		conflict(c, "somente uma votação encerrada pode ser reaberta")
		return
	}
	if _, err := tx.ExecContext(c, `INSERT INTO election_reopenings (election_id, reopened_by, closed_at, reopened_at) VALUES (?,?,?,UTC_TIMESTAMP())`,
		eid, userID(c), closedAt); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `UPDATE elections SET status='open', closed_at=NULL, urna_token_hash=NULL WHERE id=?`, eid); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}

	// um celular que ficou conectado com o link antigo não pode receber liberações
	h.Hub.SendUrna(eid, realtime.Message{Type: "revoked"})
	h.Hub.BroadcastAdmins(eid, realtime.Message{Type: "election_status", Payload: gin.H{"status": "open"}})
	e, err := h.loadElection(c, eid)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, e)
}

// Reopening é uma reabertura registrada.
type Reopening struct {
	ByName     string     `json:"by_name"`
	ClosedAt   *time.Time `json:"closed_at"`
	ReopenedAt time.Time  `json:"reopened_at"`
}

func (h *Handler) listReopenings(ctx context.Context, electionID uint64) ([]Reopening, error) {
	rows, err := h.DB.QueryContext(ctx, `SELECT COALESCE(u.name,''), r.closed_at, r.reopened_at
		FROM election_reopenings r LEFT JOIN users u ON u.id = r.reopened_by
		WHERE r.election_id=? ORDER BY r.id`, electionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reopening{}
	for rows.Next() {
		var (
			r      Reopening
			closed sql.NullTime
		)
		if err := rows.Scan(&r.ByName, &closed, &r.ReopenedAt); err != nil {
			return nil, err
		}
		r.ClosedAt = nullTime(closed)
		out = append(out, r)
	}
	return out, rows.Err()
}
