CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  name          VARCHAR(120) NOT NULL,
  email         VARCHAR(160) NOT NULL UNIQUE,
  password_hash VARCHAR(100) NOT NULL,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS elections (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  title           VARCHAR(160) NOT NULL,
  description     TEXT NULL,
  office          VARCHAR(120) NOT NULL DEFAULT 'Representante',
  digits          TINYINT UNSIGNED NOT NULL DEFAULT 2,
  status          ENUM('draft','open','closed') NOT NULL DEFAULT 'draft',
  urna_token_hash CHAR(64) NULL,
  created_by      BIGINT UNSIGNED NULL,
  created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  opened_at       DATETIME NULL,
  closed_at       DATETIME NULL,
  UNIQUE KEY uq_elections_urna_token (urna_token_hash),
  CONSTRAINT fk_elections_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS candidates (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  election_id BIGINT UNSIGNED NOT NULL,
  number      VARCHAR(5) NOT NULL,
  name        VARCHAR(120) NOT NULL,
  party       VARCHAR(120) NULL,
  photo_url   VARCHAR(500) NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_candidates_number (election_id, number),
  CONSTRAINT fk_candidates_election FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS voters (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  election_id BIGINT UNSIGNED NOT NULL,
  name        VARCHAR(160) NOT NULL,
  document    VARCHAR(40) NOT NULL,
  voted_at    DATETIME NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_voters_document (election_id, document),
  KEY idx_voters_name (election_id, name),
  CONSTRAINT fk_voters_election FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS vote_sessions (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  election_id BIGINT UNSIGNED NOT NULL,
  voter_id    BIGINT UNSIGNED NOT NULL,
  status      ENUM('active','completed','cancelled') NOT NULL DEFAULT 'active',
  released_by BIGINT UNSIGNED NULL,
  released_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at DATETIME NULL,
  KEY idx_sessions_status (election_id, status),
  CONSTRAINT fk_sessions_election FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE,
  CONSTRAINT fk_sessions_voter FOREIGN KEY (voter_id) REFERENCES voters(id) ON DELETE CASCADE,
  CONSTRAINT fk_sessions_user FOREIGN KEY (released_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS votes (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  election_id  BIGINT UNSIGNED NOT NULL,
  voter_id     BIGINT UNSIGNED NOT NULL,
  session_id   BIGINT UNSIGNED NOT NULL,
  kind         ENUM('candidate','blank','null') NOT NULL,
  candidate_id BIGINT UNSIGNED NULL,
  typed_number VARCHAR(5) NULL,
  prev_hash    CHAR(64) NOT NULL,
  hash         CHAR(64) NOT NULL,
  created_at   DATETIME(6) NOT NULL,
  UNIQUE KEY uq_votes_voter (election_id, voter_id),
  UNIQUE KEY uq_votes_session (session_id),
  KEY idx_votes_candidate (election_id, candidate_id),
  CONSTRAINT fk_votes_election FOREIGN KEY (election_id) REFERENCES elections(id) ON DELETE CASCADE,
  CONSTRAINT fk_votes_voter FOREIGN KEY (voter_id) REFERENCES voters(id) ON DELETE CASCADE,
  CONSTRAINT fk_votes_session FOREIGN KEY (session_id) REFERENCES vote_sessions(id) ON DELETE CASCADE,
  CONSTRAINT fk_votes_candidate FOREIGN KEY (candidate_id) REFERENCES candidates(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
