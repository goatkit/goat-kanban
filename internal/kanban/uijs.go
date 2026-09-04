package kanban

import (
	"strings"
)

// kanbanJS is the self-contained interaction layer: an Alpine component for
// the board page plus small window-level helpers used by the board list page
// and both pages' toasts. No external JS beyond the shell's htmx/Alpine.
var kanbanJS = `<script>
window.kanbanToast = function(msg, isErr) {
  let el = document.querySelector('.kb-toast');
  if (!el) return;
  el.textContent = msg;
  el.classList.toggle('err', !!isErr);
  el.style.display = 'block';
  clearTimeout(window.__kbToastT);
  window.__kbToastT = setTimeout(() => { el.style.display = 'none'; }, 4000);
};

window.kanbanBase = (function () {
  var p = location.pathname.replace(/\/+$/, '');
  var i = p.indexOf('/board/');
  return i >= 0 ? p.slice(0, i) : p;
})();

window.kanbanPost = async function(path, payload) {
  let res;
  try {
    res = await fetch(path, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(payload || {})
    });
  } catch (e) {
    return {error: 'request failed', status: 0};
  }
  try { return await res.json(); } catch (e) { return {error: 'invalid response', status: res.status}; }
};

window.kanbanCreateBoard = async function(name, queueId) {
  name = (name || '').trim();
  if (!name) { window.kanbanToast('` + jsI18n("board_name_required") + `', true); return; }
  const payload = {name: name};
  if (queueId) payload.queue_id = parseInt(queueId, 10);
  const out = await window.kanbanPost(window.kanbanBase + '/api/board/create', payload);
  if (out && out.ok) { location.reload(); return; }
  window.kanbanToast((out && out.error) || '` + jsI18n("err_generic") + `', true);
};

window.kanbanDeleteBoard = function(boardId, confirmMsg) {
  if (!confirm(confirmMsg)) return;
  window.kanbanPost(window.kanbanBase + '/api/board/delete', {board_id: boardId}).then(out => {
    if (out && out.ok) location.reload();
    else window.kanbanToast((out && out.error) || '` + jsI18n("err_generic") + `', true);
  });
};

// kanbanBoard is the Alpine component for one board page. State island:
// {boardId, isCreator, columns:[{id,name,color,pending}], cards:[...], i18n}.
window.kanbanBoard = function(state) {
  return {
    boardId: state.boardId,
    isCreator: !!state.isCreator,
    columns: (state.columns || []).map(c => Object.assign({visible: true}, c)),
    states: state.states || [],
    cards: state.cards || [],
    ticketView: state.ticketView || '',
    ticketViews: state.ticketViews || [],
    i18n: state.i18n || {},

    paletteOpen: false,
    q: '',
    paletteResults: [],
    paletteLoaded: false,
    dragOverCol: null,
    pending: {open: false, card: null, colId: 0, untilLocal: ''},
    configOpen: false,
    toast: {msg: '', err: false},
    announce: '',

    // ---- helpers ----
    cardsByState(stateId) { return this.cards.filter(c => c.state_id === stateId); },
    columnById(id) { return this.columns.find(c => c.id === id); },
    // States not in the visible column list (hidden in config, or never added).
    hiddenStates() { return this.states.filter(st => !this.columns.some(c => c.id === st.id)); },

    ageLabel(ct) {
      if (!ct) return '';
      const t = new Date(ct.replace(' ', 'T'));
      if (isNaN(t)) return '';
      const mins = Math.max(0, Math.floor((Date.now() - t.getTime()) / 60000));
      if (mins < 60) return mins + 'm';
      const hrs = Math.floor(mins / 60);
      if (hrs < 48) return hrs + 'h';
      return Math.floor(hrs / 24) + 'd';
    },
    ageClass(ct) {
      if (!ct) return '';
      const t = new Date(ct.replace(' ', 'T'));
      if (isNaN(t)) return '';
      const days = (Date.now() - t.getTime()) / 86400000;
      if (days > 30) return 'age-old';
      if (days > 7) return 'age-warn';
      return '';
    },

    ticketURL(card) {
      if (this.ticketView) {
        const v = this.ticketViews.find(v => v.ref === this.ticketView);
        if (v) return v.url_template.replace('{ticket_id}', String(card.id));
      }
      return '/ticket/' + encodeURIComponent(card.tn);
    },
    openTicket(card) { window.location.href = this.ticketURL(card); },

    // ---- palette ----
    async loadPalette() {
      const params = new URLSearchParams({board_id: this.boardId, limit: '20'});
      if (this.q) params.set('q', this.q);
      try {
        const res = await fetch(window.kanbanBase + '/api/palette?' + params.toString());
        const out = await res.json();
        if (Array.isArray(out)) { this.paletteResults = out; }
        else { window.kanbanToast((out && out.error) || 'error', true); }
      } catch (e) { /* network blip */ }
      this.paletteLoaded = true;
    },
    async addTicket(r) {
      const out = await window.kanbanPost(window.kanbanBase + '/api/board/add', {board_id: this.boardId, ticket_id: r.id});
      if (out && out.ok) {
        this.cards.push(Object.assign({}, r));
        this.paletteResults = this.paletteResults.filter(x => x.id !== r.id);
        const col = this.columnById(r.state_id);
        this.announce = 'Ticket ' + r.tn + ' added to ' + (col ? col.name : '');
      } else {
        window.kanbanToast((out && out.error) || 'error', true);
      }
    },

    // ---- drag & drop ----
    onDragStart(ev, card) {
      ev.dataTransfer.effectAllowed = 'move';
      ev.dataTransfer.setData('text/plain', String(card.id));
      const el = ev.currentTarget;
      if (el && el.classList) el.classList.add('dragging');
    },
    onDragEnd() { this.dragOverCol = null; document.querySelectorAll('.kb-card.dragging').forEach(el => el.classList.remove('dragging')); },
    setDragOver(colId) { this.dragOverCol = colId; },

    // Optimistic move: relocate the card in local state immediately, then POST.
    // On failure the previous state is restored and a toast shown.
    doMove(card, targetCol, untilTime) {
      const prevState = card.state_id;
      card.state_id = targetCol.id;
      if (untilTime) card.until_time = untilTime;
      this.announce = 'Ticket ' + card.tn + ' moved to ' + targetCol.name;
      window.kanbanPost(window.kanbanBase + '/api/move', {
        board_id: this.boardId,
        ticket_id: card.id,
        state_id: targetCol.id,
        until_time: untilTime || 0
      }).then(out => {
        if (out && out.ok) return;
        card.state_id = prevState; // revert
        delete card.until_time;
        this.announce = 'Move failed: ' + ((out && out.error) || 'move failed');
        window.kanbanToast((out && out.error) || 'move failed', true);
      });
    },

    onDrop(ev, col) {
      this.dragOverCol = null;
      const id = parseInt(ev.dataTransfer.getData('text/plain'), 10);
      const card = this.cards.find(c => c.id === id);
      if (!card) return;
      if (card.state_id === col.id) return;
      if (col.pending) { this.openPending(card, col); return; }
      this.doMove(card, col, 0);
    },

    // ---- pending due-time dialog ----
    openPending(card, col) {
      const d = new Date(Date.now() + 24 * 3600 * 1000);
      d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
      this.pending = {open: true, card: card, colId: col.id, untilLocal: d.toISOString().slice(0, 16)};
    },
    cancelPending() { this.pending.open = false; },
    confirmPending() {
      const p = this.pending;
      this.pending.open = false;
      if (!p.card) return;
      const t = new Date(p.untilLocal);
      const until = isNaN(t) ? 0 : Math.floor(t.getTime() / 1000);
      const col = this.columnById(p.colId);
      if (col) this.doMove(p.card, col, until);
    },

    // ---- keyboard [ ] move ----
    moveToAdjacent(card, dir) {
      const idx = this.columns.findIndex(c => c.id === card.state_id);
      if (idx < 0) return;
      let next = idx + dir;
      while (next >= 0 && next < this.columns.length && !this.columns[next].visible) next += dir;
      if (next < 0 || next >= this.columns.length) return;
      const col = this.columns[next];
      if (col.pending) { this.openPending(card, col); return; }
      this.doMove(card, col, 0);
    },
    // onCardKey handles [ / ] keys; Alpine has no reliable named modifiers for them.
    onCardKey(ev, card) {
      if (ev.key === '[') this.moveToAdjacent(card, -1);
      else if (ev.key === ']') this.moveToAdjacent(card, 1);
    },

    // ---- column config (hide / reorder) ----
    moveColumn(ci, dir) {
      const j = ci + dir;
      if (j < 0 || j >= this.columns.length) return;
      const a = this.columns.splice(ci, 1)[0];
      this.columns.splice(j, 0, a);
    },
    async saveConfig() {
      const states = [];
      let order = 0;
      for (const col of this.columns) {
        if (!col.visible) continue;
        order += 1;
        states.push({state_id: col.id, sort_order: order});
      }
      for (const st of this.hiddenStates()) {
        if (!st.visible) continue;
        order += 1;
        states.push({state_id: st.id, sort_order: order});
      }
      const out = await window.kanbanPost(window.kanbanBase + '/api/board/config', {board_id: this.boardId, states: states, ticket_view: this.ticketView});
      if (out && out.ok) { this.configOpen = false; location.reload(); return; }
      window.kanbanToast((out && out.error) || 'error', true);
    }
  };
};
</script>`

// jsI18n returns the English text for a key, escaped for safe interpolation
// into the JS string literals above (single quotes).
func jsI18n(key string) string {
	s := t("en", key)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return s
}
