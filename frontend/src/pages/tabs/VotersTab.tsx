import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { api, errMsg } from '../../lib/api';
import type { Election, Voter } from '../../lib/types';

interface ImportRes {
  created: number;
  duplicated: string[] | null;
  invalid: string[] | null;
}

export default function VotersTab({ election, onChange }: { election: Election; onChange: () => void }) {
  const [list, setList] = useState<Voter[]>([]);
  const [q, setQ] = useState('');
  const [form, setForm] = useState({ name: '', document: '' });
  const [bulk, setBulk] = useState('');
  const [mode, setMode] = useState<'one' | 'bulk'>('one');
  const [msg, setMsg] = useState<{ kind: 'error' | 'ok'; text: string } | null>(null);
  const editable = election.status !== 'closed';

  const load = useCallback(
    () => api<Voter[]>(`/api/elections/${election.id}/voters?q=${encodeURIComponent(q)}`).then(setList),
    [election.id, q],
  );
  useEffect(() => {
    const t = setTimeout(() => void load(), 250);
    return () => clearTimeout(t);
  }, [load]);

  const addOne = async (e: FormEvent) => {
    e.preventDefault();
    setMsg(null);
    try {
      await api(`/api/elections/${election.id}/voters`, { method: 'POST', body: form });
      setForm({ name: '', document: '' });
      await load();
      onChange();
    } catch (err) {
      setMsg({ kind: 'error', text: errMsg(err) });
    }
  };

  const importBulk = async (e: FormEvent) => {
    e.preventDefault();
    setMsg(null);
    try {
      const r = await api<ImportRes>(`/api/elections/${election.id}/voters/import`, { method: 'POST', body: { text: bulk } });
      const parts = [`${r.created} eleitor(es) importado(s).`];
      if (r.duplicated?.length) parts.push(`Duplicados ignorados: ${r.duplicated.join(', ')}.`);
      if (r.invalid?.length) parts.push(`Inválidos: ${r.invalid.join('; ')}.`);
      setMsg({ kind: r.invalid?.length || r.duplicated?.length ? 'error' : 'ok', text: parts.join(' ') });
      if (r.created) setBulk('');
      await load();
      onChange();
    } catch (err) {
      setMsg({ kind: 'error', text: errMsg(err) });
    }
  };

  const remove = async (v: Voter) => {
    if (!confirm(`Remover ${v.name}?`)) return;
    try {
      await api(`/api/elections/${election.id}/voters/${v.id}`, { method: 'DELETE' });
      await load();
      onChange();
    } catch (err) {
      alert(errMsg(err));
    }
  };

  const onFile = async (f: File | undefined) => {
    if (f) setBulk(await f.text());
  };

  return (
    <div className="two-col">
      {editable ? (
        <div className="card form-stack">
          <div className="seg">
            <button className={mode === 'one' ? 'active' : ''} onClick={() => setMode('one')}>
              Individual
            </button>
            <button className={mode === 'bulk' ? 'active' : ''} onClick={() => setMode('bulk')}>
              Importar lista
            </button>
          </div>
          {mode === 'one' ? (
            <form className="form-stack" onSubmit={addOne}>
              <label>
                Nome completo
                <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
              </label>
              <label>
                Identificação (matrícula, nº de inscrição…) — opcional
                <input value={form.document} onChange={(e) => setForm({ ...form, document: e.target.value })} placeholder="gerada automaticamente se vazio" />
              </label>
              <button className="btn primary">Adicionar eleitor</button>
            </form>
          ) : (
            <form className="form-stack" onSubmit={importBulk}>
              <p className="muted small">
                Uma pessoa por linha, no formato <code>Nome;Identificação</code> (a identificação é opcional). Aceita CSV com vírgula ou colado do Excel.
              </p>
              <textarea rows={10} value={bulk} onChange={(e) => setBulk(e.target.value)} placeholder={'Maria da Silva;001\nJosé Santos;002\nAna Lima'} />
              <input type="file" accept=".csv,.txt" onChange={(e) => void onFile(e.target.files?.[0])} />
              <button className="btn primary" disabled={!bulk.trim()}>
                Importar
              </button>
            </form>
          )}
          {msg && <div className={`alert ${msg.kind}`}>{msg.text}</div>}
        </div>
      ) : (
        <div className="card muted">Votação encerrada — cadastro bloqueado.</div>
      )}

      <div className="card">
        <div className="row-between">
          <input className="search" placeholder="Buscar por nome ou identificação…" value={q} onChange={(e) => setQ(e.target.value)} />
          <span className="muted small">
            {election.voted_count}/{election.voters_count} votaram
          </span>
        </div>
        <table className="table">
          <thead>
            <tr>
              <th>Nome</th>
              <th>Identificação</th>
              <th>Situação</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.map((v) => (
              <tr key={v.id}>
                <td>{v.name}</td>
                <td className="mono">{v.document}</td>
                <td>{v.voted_at ? <span className="pill ok">Votou</span> : <span className="pill">Pendente</span>}</td>
                <td className="right">
                  {editable && !v.voted_at && (
                    <button className="btn ghost sm danger-text" onClick={() => remove(v)}>
                      Remover
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {list.length === 0 && <p className="muted">Nenhum eleitor encontrado.</p>}
      </div>
    </div>
  );
}
