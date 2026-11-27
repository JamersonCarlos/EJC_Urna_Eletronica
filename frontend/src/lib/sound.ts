// Sons sintetizados via Web Audio (sem arquivos de áudio).
let ctx: AudioContext | null = null;

function audio() {
  if (!ctx) {
    const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
    // iPhone (iOS 17+): toca mesmo com a chave de silencioso ligada
    const session = (navigator as Navigator & { audioSession?: { type: string } }).audioSession;
    if (session) session.type = 'playback';
    ctx = new AC();
  }
  // além de "suspended", o iOS usa "interrupted" depois de bloquear a tela
  if (ctx.state !== 'running') void ctx.resume().catch(() => undefined);
  return ctx;
}

function tone(freq: number, start: number, dur: number, gain = 0.15) {
  const a = audio();
  const osc = a.createOscillator();
  const g = a.createGain();
  osc.type = 'square';
  osc.frequency.value = freq;
  g.gain.setValueAtTime(gain, a.currentTime + start);
  g.gain.exponentialRampToValueAtTime(0.0001, a.currentTime + start + dur);
  osc.connect(g).connect(a.destination);
  osc.start(a.currentTime + start);
  osc.stop(a.currentTime + start + dur + 0.02);
}

export const sound = {
  key: () => {
    try {
      tone(1200, 0, 0.07, 0.15);
    } catch {
      /* sem áudio */
    }
  },
  confirm: () => {
    try {
      // sequência curta de bipes ao concluir o voto
      [0, 0.12, 0.24, 0.36, 0.48].forEach((t, i) => tone(i % 2 ? 1500 : 1900, t, 0.1, 0.2));
      tone(1700, 0.62, 0.35, 0.2);
    } catch {
      /* sem áudio */
    }
  },
  error: () => {
    try {
      tone(300, 0, 0.25, 0.12);
    } catch {
      /* sem áudio */
    }
  },
};
