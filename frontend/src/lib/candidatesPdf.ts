import type { Candidate, Election } from './types';

// proporção da foto na urna (4:5)
const PHOTO_W = 400;
const PHOTO_H = 500;

/** Baixa a foto e a recorta em JPEG 4:5. Devolve null se não for possível (link quebrado ou sem CORS). */
async function loadPhoto(url: string): Promise<string | null> {
  try {
    // 'reload' evita reaproveitar do cache uma resposta obtida sem CORS pela <img> da lista
    const res = await fetch(url, { mode: 'cors', cache: 'reload', signal: AbortSignal.timeout(15000) });
    if (!res.ok) return null;
    const bmp = await createImageBitmap(await res.blob(), { imageOrientation: 'from-image' });
    const canvas = document.createElement('canvas');
    canvas.width = PHOTO_W;
    canvas.height = PHOTO_H;
    const ctx = canvas.getContext('2d')!;
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, PHOTO_W, PHOTO_H);
    const scale = Math.max(PHOTO_W / bmp.width, PHOTO_H / bmp.height);
    const w = bmp.width * scale;
    const h = bmp.height * scale;
    ctx.drawImage(bmp, (PHOTO_W - w) / 2, (PHOTO_H - h) / 2, w, h);
    bmp.close();
    return canvas.toDataURL('image/jpeg', 0.88);
  } catch {
    return null;
  }
}

/** Primeiro e segundo nome. Partículas acompanham a palavra seguinte ("da Conceição"). */
function shortName(full: string): string[] {
  const [first = '', ...rest] = full.trim().split(/\s+/);
  const end = rest.findIndex((w) => !/^(d[aeo]s?|e)$/i.test(w));
  const second = rest.slice(0, end + 1).join(' ');
  return second ? [first, second] : [first];
}

/** Escreve em uma linha, reduzindo a fonte (e por fim cortando) até caber na largura. */
function fitText(doc: import('jspdf').jsPDF, text: string, x: number, y: number, maxW: number, size: number) {
  doc.setFontSize(size);
  while (size > 8 && doc.getTextWidth(text) > maxW) doc.setFontSize(--size);
  if (doc.getTextWidth(text) > maxW) {
    while (text.length > 1 && doc.getTextWidth(`${text}...`) > maxW) text = text.slice(0, -1);
    text += '...';
  }
  doc.text(text, x, y);
}

/**
 * Gera e baixa o PDF com foto, número e nome de cada candidato (A4, 3 colunas x 6 linhas,
 * 18 por página).
 * Devolve os nomes dos candidatos cuja foto não pôde ser carregada.
 */
export async function downloadCandidatesPdf(election: Election, candidates: Candidate[]): Promise<string[]> {
  const { jsPDF } = await import('jspdf'); // carregado só quando usado
  const photos = await Promise.all(candidates.map((c) => (c.photo_url ? loadPhoto(c.photo_url) : null)));

  const doc = new jsPDF({ unit: 'mm', format: 'a4' });
  const pageW = doc.internal.pageSize.getWidth();
  const pageH = doc.internal.pageSize.getHeight();
  const margin = 10;
  const gap = 4;
  const cols = 3;
  const rows = 6;
  const perPage = cols * rows;
  const top = 28;
  const cardW = (pageW - margin * 2 - gap * (cols - 1)) / cols;
  const cardH = (pageH - top - margin - gap * (rows - 1)) / rows;
  const pad = 2.5;
  const imgH = cardH - pad * 2;
  const imgW = (imgH * PHOTO_W) / PHOTO_H;
  const textW = cardW - imgW - pad * 3;

  const header = () => {
    doc.setTextColor(20);
    doc.setFont('helvetica', 'bold').setFontSize(15);
    doc.text(doc.splitTextToSize(election.title, pageW - margin * 2)[0], pageW / 2, 14, { align: 'center' });
    doc.setFont('helvetica', 'normal').setFontSize(10);
    doc.text(doc.splitTextToSize(`Candidatos a ${election.office}`, pageW - margin * 2)[0], pageW / 2, 21, { align: 'center' });
  };

  candidates.forEach((c, i) => {
    if (i % perPage === 0) {
      if (i > 0) doc.addPage();
      header();
    }
    const pos = i % perPage;
    const x = margin + (pos % cols) * (cardW + gap);
    const y = top + Math.floor(pos / cols) * (cardH + gap);

    doc.setDrawColor(200);
    doc.roundedRect(x, y, cardW, cardH, 2, 2, 'S');

    // foto à esquerda
    const photo = photos[i];
    if (photo) {
      doc.addImage(photo, 'JPEG', x + pad, y + pad, imgW, imgH);
    } else {
      doc.setFillColor(232, 235, 242);
      doc.rect(x + pad, y + pad, imgW, imgH, 'F');
      doc.setFont('helvetica', 'normal').setFontSize(8).setTextColor(110);
      doc.text('sem foto', x + pad + imgW / 2, y + cardH / 2, { align: 'center' });
    }

    // à direita: primeiro nome, segundo nome e o número abaixo, centralizados na altura
    const tx = x + pad * 2 + imgW;
    const name = shortName(c.name);
    const lineH = 5;
    const numberGap = 10;
    const baseline = y + (cardH - ((name.length - 1) * lineH + numberGap)) / 2 + 1.5;

    doc.setFont('helvetica', 'bold').setTextColor(20);
    name.forEach((line, n) => fitText(doc, line, tx, baseline + n * lineH, textW, 12));
    doc.setFontSize(26);
    doc.text(c.number, tx, baseline + (name.length - 1) * lineH + numberGap);
  });

  const slug = election.title
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');
  doc.save(`candidatos-${slug || election.id}.pdf`);

  return candidates.filter((c, i) => c.photo_url && !photos[i]).map((c) => c.name);
}
