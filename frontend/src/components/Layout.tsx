import { Link, Outlet } from 'react-router-dom';
import { useAuth } from '../lib/auth';
import type { ElectionStatus } from '../lib/types';

export default function Layout() {
  const { user, logout } = useAuth();
  return (
    <div className="app">
      <header className="topbar">
        <Link to="/" className="brand">
          🗳️ Urna Digital
        </Link>
        <div className="topbar-right">
          <span className="muted">{user?.name}</span>
          <button className="btn ghost sm" onClick={logout}>
            Sair
          </button>
        </div>
      </header>
      <main className="container">
        <Outlet />
      </main>
    </div>
  );
}

const LABEL: Record<ElectionStatus, string> = { draft: 'Em preparação', open: 'Votação aberta', closed: 'Encerrada' };

export function StatusBadge({ status }: { status: ElectionStatus }) {
  return <span className={`badge ${status}`}>{LABEL[status]}</span>;
}
