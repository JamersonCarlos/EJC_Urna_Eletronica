-- Desfazer voto: o voto nunca é apagado, apenas marcado como anulado.
-- active_voter_id só tem valor no voto vigente, o que mantém "um voto válido por eleitor".
ALTER TABLE votes
  ADD COLUMN annulled_at DATETIME(6) NULL,
  ADD COLUMN active_voter_id BIGINT UNSIGNED AS (IF(annulled_at IS NULL, voter_id, NULL)) VIRTUAL,
  ADD KEY idx_votes_prev (election_id, prev_hash),
  ADD UNIQUE KEY uq_votes_active_voter (election_id, active_voter_id),
  DROP INDEX uq_votes_voter;

CREATE TABLE IF NOT EXISTS vote_annulments (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  election_id BIGINT UNSIGNED NOT NULL,
  vote_id     BIGINT UNSIGNED NOT NULL,
  annulled_by BIGINT UNSIGNED NOT NULL,
  prev_hash   CHAR(64) NOT NULL,
  hash        CHAR(64) NOT NULL,
  created_at  DATETIME(6) NOT NULL,
  UNIQUE KEY uq_annulments_vote (vote_id),
  KEY idx_annulments_prev (election_id, prev_hash),
  CONSTRAINT fk_annulments_election FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE,
  CONSTRAINT fk_annulments_vote FOREIGN KEY (vote_id) REFERENCES votes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
