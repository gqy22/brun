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

// ===== Toast 通知 =====
function toast(type, message, duration) {
  duration = duration || 2500;
  var container = document.getElementById('toast-container');
  if (!container) return;

  var icons = { success: '✓', error: '✗', info: 'i' };
  var el = document.createElement('div');
  el.className = 'toast toast-' + type;
  el.innerHTML =
    '<span class="toast-icon">' + (icons[type] || '') + '</span>' +
    '<span class="toast-message">' + esc(message) + '</span>';
  container.appendChild(el);

  setTimeout(function() {
    el.classList.add('toast-exit');
    el.addEventListener('animationend', function() { el.remove(); });
  }, duration);
}

// Format duration in ms to human-readable string
function formatDuration(ms) {
  if (!ms || ms < 0) return '-';
  if (ms < 1000) return ms + 'ms';
  var s = Math.floor(ms / 1000);
  if (s < 60) return s + 's';
  var m = Math.floor(s / 60); var sec = s % 60;
  if (m < 60) return m + 'm ' + sec + 's';
  var h = Math.floor(m / 60); var min = m % 60;
  return h + 'h ' + min + 'm';
}

// Format memory from KB
function fmtMem(kb) {
  if (!kb) return '-';
  if (kb < 1024) return kb + ' KB';
  return (kb / 1024).toFixed(1) + ' MB';
}

// Format CPU time from ms
function fmtCPU(ms) {
  if (!ms) return '-';
  if (ms < 1000) return ms + 'ms';
  return (ms / 1000).toFixed(2) + 's';
}

// Truncate string with ellipsis
function truncate(s, maxLen) {
  if (!s || s.length <= maxLen) return s || '';
  return s.substring(0, maxLen) + '...';
}

// ===== Modal 对话框 =====
function modalConfirm(title, bodyText, onConfirm) {
  var overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.innerHTML =
    '<div class="modal-box">' +
      '<div class="modal-title">' + esc(title) + '</div>' +
      '<div class="modal-body">' + esc(bodyText).replace(/\n/g, '<br>') + '</div>' +
      '<div class="modal-actions">' +
        '<button class="modal-btn-cancel">取消</button>' +
        '<button class="modal-btn-confirm">确认</button>' +
      '</div>' +
    '</div>';

  // 点击遮罩关闭
  overlay.addEventListener('click', function(e) {
    if (e.target === overlay) closeModal();
  });

  function closeModal() {
    overlay.classList.add('modal-exit');
    overlay.addEventListener('animationend', function() { overlay.remove(); });
  }

  overlay.querySelector('.modal-btn-cancel').addEventListener('click', closeModal);
  overlay.querySelector('.modal-btn-confirm').addEventListener('click', function() {
    closeModal();
    if (onConfirm) onConfirm();
  });

  document.body.appendChild(overlay);

  // ESC 关闭
  function onEsc(e) { if (e.key === 'Escape') { closeModal(); document.removeEventListener('keydown', onEsc); } }
  document.addEventListener('keydown', onEsc);
}
