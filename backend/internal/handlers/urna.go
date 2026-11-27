package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/auth"
	"github.com/agreser-innovations/urna-backend/internal/realtime"
	"github.com/gin-gonic/gin"
)

type urnaElection struct {
	ID     uint64
	Title  string
	Office string
	Digits int
	Status string
}

// urnaFromToken resolve a eleição pelo token do link (comparando o hash SHA-256).
func (h *Handler) urnaFromToken(c *gin.Context) (*urnaElection, bool) {
	token := c.Param("token")
	if len(token) < 32 || len(token) > 128 {
		notFound(c, "urna não encontrada")
		return nil, false
	}
	var e urnaElection
	err := h.DB.QueryRowContext(c, `SELECT id, title, office, digits, status FROM elections WHERE urna_token_hash=?`,
		auth.SHA256Hex(token)).Scan(&e.ID, &e.Title, &e.Office, &e.Digits, &e.Status)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "link da urna inválido ou expirado")
		return nil, false
	}
	if err != nil {
		serverError(c, err)
		return nil, false
	}
	return &e, true
}

func (h *Handler) UrnaBallot(c *gin.Context) {
	e, ok := h.urnaFromToken(c)
	if !ok {
		return
	}
	cands, err := h.listCandidates(c, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"election":   gin.H{"id": e.ID, "title": e.Title, "office": e.Office, "digits": e.Digits, "status": e.Status},
		"candidates": cands,
	})
}

func (h *Handler) UrnaState(c *gin.Context) {
	e, ok := h.urnaFromToken(c)
	if !ok {
		return
	}
	s, err := activeSession(c, h.DB, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	var sess any
	if s != nil {
		sess = gin.H{"session_id": s.ID, "voter_name": s.VoterName}
	}
	c.JSON(http.StatusOK, gin.H{"status": e.Status, "session": sess})
}

// UrnaVote registra o voto ao clicar em CONFIRMA.
//
// Regras (como na urna oficial):
//   - kind "blank"    → voto em branco
//   - kind "candidate" com número cadastrado → voto no candidato
//   - kind "candidate" com número inexistente → voto nulo
//
// Cada voto encadeia o hash do voto anterior (SHA-256), permitindo auditar
// se algum registro foi alterado/removido diretamente no banco.
func (h *Handler) UrnaVote(c *gin.Context) {
	e, ok := h.urnaFromToken(c)
	if !ok {
		return
	}
	var in struct {
		SessionID uint64 `json:"session_id"`
		Kind      string `json:"kind"`
		Number    string `json:"number"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.SessionID == 0 {
		badRequest(c, "dados inválidos")
		return
	}
	in.Number = strings.TrimSpace(in.Number)
	if in.Kind != "blank" && in.Kind != "candidate" {
		badRequest(c, "tipo de voto inválido")
		return
	}
	if in.Kind == "candidate" && (!onlyDigits.MatchString(in.Number) || len(in.Number) != e.Digits) {
		badRequest(c, fmt.Sprintf("digite os %d dígitos", e.Digits))
		return
	}

	tx, err := h.DB.BeginTx(c, nil)
	if err != nil {
		serverError(c, err)
		return
	}
	defer tx.Rollback()

	// trava a eleição: serializa os votos e mantém a cadeia de hash consistente
	var status string
	if err := tx.QueryRowContext(c, `SELECT status FROM elections WHERE id=? FOR UPDATE`, e.ID).Scan(&status); err != nil {
		serverError(c, err)
		return
	}
	if status != "open" {
		conflict(c, "a votação não está aberta")
		return
	}
	var voterID uint64
	err = tx.QueryRowContext(c, `SELECT voter_id FROM vote_sessions WHERE id=? AND election_id=? AND status='active' FOR UPDATE`,
		in.SessionID, e.ID).Scan(&voterID)
	if errors.Is(err, sql.ErrNoRows) {
		conflict(c, "esta liberação não está mais ativa")
		return
	}
	if err != nil {
		serverError(c, err)
		return
	}

	kind := "blank"
	var candidateID sql.NullInt64
	typed := sql.NullString{}
	if in.Kind == "candidate" {
		typed = sql.NullString{String: in.Number, Valid: true}
		var cid int64
		err := tx.QueryRowContext(c, `SELECT id FROM candidates WHERE election_id=? AND number=?`, e.ID, in.Number).Scan(&cid)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			kind = "null"
		case err != nil:
			serverError(c, err)
			return
		default:
			kind = "candidate"
			candidateID = sql.NullInt64{Int64: cid, Valid: true}
		}
	}

	prev, err := chainHead(c, tx, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	hash := voteHash(prev, e.ID, voterID, in.SessionID, kind, candidateID.Int64, typed.String, now)

	if _, err := tx.ExecContext(c, `INSERT INTO votes (election_id, voter_id, session_id, kind, candidate_id, typed_number, prev_hash, hash, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, e.ID, voterID, in.SessionID, kind, candidateID, typed, prev, hash, now); err != nil {
		if isDuplicate(err) {
			conflict(c, "este eleitor já votou")
			return
		}
		serverError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `UPDATE voters SET voted_at=? WHERE id=?`, now, voterID); err != nil {
		serverError(c, err)
		return
	}
	if _, err := tx.ExecContext(c, `UPDATE vote_sessions SET status='completed', finished_at=? WHERE id=?`, now, in.SessionID); err != nil {
		serverError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(c, err)
		return
	}

	h.Hub.ClearTyping(e.ID)
	h.Hub.BroadcastAdmins(e.ID, realtime.Message{Type: "vote_registered", Payload: gin.H{
		"session_id": in.SessionID, "voter_id": voterID, "at": now,
	}})
	c.JSON(http.StatusCreated, gin.H{"ok": true, "receipt": hash[:12]})
}

// onUrnaMessage repassa ao painel do mesário o que está sendo digitado na urna.
func (h *Handler) onUrnaMessage(c *realtime.Client, msgType string, payload json.RawMessage) {
	switch msgType {
	case "typing":
		if len(payload) > 512 {
			return
		}
		h.Hub.SetTyping(c.ElectionID, payload)
		h.Hub.BroadcastAdmins(c.ElectionID, realtime.Message{Type: "urna_typing", Payload: payload})
	}
}
