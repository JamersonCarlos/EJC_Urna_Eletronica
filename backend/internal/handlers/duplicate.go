package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// copyTitle acrescenta " (cópia)" respeitando o limite de 160 do título.
func copyTitle(title string) string {
	const suffix, max = " (cópia)", 160
	r := []rune(title)
	for len(string(r))+len(suffix) > max {
		r = r[:len(r)-1]
	}
	return string(r) + suffix
}

// DuplicateElection cria uma eleição em preparação com os mesmos dados, candidatos
// e eleitores. Votos, liberações e link da urna não são copiados.
func (h *Handler) DuplicateElection(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	tx, err := h.DB.BeginTx(c, nil)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(c, `INSERT INTO elections (title, description, office, digits, created_by)
		SELECT ?, description, office, digits, ? FROM elections WHERE id=?`, copyTitle(e.Title), userID(c), e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `INSERT INTO candidates (election_id, number, name, party, photo_url)
		SELECT ?, number, name, party, photo_url FROM candidates WHERE election_id=? ORDER BY id`, id, e.ID); err != nil {
		serverError(c, err)
		return
	}
	// o número é único na eleição: serve para religar cada eleitor ao candidato copiado
	if _, err := tx.ExecContext(c, `INSERT INTO voters (election_id, name, document, candidate_id)
		SELECT ?, v.name, v.document, nc.id FROM voters v
		LEFT JOIN candidates oc ON oc.id = v.candidate_id
		LEFT JOIN candidates nc ON nc.election_id = ? AND nc.number = oc.number
		WHERE v.election_id = ? ORDER BY v.id`, id, id, e.ID); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}
	copied, err := h.loadElection(c, uint64(id))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, copied)
}
