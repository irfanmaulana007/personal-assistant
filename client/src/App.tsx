import { useAuth } from './hooks/useAuth';
import { Login } from './components/Login';
import { Layout } from './components/Layout';
import { Chat } from './components/Chat';

function App() {
  const { authenticated, login, logout, error, loading } = useAuth();

  if (!authenticated) {
    return <Login onLogin={login} error={error} loading={loading} />;
  }

  return (
    <Layout onLogout={logout}>
      <Chat />
    </Layout>
  );
}

export default App;
