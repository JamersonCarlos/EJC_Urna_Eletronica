package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-gonic/gin"
)

type Election struct {
	ID           uint64     `json:"id"`
	Title        string     `json:"title"`
	Description  *string    `json:"description"`
	Office       string     `json:"office"`
	Digits       int        `json:"digits"`
	Status       string     `json:"status"`
	HasUrnaLink  bool       `json:"has_urna_link"`
	CreatedAt    time.Time  `json:"created_at"`
	OpenedAt     *time.Time `json:"opened_at"`
	ClosedAt     *time.Time `json:"closed_at"`
	CandidateCnt int        `json:"candidates_count"`
	VoterCnt     int        `json:"voters_count"`
	VotedCnt     int        `json:"voted_count"`
	UrnaOnline   bool       `json:"urna_online"`
}

const electionSelect = `
SELECT e.id, e.title, e.description, e.office, e.digits, e.status, e.urna_token_hash IS NOT NULL,
       e.created_at, e.opened_at, e.closed_at,
       (SELECT COUNT(*) FROM candidates c WHERE c.election_id = e.id),
       (SELECT COUNT(*) FROM voters v WHERE v.election_id = e.id),
       (SELECT COUNT(*) FROM voters v WHERE v.election_id = e.id AND v.voted_at IS NOT NULL)
FROM elections e`

type scanner interface{ Scan(dest ...any) error }

func (h *Handler) scanElection(row scanner) (*Election, error) {
	var (
		e      Election
		desc   sql.NullString
		op, cl sql.NullTime
	)
	if err := row.Scan(&e.ID, &e.Title, &desc, &e.Office, &e.Digits, &e.Status, &e.HasUrnaLink,
		&e.CreatedAt, &op, &cl, &e.CandidateCnt, &e.VoterCnt, &e.VotedCnt); err != nil {
		return nil, err
	}
	e.Description, e.OpenedAt, e.ClosedAt = nullStr(desc), nullTime(op), nullTime(cl)
	e.UrnaOnline = h.Hub.UrnaOnline(e.ID)
	return &e, nil
}

func (h *Handler) loadElection(ctx context.Context, id uint64) (*Election, error) {
	e, err := h.scanElection(h.DB.QueryRowContext(ctx, electionSelect+` WHERE e.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return e, err
}

// electionOr404 carrega a eleição ou responde 404.
func (h *Handler) electionOr404(c *gin.Context) (*Election, bool) {
	id, ok := idParam(c, "id")
	if !ok {
		return nil, false
	}
	e, err := h.loadElection(c, id)
	if errors.Is(err, errNotFound) {
		notFound(c, "eleição não encontrada")
		return nil, false
	}
	if err != nil {
		serverError(c, err)
		return nil, false
	}
	return e, true
}

func (h *Handler) ListElections(c *gin.Context) {
	rows, err := h.DB.QueryContext(c, electionSelect+` ORDER BY e.created_at DESC`)
	if err != nil {
		serverError(c, err)
		return
	}
	defer rows.Close()
	out := []*Election{}
	for rows.Next() {
		e, err := h.scanElection(rows)
		if err != nil {
			serverError(c, err)
			return
		}
		out = append(out, e)
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) GetElection(c *gin.Context) {
	if e, ok := h.electionOr404(c); ok {
		c.JSON(http.StatusOK, e)
	}
}

type electionInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Office      string `json:"office"`
	Digits      int    `json:"digits"`
}

func (in *electionInput) validate() string {
	in.Title = strings.TrimSpace(in.Title)
	in.Office = strings.TrimSpace(in.Office)
	if in.Title == "" || len(in.Title) > 160 {
		return "informe o título da eleição"
	}
	if in.Office == "" {
		in.Office = "Representante"
	}
	if in.Digits == 0 {
		in.Digits = 2
	}
	if in.Digits < 1 || in.Digits > 5 {
		return "a quantidade de dígitos deve ser entre 1 e 5"
	}
	return ""
}

func (h *Handler) CreateElection(c *gin.Context) {
	var in electionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	if msg := in.validate(); msg != "" {
		badRequest(c, msg)
		return
	}
	res, err := h.DB.ExecContext(c, `INSERT INTO elections (title, description, office, digits, created_by) VALUES (?,?,?,?,?)`,
		in.Title, emptyToNull(strings.TrimSpace(in.Description)), in.Office, in.Digits, userID(c))
	if err != nil {
		serverError(c, err)
		return
	}
	id, _ := res.LastInsertId()
	e, err := h.loadElection(c, uint64(id))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, e)
}

func (h *Handler) UpdateElection(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	var in electionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		badRequest(c, "dados inválidos")
		return
	}
	if msg := in.validate(); msg != "" {
		badRequest(c, msg)
		return
	}
	if e.Status != "draft" && in.Digits != e.Digits {
		conflict(c, "não é possível alterar os dígitos após abrir a votação")
		return
	}
	if e.Status == "draft" && in.Digits != e.Digits && e.CandidateCnt > 0 {
		conflict(c, "remova os candidatos antes de alterar a quantidade de dígitos")
		return
	}
	if _, err := h.DB.ExecContext(c, `UPDATE elections SET title=?, description=?, office=?, digits=? WHERE id=?`,
		in.Title, emptyToNull(strings.TrimSpace(in.Description)), in.Office, in.Digits, e.ID); err != nil {
		serverError(c, err)
		return
	}
	e, _ = h.loadElection(c, e.ID)
	c.JSON(http.StatusOK, e)
}

func (h *Handler) DeleteElection(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status == "open" {
		conflict(c, "encerre a votação antes de excluir")
		return
	}
	if _, err := h.DB.ExecContext(c, `DELETE FROM elections WHERE id = ?`, e.ID); err != nil {
		serverError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) OpenElection(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status != "draft" {
		conflict(c, "a votação já foi aberta")
		return
	}
	if e.CandidateCnt == 0 || e.VoterCnt == 0 {
		badRequest(c, "cadastre ao menos um candidato e um eleitor antes de abrir")
		return
	}
	if _, err := h.DB.ExecContext(c, `UPDATE elections SET status='open', opened_at=UTC_TIMESTAMP() WHERE id=? AND status='draft'`, e.ID); err != nil {
		serverError(c, err)
		return
	}
	h.Hub.SendUrna(e.ID, realtime.Message{Type: "election_status", Payload: gin.H{"status": "open"}})
	h.Hub.BroadcastAdmins(e.ID, realtime.Message{Type: "election_status", Payload: gin.H{"status": "open"}})
	e, _ = h.loadElection(c, e.ID)
	c.JSON(http.StatusOK, e)
}

func (h *Handler) CloseElection(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status != "open" {
		conflict(c, "a votação não está aberta")
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
	if _, err := tx.ExecContext(c, `UPDATE vote_sessions SET status='cancelled', finished_at=UTC_TIMESTAMP() WHERE election_id=? AND status='active'`, e.ID); err != nil {
		serverError(c, err)
		return
	}
	// Ao encerrar, o link da urna é invalidado.
	if _, err := tx.ExecContext(c, `UPDATE elections SET status='closed', closed_at=UTC_TIMESTAMP(), urna_token_hash=NULL WHERE id=?`, e.ID); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}
	h.Hub.ClearTyping(e.ID)
	h.Hub.SendUrna(e.ID, realtime.Message{Type: "election_status", Payload: gin.H{"status": "closed"}})
	h.Hub.BroadcastAdmins(e.ID, realtime.Message{Type: "election_status", Payload: gin.H{"status": "closed"}})
	e, _ = h.loadElection(c, e.ID)
	c.JSON(http.StatusOK, e)
}

// GenerateUrnaLink cria um novo token para a urna (invalida o anterior).
// O token só é exibido neste momento; no banco fica apenas o hash SHA-256.
func (h *Handler) GenerateUrnaLink(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status == "closed" {
		conflict(c, "a votação está encerrada")
		return
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		serverError(c, err)
		return
	}
	if _, err := h.DB.ExecContext(c, `UPDATE elections SET urna_token_hash=? WHERE id=?`, auth.SHA256Hex(token), e.ID); err != nil {
		serverError(c, err)
		return
	}
	// derruba uma urna conectada com o link antigo
	h.Hub.SendUrna(e.ID, realtime.Message{Type: "revoked"})
	origins := h.Cfg.AllowedOrigins()
	base := ""
	if len(origins) > 0 {
		base = origins[0]
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "url": base + "/urna/" + token})
}
