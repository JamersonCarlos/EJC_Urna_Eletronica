import { useCallback, useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import UrnaScreen, { type UrnaMode } from '../components/UrnaScreen';
import { api, ApiError, errMsg, WS_URL } from '../lib/api';
import { sound } from '../lib/sound';
import { useSocket } from '../lib/useSocket';
import type { Candidate, ElectionStatus, TypingState, WsMessage } from '../lib/types';

interface Ballot {
  election: { id: number; title: string; office: string; digits: number; status: ElectionStatus };
  candidates: Candidate[];
}
interface UrnaStateRes {
  status: ElectionStatus;
  session: { session_id: number; voter_name: string } | null;
}

const EMPTY: TypingState = { digits: '', blank: false, phase: 'idle' };

/** Urna que roda no celular. */
export default function UrnaPage() {
  const { token = '' } = useParams();
  const [ballot, setBallot] = useState<Ballot | null>(null);
  const [mode, setMode] = useState<UrnaMode>('loading');
  const [session, setSession] = useState<UrnaStateRes['session']>(null);
  const [typing, setTyping] = useState<TypingState>(EMPTY);
  const [message, setMessage] = useState<string>('');
  const [started, setStarted] = useState(false);
  const [dead, setDead] = useState(false); // link inválido/revogado: não reconecta
  const doneTimer = useRef<number>();
  const modeRef = useRef(mode);
  modeRef.current = mode;

  const applyState = useCallback((st: UrnaStateRes) => {
    if (st.status === 'closed') return setMode('closed');
    if (st.status === 'draft') return setMode('draft');
    if (modeRef.current === 'done') return; // deixa a tela FIM terminar
    setSession(st.session);
    setTyping(EMPTY);
    setMode(st.session ? 'voting' : 'locked');
  }, []);

  const refresh = useCallback(async () => {
    try {
      const [b, st] = await Promise.all([api<Ballot>(`/api/urna/${token}/ballot`, { auth: false }), api<UrnaStateRes>(`/api/urna/${token}/state`, { auth: false })]);
      setBallot(b);
      applyState(st);
    } catch (e) {
      setMode('error');
      setMessage(errMsg(e));
      if (e instanceof ApiError && e.status === 404) setDead(true);
    }
  }, [token, applyState]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const onMessage = useCallback(
    (m: WsMessage) => {
      switch (m.type) {
        case 'session_started': {
          const p = m.payload as { session_id: number; voter_name: string };
          window.clearTimeout(doneTimer.current);
          setSession(p);
          setTyping(EMPTY);
          setMode('voting');
          sound.key();
          break;
        }
        case 'session_cancelled':
          setSession(null);
          setTyping(EMPTY);
          setMode('locked');
          break;
        case 'election_status': {
          const s = (m.payload as { status: ElectionStatus }).status;
          if (s === 'closed') setMode('closed');
          else void refresh();
          break;
        }
        case 'revoked':
          setMode('error');
          setMessage('Este link foi substituído por um novo. Use o novo link do mesário.');
          setDead(true);
          break;
        case 'replaced':
          setMode('error');
          setMessage('A urna foi aberta em outro dispositivo.');
          setDead(true);
          break;
      }
    },
    [refresh],
  );

  const { connected, send } = useSocket(token && !dead ? `${WS_URL}/api/urna/${token}/ws` : null, onMessage, () => void refresh());

  // espelha a digitação para o mesário
  useEffect(() => {
    send({ type: 'typing', payload: mode === 'voting' ? typing : { ...EMPTY, phase: mode === 'done' ? 'done' : 'idle' } });
  }, [typing, mode, send, connected]);

  // mantém a tela ligada
  useEffect(() => {
    let lock: { release: () => Promise<void> } | null = null;
    const nav = navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<{ release: () => Promise<void> }> } };
    const req = () => nav.wakeLock?.request('screen').then((l) => (lock = l)).catch(() => undefined);
    void req();
    const onVis = () => document.visibilityState === 'visible' && void req();
    document.addEventListener('visibilitychange', onVis);
    return () => {
      document.removeEventListener('visibilitychange', onVis);
      void lock?.release();
    };
  }, []);

  const digits = ballot?.election.digits ?? 2;
  const canType = mode === 'voting' && typing.phase !== 'sending';

  const press = (d: string) => {
    if (!canType || typing.blank || typing.digits.length >= digits) return;
    sound.key();
    setTyping((t) => ({ ...t, digits: t.digits + d, phase: 'typing' }));
  };
  const blank = () => {
    if (!canType) return;
    if (typing.digits.length > 0) return sound.error(); // como na urna: corrija antes
    sound.key();
    setTyping({ digits: '', blank: true, phase: 'typing' });
  };
  const correct = () => {
    if (!canType) return;
    sound.key();
    setTyping(EMPTY);
  };
  const confirm = async () => {
    if (!canType || !session) return;
    const complete = typing.digits.length === digits;
    if (!typing.blank && !complete) return sound.error();
    setTyping((t) => ({ ...t, phase: 'sending' }));
    try {
      await api(`/api/urna/${token}/vote`, {
        method: 'POST',
        auth: false,
        body: { session_id: session.session_id, kind: typing.blank ? 'blank' : 'candidate', number: typing.blank ? '' : typing.digits },
      });
      sound.confirm();
      setSession(null);
      setTyping(EMPTY);
      setMode('done');
      doneTimer.current = window.setTimeout(() => {
        setMode((m) => (m === 'done' ? 'locked' : m));
      }, 4000);
    } catch (e) {
      sound.error();
      setTyping((t) => ({ ...t, phase: 'typing' }));
      alert(errMsg(e));
      void refresh();
    }
  };

  // teclado físico (útil em testes no computador)
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (/^[0-9]$/.test(e.key)) press(e.key);
      else if (e.key === 'Enter') void confirm();
      else if (e.key === 'Backspace' || e.key === 'Escape') correct();
      else if (e.key.toLowerCase() === 'b') blank();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const goFullscreen = () => {
    setStarted(true);
    sound.key(); // libera o áudio no iOS/Android (precisa de gesto do usuário)
    void document.documentElement.requestFullscreen?.().catch(() => undefined);
  };

  return (
    <div className="urna-page">
      <div className="urna-device">
        <div className="urna-header">
          <span className="urna-brand">URNA DIGITAL · {ballot?.election.title ?? 'Urna'}</span>
          <span className={`dot ${connected ? 'on' : 'off'}`} title={connected ? 'Conectada' : 'Reconectando…'} />
        </div>
        <UrnaScreen
          mode={mode}
          office={ballot?.election.office ?? ''}
          digits={digits}
          candidates={ballot?.candidates ?? []}
          typing={typing}
          voterName={session?.voter_name}
          message={message}
        />
        <div className="keypad">
          {['1', '2', '3', '4', '5', '6', '7', '8', '9'].map((k) => (
            <button key={k} className="key" onClick={() => press(k)} disabled={!canType}>
              {k}
            </button>
          ))}
          <span />
          <button className="key" onClick={() => press('0')} disabled={!canType}>
            0
          </button>
          <span />
          <button className="key-action branco" onClick={blank} disabled={!canType}>
            BRANCO
          </button>
          <button className="key-action corrige" onClick={correct} disabled={!canType}>
            CORRIGE
          </button>
          <button className="key-action confirma" onClick={() => void confirm()} disabled={!canType}>
            CONFIRMA
          </button>
        </div>
      </div>
      {!started && mode !== 'error' && (
        <button className="urna-start" onClick={goFullscreen}>
          Toque para iniciar a urna (tela cheia)
        </button>
      )}
    </div>
  );
}
