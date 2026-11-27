# Urna Eletrônica Digital

Sistema de votação para grupos (escolha de representante) com **urna no celular**, **mesa receptora no computador** e **apuração com auditoria de quem votou em quem**.

- **Backend:** Go 1.24 + Gin + gorilla/websocket + MySQL 8
- **Frontend:** React 18 + TypeScript + Vite (servido por Nginx)
- **Deploy:** imagens no GHCR (`ghcr.io/jamersoncarlos/urna-*`), Traefik na rede `agresernet`, CI/CD no GitHub Actions

## Como funciona

```
 Computador (mesário)                       Celular (urna)
 ─────────────────────                      ──────────────
 1. Cadastra candidatos e eleitores
 2. Abre a votação
 3. Gera o link da urna  ── QR Code ──▶  abre /urna/<token>  → "AGUARDE"
 4. Clica "Liberar" no eleitor ── WS ──▶  urna desbloqueia com o nome do eleitor
 5. Vê ao vivo o que é digitado ◀── WS ──  eleitor digita / BRANCO / CORRIGE
 6.                                        CONFIRMA → POST /vote → "FIM"
 7. Lista atualiza: "Votou"  ◀── WS ──────  voto gravado (transação + hash)
 8. Encerra a votação → resultado, vencedor, quem votou em quem, CSV
```

Regras da urna (iguais às da urna oficial):

| Ação do eleitor | Resultado |
|---|---|
| Digita número de candidato cadastrado + CONFIRMA | voto no candidato |
| Digita número inexistente + CONFIRMA | **voto nulo** (o número digitado fica registrado) |
| BRANCO (sem ter digitado nada) + CONFIRMA | voto em branco |
| CORRIGE | limpa a tela |

## Candidatos

- **Número automático:** gerado na ordem de inclusão, de 1 a 99 (com zeros à esquerda conforme os dígitos da eleição; com 1 dígito, o limite é 9). O número não muda na edição. Se o limite for atingido, o menor número livre é reaproveitado.
- **Foto:** upload para o Cloudinary (JPG, PNG ou WEBP, até 5 MB). Configure `CLOUDINARY_URL=cloudinary://API_KEY:API_SECRET@CLOUD_NAME` no backend (`CLOUDINARY_FOLDER` é opcional). Sem ela, ainda é possível colar a URL da foto.
- **PDF:** o botão "Baixar PDF" gera cards com foto, primeiro e segundo nome e número (A4, 3 colunas x 6 linhas, 18 por página).

## Desfazer voto

Na mesa receptora, marque "mostrar quem já votou" e use **Desfazer voto**. Só funciona com a votação aberta. O voto anterior não é apagado: é marcado como anulado, deixa de contar, e a anulação entra na cadeia de hash. A apuração lista quem teve o voto desfeito, quando e por quem (sem revelar o voto anulado).

## Reabrir a urna

Em uma votação encerrada, **Reabrir urna** volta o status para aberta. Os votos já registrados são mantidos e quem já votou continua marcado. O link antigo continua inválido: gere um novo na mesa receptora. Como o resultado fica visível após o encerramento, cada reabertura é registrada e exibida na apuração.

## Garantias implementadas

- **Um voto válido por eleitor:** índice único sobre os votos não anulados + verificação na liberação.
- **Uma liberação por vez:** a liberação trava a linha da eleição (`SELECT … FOR UPDATE`); só existe uma sessão `active` por eleição.
- **Voto só com liberação:** o `POST /vote` exige a `session_id` ativa; após gravar, a sessão vira `completed` (não dá para reaproveitar).
- **Integridade (cadeia de hash):** cada voto guarda `hash = SHA-256(hash_anterior | dados do voto)`. Na apuração a cadeia é recalculada — qualquer `UPDATE`/`DELETE` feito direto no banco aparece como **"Violado"** indicando o voto.
- **Link da urna:** token aleatório de 192 bits; no banco fica só o hash SHA-256. Gerar um novo link derruba a urna antiga; encerrar a votação invalida o link.
- **Resultado só após encerrar** (não há parcial durante a votação).
- JWT (HS256) para o mesário, bcrypt (custo 12), rate limit no login, CORS restrito ao `FRONTEND_URL`, checagem de `Origin` no WebSocket, headers OWASP no Nginx e na API.

> **Sobre o sigilo do voto:** por requisito, o sistema associa cada voto ao eleitor e permite ver quem votou em quem após o encerramento. Deixe isso claro para os participantes antes da votação.

## Estrutura

```
urna/
├── backend/
│   ├── cmd/api/            # servidor HTTP + WebSocket
│   ├── cmd/seed/           # migrations + usuário mesário (+ demo opcional)
│   ├── internal/
│   │   ├── auth/           # JWT, bcrypt, tokens
│   │   ├── config/
│   │   ├── database/       # conexão + migrations embutidas (migrations/*.sql)
│   │   ├── handlers/       # rotas REST/WS (eleições, candidatos, eleitores, sessões, urna, resultados)
│   │   ├── middleware/
│   │   └── realtime/       # hub WebSocket por eleição (mesários + urna)
│   ├── Dockerfile
│   └── Makefile
├── frontend/
│   ├── src/
│   │   ├── components/UrnaScreen.tsx   # tela da urna (usada no celular E no espelho do mesário)
│   │   ├── pages/UrnaPage.tsx          # urna mobile (teclado, sons, wake lock, tela cheia)
│   │   └── pages/tabs/                 # Candidatos, Eleitores, Mesa receptora, Resultado
│   ├── nginx.conf
│   ├── entrypoint.sh       # gera env-config.js em runtime (VITE_API_URL)
│   └── Dockerfile
├── docker-compose.yml
├── .env.example
└── .github/workflows/deploy.yml
```

## Rodando localmente

```bash
# MySQL
docker run -d --name urna-mysql -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=urna mysql:8.4

# Backend
cd backend
go mod tidy                     # gera o go.sum (commite-o)
export DB_PASSWORD=root ADMIN_EMAIL=admin@local ADMIN_PASSWORD=admin12345 SEED_DEMO=true
make seed                       # migrations + mesário + eleição de demonstração
FRONTEND_URL=http://localhost:5173 make run

# Frontend
cd ../frontend
npm install
VITE_API_URL=http://localhost:8080 npm run dev
```

Acesse `http://localhost:5173`. Para testar a urna no celular na mesma rede, use o IP da máquina em `FRONTEND_URL`/`VITE_API_URL` (ex.: `http://192.168.0.10:5173`). No computador, a urna também aceita o teclado físico (números, `B` = branco, `Backspace` = corrige, `Enter` = confirma).

## Deploy (VPS + Traefik)

1. Ajuste os domínios no `docker-compose.yml` (padrão: `urna.agreser.tech` e `api-urna.agreser.tech`) e o `FRONTEND_URL`.
2. Cadastre os secrets do repositório: `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`, `VPS_PORT` (opcional), `GHCR_TOKEN`, `JWT_SECRET`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `MYSQL_ROOT_PASSWORD`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `CLOUDINARY_URL`.
3. Push na `main` → testes (Go + MySQL de serviço, build do front) → imagens no GHCR → `docker compose pull && up -d` na VPS em `/opt/apps/urna`.

O `urna-seed` roda a cada deploy e é idempotente (aplica migrations novas e só cria o mesário se ele não existir). Se você já tem um MySQL compartilhado na `agresernet`, remova o serviço `urna-mysql` e aponte `DB_HOST` para ele.

O WebSocket passa pelo Traefik sem configuração extra. Se o domínio estiver atrás do proxy da Cloudflare, mantenha **WebSockets** habilitado (Network → WebSockets).

## API

Autenticadas com `Authorization: Bearer <jwt>` (WS: `?access_token=`).

| Método | Rota | Descrição |
|---|---|---|
| POST | `/api/auth/login` | login do mesário |
| GET | `/api/auth/me` · PUT `/api/auth/password` | perfil / troca de senha |
| GET/POST | `/api/elections` | listar / criar |
| GET/PUT/DELETE | `/api/elections/:id` | detalhar / editar / excluir |
| POST | `/api/elections/:id/open` · `/close` · `/reopen` | abrir / encerrar / reabrir |
| POST | `/api/elections/:id/urna-link` | gera novo link da urna |
| GET/POST/PUT/DELETE | `/api/elections/:id/candidates[/:cid]` | candidatos (só em preparação) |
| GET/POST/PUT/DELETE | `/api/elections/:id/voters[/:vid]` | eleitores (`?q=`, `?status=voted\|pending`) |
| POST | `/api/elections/:id/voters/:vid/undo-vote` | desfaz o voto (só com a votação aberta) |
| POST | `/api/uploads/candidate-photo` | upload da foto (multipart, campo `file`) |
| POST | `/api/elections/:id/voters/import` | importação em lote `{ "text": "Nome;Id\n…" }` |
| GET/POST/DELETE | `/api/elections/:id/sessions[/active]` | liberação da urna |
| GET | `/api/elections/:id/results` | apuração + integridade (após encerrar) |
| GET | `/api/elections/:id/results/votes` | quem votou em quem (`?candidate_id=`, `?kind=blank\|null`) |
| WS | `/api/elections/:id/ws` | eventos ao vivo para o mesário |

Urna (pública, autenticada pelo token do link): `GET /api/urna/:token/ballot`, `GET /api/urna/:token/state`, `POST /api/urna/:token/vote`, `WS /api/urna/:token/ws`.

Eventos WebSocket: `urna_status`, `urna_typing`, `session_started`, `session_cancelled`, `vote_registered`, `vote_annulled`, `election_status`, `revoked`, `replaced`.

## Testes

```bash
cd backend && make test         # unitários (hash, validações, rate limit)
cd frontend && npm run build    # typecheck + build
```

## Telas

| Urna (celular) | Mesa receptora | Resultado |
|---|---|---|
| ![](docs/screenshots/urna-celular.png) | ![](docs/screenshots/mesa-receptora.png) | ![](docs/screenshots/resultado.png) |
