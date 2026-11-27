package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-gonic/gin"
)

// UndoVote desfaz o voto vigente de um eleitor para que ele possa votar de novo.
// Só é permitido com a votação aberta. O voto anterior permanece no banco, marcado
// como anulado, e a anulação entra na cadeia de hash.
func (h *Handler) UndoVote(c *gin.Context) {
	eid, ok := idParam(c, "id")
	if !ok {
		return
	}
	vid, ok := idParam(c, "vid")
	if !ok {
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
		conflict(c, "só é possível desfazer um voto com a votação aberta")
		return
	}

	var name string
	err = tx.QueryRowContext(c, `SELECT name FROM voters WHERE id=? AND election_id=? FOR UPDATE`, vid, eid).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "eleitor não encontrado nesta eleição")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	var (
		voteID uint64
		hash   string
	)
	err = tx.QueryRowContext(c, `SELECT id, hash FROM votes WHERE election_id=? AND voter_id=? AND annulled_at IS NULL FOR UPDATE`, eid, vid).
		Scan(&voteID, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		conflict(c, name+" não tem voto para desfazer")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	prev, err := chainHead(c, tx, eid)
	if err != nil {
		serverError(c, err)
		return
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	uid := userID(c)
	if _, err := tx.ExecContext(c, `INSERT INTO vote_annulments (election_id, vote_id, annulled_by, prev_hash, hash, created_at) VALUES (?,?,?,?,?,?)`,
		eid, voteID, uid, prev, annulmentHash(prev, eid, voteID, hash, uid, now), now); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `UPDATE votes SET annulled_at=? WHERE id=?`, now, voteID); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `UPDATE voters SET voted_at=NULL WHERE id=?`, vid); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}

	h.Hub.BroadcastAdmins(eid, realtime.Message{Type: "vote_annulled", Payload: gin.H{"voter_id": vid, "voter_name": name}})
	c.Status(http.StatusNoContent)
}
