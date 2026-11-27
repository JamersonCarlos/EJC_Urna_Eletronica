package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

type Candidate struct {
	ID       uint64  `json:"id"`
	Number   string  `json:"number"`
	Name     string  `json:"name"`
	Party    *string `json:"party"`
	PhotoURL *string `json:"photo_url"`
}

var onlyDigits = regexp.MustCompile(`^[0-9]+$`)

func isDuplicate(err error) bool {
	if me, ok := err.(*mysql.MySQLError); ok {
		return me.Number == 1062
	}
	return false
}

func (h *Handler) listCandidates(c *gin.Context, electionID uint64) ([]Candidate, error) {
	rows, err := h.DB.QueryContext(c, `SELECT id, number, name, party, photo_url FROM candidates WHERE election_id=? ORDER BY number`, electionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var (
			cd          Candidate
			party, foto sql.NullString
		)
		if err := rows.Scan(&cd.ID, &cd.Number, &cd.Name, &party, &foto); err != nil {
			return nil, err
		}
		cd.Party, cd.PhotoURL = nullStr(party), nullStr(foto)
		out = append(out, cd)
	}
	return out, rows.Err()
}

func (h *Handler) ListCandidates(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	list, err := h.listCandidates(c, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

const maxCandidateNumber = 99

// nextCandidateNumber devolve o próximo número na ordem de inserção (1 a 99, ou
// 1 a 9 em eleições de um dígito), com zeros à esquerda. Ao chegar no limite,
// reaproveita o menor número livre.
func nextCandidateNumber(used []string, digits int) (string, bool) {
	limit := maxCandidateNumber
	if digits == 1 {
		limit = 9
	}
	taken := map[int]bool{}
	highest := 0
	for _, s := range used {
		n, err := strconv.Atoi(s)
		if err != nil {
			continue
		}
		taken[n] = true
		if n > highest && n <= limit {
			highest = n
		}
	}
	next := highest + 1
	if next > limit {
		for next = 1; next <= limit && taken[next]; next++ {
		}
	}
	if next > limit {
		return "", false
	}
	return fmt.Sprintf("%0*d", digits, next), true
}

type candidateInput struct {
	Name     string `json:"name"`
	Party    string `json:"party"`
	PhotoURL string `json:"photo_url"`
}

func (in *candidateInput) validate() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Party = strings.TrimSpace(in.Party)
	in.PhotoURL = strings.TrimSpace(in.PhotoURL)
	if in.Name == "" || len(in.Name) > 120 {
		return "informe o nome do candidato"
	}
	if len(in.PhotoURL) > 500 {
		return "a URL da foto é longa demais"
	}
	if in.PhotoURL != "" && !strings.HasPrefix(in.PhotoURL, "https://") {
		return "a URL da foto deve começar com https://"
	}
	return ""
}

var errNoNumberLeft = errors.New("todos os números disponíveis já estão em uso")

// insertCandidate gera o número e grava o candidato com a eleição travada,
// para que cadastros simultâneos não recebam o mesmo número.
func (h *Handler) insertCandidate(ctx context.Context, electionID uint64, digits int, in candidateInput) (uint64, string, error) {
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT id FROM elections WHERE id=? FOR UPDATE`, electionID); err != nil {
		return 0, "", err
	}
	rows, err := tx.QueryContext(ctx, `SELECT number FROM candidates WHERE election_id=?`, electionID)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	var used []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return 0, "", err
		}
		used = append(used, n)
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	number, ok := nextCandidateNumber(used, digits)
	if !ok {
		return 0, "", errNoNumberLeft
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO candidates (election_id, number, name, party, photo_url) VALUES (?,?,?,?,?)`,
		electionID, number, in.Name, emptyToNull(in.Party), emptyToNull(in.PhotoURL))
	if err != nil {
		return 0, "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}
	return uint64(id), number, tx.Commit()
}

func (h *Handler) CreateCandidate(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status != "draft" {
		conflict(c, "candidatos só podem ser alterados antes de abrir a votação")
		return
	}
	var in candidateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	if msg := in.validate(); msg != "" {
		badRequest(c, msg)
		return
	}
	id, number, err := h.insertCandidate(c, e.ID, e.Digits, in)
	if errors.Is(err, errNoNumberLeft) {
		conflict(c, err.Error())
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, Candidate{ID: id, Number: number, Name: in.Name,
		Party: nullStr(emptyToNull(in.Party)), PhotoURL: nullStr(emptyToNull(in.PhotoURL))})
}

// UpdateCandidate altera os dados do candidato. O número é definido na inclusão e não muda.
func (h *Handler) UpdateCandidate(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	cid, ok := idParam(c, "cid")
	if !ok {
		return
	}
	if e.Status != "draft" {
		conflict(c, "candidatos só podem ser alterados antes de abrir a votação")
		return
	}
	var in candidateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	if msg := in.validate(); msg != "" {
		badRequest(c, msg)
		return
	}
	var number string
	err := h.DB.QueryRowContext(c, `SELECT number FROM candidates WHERE id=? AND election_id=?`, cid, e.ID).Scan(&number)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "candidato não encontrado")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}
	if _, err := h.DB.ExecContext(c, `UPDATE candidates SET name=?, party=?, photo_url=? WHERE id=? AND election_id=?`,
		in.Name, emptyToNull(in.Party), emptyToNull(in.PhotoURL), cid, e.ID); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, Candidate{ID: cid, Number: number, Name: in.Name,
		Party: nullStr(emptyToNull(in.Party)), PhotoURL: nullStr(emptyToNull(in.PhotoURL))})
}

func (h *Handler) DeleteCandidate(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	cid, ok := idParam(c, "cid")
	if !ok {
		return
	}
	if e.Status != "draft" {
		conflict(c, "candidatos só podem ser removidos antes de abrir a votação")
		return
	}
	res, err := h.DB.ExecContext(c, `DELETE FROM candidates WHERE id=? AND election_id=?`, cid, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(c, "candidato não encontrado")
		return
	}
	c.Status(http.StatusNoContent)
}
