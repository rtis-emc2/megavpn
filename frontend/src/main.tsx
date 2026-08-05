import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './app/App';
import './styles/tokens.css';
import './styles/base.css';
import './styles/layout.css';
import './styles/components.css';

const root = document.getElementById('root');
if (!root) {
  throw new Error('MegaVPN Console root element is missing');
}

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
