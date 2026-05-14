// brun web app.js — shared utilities

// Escape HTML to prevent XSS
function esc(s) {
  if (s === null || s === undefined) return '';
  const d = document.createElement('div');
  d.textContent = String(s);
  return d.innerHTML;
}

// Format time for display
function fmtTime(s) {
  if (!s) return '-';
  try {
    const d = new Date(s);
    return d.toLocaleString('zh-CN', {
      month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit'
    });
  } catch (e) {
    return String(s);
  }
}

// Format relative time
function timeAgo(s) {
  if (!s) return '';
  const diff = Date.now() - new Date(s).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return '刚刚';
  if (mins < 60) return mins + '分钟前';
  const hours = Math.floor(mins / 60);
  if (hours < 24) return hours + '小时前';
  const days = Math.floor(hours / 24);
  return days + '天前';
}

// Debounce utility
function debounce(fn, ms) {
  let timer = null;
  return function(...args) {
    clearTimeout(timer);
    timer = setTimeout(() => fn.apply(this, args), ms);
  };
}

// Simple fetch wrapper with error handling
async function apiGet(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(`HTTP ${r.status}`);
  return r.json();
}

async function apiPost(url, body) {
  const r = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body || {})
  });
  return r.json();
}
