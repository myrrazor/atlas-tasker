package render

import (
	"fmt"
	"html"
	"strings"
)

// BoardAppCSS is shared by the self-contained chat fragment and the MCP App.
// Host theme variables take precedence; the defaults remain readable on their own.
const BoardAppCSS = `.atlas-board{--bw-light-surface:#fff;--bw-light-surface-muted:#f4f6f8;--bw-light-text:#1d2935;--bw-light-muted:#526170;--bw-light-border:#d8e0e7;--bw-light-accent:#365f88;--bw-light-shadow:0 8px 22px rgba(20,37,52,.08);--bw-surface:var(--hatch-widget-surface,var(--bw-light-surface));--bw-surface-muted:var(--hatch-widget-surface-muted,var(--bw-light-surface-muted));--bw-text:var(--hatch-widget-text,var(--bw-light-text));--bw-muted:var(--hatch-widget-muted,var(--bw-light-muted));--bw-border:var(--hatch-widget-border,var(--bw-light-border));--bw-accent:var(--hatch-widget-accent,var(--bw-light-accent));--bw-shadow:var(--hatch-widget-shadow,var(--bw-light-shadow));box-sizing:border-box;width:100%;max-width:100%;min-width:0;padding:.4rem;color:var(--bw-text);font-family:var(--hatch-widget-font,system-ui,-apple-system,"Segoe UI",sans-serif);line-height:1.45}
.atlas-board *{box-sizing:border-box;min-width:0}
@media(prefers-color-scheme:dark){.atlas-board{--bw-light-surface:#151c25;--bw-light-surface-muted:#1f2935;--bw-light-text:#edf2f7;--bw-light-muted:#aab9c7;--bw-light-border:#34414f;--bw-light-accent:#8eb8ed;--bw-light-shadow:0 8px 22px rgba(0,0,0,.22)}}
:root[data-theme=dark] .atlas-board{--bw-light-surface:#151c25;--bw-light-surface-muted:#1f2935;--bw-light-text:#edf2f7;--bw-light-muted:#aab9c7;--bw-light-border:#34414f;--bw-light-accent:#8eb8ed;--bw-light-shadow:0 8px 22px rgba(0,0,0,.22)}
:root[data-theme=light] .atlas-board{--bw-light-surface:#fff;--bw-light-surface-muted:#f4f6f8;--bw-light-text:#1d2935;--bw-light-muted:#526170;--bw-light-border:#d8e0e7;--bw-light-accent:#365f88;--bw-light-shadow:0 8px 22px rgba(20,37,52,.08)}
.atlas-board .board-heading{display:flex;align-items:end;justify-content:space-between;gap:.65rem;flex-wrap:wrap;margin:0 0 1rem;padding:0 0 .8rem;border-bottom:1px solid var(--bw-border)}
.atlas-board .board-eyebrow{display:block;margin:0 0 .2rem;color:var(--bw-muted);font-size:.68rem;font-weight:700;letter-spacing:.15em;text-transform:uppercase}
.atlas-board h1{margin:0;font-size:clamp(1.2rem,2.5vw,1.65rem);line-height:1.16;letter-spacing:-.035em;font-weight:720;overflow-wrap:anywhere}
.atlas-board .board-total{color:var(--bw-muted);font-size:.81rem;font-variant-numeric:tabular-nums;white-space:nowrap}
.atlas-board .board-note{margin:.15rem 0 .6rem;color:var(--bw-muted);font-size:.8rem;overflow-wrap:anywhere}
.atlas-board .lanes{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,14.4rem),1fr));align-items:start;gap:.65rem;width:100%;max-width:100%}
.atlas-board .lane{padding:.55rem;border:1px solid var(--bw-border);border-radius:12px;background:var(--bw-surface-muted);min-width:0}
.atlas-board .lane-head{display:flex;align-items:center;justify-content:space-between;gap:.45rem;padding:.18rem .22rem .55rem;border-bottom:1px solid var(--bw-border)}
.atlas-board h2{margin:0;font-size:.76rem;font-weight:700;line-height:1.25;letter-spacing:.055em;text-transform:uppercase;overflow-wrap:anywhere}
.atlas-board .lane-count{display:inline-block;min-width:1.45rem;padding:.08rem .35rem;border:1px solid var(--bw-border);border-radius:999px;color:var(--bw-muted);text-align:center;font-size:.7rem;font-weight:700;font-variant-numeric:tabular-nums}
.atlas-board .lane-list{display:grid;gap:.4rem;padding-top:.5rem}
.atlas-board .lane-empty{margin:.28rem .2rem .45rem;color:var(--bw-muted);font-size:.76rem}
.atlas-board .card{display:block;margin:0;border:1px solid var(--bw-border);border-radius:9px;background:var(--bw-surface);box-shadow:var(--bw-shadow);overflow:hidden}
.atlas-board .card[open]{border-color:var(--bw-accent)}
.atlas-board .card summary{display:block;padding:.65rem .7rem;cursor:pointer;list-style:none;position:relative}
.atlas-board .card summary::-webkit-details-marker{display:none}
.atlas-board .card summary:focus-visible{outline:2px solid var(--bw-accent);outline-offset:-3px;border-radius:8px}
.atlas-board .card summary::after{content:"";position:absolute;right:.78rem;top:.84rem;width:.36rem;height:.36rem;border-right:1.5px solid var(--bw-muted);border-bottom:1.5px solid var(--bw-muted);transform:rotate(45deg)}
.atlas-board .card[open] summary::after{transform:rotate(225deg);top:1rem}
.atlas-board .card-id{display:block;padding-right:.8rem;color:var(--bw-muted);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.7rem;font-weight:650;letter-spacing:.02em;overflow-wrap:anywhere}
.atlas-board .card-title{display:block;margin:.28rem 0 .4rem;padding-right:.55rem;font-size:.87rem;font-weight:680;line-height:1.32;letter-spacing:-.012em;overflow-wrap:anywhere}
.atlas-board .card-meta,.atlas-board .card-pills,.atlas-board .card-blockers{display:flex;flex-wrap:wrap;align-items:center;gap:.27rem;min-width:0}
.atlas-board .card-meta{margin-bottom:.35rem;color:var(--bw-muted);font-size:.69rem}
.atlas-board .priority-dot{flex:none;width:.46rem;height:.46rem;border-radius:50%;background:#8793a0}
.atlas-board .priority-critical,.atlas-board .priority-high{background:#cf4d48}
.atlas-board .priority-medium{background:#c88a20}
.atlas-board .priority-low{background:#8793a0}
.atlas-board .pill{display:inline-block;max-width:100%;padding:.09rem .37rem;border:1px solid var(--bw-border);border-radius:999px;background:var(--bw-surface-muted);color:var(--bw-muted);font-size:.68rem;line-height:1.25;overflow-wrap:anywhere}
.atlas-board .pill-type{color:var(--bw-accent)}
.atlas-board .card-blockers{margin-top:.36rem}
.atlas-board .pill-blocked{color:#a83b37;border-color:#c88683}
@media(prefers-color-scheme:dark){.atlas-board .pill-blocked{color:#ffaaa5;border-color:#ad5f5a}}
:root[data-theme=dark] .atlas-board .pill-blocked{color:#ffaaa5;border-color:#ad5f5a}
.atlas-board .card-detail{padding:.7rem;border-top:1px solid var(--bw-border);font-size:.76rem;overflow-wrap:anywhere}
.atlas-board .card-detail h3{margin:.1rem 0 .26rem;color:var(--bw-muted);font-size:.68rem;line-height:1.3;font-weight:700;letter-spacing:.075em;text-transform:uppercase}
.atlas-board .card-detail p{margin:0 0 .65rem;white-space:pre-wrap}
.atlas-board .card-detail ul{margin:.05rem 0 .65rem;padding-left:1.12rem}
.atlas-board .card-detail li+li{margin-top:.25rem}
.atlas-board .card-detail dl{display:grid;grid-template-columns:auto minmax(0,1fr);gap:.15rem .55rem;margin:0}
.atlas-board .card-detail dt{color:var(--bw-muted)}
.atlas-board .card-detail dd{margin:0;overflow-wrap:anywhere}
@media(max-width:390px){.atlas-board .lanes{grid-template-columns:minmax(0,1fr)}.atlas-board .board-heading{align-items:start}}
@media(prefers-reduced-motion:reduce){.atlas-board *{scroll-behavior:auto!important;animation:none!important;transition:none!important}}
.atlas-board .view-switch{display:none;position:relative;margin:.1rem 0 .7rem;cursor:pointer}
.atlas-board .view-switch .view-toggle{position:absolute;width:1px;height:1px;margin:0;opacity:0}
.atlas-board .view-switch span{display:inline-block;padding:.3rem .75rem;border:1px solid var(--bw-border);border-radius:999px;background:var(--bw-surface);color:var(--bw-muted);font-size:.72rem;font-weight:650;user-select:none;-webkit-user-select:none}
.atlas-board .view-switch .view-toggle:focus-visible+span{outline:2px solid var(--bw-accent);outline-offset:2px}
.atlas-board .view-switch .view-toggle:checked+span{border-color:var(--bw-accent);color:var(--bw-accent)}
.atlas-board .lanes:focus-visible{outline:2px solid var(--bw-accent);outline-offset:-2px}
@supports selector(:has(*)){.atlas-board .view-switch{display:inline-flex}.atlas-board .view-switch:has(.view-toggle:checked)~.lanes{grid-template-columns:none;grid-auto-flow:column;grid-auto-columns:minmax(12.5rem,15rem);overflow-x:auto;align-items:stretch;padding-bottom:.45rem}.atlas-board .view-switch:has(.view-toggle:checked)~.lanes .lane{min-height:9rem}}`

// CompactBoardWidgetFragment is one self-contained HTML fragment for chat hosts
// that render HTML in a message. It has no file, network, font, or script dependency.
func CompactBoardWidgetFragment(board CompactBoard) string {
	return "<style>\n" + BoardAppCSS + "\n</style>\n" + CompactBoardAppBody(board)
}

func boardWidgetBody(board CompactBoard) string {
	var b strings.Builder
	b.WriteString(`<section class="atlas-board" aria-label="Atlas Tasker board">`)
	b.WriteString(`<header class="board-heading"><div><span class="board-eyebrow">Atlas Tasker / Board</span><h1>`)
	b.WriteString(html.EscapeString(board.Title))
	b.WriteString(`</h1></div><span class="board-total">`)
	b.WriteString(html.EscapeString(ticketCountPhrase(board.TotalCards, board.ShownCards, board.Truncated)))
	b.WriteString(`</span></header>`)
	b.WriteString(`<p class="board-note">Select a card to expand its details.</p>`)
	if board.Truncated {
		b.WriteString(`<p class="board-note">Showing a partial board. Use a project filter or page through the remaining tickets.</p>`)
	}
	if len(board.Attention) > 0 {
		writeBoardNote(&b, "Attention: "+strings.Join(board.Attention, "; "))
	}
	if len(board.NextActions) > 0 {
		writeBoardNote(&b, "Next: "+board.NextActions[0])
	}
	if board.Backup != nil {
		writeBoardNote(&b, "Backup: "+board.Backup.SummaryLine())
	}
	b.WriteString(`<label class="view-switch"><input class="view-toggle" type="checkbox"><span>Side-scroll lanes</span></label><div class="lanes" role="region" aria-label="Board lanes" tabindex="0">`)
	for _, col := range board.Columns {
		if col.Status == "canceled" && col.Total == 0 {
			continue
		}
		b.WriteString(`<section class="lane lane-`)
		b.WriteString(html.EscapeString(col.Status))
		b.WriteString(`" aria-label="`)
		b.WriteString(html.EscapeString(col.Label))
		b.WriteString(`"><div class="lane-head"><h2>`)
		b.WriteString(html.EscapeString(col.Label))
		b.WriteString(`</h2><span class="lane-count" aria-label="`)
		b.WriteString(html.EscapeString(ticketCountLabel(col.Total)))
		b.WriteString(`">`)
		b.WriteString(fmt.Sprint(col.Total))
		b.WriteString(`</span></div><div class="lane-list">`)
		if len(col.Cards) == 0 {
			b.WriteString(`<p class="lane-empty">No tickets</p>`)
		}
		for _, card := range col.Cards {
			writeBoardWidgetCard(&b, card)
		}
		b.WriteString(`</div></section>`)
	}
	b.WriteString(`</div></section>`)
	return b.String()
}

func writeBoardWidgetCard(b *strings.Builder, card CompactCard) {
	priority := strings.TrimSpace(card.Priority)
	priorityLabel := strings.ReplaceAll(priority, "_", " ") + " priority"
	if priority == "" {
		priority = "low"
		priorityLabel = "No priority"
	}
	b.WriteString(`<article><details class="card"><summary><span class="card-id">`)
	b.WriteString(html.EscapeString(card.ID))
	b.WriteString(`</span><span class="card-title">`)
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = "(untitled)"
	}
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</span><span class="card-meta"><span class="priority-dot priority-`)
	b.WriteString(priorityCSSClass(priority))
	b.WriteString(`" aria-hidden="true"></span><span>`)
	b.WriteString(html.EscapeString(priorityLabel))
	b.WriteString(`</span></span><span class="card-pills">`)
	if card.Type != "" {
		writeBoardPill(b, "pill-type", card.Type)
	}
	for _, label := range card.Labels {
		writeBoardPill(b, "", label)
	}
	b.WriteString(`</span>`)
	if len(card.BlockedBy) > 0 {
		b.WriteString(`<span class="card-blockers">`)
		for _, blocker := range card.BlockedBy {
			writeBoardPill(b, "pill-blocked", "Blocked by "+blocker)
		}
		b.WriteString(`</span>`)
	}
	b.WriteString(`</summary><div class="card-detail">`)
	if strings.TrimSpace(card.Description) != "" {
		b.WriteString(`<h3>Description</h3><p>`)
		b.WriteString(html.EscapeString(strings.TrimSpace(card.Description)))
		b.WriteString(`</p>`)
	}
	if len(card.Acceptance) > 0 {
		b.WriteString(`<h3>Acceptance criteria</h3><ul>`)
		for _, criterion := range card.Acceptance {
			b.WriteString(`<li>`)
			b.WriteString(html.EscapeString(criterion))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<h3>Relations and review</h3><dl>`)
	writeBoardDetail(b, "Assignee", emptyBoardValue(card.Assignee))
	writeBoardDetail(b, "Reviewer", emptyBoardValue(card.Reviewer))
	writeBoardDetail(b, "Review state", emptyBoardValue(card.ReviewState))
	if card.Parent != "" {
		writeBoardDetail(b, "Parent", card.Parent)
	}
	if len(card.BlockedBy) > 0 {
		writeBoardDetail(b, "Blocked by", strings.Join(card.BlockedBy, ", "))
	}
	if len(card.Blocks) > 0 {
		writeBoardDetail(b, "Blocks", strings.Join(card.Blocks, ", "))
	}
	b.WriteString(`</dl></div></details></article>`)
}

func writeBoardPill(b *strings.Builder, kind, value string) {
	b.WriteString(`<span class="pill`)
	if kind != "" {
		b.WriteString(" ")
		b.WriteString(kind)
	}
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</span>`)
}

func writeBoardDetail(b *strings.Builder, name, value string) {
	b.WriteString(`<dt>`)
	b.WriteString(html.EscapeString(name))
	b.WriteString(`</dt><dd>`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</dd>`)
}

func writeBoardNote(b *strings.Builder, value string) {
	b.WriteString(`<p class="board-note">`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</p>`)
}

func priorityCSSClass(priority string) string {
	switch priority {
	case "critical", "high", "medium", "low":
		return priority
	default:
		return "low"
	}
}

func emptyBoardValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "None recorded"
	}
	return value
}

func ticketCountLabel(count int) string {
	if count == 1 {
		return "1 ticket"
	}
	return fmt.Sprintf("%d tickets", count)
}
