import type { Candidate, TypingState } from '../lib/types';

export type UrnaMode = 'loading' | 'locked' | 'voting' | 'done' | 'closed' | 'draft' | 'error';

interface Props {
  mode: UrnaMode;
  office: string;
  digits: number;
  candidates: Candidate[];
  typing: TypingState;
  voterName?: string | null;
  message?: string;
  compact?: boolean;
}

/** Tela (display) da urna. Usada no celular e no espelho do painel do mesário. */
export default function UrnaScreen({ mode, office, digits, candidates, typing, voterName, message, compact }: Props) {
  const cls = `urna-screen${compact ? ' compact' : ''}`;

  if (mode !== 'voting') {
    const big: Record<Exclude<UrnaMode, 'voting'>, string> = {
      loading: 'CARREGANDO…',
      locked: 'AGUARDE',
      done: 'FIM',
      closed: 'VOTAÇÃO ENCERRADA',
      draft: 'VOTAÇÃO NÃO INICIADA',
      error: 'URNA INDISPONÍVEL',
    };
    const sub: Partial<Record<UrnaMode, string>> = {
      locked: 'Urna bloqueada. Aguarde a liberação pelo mesário.',
      done: 'Voto registrado com sucesso.',
      closed: 'Obrigado pela participação.',
      draft: 'O mesário ainda não abriu a votação.',
    };
    return (
      <div className={cls}>
        <div className={`urna-center ${mode === 'done' ? 'fim' : ''}`}>
          <div className="urna-big">{big[mode]}</div>
          <div className="urna-sub">{message || sub[mode]}</div>
        </div>
      </div>
    );
  }

  const typed = typing.digits;
  const complete = typed.length === digits;
  const cand = complete ? candidates.find((c) => c.number === typed) : undefined;
  const isNull = complete && !cand;
  const showInstructions = typing.blank || complete;

  return (
    <div className={cls}>
      <div className="urna-body">
        <div className="urna-main">
          <div className="urna-top">SEU VOTO PARA</div>
          <div className="urna-office">{office}</div>

          {typing.blank ? (
            <div className="urna-blank">VOTO EM BRANCO</div>
          ) : (
            <>
              <div className="urna-number-row">
                <span className="urna-label">Número:</span>
                <div className="urna-boxes">
                  {Array.from({ length: digits }).map((_, i) => (
                    <span key={i} className={`urna-box ${i === typed.length ? 'cursor' : ''}`}>
                      {typed[i] ?? ''}
                    </span>
                  ))}
                </div>
              </div>
              {cand && (
                <div className="urna-cand-info">
                  <div>
                    <span className="urna-label">Nome:</span> <b>{cand.name}</b>
                  </div>
                  {cand.party && (
                    <div>
                      <span className="urna-label">Chapa:</span> {cand.party}
                    </div>
                  )}
                </div>
              )}
              {isNull && (
                <div className="urna-null">
                  <div>NÚMERO ERRADO</div>
                  <div className="urna-null-big">VOTO NULO</div>
                </div>
              )}
            </>
          )}
        </div>
        {cand && (
          <div className="urna-photo">
            {cand.photo_url ? <img src={cand.photo_url} alt={cand.name} /> : <div className="urna-photo-ph">{initials(cand.name)}</div>}
          </div>
        )}
      </div>
      {voterName && <div className="urna-voter">Eleitor(a): {voterName}</div>}
      <div className="urna-footer">
        {showInstructions ? (
          <>
            <div>Aperte a tecla:</div>
            <div>
              <b className="t-green">VERDE</b> para CONFIRMAR este voto
            </div>
            <div>
              <b className="t-orange">LARANJA</b> para REINICIAR este voto
            </div>
          </>
        ) : (
          <div>Digite o número do candidato ou aperte BRANCO</div>
        )}
      </div>
      {typing.phase === 'sending' && <div className="urna-sending">GRAVANDO…</div>}
    </div>
  );
}

const initials = (n: string) =>
  n
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join('');
