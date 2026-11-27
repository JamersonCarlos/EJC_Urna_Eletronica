-- Reabertura de votação encerrada: cada ocorrência fica registrada para a apuração.
CREATE TABLE IF NOT EXISTS election_reopenings (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  election_id BIGINT UNSIGNED NOT NULL,
  reopened_by BIGINT UNSIGNED NOT NULL,
  closed_at   DATETIME NULL,
  reopened_at DATETIME NOT NULL,
  KEY idx_reopenings_election (election_id),
  CONSTRAINT fk_reopenings_election FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
