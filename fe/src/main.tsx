import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)

// registerServiceWorker 注册 PWA service worker。
function registerServiceWorker() {
  if (!('serviceWorker' in navigator)) {
    return
  }
  window.addEventListener('load', () => {
    const scriptURL = new URL('./sw.js', window.location.href).toString()
    void navigator.serviceWorker.register(scriptURL, { scope: './' })
  })
}

registerServiceWorker()
