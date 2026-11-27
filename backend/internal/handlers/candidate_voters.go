package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type linkResult string

const (
	voterCreated linkResult = "created" // entrou na lista agora
	voterLinked  linkResult = "linked"  // já estava na lista com o mesmo nome
	voterAlready linkResult = "already" // já estava vinculado
)

// ensureCandidateVoter garante que o candidato esteja na lista de eleitores.
// Um eleitor com o mesmo nome é apenas vinculado, para a pessoa não votar duas vezes.
func (h *Handler) ensureCandidateVoter(ctx context.Context, tx *sql.Tx, electionID, candidateID uint64, name string) (linkResult, error) {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM voters WHERE candidate_id=?`, candidateID).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return voterAlready, nil
	}
	name = strings.Join(strings.Fields(name), " ")

	var voterID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM voters WHERE election_id=? AND name=? AND candidate_id IS NULL ORDER BY id LIMIT 1`,
		electionID, name).Scan(&voterID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE voters SET candidate_id=? WHERE id=?`, candidateID, voterID)
		return voterLinked, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	code, err := h.nextCode(ctx, tx, electionID)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO voters (election_id, name, document, candidate_id) VALUES (?,?,?,?)`,
		electionID, name, code, candidateID)
	return voterCreated, err
}

// AddCandidateVoters inclui na lista de eleitores os candidatos selecionados.
func (h *Handler) AddCandidateVoters(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status == "closed" {
		conflict(c, "a votação está encerrada")
		return
	}
	var in struct {
		CandidateIDs []uint64 `json:"candidate_ids"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || len(in.CandidateIDs) == 0 || len(in.CandidateIDs) > maxCandidateNumber {
		badRequest(c, "selecione ao menos um candidato")
		return
	}
	tx, err := h.DB.BeginTx(c, nil)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(c, `SELECT id FROM elections WHERE id=? FOR UPDATE`, e.ID); err != nil {
		serverError(c, err)
		return
	}
	counts := map[linkResult]int{}
	for _, id := range in.CandidateIDs {
		var name string
		err := tx.QueryRowContext(c, `SELECT name FROM candidates WHERE id=? AND election_id=?`, id, e.ID).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			notFound(c, "candidato não encontrado")
			return
		}
		if err != nil {
			serverError(c, err)
			return
		}
		res, err := h.ensureCandidateVoter(c, tx, e.ID, id, name)
		if err != nil {
			serverError(c, err)
			return
		}
		counts[res]++
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": counts[voterCreated], "linked": counts[voterLinked], "already": counts[voterAlready]})
}
