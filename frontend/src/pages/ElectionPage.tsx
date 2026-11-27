import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { StatusBadge } from '../components/Layout';
import { api, errMsg } from '../lib/api';
import type { Election } from '../lib/types';
import CandidatesTab from './tabs/CandidatesTab';
import VotersTab from './tabs/VotersTab';
import MesaTab from './tabs/MesaTab';
import ResultsTab from './tabs/ResultsTab';

type Tab = 'candidatos' | 'eleitores' | 'mesa' | 'resultado';

export default function ElectionPage() {
  const { id } = useParams();
  const eid = Number(id);
  const nav = useNavigate();
  const [params, setParams] = useSearchParams();
  const [election, setElection] = useState<Election | null>(null);
  const [error, setError] = useState('');

  const reload = useCallback(async () => {
    try {
      setElection(await api<Election>(`/api/elections/${eid}`));
    } catch (e) {
      setError(errMsg(e));
    }
  }, [eid]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const defaultTab: Tab = election?.status === 'open' ? 'mesa' : election?.status === 'closed' ? 'resultado' : 'candidatos';
  const tab = (params.get('aba') as Tab) || defaultTab;
  const setTab = (t: Tab) => setParams({ aba: t }, { replace: true });

  const action = async (path: string, confirmMsg: string) => {
    if (!window.confirm(confirmMsg)) return;
    try {
      setElection(await api<Election>(`/api/elections/${eid}/${path}`, { method: 'POST' }));
      // o encerramento invalida o link da urna: descarta o que estava guardado
      if (path !== 'open') sessionStorage.removeItem(`urna_link_${eid}`);
      setParams({}, { replace: true });
    } catch (e) {
      alert(errMsg(e));
    }
  };

  const remove = async () => {
    if (window.prompt('Para excluir definitivamente, digite EXCLUIR') !== 'EXCLUIR') return;
    try {
      await api(`/api/elections/${eid}`, { method: 'DELETE' });
      nav('/');
    } catch (e) {
      alert(errMsg(e));
    }
  };

  if (error) return <div className="alert error">{error}</div>;
  if (!election) return <p className="muted">Carregando…</p>;

  const tabs: { key: Tab; label: string; show: boolean }[] = [
    { key: 'candidatos', label: `Candidatos (${election.candidates_count})`, show: true },
    { key: 'eleitores', label: `Eleitores (${election.voters_count})`, show: true },
    { key: 'mesa', label: 'Mesa receptora', show: election.status === 'open' },
    { key: 'resultado', label: 'Resultado', show: election.status === 'closed' },
  ];

  return (
    <>
      <Link to="/" className="back">
        ← Eleições
      </Link>
      <div className="page-head">
        <div>
          <h1>
            {election.title} <StatusBadge status={election.status} />
          </h1>
          <p className="muted">
            Cargo: {election.office} · Número com {election.digits} dígito(s)
            {election.description ? ` · ${election.description}` : ''}
          </p>
        </div>
        <div className="actions">
          {election.status === 'draft' && (
            <button className="btn success" onClick={() => action('open', 'Abrir a votação? Depois disso não será possível alterar os candidatos.')}>
              ▶ Abrir votação
            </button>
          )}
          {election.status === 'open' && (
            <button
              className="btn danger"
              onClick={() =>
                action('close', `Encerrar a votação? ${election.voters_count - election.voted_count} eleitor(es) ainda não votaram. O link da urna deixará de funcionar.`)
              }
            >
              ■ Encerrar votação
            </button>
          )}
          {election.status === 'closed' && (
            <button
              className="btn"
              onClick={() =>
                action(
                  'reopen',
                  'Reabrir a urna? Os votos já registrados são mantidos e será preciso gerar um novo link. A reabertura ficará registrada no resultado.',
                )
              }
            >
              ↺ Reabrir urna
            </button>
          )}
          {election.status !== 'open' && (
            <button className="btn ghost danger-text" onClick={remove}>
              Excluir
            </button>
          )}
        </div>
      </div>

      <nav className="tabs">
        {tabs
          .filter((t) => t.show)
          .map((t) => (
            <button key={t.key} className={tab === t.key ? 'active' : ''} onClick={() => setTab(t.key)}>
              {t.label}
            </button>
          ))}
      </nav>

      {tab === 'candidatos' && <CandidatesTab election={election} onChange={reload} />}
      {tab === 'eleitores' && <VotersTab election={election} onChange={reload} />}
      {tab === 'mesa' && election.status === 'open' && <MesaTab election={election} onChange={reload} />}
      {tab === 'resultado' && election.status === 'closed' && <ResultsTab election={election} />}
    </>
  );
}
