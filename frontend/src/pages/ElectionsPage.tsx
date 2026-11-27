import { useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { StatusBadge } from '../components/Layout';
import { api, errMsg } from '../lib/api';
import type { Election } from '../lib/types';

export default function ElectionsPage() {
  const [list, setList] = useState<Election[] | null>(null);
  const [error, setError] = useState('');
  const [showNew, setShowNew] = useState(false);
  const [form, setForm] = useState({ title: '', office: 'Representante', digits: 2, description: '' });
  const nav = useNavigate();

  useEffect(() => {
    api<Election[]>('/api/elections').then(setList).catch((e) => setError(errMsg(e)));
  }, []);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    try {
      const el = await api<Election>('/api/elections', { method: 'POST', body: form });
      nav(`/eleicoes/${el.id}`);
    } catch (err) {
      setError(errMsg(err));
    }
  };

  return (
    <>
      <div className="page-head">
        <h1>Eleições</h1>
        <button className="btn primary" onClick={() => setShowNew((v) => !v)}>
          + Nova eleição
        </button>
      </div>
      {error && <div className="alert error">{error}</div>}

      {showNew && (
        <form className="card form-grid" onSubmit={create}>
          <label className="span2">
            Título
            <input value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} placeholder="Ex.: Eleição do representante 2026" required />
          </label>
          <label>
            Cargo (aparece na urna)
            <input value={form.office} onChange={(e) => setForm({ ...form, office: e.target.value })} />
          </label>
          <label>
            Dígitos do número
            <select value={form.digits} onChange={(e) => setForm({ ...form, digits: Number(e.target.value) })}>
              {[1, 2, 3, 4, 5].map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          </label>
          <label className="span2">
            Descrição (opcional)
            <textarea rows={2} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </label>
          <div className="span2 right">
            <button className="btn primary">Criar</button>
          </div>
        </form>
      )}

      {list === null ? (
        <p className="muted">Carregando…</p>
      ) : list.length === 0 ? (
        <div className="card empty">Nenhuma eleição criada ainda.</div>
      ) : (
        <div className="grid-cards">
          {list.map((e) => (
            <Link key={e.id} to={`/eleicoes/${e.id}`} className="card election-card">
              <div className="row-between">
                <h3>{e.title}</h3>
                <StatusBadge status={e.status} />
              </div>
              <p className="muted">{e.office}</p>
              <div className="stats-inline">
                <span>
                  <b>{e.candidates_count}</b> candidatos
                </span>
                <span>
                  <b>
                    {e.voted_count}/{e.voters_count}
                  </b>{' '}
                  votaram
                </span>
              </div>
              <div className="progress">
                <div style={{ width: `${e.voters_count ? (100 * e.voted_count) / e.voters_count : 0}%` }} />
              </div>
            </Link>
          ))}
        </div>
      )}
    </>
  );
}
