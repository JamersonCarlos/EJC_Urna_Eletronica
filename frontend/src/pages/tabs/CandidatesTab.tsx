import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { api, errMsg } from '../../lib/api';
import { downloadCandidatesPdf } from '../../lib/candidatesPdf';
import type { Candidate, Election } from '../../lib/types';

const EMPTY = { name: '', party: '', photo_url: '' };

export default function CandidatesTab({ election, onChange }: { election: Election; onChange: () => void }) {
  const [list, setList] = useState<Candidate[]>([]);
  const [form, setForm] = useState(EMPTY);
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [uploading, setUploading] = useState(false);
  const [generating, setGenerating] = useState(false);
  const editingNumber = list.find((c) => c.id === editing)?.number;
  const editable = election.status === 'draft';

  const load = useCallback(() => api<Candidate[]>(`/api/elections/${election.id}/candidates`).then(setList), [election.id]);
  useEffect(() => {
    void load();
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    try {
      if (editing) await api(`/api/elections/${election.id}/candidates/${editing}`, { method: 'PUT', body: form });
      else await api(`/api/elections/${election.id}/candidates`, { method: 'POST', body: form });
      setForm(EMPTY);
      setEditing(null);
      await load();
      onChange();
    } catch (err) {
      setError(errMsg(err));
    }
  };

  const upload = async (input: HTMLInputElement) => {
    const file = input.files?.[0];
    input.value = ''; // permite escolher o mesmo arquivo de novo
    if (!file) return;
    setError('');
    if (file.size > 5 * 1024 * 1024) return setError('A imagem deve ter no máximo 5 MB.');
    setUploading(true);
    try {
      const body = new FormData();
      body.append('file', file);
      const r = await api<{ url: string }>('/api/uploads/candidate-photo', { method: 'POST', body });
      setForm((f) => ({ ...f, photo_url: r.url }));
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setUploading(false);
    }
  };

  const downloadPdf = async () => {
    setGenerating(true);
    try {
      const missing = await downloadCandidatesPdf(election, list);
      if (missing.length) alert(`PDF gerado, mas não foi possível carregar a foto de: ${missing.join(', ')}.`);
    } catch (err) {
      alert(`Não foi possível gerar o PDF: ${errMsg(err)}`);
    } finally {
      setGenerating(false);
    }
  };

  const remove = async (c: Candidate) => {
    if (!confirm(`Remover ${c.name} (${c.number})?`)) return;
    try {
      await api(`/api/elections/${election.id}/candidates/${c.id}`, { method: 'DELETE' });
      await load();
      onChange();
    } catch (err) {
      alert(errMsg(err));
    }
  };

  return (
    <div className="two-col">
      {editable ? (
        <form className="card form-stack" onSubmit={submit}>
          <h3>{editing ? 'Editar candidato' : 'Novo candidato'}</h3>
          <p className="muted small">
            {editingNumber ? (
              <>
                Número <b>{editingNumber}</b> (não muda).
              </>
            ) : (
              'O número é gerado automaticamente, na ordem de inclusão.'
            )}
          </p>
          <label>
            Nome na urna
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </label>
          <label>
            Chapa / grupo (opcional)
            <input value={form.party} onChange={(e) => setForm({ ...form, party: e.target.value })} />
          </label>
          <label>
            Foto (opcional — JPG, PNG ou WEBP, até 5 MB)
            <input type="file" accept="image/jpeg,image/png,image/webp" disabled={uploading} onChange={(e) => void upload(e.target)} />
          </label>
          {uploading && <p className="muted small">Enviando foto…</p>}
          {form.photo_url && (
            <div className="photo-preview">
              <img src={form.photo_url} alt="Prévia da foto" />
              <button type="button" className="btn ghost sm danger-text" onClick={() => setForm({ ...form, photo_url: '' })}>
                Remover foto
              </button>
            </div>
          )}
          <label>
            Ou cole a URL da foto (https)
            <input value={form.photo_url} onChange={(e) => setForm({ ...form, photo_url: e.target.value })} placeholder="https://…" />
          </label>
          {error && <div className="alert error">{error}</div>}
          <div className="row gap">
            <button className="btn primary" disabled={uploading}>
              {editing ? 'Salvar' : 'Adicionar'}
            </button>
            {editing && (
              <button
                type="button"
                className="btn ghost"
                onClick={() => {
                  setEditing(null);
                  setForm(EMPTY);
                }}
              >
                Cancelar
              </button>
            )}
          </div>
        </form>
      ) : (
        <div className="card muted">Os candidatos ficam bloqueados para edição depois que a votação é aberta.</div>
      )}

      <div className="card">
        <div className="row-between">
          <h3>Candidatos</h3>
          <button className="btn ghost sm" disabled={!list.length || generating} onClick={() => void downloadPdf()}>
            {generating ? 'Gerando PDF…' : 'Baixar PDF'}
          </button>
        </div>
        {list.length === 0 ? (
          <p className="muted">Nenhum candidato cadastrado.</p>
        ) : (
          <ul className="cand-list">
            {list.map((c) => (
              <li key={c.id}>
                <span className="cand-num">{c.number}</span>
                {c.photo_url ? <img src={c.photo_url} alt="" className="avatar" /> : <span className="avatar ph">{c.name[0]}</span>}
                <div className="grow">
                  <b>{c.name}</b>
                  {c.party && <div className="muted small">{c.party}</div>}
                </div>
                {editable && (
                  <div className="row gap">
                    <button
                      className="btn ghost sm"
                      onClick={() => {
                        setEditing(c.id);
                        setForm({ name: c.name, party: c.party ?? '', photo_url: c.photo_url ?? '' });
                      }}
                    >
                      Editar
                    </button>
                    <button className="btn ghost sm danger-text" onClick={() => remove(c)}>
                      Remover
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
