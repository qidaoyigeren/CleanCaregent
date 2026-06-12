import { Link, useLocation } from 'react-router-dom';
import ConversationList from '../chat/ConversationList';

export default function Header() {
  const location = useLocation();

  const isActive = (path: string) => {
    if (path === '/chat') return location.pathname.startsWith('/chat');
    if (path === '/admin') return location.pathname.startsWith('/admin');
    return false;
  };

  return (
    <header className="app-header">
      <Link to="/chat" className="app-header__logo" style={{ textDecoration: 'none' }}>
        {'♦'} CleanCare Agent
      </Link>

      <nav>
        <ul className="app-header__nav">
          <li>
            <Link to="/chat" className={isActive('/chat') ? 'active' : ''}>
              Chat
            </Link>
          </li>
          <li>
            <Link to="/admin" className={isActive('/admin') ? 'active' : ''}>
              Admin
            </Link>
          </li>
        </ul>
      </nav>

      <div className="app-header__right">
        <ConversationList />
      </div>
    </header>
  );
}
