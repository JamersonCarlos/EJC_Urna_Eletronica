import { Navigate, Route, Routes } from 'react-router-dom';
import Layout from './components/Layout';
import { useAuth } from './lib/auth';
import ElectionPage from './pages/ElectionPage';
import ElectionsPage from './pages/ElectionsPage';
import LoginPage from './pages/LoginPage';
import UrnaPage from './pages/UrnaPage';
import type { ReactNode } from 'react';

function Private({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return <div className="center-screen muted">Carregando…</div>;
  return user ? <>{children}</> : <Navigate to="/login" replace />;
}

export default function App() {
  return (
    <Routes>
      <Route path="/urna/:token" element={<UrnaPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <Private>
            <Layout />
          </Private>
        }
      >
        <Route path="/" element={<ElectionsPage />} />
        <Route path="/eleicoes/:id" element={<ElectionPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
