export type ElectionStatus = 'draft' | 'open' | 'closed';

export interface Election {
  id: number;
  title: string;
  description: string | null;
  office: string;
  digits: number;
  status: ElectionStatus;
  has_urna_link: boolean;
  created_at: string;
  opened_at: string | null;
  closed_at: string | null;
  candidates_count: number;
  voters_count: number;
  voted_count: number;
  urna_online: boolean;
}

export interface Candidate {
  id: number;
  number: string;
  name: string;
  party: string | null;
  photo_url: string | null;
}

export interface Voter {
  id: number;
  name: string;
  document: string;
  voted_at: string | null;
}

export interface Session {
  id: number;
  voter_id: number;
  voter_name: string;
  voter_document: string;
  released_at: string;
}

/** Estado da tela da urna — compartilhado entre o celular e o espelho do mesário. */
export interface TypingState {
  digits: string;
  blank: boolean;
  phase: 'idle' | 'typing' | 'sending' | 'done';
}

export interface CandidateResult extends Candidate {
  votes: number;
}

export interface Results {
  election: Election;
  candidates: CandidateResult[];
  blank: number;
  null: number;
  total_votes: number;
  valid_votes: number;
  voters_total: number;
  abstentions: number;
  winners: CandidateResult[] | null;
  tie: boolean;
  integrity_ok: boolean;
  integrity_fail: number | null;
  annulments: Annulment[];
  reopenings: Reopening[];
}

export interface Reopening {
  by_name: string;
  closed_at: string | null;
  reopened_at: string;
}

export interface Annulment {
  vote_id: number;
  voter_name: string;
  voter_document: string;
  by_name: string;
  annulled_at: string;
}

export interface VoteRecord {
  vote_id: number;
  voter_id: number;
  voter_name: string;
  voter_document: string;
  kind: 'candidate' | 'blank' | 'null';
  candidate_id: number | null;
  candidate_number: string | null;
  candidate_name: string | null;
  typed_number: string | null;
  voted_at: string;
  receipt: string;
}

export interface WsMessage<T = unknown> {
  type: string;
  payload?: T;
}
