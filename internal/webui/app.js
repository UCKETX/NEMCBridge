const $ = id => document.getElementById(id);
let state = null, loaded = false, lastEvents = '', message = '';
const phases = { stopped: '已停止', checking_upstream: '检查本地服务器', authenticating: '正在鉴权', creating: '正在创建房间', online: '房间在线', retrying: '等待自动重连', upstream_offline: '本地服务器离线' };
const MAX_ROOMS = 8;
// Token rows: `keep` is the index of a saved token (tokens never reach the page), null for a new room.
let tokenRows = [];

function notice(text, kind = '') { $('notice').textContent = text; $('notice').hidden = !text; $('notice').dataset.kind = kind; }
// Keep a message across the 2-second status refresh, which otherwise replaces it.
let messageKind = '';
function flash(text, ms = 5000, kind = '') {
  message = text;
  messageKind = kind;
  notice(text, kind);
  clearTimeout(flash.timer);
  flash.timer = setTimeout(() => { message = ''; }, ms);
}

async function api(path, data) {
  const r = await fetch('/api/' + path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(data ?? {}) });
  if (r.status === 401) { window.location.replace('/login'); throw Error('登录已过期，请重新登录'); }
  if (!r.ok) throw Error(await r.text());
  return r.json();
}

function loadForm(c) {
  $('auth-server').value = c.auth_server; $('server').value = c.upstream; $('room-name').value = c.room_name;
  $('room-game-version').value = c.room_game_version; $('password').value = c.room_password; $('max-players').value = c.max_players;
  $('friends').checked = c.friends_only; $('auto-friends').checked = c.auto_accept_friends; $('pvp').checked = c.allow_pvp; $('auto').checked = c.auto_start;
}

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function resetTokenRows(count) {
  tokenRows = count ? Array.from({ length: count }, (_, i) => ({ keep: i, value: '' })) : [{ keep: null, value: '' }];
  renderTokenRows();
}

function renderTokenRows() {
  const list = $('token-list');
  list.replaceChildren(...tokenRows.map((row, i) => {
    const item = el('div', 'token-row');
    const input = el('input');
    input.type = 'password';
    input.autocomplete = 'off';
    input.value = row.value;
    input.placeholder = row.keep === null ? '粘贴 FGAuth 机器人令牌' : '留空保留当前令牌';
    input.setAttribute('aria-label', '房间 ' + (i + 1) + ' 的机器人令牌');
    input.oninput = () => { row.value = input.value; };
    const badge = el('span', 'badge', row.keep === null ? '新房间' : '已保存');
    badge.dataset.state = row.keep === null ? 'missing' : 'ok';
    const remove = el('button', 'ghost token-remove', '移除');
    remove.type = 'button';
    remove.onclick = () => {
      tokenRows.splice(i, 1);
      if (!tokenRows.length) tokenRows.push({ keep: null, value: '' });
      renderTokenRows();
    };
    item.append(el('span', 'token-index', '房间 ' + (i + 1)), input, badge, remove);
    return item;
  }));
  syncTokenControls();
}

function syncTokenControls() {
  const running = !!state?.running;
  $('add-room').disabled = running || tokenRows.length >= MAX_ROOMS;
  // A lone empty row is the placeholder for the first room; removing it changes nothing.
  const placeholder = tokenRows.length === 1 && tokenRows[0].keep === null;
  $('token-list').querySelectorAll('input, .token-remove').forEach(x => {
    x.disabled = running || (placeholder && x.classList.contains('token-remove'));
  });
}

function renderRooms(rooms, maxPlayers) {
  const list = $('room-list');
  const multi = rooms.length > 1;
  $('room-id').hidden = multi;
  list.hidden = !multi;
  if (!multi) return;
  list.replaceChildren(...rooms.map((r, i) => {
    const tile = el('div', 'room-tile');
    tile.dataset.phase = r.phase;
    const status = el('small', 'room-tile-status');
    status.append(el('i'), document.createTextNode((phases[r.phase] ?? r.phase) + ' · ' + r.players + ' / ' + (maxPlayers - 1) + ' 人'));
    tile.append(el('span', 'room-tile-label', '房间 ' + (i + 1)), el('strong', r.room_id ? '' : 'is-empty', r.room_id || '—'), status, el('small', 'room-tile-account', r.account || '等待鉴权'));
    return tile;
  }));
}

function renderEvents(events) {
  const key = JSON.stringify(events);
  if (key === lastEvents) return;
  lastEvents = key;
  const box = $('events');
  const atBottom = box.scrollHeight - box.scrollTop - box.clientHeight < 40;
  box.replaceChildren();
  for (const e of events) {
    const row = document.createElement('div');
    row.className = 'event';
    if (e.message.startsWith('[DEBUG]')) row.classList.add('is-debug');
    else if (/失败|错误|断开|离线|超时/.test(e.message)) row.classList.add('is-warn');
    const time = document.createElement('time');
    time.textContent = e.time;
    row.append(time, document.createTextNode(e.message));
    box.append(row);
  }
  if (atBottom) box.scrollTop = box.scrollHeight;
}

const roomIDs = () => (state?.rooms?.length ? state.rooms.map(r => r.room_id) : [state?.room_id]).filter(Boolean);

async function copyText(text) {
  if (navigator.clipboard?.writeText) {
    try { await navigator.clipboard.writeText(text); return; } catch { /* Try the HTTP-compatible path below. */ }
  }
  // Clipboard API is unavailable on LAN HTTP. Run the legacy copy command
  // from the button's click, with an actual selected, focusable text field.
  const active = document.activeElement;
  const selection = document.getSelection();
  const ranges = selection ? Array.from({ length: selection.rangeCount }, (_, i) => selection.getRangeAt(i).cloneRange()) : [];
  const field = document.createElement('textarea');
  field.value = text;
  field.readOnly = true;
  field.setAttribute('aria-label', '待复制的房间号');
  Object.assign(field.style, { position: 'fixed', top: '0', left: '0', width: '1px', height: '1px', opacity: '0', fontSize: '16px' });
  document.body.append(field);
  try {
    field.focus({ preventScroll: true });
    field.select();
    field.setSelectionRange(0, text.length);
    if (!document.execCommand('copy')) throw Error('浏览器未允许复制');
  } finally {
    field.remove();
    active?.focus({ preventScroll: true });
    if (selection) {
      selection.removeAllRanges();
      ranges.forEach(range => selection.addRange(range));
    }
  }
}

async function refresh() {
  try {
    const r = await fetch('/api/status');
    if (r.status === 401) { window.location.replace('/login'); return; }
    if (!r.ok) throw Error('控制台暂不可用');
    state = await r.json();
    if (!loaded) { loadForm(state.config); resetTokenRows(state.token_count); loaded = true; }

    const rooms = state.rooms || [];
    const multi = rooms.length > 1;
    const guests = state.config.max_players - 1;
    document.body.dataset.phase = state.phase;
    $('phase').textContent = (phases[state.phase] ?? state.phase) + (multi ? ' · ' + rooms.length + ' 个房间' : '');
    $('room-id').textContent = state.room_id || '—';
    $('room-id').classList.toggle('is-empty', !state.room_id);
    renderRooms(rooms, state.config.max_players);
    $('room-note').textContent = multi ? '在网易我的世界“本地联机”中搜索任一房间号，所有房间进入同一个世界；一个房间满员时换另一个。'
      : state.room_id ? '在网易我的世界“本地联机”中搜索此房间号。' : '启动房主机器人后，房间号会显示在这里。';

    $('upstream').textContent = state.upstream_online ? '在线' : '未连接';
    $('upstream').closest('.stat').dataset.state = state.upstream_online ? 'ok' : (state.running ? 'bad' : '');
    $('version').textContent = state.config.upstream + (state.upstream_version ? ' · ' + state.upstream_version : '');
    $('players').textContent = state.players;
    $('players-note').textContent = multi ? rooms.length + ' 个房间 · 每间最多 ' + guests + ' 名网易玩家' : '房主机器人持续连接';
    if (multi) {
      $('account').textContent = rooms.length + ' 个机器人';
      $('protocol').textContent = rooms.map(r => r.account).filter(Boolean).join('、') || '等待鉴权';
    } else {
      $('account').textContent = state.account || '—';
      $('protocol').textContent = state.protocol_id ? '房间协议 ' + state.protocol_id : '等待鉴权';
    }

    const seconds = state.room_id && state.connected_at ? Math.max(0, Math.floor((Date.now() - Date.parse(state.connected_at)) / 1000)) : 0;
    $('uptime').textContent = state.room_id ? Math.floor(seconds / 3600) + '时 ' + Math.floor(seconds % 3600 / 60) + '分' : '—';
    $('retry').textContent = state.retry_at ? '将在 ' + new Date(state.retry_at).toLocaleTimeString() + ' 重试' : '自动断线重连已启用';

    $('start').disabled = state.running || !state.token_set;
    $('stop').disabled = !state.running;
    $('copy').disabled = !roomIDs().length;
    $('copy').title = multi ? '复制全部房间号，每行一个' : '';
    if (!$('copy').classList.contains('is-done')) $('copy').querySelector('[data-label]').textContent = '复制房间号';
    document.querySelectorAll('#config input,#config button').forEach(x => x.disabled = state.running);
    // Only sync button state here; rebuilding the rows would drop a token being typed.
    if (!state.running) syncTokenControls();
    $('debug').checked = !!state.config.debug;

    notice(message || state.last_error, message ? messageKind : '');
    renderEvents(state.events);
  } catch (e) {
    document.body.dataset.phase = 'disconnected';
    $('phase').textContent = '控制台连接中断';
    notice(e.message);
  }
}

$('start').onclick = async () => { try { message = ''; await api('start'); await refresh(); } catch (e) { flash(e.message); } };
$('stop').onclick = async () => { try { message = ''; await api('stop'); await refresh(); } catch (e) { flash(e.message); } };
$('copy').onclick = async () => {
  const ids = roomIDs();
  if (!ids.length) { flash('当前没有可复制的房间号'); return; }
  const button = $('copy');
  const label = button.querySelector('[data-label]');
  try {
    await copyText(ids.join('\n'));
    button.classList.add('is-done');
    label.textContent = '已复制';
    clearTimeout(button.copyTimer);
    button.copyTimer = setTimeout(() => { button.classList.remove('is-done'); refresh(); }, 1500);
  } catch { flash('浏览器未允许自动复制，请手动复制房间号：' + ids.join('、')); }
};
$('add-room').onclick = () => {
  if (tokenRows.length >= MAX_ROOMS) return;
  tokenRows.push({ keep: null, value: '' });
  renderTokenRows();
  $('token-list').lastElementChild.querySelector('input').focus();
};

$('config').onsubmit = async e => {
  e.preventDefault();
  try {
    const config = {
      ...state.config,
      auth_server: $('auth-server').value.trim(), upstream: $('server').value.trim(), room_name: $('room-name').value.trim(),
      room_game_version: $('room-game-version').value.trim(), room_password: $('password').value, max_players: Number($('max-players').value),
      friends_only: $('friends').checked, auto_accept_friends: $('auto-friends').checked, allow_pvp: $('pvp').checked, auto_start: $('auto').checked
    };
    // A typed value replaces the token; an untouched saved row keeps it; an empty new row is skipped.
    const rooms = tokenRows.map(r => r.value.trim() ? { token: r.value.trim() } : r.keep !== null ? { keep: r.keep } : null).filter(Boolean);
    await api('config', { config, rooms });
    flash('设置已保存', 3500, 'ok');
    await refresh();
    resetTokenRows(state.token_count);
  } catch (e) { flash(e.message); }
};

refresh();
setInterval(refresh, 2000);

$('logout').onclick = async () => { try { await api('logout'); window.location.replace('/login'); } catch (e) { notice(e.message); } };

$('debug').onchange = async () => {
  const input = $('debug');
  input.disabled = true;
  try { await api('debug', { enabled: input.checked }); await refresh(); }
  catch (e) { input.checked = !!state?.config.debug; notice(e.message); }
  finally { input.disabled = false; }
};
