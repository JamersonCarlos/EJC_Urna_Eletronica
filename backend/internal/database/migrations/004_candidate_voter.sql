-- Vincula o eleitor ao candidato que ele representa, para a mesma pessoa não ser cadastrada duas vezes.
ALTER TABLE voters
  ADD COLUMN candidate_id BIGINT UNSIGNED NULL,
  ADD UNIQUE KEY uq_voters_candidate (candidate_id),
  ADD CONSTRAINT fk_voters_candidate FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE SET NULL;
