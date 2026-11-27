import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import UrnaScreen, { type UrnaMode } from '../../components/UrnaScreen';
import { api, errMsg, tokenStore, WS_URL } from '../../lib/api';
import { useSocket } from '../../lib/useSocket';
import type { Candidate, Election, Session, TypingState, Voter, WsMessage } from '../../lib/types';

const EMPTY: TypingState = { digits: '', blank: false, phase: 'idle' };

interface LogItem {
  at: Date;
  text: string;
  kind?: 'ok' | 'warn';
}

export default function MesaTab({ election, onChange }: { election: Election; onChange: () => void }) {
  const eid = election.id;
  const linkKey = `urna_link_${eid}`;
  const [candidates, setCandidates] = useState<Candidate[]>([]);
  const [voters, setVoters] = useState<Voter[]>([]);
  const [session, setSession] = useState<Session | null>(null);
  const [typing, setTyping] = useState<TypingState>(EMPTY);
  const [urnaOnline, setUrnaOnline] = useState(election.urna_online);
  const [link, setLink] = useState<string | null>(() => (election.has_urna_link ? sessionStorage.getItem(linkKey) : null));
  const [showQr, setShowQr] = useState(false);
  const [q, setQ] = useState('');
  const [showVoted, setShowVoted] = useState(false);
  const [busy, setBusy] = useState<number | null>(null);
  const [log, setLog] = useState<LogItem[]>([]);
  const [justVoted, setJustVoted] = useState(false);
  const doneTimer = useRef<number>();
  const sessionRef = useRef(session);
  sessionRef.current = session;

  const addLog = (text: string, kind?: LogItem['kind']) => setLog((l) => [{ at: new Date(), text, kind }, ...l].slice(0, 50));

  const loadAll = useCallback(async () => {
    const [c, v, s] = await Promise.all([
      api<Candidate[]>(`/api/elections/${eid}/candidates`),
      api<Voter[]>(`/api/elections/${eid}/voters`),
      api<{ session: Session | null }>(`/api/elections/${eid}/sessions/active`),
    ]);
    setCandidates(c);
    setVoters(v);
    setSession(s.session);
  }, [eid]);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  const onMessage = useCallback(
    (m: WsMessage) => {
      switch (m.type) {
        case 'urna_status': {
          const on = (m.payload as { online: boolean }).online;
          setUrnaOnline(on);
          addLog(on ? 'Urna conectada' : 'Urna desconectada', on ? 'ok' : 'warn');
          break;
        }
        case 'urna_typing':
          setTyping({ ...EMPTY, ...(m.payload as Partial<TypingState>) });
          break;
        case 'session_started':
          setSession(m.payload as Session);
          setTyping(EMPTY);
          break;
        case 'session_cancelled':
          setSession(null);
          setTyping(EMPTY);
          break;
        case 'vote_registered': {
          const p = m.payload as { voter_id: number; at: string };
          const cur = sessionRef.current;
          addLog(`Voto registrado${cur ? `: ${cur.voter_name}` : ''}`, 'ok');
          setSession(null);
          setVoters((vs) => vs.map((v) => (v.id === p.voter_id ? { ...v, voted_at: p.at } : v)));
          setTyping(EMPTY);
          setJustVoted(true);
          window.clearTimeout(doneTimer.current);
          doneTimer.current = window.setTimeout(() => setJustVoted(false), 3000);
          onChange();
          break;
        }
        case 'vote_annulled': {
          const p = m.payload as { voter_id: number; voter_name: string };
          addLog(`Voto desfeito: ${p.voter_name}`, 'warn');
          setVoters((vs) => vs.map((v) => (v.id === p.voter_id ? { ...v, voted_at: null } : v)));
          setJustVoted(false);
          setTyping(EMPTY);
          onChange();
          break;
        }
        case 'election_status':
          onChange();
          break;
      }
    },
    [onChange],
  );

  const token = tokenStore.get();
  const { connected } = useSocket(token ? `${WS_URL}/api/elections/${eid}/ws?access_token=${encodeURIComponent(token)}` : null, onMessage, () => void loadAll());

  const generateLink = async () => {
    if ((election.has_urna_link || link) && !confirm('Gerar um novo link? O link anterior deixará de funcionar e a urna atual será desconectada.')) return;
    try {
      const r = await api<{ url: string }>(`/api/elections/${eid}/urna-link`, { method: 'POST' });
      setLink(r.url);
      sessionStorage.setItem(linkKey, r.url);
      setShowQr(true);
      addLog('Novo link da urna gerado');
      onChange(); // atualiza has_urna_link
    } catch (e) {
      alert(errMsg(e));
    }
  };

  const release = async (v: Voter) => {
    setBusy(v.id);
    try {
      const r = await api<{ session: Session; urna_online: boolean }>(`/api/elections/${eid}/sessions`, { method: 'POST', body: { voter_id: v.id } });
      setSession(r.session);
      setTyping(EMPTY);
      setJustVoted(false);
      addLog(`Urna liberada para ${v.name}`);
      if (!r.urna_online) addLog('Atenção: a urna não está conectada', 'warn');
    } catch (e) {
      alert(errMsg(e));
    } finally {
      setBusy(null);
    }
  };

  const cancel = async () => {
    if (!session || !confirm(`Cancelar a liberação de ${session.voter_name}? O voto não será registrado.`)) return;
    try {
      await api(`/api/elections/${eid}/sessions/active`, { method: 'DELETE' });
      addLog(`Liberação cancelada: ${session.voter_name}`, 'warn');
      setSession(null);
      setTyping(EMPTY);
    } catch (e) {
      alert(errMsg(e));
    }
  };

  const undoVote = async (v: Voter) => {
    if (!confirm(`Desfazer o voto de ${v.name}?\n\nO voto deixa de contar e a pessoa poderá votar novamente. A ação fica registrada na auditoria.`)) return;
    setBusy(v.id);
    try {
      await api(`/api/elections/${eid}/voters/${v.id}/undo-vote`, { method: 'POST' });
      // atualiza mesmo se o WebSocket estiver reconectando
      setVoters((vs) => vs.map((x) => (x.id === v.id ? { ...x, voted_at: null } : x)));
      setJustVoted(false);
      onChange();
    } catch (e) {
      alert(errMsg(e));
    } finally {
      setBusy(null);
    }
  };

  const filtered = useMemo(() => {
    const term = q.trim().toLowerCase();
    return voters
      .filter((v) => (showVoted ? true : !v.voted_at))
      .filter((v) => !term || v.name.toLowerCase().includes(term) || v.document.toLowerCase().includes(term));
  }, [voters, q, showVoted]);

  const votedCount = voters.filter((v) => v.voted_at).length;
  const mirrorMode: UrnaMode = session ? 'voting' : justVoted || typing.phase === 'done' ? 'done' : 'locked';

  return (
    <div className="mesa">
      <section className="mesa-left">
        <div className="card">
          <div className="row-between">
            <h3>Urna (celular)</h3>
            <span className={`status-chip ${urnaOnline ? 'on' : 'off'}`}>{urnaOnline ? 'Conectada' : 'Desconectada'}</span>
          </div>
          {link ? (
            <>
              <div className="link-box">
                <input readOnly value={link} onFocus={(e) => e.target.select()} />
                <button className="btn sm" onClick={() => void navigator.clipboard?.writeText(link)}>
                  Copiar
                </button>
              </div>
              <div className="row gap">
                <button className="btn ghost sm" onClick={() => setShowQr((s) => !s)}>
                  {showQr ? 'Ocultar QR Code' : 'Mostrar QR Code'}
                </button>
                <a className="btn ghost sm" href={link} target="_blank" rel="noreferrer">
                  Abrir
                </a>
                <button className="btn ghost sm" onClick={generateLink}>
                  Novo link
                </button>
              </div>
              {showQr && (
                <div className="qr">
                  <QRCodeSVG value={link} size={200} includeMargin />
                  <p className="muted small">Escaneie com o celular que será a urna.</p>
                </div>
              )}
            </>
          ) : (
            <>
              <p className="muted small">
                {election.has_urna_link
                  ? 'Já existe um link ativo (gerado em outro navegador). Gere um novo para exibi-lo aqui.'
                  : 'Gere o link e abra-o no celular que servirá de urna.'}
              </p>
              <button className="btn primary" onClick={generateLink}>
                Gerar link da urna
              </button>
            </>
          )}
        </div>

        <div className="card">
          <h3>Comparecimento</h3>
          <div className="big-number">
            {votedCount}
            <span>/{voters.length}</span>
          </div>
          <div className="progress">
            <div style={{ width: `${voters.length ? (100 * votedCount) / voters.length : 0}%` }} />
          </div>
        </div>

        <div className="card log">
          <h3>Registro</h3>
          {!connected && <div className="alert warn small">Painel reconectando…</div>}
          <ul>
            {log.map((l, i) => (
              <li key={i} className={l.kind}>
                <span className="mono">{l.at.toLocaleTimeString('pt-BR')}</span> {l.text}
              </li>
            ))}
            {log.length === 0 && <li className="muted">Sem eventos ainda.</li>}
          </ul>
        </div>
      </section>

      <section className="mesa-center">
        <div className="card mirror">
          <div className="row-between">
            <h3>Espelho da urna — ao vivo</h3>
            {session && (
              <button className="btn ghost sm danger-text" onClick={cancel}>
                Cancelar liberação
              </button>
            )}
          </div>
          <div className="mirror-voter">
            {session ? (
              <>
                Votando agora: <b>{session.voter_name}</b> <span className="mono muted">({session.voter_document})</span>
              </>
            ) : (
              <span className="muted">Nenhum eleitor liberado. Selecione na lista ao lado.</span>
            )}
          </div>
          <UrnaScreen mode={mirrorMode} office={election.office} digits={election.digits} candidates={candidates} typing={typing} voterName={session?.voter_name} compact />
          {session && (
            <p className="muted small center">O voto só é gravado quando o eleitor aperta CONFIRMA na urna.</p>
          )}
        </div>
      </section>

      <section className="mesa-right">
        <div className="card">
          <h3>Eleitores</h3>
          <input className="search" placeholder="Buscar nome ou identificação…" value={q} onChange={(e) => setQ(e.target.value)} autoFocus />
          <label className="check">
            <input type="checkbox" checked={showVoted} onChange={(e) => setShowVoted(e.target.checked)} /> mostrar quem já votou (para desfazer um voto)
          </label>
          <ul className="voter-list">
            {filtered.map((v) => (
              <li key={v.id} className={v.voted_at ? 'voted' : session?.voter_id === v.id ? 'current' : ''}>
                <div className="grow">
                  <div>{v.name}</div>
                  <div className="mono muted small">{v.document}</div>
                </div>
                {v.voted_at ? (
                  <>
                    <span className="pill ok">Votou</span>
                    <button className="btn ghost sm danger-text" disabled={busy !== null} onClick={() => undoVote(v)}>
                      Desfazer voto
                    </button>
                  </>
                ) : session?.voter_id === v.id ? (
                  <span className="pill current">Na urna</span>
                ) : (
                  <button className="btn success sm" disabled={!!session || busy !== null} onClick={() => release(v)}>
                    Liberar
                  </button>
                )}
              </li>
            ))}
            {filtered.length === 0 && <li className="muted">Nenhum eleitor pendente{q ? ' com esse filtro' : ''}.</li>}
          </ul>
        </div>
      </section>
    </div>
  );
}
