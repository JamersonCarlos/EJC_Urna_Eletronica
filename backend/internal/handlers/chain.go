// Cadeia de auditoria: votos e anulações formam uma única sequência de hashes.
package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agreser-innovations/urna-backend/internal/auth"
)

func genesisHash(electionID uint64) string {
	return auth.SHA256Hex(fmt.Sprintf("urna-agreser:genesis:%d", electionID))
}

func voteHash(prev string, electionID, voterID, sessionID uint64, kind string, candidateID int64, typed string, at time.Time) string {
	return auth.SHA256Hex(fmt.Sprintf("%s|%d|%d|%d|%s|%d|%s|%s",
		prev, electionID, voterID, sessionID, kind, candidateID, typed, at.UTC().Format(time.RFC3339Nano)))
}

func annulmentHash(prev string, electionID, voteID uint64, voteHash string, userID uint64, at time.Time) string {
	return auth.SHA256Hex(fmt.Sprintf("annul|%s|%d|%d|%s|%d|%s",
		prev, electionID, voteID, voteHash, userID, at.UTC().Format(time.RFC3339Nano)))
}

// chainHead devolve o último hash da cadeia (o que ninguém referencia como anterior).
// Deve ser chamado com a linha da eleição travada.
func chainHead(ctx context.Context, tx *sql.Tx, electionID uint64) (string, error) {
	var head string
	err := tx.QueryRowContext(ctx, `
		SELECT t.hash FROM (
			SELECT hash FROM votes WHERE election_id=?
			UNION ALL
			SELECT hash FROM vote_annulments WHERE election_id=?
		) t
		WHERE NOT EXISTS (SELECT 1 FROM votes v WHERE v.election_id=? AND v.prev_hash=t.hash)
		  AND NOT EXISTS (SELECT 1 FROM vote_annulments a WHERE a.election_id=? AND a.prev_hash=t.hash)
		LIMIT 1`, electionID, electionID, electionID, electionID).Scan(&head)
	if errors.Is(err, sql.ErrNoRows) {
		return genesisHash(electionID), nil
	}
	return head, err
}

// chainLink é um elo (voto ou anulação) já com o hash recalculado a partir dos dados.
type chainLink struct {
	voteID               uint64
	prev, hash, expected string
}

// brokenLink percorre a cadeia desde a gênese e devolve o voto onde ela se rompe.
func brokenLink(genesis string, links []chainLink) (uint64, bool) {
	next := make(map[string]int, len(links))
	for i, l := range links {
		if _, fork := next[l.prev]; fork {
			return l.voteID, true
		}
		next[l.prev] = i
	}
	visited := make([]bool, len(links))
	cur := genesis
	for {
		i, ok := next[cur]
		if !ok || visited[i] {
			break
		}
		l := links[i]
		if l.expected != l.hash {
			return l.voteID, true
		}
		visited[i] = true
		cur = l.hash
	}
	var first uint64
	broken := false
	for i, l := range links {
		if !visited[i] && (!broken || l.voteID < first) {
			first, broken = l.voteID, true
		}
	}
	return first, broken
}

// verifyChain recalcula a cadeia e confere se cada voto marcado como anulado
// tem o respectivo registro de anulação (e vice-versa).
func (h *Handler) verifyChain(ctx context.Context, electionID uint64) (bool, *uint64, error) {
	var links []chainLink
	annulledAt := map[uint64]time.Time{} // votos marcados como anulados

	votes, err := h.DB.QueryContext(ctx, `SELECT id, voter_id, session_id, kind, COALESCE(candidate_id,0), COALESCE(typed_number,''), prev_hash, hash, created_at, annulled_at
		FROM votes WHERE election_id=? ORDER BY id`, electionID)
	if err != nil {
		return false, nil, err
	}
	defer votes.Close()
	for votes.Next() {
		var (
			l                  chainLink
			voterID, sessionID uint64
			kind, typed        string
			cid                int64
			at                 time.Time
			annulled           sql.NullTime
		)
		if err := votes.Scan(&l.voteID, &voterID, &sessionID, &kind, &cid, &typed, &l.prev, &l.hash, &at, &annulled); err != nil {
			return false, nil, err
		}
		l.expected = voteHash(l.prev, electionID, voterID, sessionID, kind, cid, typed, at)
		links = append(links, l)
		if annulled.Valid {
			annulledAt[l.voteID] = annulled.Time
		}
	}
	if err := votes.Err(); err != nil {
		return false, nil, err
	}

	annulments, err := h.DB.QueryContext(ctx, `SELECT a.vote_id, a.annulled_by, a.prev_hash, a.hash, a.created_at, v.hash
		FROM vote_annulments a JOIN votes v ON v.id = a.vote_id WHERE a.election_id=? ORDER BY a.id`, electionID)
	if err != nil {
		return false, nil, err
	}
	defer annulments.Close()
	for annulments.Next() {
		var (
			l     chainLink
			by    uint64
			at    time.Time
			vhash string
		)
		if err := annulments.Scan(&l.voteID, &by, &l.prev, &l.hash, &at, &vhash); err != nil {
			return false, nil, err
		}
		l.expected = annulmentHash(l.prev, electionID, l.voteID, vhash, by, at)
		links = append(links, l)
		flag, ok := annulledAt[l.voteID]
		if !ok || !flag.Equal(at) {
			id := l.voteID
			return false, &id, nil
		}
		delete(annulledAt, l.voteID)
	}
	if err := annulments.Err(); err != nil {
		return false, nil, err
	}
	for id := range annulledAt { // marcado como anulado sem registro
		return false, &id, nil
	}

	if id, broken := brokenLink(genesisHash(electionID), links); broken {
		return false, &id, nil
	}
	return true, nil, nil
}
