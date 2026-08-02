import { useEffect } from 'react';
import { Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { Spin } from 'antd';

import { useAuthStore } from '@/stores/authStore';
import MainLayout from '@/layouts/MainLayout';
import LoginPage from '@/features/auth/LoginPage';
import DashboardPage from '@/features/dashboard/DashboardPage';
import ChainListPage from '@/features/chain/ChainListPage';
import ChainEditorPage from '@/features/chain/canvas/ChainEditorPage';
import RunLogPage from '@/features/run/RunLogPage';
import LlmConfigPage from '@/features/llm/LlmConfigPage';
import AgentPage from '@/features/agent/AgentPage';
import SkillPage from '@/features/skill/SkillPage';
import McpPage from '@/features/mcp/McpPage';
import BoardListPage from '@/features/board/BoardListPage';
import BoardPage from '@/features/board/BoardPage';
import AuditPage from '@/features/audit/AuditPage';
import CronPage from '@/features/cron/CronPage';

function RequireAuth({ children }: { children: JSX.Element }) {
  const { user, loaded, fetchMe } = useAuthStore();
  const location = useLocation();

  useEffect(() => {
    if (!loaded) fetchMe();
  }, [loaded, fetchMe]);

  if (!loaded) {
    return (
      <div style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Spin size="large" tip="加载中…" />
      </div>
    );
  }
  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  return children;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      {/* 画布编辑器：整页覆盖，不套 MainLayout */}
      <Route
        path="/chains/:id/edit"
        element={
          <RequireAuth>
            <ChainEditorPage />
          </RequireAuth>
        }
      />
      <Route
        path="/"
        element={
          <RequireAuth>
            <MainLayout />
          </RequireAuth>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="chains" element={<ChainListPage />} />
        <Route path="runs" element={<RunLogPage />} />
        <Route path="agents" element={<AgentPage />} />
        <Route path="skills" element={<SkillPage />} />
        <Route path="mcp" element={<McpPage />} />
        <Route path="boards" element={<BoardListPage />} />
        <Route path="boards/:id" element={<BoardPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="cron" element={<CronPage />} />
        <Route path="settings/llm" element={<LlmConfigPage />} />
        {/* 后续里程碑页面占位 */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
