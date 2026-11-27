import { useEffect, useMemo, useState } from 'react';
import { api, errMsg } from '../../lib/api';
import type { Election, Results, VoteRecord } from '../../lib/types';

type Filter = { kind: 'all' } | { kind: 'candidate'; id: number; label: string } | { kind: 'blank' } | { kind: 'null' };

const pct = (n: number, d: number) => (d ? ((100 * n) / d).toFixed(1).replace('.', ',') + '%' : '0%');
const fmt = (s: string) => new Date(s).toLocaleString('pt-BR');

export default function ResultsTab({ election }: { election: Election }) {
  const [res, setRes] = useState<Results | null>(null);
  const [votes, setVotes] = useState<VoteRecord[]>([]);
  const [filter, setFilter] = useState<Filter>({ kind: 'all' });
  const [error, setError] = useState('');

  useEffect(() => {
    Promise.all([api<Results>(`/api/elections/${election.id}/results`), api<VoteRecord[]>(`/api/elections/${election.id}/results/votes`)])
      .then(([r, v]) => {
        setRes(r);
        setVotes(v);
      })
      .catch((e) => setError(errMsg(e)));
  }, [election.id]);

  const shown = useMemo(() => {
    switch (filter.kind) {
      case 'candidate':
        return votes.filter((v) => v.candidate_id === filter.id);
      case 'blank':
        return votes.filter((v) => v.kind === 'blank');
      case 'null':
        return votes.filter((v) => v.kind === 'null');
      default:
        return votes;
    }
  }, [votes, filter]);

  if (error) return <div className="alert error">{error}</div>;
  if (!res) return <p className="muted">Apurando…</p>;

  const max = Math.max(1, ...res.candidates.map((c) => c.votes), res.blank, res.null);
  const filterLabel =
    filter.kind === 'candidate' ? filter.label : filter.kind === 'blank' ? 'Votos em branco' : filter.kind === 'null' ? 'Votos nulos' : 'Todos os votos';

  const exportCsv = () => {
    const rows = [['Eleitor', 'Identificação', 'Voto', 'Número digitado', 'Data/hora', 'Recibo']];
    for (const v of votes) {
      const voto = v.kind === 'candidate' ? `${v.candidate_number} - ${v.candidate_name}` : v.kind === 'blank' ? 'BRANCO' : 'NULO';
      rows.push([v.voter_name, v.voter_document, voto, v.typed_number ?? '', fmt(v.voted_at), v.receipt]);
    }
    const csv = '﻿' + rows.map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(';')).join('\n');
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
    a.download = `votos-${election.id}.csv`;
    a.click();
  };

  return (
    <div className="results">
      <div className="stat-row">
        <div className="card stat">
          <span>Votos apurados</span>
          <b>{res.total_votes}</b>
        </div>
        <div className="card stat">
          <span>Válidos</span>
          <b>{res.valid_votes}</b>
        </div>
        <div className="card stat">
          <span>Comparecimento</span>
          <b>{pct(res.total_votes, res.voters_total)}</b>
          <small>
            {res.total_votes} de {res.voters_total}
          </small>
        </div>
        <div className="card stat">
          <span>Abstenções</span>
          <b>{res.abstentions}</b>
        </div>
        <div className={`card stat ${res.integrity_ok ? 'ok' : 'bad'}`}>
          <span>Integridade</span>
          <b>{res.integrity_ok ? '✔ Íntegro' : '✖ Violado'}</b>
          <small>{res.integrity_ok ? 'cadeia de hash verificada' : `divergência no voto #${res.integrity_fail}`}</small>
        </div>
      </div>

      {res.reopenings.length > 0 && (
        <div className="alert warn">
          <b>Esta votação foi reaberta {res.reopenings.length} vez(es):</b>
          {res.reopenings.map((r) => (
            <div key={r.reopened_at}>
              {r.closed_at ? `encerrada em ${fmt(r.closed_at)}, ` : ''}reaberta em {fmt(r.reopened_at)} por {r.by_name}
            </div>
          ))}
        </div>
      )}

      {res.winners && res.winners.length > 0 && (
        <div className={`card winner ${res.tie ? 'tie' : ''}`}>
          {res.tie ? (
            <>
              <b>Empate</b> entre {res.winners.map((w) => `${w.name} (${w.number})`).join(', ')} com {res.winners[0]!.votes} votos cada.
            </>
          ) : (
            <>
              🏆 Eleito(a): <b>{res.winners[0]!.name}</b> ({res.winners[0]!.number}) com {res.winners[0]!.votes} votos — {pct(res.winners[0]!.votes, res.valid_votes)} dos válidos
            </>
          )}
        </div>
      )}

      <div className="card">
        <div className="row-between">
          <h3>Votos por candidato</h3>
          <span className="muted small">Clique para ver quem votou</span>
        </div>
        <ul className="bars">
          {res.candidates.map((c) => (
            <li
              key={c.id}
              className={filter.kind === 'candidate' && filter.id === c.id ? 'sel' : ''}
              onClick={() => setFilter({ kind: 'candidate', id: c.id, label: `${c.number} - ${c.name}` })}
            >
              <span className="bar-label">
                <b>{c.number}</b> {c.name}
              </span>
              <span className="bar">
                <span style={{ width: `${(100 * c.votes) / max}%` }} />
              </span>
              <span className="bar-val">
                {c.votes} <small>{pct(c.votes, res.valid_votes)}</small>
              </span>
            </li>
          ))}
          <li className={`alt ${filter.kind === 'blank' ? 'sel' : ''}`} onClick={() => setFilter({ kind: 'blank' })}>
            <span className="bar-label">Branco</span>
            <span className="bar">
              <span style={{ width: `${(100 * res.blank) / max}%` }} />
            </span>
            <span className="bar-val">
              {res.blank} <small>{pct(res.blank, res.total_votes)}</small>
            </span>
          </li>
          <li className={`alt ${filter.kind === 'null' ? 'sel' : ''}`} onClick={() => setFilter({ kind: 'null' })}>
            <span className="bar-label">Nulo</span>
            <span className="bar">
              <span style={{ width: `${(100 * res.null) / max}%` }} />
            </span>
            <span className="bar-val">
              {res.null} <small>{pct(res.null, res.total_votes)}</small>
            </span>
          </li>
        </ul>
        <p className="muted small">Percentual de candidatos sobre votos válidos; branco e nulo sobre o total apurado.</p>
      </div>

      <div className="card">
        <div className="row-between">
          <h3>
            Quem votou — {filterLabel} ({shown.length})
          </h3>
          <div className="row gap">
            {filter.kind !== 'all' && (
              <button className="btn ghost sm" onClick={() => setFilter({ kind: 'all' })}>
                Ver todos
              </button>
            )}
            <button className="btn ghost sm" onClick={exportCsv}>
              Exportar CSV
            </button>
            <button className="btn ghost sm" onClick={() => window.print()}>
              Imprimir
            </button>
          </div>
        </div>
        <table className="table">
          <thead>
            <tr>
              <th>Eleitor</th>
              <th>Identificação</th>
              <th>Voto</th>
              <th>Data/hora</th>
              <th>Recibo</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((v) => (
              <tr key={v.vote_id}>
                <td>{v.voter_name}</td>
                <td className="mono">{v.voter_document}</td>
                <td>
                  {v.kind === 'candidate' ? (
                    `${v.candidate_number} - ${v.candidate_name}`
                  ) : v.kind === 'blank' ? (
                    <span className="pill">BRANCO</span>
                  ) : (
                    <span className="pill warn">NULO ({v.typed_number})</span>
                  )}
                </td>
                <td>{fmt(v.voted_at)}</td>
                <td className="mono muted">{v.receipt}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {shown.length === 0 && <p className="muted">Nenhum voto nesta categoria.</p>}
      </div>

      {res.annulments.length > 0 && (
        <div className="card">
          <h3>Votos desfeitos ({res.annulments.length})</h3>
          <p className="muted small">Não entram na apuração. O registro fica guardado para auditoria.</p>
          <table className="table">
            <thead>
              <tr>
                <th>Eleitor</th>
                <th>Identificação</th>
                <th>Desfeito por</th>
                <th>Data/hora</th>
              </tr>
            </thead>
            <tbody>
              {res.annulments.map((a) => (
                <tr key={a.vote_id}>
                  <td>{a.voter_name}</td>
                  <td className="mono">{a.voter_document}</td>
                  <td>{a.by_name}</td>
                  <td>{fmt(a.annulled_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
