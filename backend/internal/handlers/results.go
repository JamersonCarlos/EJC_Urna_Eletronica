package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type CandidateResult struct {
	Candidate
	Votes int `json:"votes"`
}

// Results só fica disponível após encerrar a votação.
func (h *Handler) Results(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status != "closed" {
		conflict(c, "o resultado só pode ser visto após encerrar a votação")
		return
	}
	rows, err := h.DB.QueryContext(c, `
		SELECT c.id, c.number, c.name, c.party, c.photo_url, COUNT(v.id)
		FROM candidates c LEFT JOIN votes v ON v.candidate_id = c.id AND v.kind='candidate' AND v.annulled_at IS NULL
		WHERE c.election_id=?
		GROUP BY c.id, c.number, c.name, c.party, c.photo_url
		ORDER BY COUNT(v.id) DESC, c.number`, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	defer rows.Close()
	list := []CandidateResult{}
	for rows.Next() {
		var (
			r           CandidateResult
			party, foto sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.Number, &r.Name, &party, &foto, &r.Votes); err != nil {
			serverError(c, err)
			return
		}
		r.Party, r.PhotoURL = nullStr(party), nullStr(foto)
		list = append(list, r)
	}

	var blank, null, total int
	if err := h.DB.QueryRowContext(c, `SELECT
		COALESCE(SUM(kind='blank'),0), COALESCE(SUM(kind='null'),0), COUNT(*) FROM votes WHERE election_id=? AND annulled_at IS NULL`, e.ID).
		Scan(&blank, &null, &total); err != nil {
		serverError(c, err)
		return
	}

	integrity, brokenAt, err := h.verifyChain(c, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}

	annulments, err := h.listAnnulments(c, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}

	reopenings, err := h.listReopenings(c, e.ID)
	if err != nil {
		serverError(c, err)
		return
	}

	// vencedor(es): maior número de votos válidos; empate é sinalizado
	var winners []CandidateResult
	if len(list) > 0 && list[0].Votes > 0 {
		for _, r := range list {
			if r.Votes == list[0].Votes {
				winners = append(winners, r)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"election":       e,
		"candidates":     list,
		"blank":          blank,
		"null":           null,
		"total_votes":    total,
		"valid_votes":    total - blank - null,
		"voters_total":   e.VoterCnt,
		"abstentions":    e.VoterCnt - total,
		"winners":        winners,
		"tie":            len(winners) > 1,
		"integrity_ok":   integrity,
		"integrity_fail": brokenAt,
		"annulments":     annulments,
		"reopenings":     reopenings,
	})
}

// Annulment é um voto desfeito. O conteúdo do voto anulado não é exposto.
type Annulment struct {
	VoteID        uint64    `json:"vote_id"`
	VoterName     string    `json:"voter_name"`
	VoterDocument string    `json:"voter_document"`
	ByName        string    `json:"by_name"`
	AnnulledAt    time.Time `json:"annulled_at"`
}

func (h *Handler) listAnnulments(ctx context.Context, electionID uint64) ([]Annulment, error) {
	rows, err := h.DB.QueryContext(ctx, `SELECT a.vote_id, vr.name, vr.document, COALESCE(u.name,''), a.created_at
		FROM vote_annulments a
		JOIN votes vt ON vt.id = a.vote_id
		JOIN voters vr ON vr.id = vt.voter_id
		LEFT JOIN users u ON u.id = a.annulled_by
		WHERE a.election_id=? ORDER BY a.id`, electionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Annulment{}
	for rows.Next() {
		var a Annulment
		if err := rows.Scan(&a.VoteID, &a.VoterName, &a.VoterDocument, &a.ByName, &a.AnnulledAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ResultVotes lista quem votou em quem (após encerrar).
// Filtros: ?candidate_id=ID ou ?kind=blank|null|candidate
func (h *Handler) ResultVotes(c *gin.Context) {
	e, ok := h.electionOr404(c)
	if !ok {
		return
	}
	if e.Status != "closed" {
		conflict(c, "disponível somente após encerrar a votação")
		return
	}
	q := `SELECT vt.id, vr.id, vr.name, vr.document, vt.kind, vt.candidate_id, cd.number, cd.name, vt.typed_number, vt.created_at, vt.hash
		FROM votes vt
		JOIN voters vr ON vr.id = vt.voter_id
		LEFT JOIN candidates cd ON cd.id = vt.candidate_id
		WHERE vt.election_id=? AND vt.annulled_at IS NULL`
	args := []any{e.ID}
	if cid := c.Query("candidate_id"); cid != "" {
		q += ` AND vt.candidate_id=?`
		args = append(args, cid)
	}
	if k := c.Query("kind"); k == "blank" || k == "null" || k == "candidate" {
		q += ` AND vt.kind=?`
		args = append(args, k)
	}
	q += ` ORDER BY vt.created_at`
	rows, err := h.DB.QueryContext(c, q, args...)
	if err != nil {
		serverError(c, err)
		return
	}
	defer rows.Close()
	type item struct {
		VoteID          uint64    `json:"vote_id"`
		VoterID         uint64    `json:"voter_id"`
		VoterName       string    `json:"voter_name"`
		VoterDocument   string    `json:"voter_document"`
		Kind            string    `json:"kind"`
		CandidateID     *int64    `json:"candidate_id"`
		CandidateNumber *string   `json:"candidate_number"`
		CandidateName   *string   `json:"candidate_name"`
		TypedNumber     *string   `json:"typed_number"`
		VotedAt         time.Time `json:"voted_at"`
		Receipt         string    `json:"receipt"`
	}
	out := []item{}
	for rows.Next() {
		var (
			it          item
			cid         sql.NullInt64
			cnum, cname sql.NullString
			typed       sql.NullString
			hash        string
		)
		if err := rows.Scan(&it.VoteID, &it.VoterID, &it.VoterName, &it.VoterDocument, &it.Kind, &cid, &cnum, &cname, &typed, &it.VotedAt, &hash); err != nil {
			serverError(c, err)
			return
		}
		if cid.Valid {
			it.CandidateID = &cid.Int64
		}
		it.CandidateNumber, it.CandidateName, it.TypedNumber = nullStr(cnum), nullStr(cname), nullStr(typed)
		it.Receipt = hash[:12]
		out = append(out, it)
	}
	c.JSON(http.StatusOK, out)
}
