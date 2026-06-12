import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from './components/layout/Layout';
import ChatPage from './pages/ChatPage';
import TraceDetailPage from './pages/TraceDetailPage';
import AdminDashboard from './pages/AdminDashboard';
import KnowledgeBasePage from './pages/KnowledgeBasePage';
import EvalPage from './pages/EvalPage';
import PromptPage from './pages/PromptPage';
import EmptyState from './components/ui/EmptyState';

function NotFound() {
  return (
    <div style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <EmptyState icon="🔗" message="Page not found. The URL may be incorrect or the page has been moved." />
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/chat" replace />} />
          <Route path="chat" element={<ChatPage />} />
          <Route path="chat/:conversationId" element={<ChatPage />} />
          <Route path="traces/:traceId" element={<TraceDetailPage />} />
          <Route path="admin" element={<AdminDashboard />} />
          <Route path="admin/kb" element={<KnowledgeBasePage />} />
          <Route path="admin/eval" element={<EvalPage />} />
          <Route path="admin/prompts" element={<PromptPage />} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
