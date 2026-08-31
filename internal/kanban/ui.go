package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// UI is built entirely in Go and shipped inside the handler's {"html"}
// response; the minimal shell provides htmx, Alpine, FontAwesome and the
// --gk-* theme variables. All JS is vanilla + Alpine — no new dependencies.

const boardListCSS = `
<style>
[x-cloak]{display:none!important;}
.kb-page{max-width:900px;margin:0 auto;}
.kb-board-card{display:flex;align-items:center;gap:12px;padding:14px 16px;border:1px solid var(--gk-border-default);border-radius:10px;background:var(--gk-bg-surface);margin-bottom:10px;text-decoration:none;color:inherit;}
.kb-board-card:hover{border-color:var(--gk-primary,#4F7CFF);}
.kb-board-name{font-weight:600;font-size:15px;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
.kb-chip{display:inline-block;padding:2px 8px;border-radius:999px;font-size:11.5px;background:var(--gk-bg-elevated);color:var(--gk-text-muted);border:1px solid var(--gk-border-default);}
.kb-board-meta{font-size:12px;color:var(--gk-text-muted);white-space:nowrap;}
.kb-del{background:none;border:none;color:var(--gk-text-muted);cursor:pointer;font-size:14px;padding:4px 6px;border-radius:6px;}
.kb-del:hover{color:#ef4444;background:var(--gk-bg-elevated);}
</style>`

const boardCSS = `
<style>
[x-cloak]{display:none!important;}
.kb-wrap{display:flex;flex-direction:column;height:calc(100vh - 112px);min-height:0;}
.kb-topbar{display:flex;align-items:center;gap:10px;padding-bottom:10px;flex-wrap:wrap;}
.kb-title{font-size:16px;font-weight:700;margin-right:auto;display:flex;align-items:center;gap:8px;}
.kb-back{color:var(--gk-text-muted);text-decoration:none;font-size:14px;}
.kb-board-row{display:flex;gap:12px;overflow-x:auto;padding-bottom:8px;flex:1;min-height:0;align-items:flex-start;}
.kb-col{width:300px;min-width:280px;max-width:320px;display:flex;flex-direction:column;background:var(--gk-bg-surface);border:1px solid var(--gk-border-default);border-radius:10px;max-height:100%;}
.kb-col-header{display:flex;align-items:center;gap:8px;padding:10px 12px;border-bottom:1px solid var(--gk-border-default);}
.kb-dot{width:10px;height:10px;border-radius:50%;flex-shrink:0;}
.kb-col-name{font-size:13.5px;font-weight:600;flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;}
.kb-count{font-size:11.5px;color:var(--gk-text-muted);background:var(--gk-bg-elevated);border-radius:999px;padding:1px 8px;}
.kb-cards{flex:1;overflow-y:auto;padding:8px;display:flex;flex-direction:column;gap:8px;min-height:60px;}
.kb-dropzone{border:2px dashed var(--gk-border-default);border-radius:8px;min-height:56px;margin-top:4px;}
.kb-col.drag-over .kb-cards{background:var(--gk-bg-elevated);}
.kb-card{background:var(--gk-bg-base);border:1px solid var(--gk-border-default);border-radius:8px;padding:10px;cursor:grab;outline:none;}
.kb-card:focus-visible{box-shadow:0 0 0 2px var(--gk-primary,#4F7CFF);}
.kb-card.dragging{opacity:.5;}
.kb-card-top{display:flex;align-items:center;gap:6px;margin-bottom:6px;}
.kb-tn{font-size:11.5px;font-weight:600;color:var(--gk-primary,#4F7CFF);text-decoration:none;background:var(--gk-bg-elevated);padding:1px 7px;border-radius:999px;}
.kb-age{margin-left:auto;font-size:11px;color:var(--gk-text-muted);}
.kb-age.age-warn{color:#d97706;font-weight:600;}
.kb-age.age-old{color:#dc2626;font-weight:600;}
.kb-title-t{font-size:13px;line-height:1.35;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;margin-bottom:8px;color:var(--gk-text-primary);}
.kb-card-bottom{display:flex;align-items:center;gap:6px;}
.kb-prio{font-size:11px;padding:1px 7px;border-radius:999px;background:var(--gk-bg-elevated);color:var(--gk-text-muted);border:1px solid var(--gk-border-default);}
.kb-assignee{margin-left:auto;font-size:11.5px;color:var(--gk-text-muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:45%;}
/* palette rail */
.kb-palette{width:36px;min-width:36px;display:flex;flex-direction:column;align-items:center;border:1px solid var(--gk-border-default);border-radius:10px;background:var(--gk-bg-surface);cursor:pointer;height:fit-content;padding:10px 4px;}
.kb-palette.open{width:280px;min-width:280px;align-items:stretch;cursor:default;}
.kb-palette-label{writing-mode:vertical-rl;font-size:12px;color:var(--gk-text-muted);letter-spacing:.05em;}
.kb-palette.open .kb-palette-label{writing-mode:horizontal-tb;margin-bottom:8px;font-weight:600;}
.kb-search{width:100%;box-sizing:border-box;padding:7px 10px;border-radius:8px;border:1px solid var(--gk-border-default);background:var(--gk-bg-base);color:var(--gk-text-primary);font-size:13px;margin-bottom:8px;}
select.kb-search option{background:var(--gk-bg-base);color:var(--gk-text-primary);}select.kb-search:focus{outline:2px solid var(--gk-primary,#4F7CFF);outline-offset:1px;}
.kb-results{display:flex;flex-direction:column;gap:6px;max-height:60vh;overflow-y:auto;}
.kb-result{padding:8px 10px;border:1px solid var(--gk-border-default);border-radius:8px;background:var(--gk-bg-base);cursor:pointer;font-size:12.5px;}
.kb-result:hover{border-color:var(--gk-primary,#4F7CFF);}
.kb-result .kb-tn{margin-right:6px;}
/* pending dialog */
.kb-pending{position:fixed;inset:0;background:rgba(0,0,0,.45);display:flex;align-items:center;justify-content:center;z-index:100;}
.kb-pending-box{background:var(--gk-bg-surface);border:1px solid var(--gk-border-default);border-radius:12px;padding:20px;width:min(360px,90vw);}
.kb-pending-box h3{margin:0 0 6px;font-size:15px;}
.kb-pending-box p{margin:0 0 14px;font-size:13px;color:var(--gk-text-muted);}
.kb-pending-box input[type=datetime-local]{width:100%;box-sizing:border-box;padding:8px 10px;border-radius:8px;border:1px solid var(--gk-border-default);background:var(--gk-bg-base);color:var(--gk-text-primary);font-size:13px;margin-bottom:14px;}
.kb-pending-actions{display:flex;justify-content:flex-end;gap:8px;}
.kb-btn{padding:7px 14px;border-radius:8px;font-size:13px;cursor:pointer;border:1px solid var(--gk-border-default);background:var(--gk-bg-base);color:var(--gk-text-primary);}
.kb-btn.primary{background:var(--gk-primary,#4F7CFF);border-color:var(--gk-primary,#4F7CFF);color:#fff;}
/* config panel */
.kb-config{position:fixed;inset:0;background:rgba(0,0,0,.45);display:flex;align-items:center;justify-content:center;z-index:100;}
.kb-config-box{background:var(--gk-bg-surface);border:1px solid var(--gk-border-default);border-radius:12px;padding:20px;width:min(420px,92vw);max-height:80vh;overflow-y:auto;}
.kb-config-row{display:flex;align-items:center;gap:8px;padding:6px 0;font-size:13.5px;}
.kb-config-row input[type=checkbox]{width:16px;height:16px;}
.kb-config-row .kb-dot{margin-left:auto;}
.kb-reorder{background:none;border:none;color:var(--gk-text-muted);cursor:pointer;font-size:12px;padding:0 4px;}
.kb-reorder:hover{color:var(--gk-text-primary);}
/* toast */
.kb-toast{position:fixed;bottom:20px;left:50%;transform:translateX(-50%);background:#1f2937;color:#fff;padding:10px 18px;border-radius:8px;font-size:13px;z-index:200;box-shadow:0 4px 16px rgba(0,0,0,.3);}
.kb-toast.err{background:#b91c1c;}
.kb-hidden{display:none!important;}
@media (max-width:640px){.kb-col{width:85vw;min-width:85vw;max-width:none;}}
</style>`

// renderBoardListHTML builds the board list page: existing boards + create form.
func renderBoardListHTML(rc reqCtx, boards []boardRow, queues []map[string]any) string {
	var b strings.Builder
	b.WriteString(boardListCSS)
	b.WriteString(`<div class="kb-page" x-data="{showForm:false, name:'', queueId:''}">`)
	fmt.Fprintf(&b, `<h2 style="font-size:18px;font-weight:700;margin-bottom:4px;">%s</h2>`, esc(t("en", "title")))
	fmt.Fprintf(&b, `<p style="color:var(--gk-text-muted);font-size:13px;margin:0 0 16px;">%s</p>`, esc(t("en", "boards_subtitle")))

	if len(boards) == 0 {
		fmt.Fprintf(&b, `<div style="padding:24px;text-align:center;color:var(--gk-text-muted);border:1px dashed var(--gk-border-default);border-radius:10px;margin-bottom:16px;">%s</div>`, esc(t("en", "no_boards")))
	}

	for _, br := range boards {
		scope := t("en", "scope_all")
		if br.QueueID != nil && br.QueueName != "" {
			scope = br.QueueName
		}
		fmt.Fprintf(&b, `<a class="kb-board-card" href="/ui/goat-kanban_board/board/%d">
  <span class="kb-board-name">%s</span>
  <span class="kb-chip">%s</span>
  <span class="kb-board-meta">%d · %s</span>`, br.ID, esc(br.Name), esc(scope), br.TicketCount, esc(br.CreateTime))
		if rc.IsAdmin || rc.UserID == br.CreatedBy {
			fmt.Fprintf(&b, `  <button class="kb-del" title="%s" onclick="event.preventDefault();event.stopPropagation();kanbanDeleteBoard(%d, '%s')">&#128465;</button>`, esc(t("en", "delete_confirm")), br.ID, escAttr(t("en", "delete_confirm")))
		}
		b.WriteString(`</a>`)
	}

	b.WriteString(`<div x-show="showForm" x-cloak style="margin-top:16px;padding:16px;border:1px solid var(--gk-border-default);border-radius:10px;background:var(--gk-bg-surface);">
  <div style="display:flex;gap:8px;flex-wrap:wrap;">
    <input x-model="name" class="kb-search" style="max-width:260px;" placeholder="` + esc(t("en", "board_name_ph")) + `" />
    <select x-model="queueId" class="kb-search" style="max-width:200px;">`)
	b.WriteString(`<option value="">` + esc(t("en", "scope_all")) + `</option>`)
	for _, q := range queues {
		fmt.Fprintf(&b, `<option value="%d">%s</option>`, toInt64(q["id"]), esc(rowStr(q["name"])))
	}
	b.WriteString(`</select>
    <button class="kb-btn primary" @click="kanbanCreateBoard(name, queueId)">` + esc(t("en", "create")) + `</button>
    <button class="kb-btn" x-show="showForm" @click="showForm=false">` + esc(t("en", "cancel")) + `</button>
  </div>
</div>`)
	fmt.Fprintf(&b, `<button class="kb-btn primary" style="margin-top:16px;" x-show="!showForm" @click="showForm=true">+ %s</button>`, esc(t("en", "new_board")))
	b.WriteString(`</div>`)
	b.WriteString(kanbanJS)
	return b.String()
}

// renderBoardHTML builds the full board page: palette rail, columns with
// cards, and all interaction JS. State is embedded as a JSON island for Alpine.
func renderBoardHTML(p *Plugin, ctx context.Context, rc reqCtx, page boardPage) string {
	var b strings.Builder
	b.WriteString(boardCSS)

	type colJSON struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Color   string `json:"color"`
		Pending bool   `json:"pending"`
		Visible bool   `json:"visible"`
	}
	cols := make([]colJSON, 0, len(page.Columns))
	for _, c := range page.Columns {
		cols = append(cols, colJSON{ID: c.StateID, Name: c.Name, Color: c.Color, Pending: c.Pending, Visible: c.Visible})
	}
	state := map[string]any{
		"boardId":     page.ID,
		"isCreator":   page.IsCreator,
		"columns":     cols,
		"cards":       page.Cards,
		"ticketView":  page.TicketView,
		"ticketViews": page.TicketViews,
		"i18n": map[string]string{
			"addTickets":    t("en", "add_tickets"),
			"searchPh":      t("en", "search_ph"),
			"unassigned":    t("en", "unassigned"),
			"pendingTitle":  t("en", "pending_title"),
			"pendingBody":   t("en", "pending_body"),
			"confirm":       t("en", "confirm"),
			"cancel":        t("en", "cancel"),
			"save":          t("en", "save"),
			"configTitle":   t("en", "config_title"),
			"noResults":     t("en", "no_results"),
			"deleteConfirm": t("en", "delete_confirm"),
			"viewLabel":     t("en", "ticket_view_label"),
			"viewStandard":  t("en", "view_standard"),
		},
	}
	stateJSON := mustJSON(state)

	b.WriteString(`<div class="kb-wrap" x-data="kanbanBoard(` + escAttr(stateJSON) + `)">`)

	// Top bar
	b.WriteString(`<div class="kb-topbar">
  <a class="kb-back" href="/ui/goat-kanban_board/">` + esc(t("en", "back")) + `</a>
  <span class="kb-title">` + esc(page.Name) + `</span>`)
	if page.IsCreator {
		b.WriteString(`<button class="kb-btn" @click="configOpen=true">` + esc(t("en", "config")) + `</button>`)
	}
	b.WriteString(`</div>`)

	// Board row: palette rail + columns
	b.WriteString(`<div class="kb-board-row">`)

	// Palette rail (virtual first column, collapsed to a slim label)
	b.WriteString(`<div class="kb-palette" :class="{open:paletteOpen}" @click="if(!paletteOpen){paletteOpen=true;loadPalette()}">
  <span class="kb-palette-label">` + esc(t("en", "add_tickets")) + `</span>
  <template x-if="paletteOpen"><div x-init="$nextTick(()=>$el.querySelector('input')?.focus())">
    <input class="kb-search" :placeholder="i18n.searchPh" x-model="q" @input.debounce.300ms="loadPalette()" />
    <div class="kb-results">
      <template x-for="r in paletteResults" :key="r.id">
        <div class="kb-result" @click="addTicket(r)">
          <a class="kb-tn" :href="ticketURL(r)" @click.stop x-text="r.tn"></a>
          <span x-text="r.title"></span>
        </div>
      </template>
      <div x-show="paletteResults.length===0 && paletteLoaded" style="color:var(--gk-text-muted);font-size:12px;padding:8px;">` + esc(t("en", "no_results")) + `</div>
    </div>
  </div></template>
</div>`)

	// Columns
	b.WriteString(`<template x-for="col in columns" :key="col.id">
  <div class="kb-col" :class="{dragOver: dragOverCol===col.id}"
       @dragover.prevent="setDragOver(col.id)"
       @dragleave="if(!event.currentTarget.contains(event.relatedTarget)) setDragOver(null)"
       @drop.prevent="onDrop($event, col)">
    <div class="kb-col-header">
      <span class="kb-dot" :style="'background:'+(col.color||'var(--gk-text-muted)')"></span>
      <span class="kb-col-name" x-text="col.name"></span>
      <span class="kb-count" x-text="cardsByState(col.id).length"></span>
    </div>
    <div class="kb-cards">
      <template x-for="card in cardsByState(col.id)" :key="card.id">` + cardHTML() + `</template>
      <div class="kb-dropzone" x-show="cardsByState(col.id).length===0"></div>
    </div>
  </div>
</template>`)
	b.WriteString(`</div>`) // board-row

	// Pending due-time dialog
	b.WriteString(`<div class="kb-pending" x-show="pending.open" x-cloak @keydown.escape.window="cancelPending()">
  <div class="kb-pending-box">
    <h3 x-text="i18n.pendingTitle"></h3>
    <p x-text="i18n.pendingBody"></p>
    <input type="datetime-local" x-model="pending.untilLocal" />
    <div class="kb-pending-actions">
      <button class="kb-btn" @click="cancelPending()" x-text="i18n.cancel"></button>
      <button class="kb-btn primary" @click="confirmPending()" x-text="i18n.confirm"></button>
    </div>
  </div>
</div>`)

	// Board config dialog (creator/admin): hide + reorder visible
	// columns, plus the ticket view override (server-rendered options so
	// it works even when zero plugins declare a view).
	b.WriteString(`<div class="kb-config" x-show="configOpen" x-cloak>
  <div class="kb-config-box">
    <h3 style="margin:0 0 12px;font-size:15px;" x-text="i18n.configTitle"></h3>
    <div class="kb-config-row">
      <label>` + esc(t("en", "ticket_view_label")) + `</label>
      <select class="kb-search" x-model="ticketView" style="max-width:220px;">
        <option value="">` + esc(t("en", "view_standard")) + `</option>`)
	for _, v := range page.TicketViews {
		b.WriteString(`<option value="` + escAttr(v.Ref) + `">` + esc(v.Label) + `</option>
`)
	}
	b.WriteString(`      </select>
    </div>
    <template x-for="(col, ci) in columns" :key="col.id">
      <div class="kb-config-row">
        <button class="kb-reorder" @click="moveColumn(ci,-1)" title="up">&#9650;</button>
        <button class="kb-reorder" @click="moveColumn(ci,1)" title="down">&#9660;</button>
        <input type="checkbox" :id="'kc'+col.id" x-model="col.visible" />
        <label :for="'kc'+col.id" x-text="col.name"></label>
        <span class="kb-dot" :style="'background:'+(col.color||'var(--gk-text-muted)')"></span>
      </div>
    </template>
    <div class="kb-pending-actions" style="margin-top:14px;">
      <button class="kb-btn" @click="configOpen=false" x-text="i18n.cancel"></button>
      <button class="kb-btn primary" @click="saveConfig()" x-text="i18n.save"></button>
    </div>
  </div>
</div>`)

	// Toast + aria-live region for screen-reader announcements
	b.WriteString(`<div class="kb-toast kb-hidden"></div>
<div aria-live="polite" style="position:absolute;left:-9999px;" x-text="announce"></div>`)

	b.WriteString(`</div>`) // kb-wrap
	b.WriteString(kanbanJS)
	return b.String()
}

// cardHTML returns the shared card markup used inside each column's x-for.
// The whole card is draggable and focusable; Enter/Space opens the ticket,
// [ / ] move it to the previous/next visible column (same pending path).
func cardHTML() string {
	return `<div class="kb-card" tabindex="0" draggable="true"
       :data-ticket-id="card.id"
       @dragstart="onDragStart($event, card)"
       @dragend="onDragEnd()"
       @click="openTicket(card)"
       @keydown.enter.prevent="openTicket(card)"
       @keydown.space.prevent="openTicket(card)"
       @keydown="onCardKey($event, card)">
  <div class="kb-card-top">
    <a class="kb-tn" :href="ticketURL(card)" @click.stop x-text="card.tn"></a>
    <span class="kb-age" :class="ageClass(card.create_time)" x-text="ageLabel(card.create_time)"></span>
  </div>
  <div class="kb-title-t" x-text="card.title"></div>
  <div class="kb-card-bottom">
    <span class="kb-prio" x-show="card.priority" x-text="card.priority"></span>
    <span class="kb-assignee" x-text="card.assignee || i18n.unassigned"></span>
  </div>
</div>`
}

// mustJSON marshals without error handling (internal data only).
func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
