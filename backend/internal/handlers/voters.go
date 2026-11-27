package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Voter struct {
	ID       uint64     `json:"id"`
	Name     string     `json:"name"`
	Document string     `json:"document"`
	VotedAt  *time.Time `json:"voted_at"`
}

func (h *Handler) ListVoters(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	q := `SELECT id, name, document, voted_at FROM voters WHERE election_id=?`
	args := []any{e.ID}
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q += ` AND (name LIKE ? OR document LIKE ?)`
		like := "%" + s + "%"
		args = append(args, like, like)
	}
	switch c.Query("status") {
	case "voted":
		q += ` AND voted_at IS NOT NULL`
	case "pending":
		q += ` AND voted_at IS NULL`
	}
	q += ` ORDER BY name`
	rows, err := h.DB.QueryContext(c, q, args...)
	if err != nil {
		serverError(c, err)
		return
	}
	defer rows.Close()
	out := []Voter{}
	for rows.Next() {
		var (
			v  Voter
			vt sql.NullTime
		)
		if err := rows.Scan(&v.ID, &v.Name, &v.Document, &vt); err != nil {
			serverError(c, err)
			return
		}
		v.VotedAt = nullTime(vt)
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

type voterInput struct {
	Name     string `json:"name"`
	Document string `json:"document"`
}

func (in *voterInput) normalize() string {
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	in.Document = strings.TrimSpace(in.Document)
	if in.Name == "" || len(in.Name) > 160 {
		return "informe o nome do eleitor"
	}
	if len(in.Document) > 40 {
		return "identificação muito longa (máx. 40)"
	}
	return ""
}

// nextCode gera uma identificação sequencial (E0001...) quando não for informada.
func (h *Handler) nextCode(c *gin.Context, tx *sql.Tx, electionID uint64) (string, error) {
	var n int
	if err := tx.QueryRowContext(c, `SELECT COUNT(*) FROM voters WHERE election_id=?`, electionID).Scan(&n); err != nil {
		return "", err
	}
	for i := n + 1; ; i++ {
		code := fmt.Sprintf("E%04d", i)
		var exists int
		if err := tx.QueryRowContext(c, `SELECT COUNT(*) FROM voters WHERE election_id=? AND document=?`, electionID, code).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return code, nil
		}
	}
}

func (h *Handler) insertVoters(c *gin.Context, electionID uint64, list []voterInput) (created int, skipped []string, err error) {
	tx, err := h.DB.BeginTx(c, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	for _, v := range list {
		if v.Document == "" {
			if v.Document, err = h.nextCode(c, tx, electionID); err != nil {
				return 0, nil, err
			}
		}
		_, err := tx.ExecContext(c, `INSERT INTO voters (election_id, name, document) VALUES (?,?,?)`, electionID, v.Name, v.Document)
		if isDuplicate(err) {
			skipped = append(skipped, v.Name+" ("+v.Document+")")
			continue
		}
		if err != nil {
			return 0, nil, err
		}
		created++
	}
	return created, skipped, tx.Commit()
}

func (h *Handler) CreateVoter(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status == "closed" {
		conflict(c, "a votação está encerrada")
		return
	}
	var in voterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	if msg := in.normalize(); msg != "" {
		badRequest(c, msg)
		return
	}
	created, skipped, err := h.insertVoters(c, e.ID, []voterInput{in})
	if err != nil {
		serverError(c, err)
		return
	}
	if created == 0 && len(skipped) > 0 {
		conflict(c, "já existe um eleitor com essa identificação")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"created": created})
}

// ImportVoters recebe um texto com uma pessoa por linha: "Nome;Identificação"
// (também aceita vírgula ou tab; a identificação é opcional).
func (h *Handler) ImportVoters(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status == "closed" {
		conflict(c, "a votação está encerrada")
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	var list []voterInput
	var invalid []string
	for i, line := range strings.Split(strings.ReplaceAll(in.Text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sep := ";"
		if !strings.Contains(line, ";") {
			if strings.Contains(line, "\t") {
				sep = "\t"
			} else {
				sep = ","
			}
		}
		parts := strings.SplitN(line, sep, 2)
		v := voterInput{Name: parts[0]}
		if len(parts) > 1 {
			v.Document = parts[1]
		}
		// ignora cabeçalho
		if i == 0 && strings.EqualFold(strings.TrimSpace(v.Name), "nome") {
			continue
		}
		if msg := v.normalize(); msg != "" {
			invalid = append(invalid, fmt.Sprintf("linha %d: %s", i+1, msg))
			continue
		}
		list = append(list, v)
	}
	if len(list) > 5000 {
		badRequest(c, "máximo de 5000 eleitores por importação")
		return
	}
	created, skipped, err := h.insertVoters(c, e.ID, list)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created, "duplicated": skipped, "invalid": invalid})
}

func (h *Handler) UpdateVoter(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	vid, ok := idParam(c, "vid")
	if !ok {
		return
	}
	var in voterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	if msg := in.normalize(); msg != "" || in.Document == "" {
		if msg == "" {
			msg = "informe a identificação"
		}
		badRequest(c, msg)
		return
	}
	_, err := h.DB.ExecContext(c, `UPDATE voters SET name=?, document=? WHERE id=? AND election_id=?`, in.Name, in.Document, vid, e.ID)
	if isDuplicate(err) {
		conflict(c, "já existe um eleitor com essa identificação")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) DeleteVoter(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	vid, ok := idParam(c, "vid")
	if !ok {
		return
	}
	var voted sql.NullTime
	err := h.DB.QueryRowContext(c, `SELECT voted_at FROM voters WHERE id=? AND election_id=?`, vid, e.ID).Scan(&voted)
	if err == sql.ErrNoRows {
		notFound(c, "eleitor não encontrado")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if voted.Valid {
		conflict(c, "não é possível remover um eleitor que já votou")
		return
	}
	var active int
	_ = h.DB.QueryRowContext(c, `SELECT COUNT(*) FROM vote_sessions WHERE voter_id=? AND status='active'`, vid).Scan(&active)
	if active > 0 {
		conflict(c, "o eleitor está com a urna liberada; cancele a liberação primeiro")
		return
	}
	// um voto desfeito continua na auditoria; remover o eleitor o apagaria em cascata
	var records int
	if err := h.DB.QueryRowContext(c, `SELECT COUNT(*) FROM votes WHERE voter_id=?`, vid).Scan(&records); err != nil {
		serverError(c, err)
		return
	}
	if records > 0 {
		conflict(c, "não é possível remover um eleitor com voto registrado na auditoria")
		return
	}
	if _, err := h.DB.ExecContext(c, `DELETE FROM voters WHERE id=? AND election_id=?`, vid, e.ID); err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
