package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/myrrazor/atlas-tasker/internal/config"
	"github.com/myrrazor/atlas-tasker/internal/contracts"
)

const defaultLanguage = "en"

var messageCatalogs = map[string]map[string]string{
	"en": {
		"common.automatic":                  "Automatic",
		"common.by":                         "by",
		"common.cancel":                     "Cancel",
		"common.close":                      "Close",
		"common.language":                   "Language",
		"common.none":                       "None",
		"common.not_set":                    "Not set",
		"common.uncolored":                  "Uncolored",
		"language.english":                  "English",
		"language.indonesian":               "Bahasa Indonesia",
		"language.spanish":                  "Español",
		"language.switcher.aria":            "Language",
		"page.board.title":                  "Atlas Tasker Web Board",
		"page.settings.title":               "Atlas Tasker Settings",
		"nav.primary":                       "Primary",
		"nav.overview":                      "Overview",
		"nav.board":                         "Board",
		"nav.welcome.aria":                  "Atlas Tasker welcome",
		"meta.workspace":                    "Workspace",
		"meta.local":                        "Local",
		"meta.actor":                        "Actor",
		"mode.read_only":                    "Read only",
		"mode.read_write":                   "Read/write",
		"action.search":                     "Search",
		"action.settings":                   "Settings",
		"action.new_ticket":                 "New ticket",
		"welcome.kicker":                    "Workspace overview",
		"welcome.title":                     "Welcome",
		"welcome.new_project":               "New project",
		"welcome.table.project":             "Project",
		"welcome.table.status":              "Status",
		"welcome.table.blockers":            "Blockers",
		"welcome.rollup.active":             "active",
		"welcome.rollup.backlog":            "backlog",
		"welcome.rollup.done":               "done",
		"welcome.rollup.blocked":            "blocked",
		"welcome.empty.projects":            "No projects yet. Create one here or run",
		"welcome.activity.kicker":           "Activity",
		"welcome.activity.title":            "Latest changes",
		"welcome.activity.empty":            "Ticket changes will appear here.",
		"welcome.settings.aria":             "Web settings",
		"welcome.project.kicker":            "Workspace",
		"welcome.project.title":             "New project",
		"welcome.project.confirm":           "Create this project?",
		"welcome.project.key":               "Project key",
		"welcome.project.name":              "Project name",
		"welcome.project.name_placeholder":  "App Project",
		"welcome.project.key_help":          "Keys use uppercase letters, numbers, underscores, or hyphens.",
		"welcome.project.create":            "Create project",
		"welcome.event.created":             "%s created",
		"welcome.event.moved":               "%s moved %s → %s",
		"welcome.event.commented":           "%s commented",
		"welcome.event.updated":             "%s updated",
		"settings.kicker":                   "Local web preferences",
		"settings.title":                    "Settings",
		"settings.back":                     "Back to overview",
		"settings.identity":                 "Identity",
		"settings.owner_name":               "Owner name",
		"settings.default_actor":            "Default actor",
		"settings.web_language":             "Web language",
		"settings.agent_colors":             "Agent colors",
		"settings.agents":                   "Agents",
		"settings.note":                     "Read only in the browser. Change these preferences with:",
		"board.aria":                        "Kanban board",
		"board.filters.title":               "Filters",
		"board.field.project":               "Project",
		"board.field.view":                  "View",
		"board.field.assignee":              "Assignee",
		"board.field.reviewer":              "Reviewer",
		"board.field.label":                 "Label",
		"board.field.labels":                "Labels",
		"board.field.search":                "Search",
		"board.field.priority":              "Priority",
		"board.field.type":                  "Type",
		"board.field.title":                 "Title",
		"board.field.description":           "Description",
		"board.field.acceptance":            "Acceptance",
		"board.field.acceptance_criteria":   "Acceptance criteria",
		"board.field.reason":                "Reason",
		"board.search.placeholder":          "Search tickets...",
		"board.filter.search_placeholder":   "handoff, release, login",
		"board.option.any":                  "Any",
		"board.priority.critical":           "Critical",
		"board.priority.high":               "High",
		"board.priority.medium":             "Medium",
		"board.priority.low":                "Low",
		"board.type.task":                   "Task",
		"board.type.bug":                    "Bug",
		"board.type.epic":                   "Epic",
		"board.type.subtask":                "Subtask",
		"board.filters.apply":               "Apply",
		"board.columns.aria":                "Board columns",
		"board.column.backlog":              "Backlog",
		"board.column.ready":                "Ready",
		"board.column.in_progress":          "In progress",
		"board.column.in_review":            "In review",
		"board.column.blocked":              "Blocked",
		"board.column.done":                 "Done",
		"board.shortcuts.aria":              "Keyboard shortcuts",
		"board.shortcuts.new_ticket":        "n New ticket",
		"board.shortcuts.search":            "/ Search",
		"board.shortcuts.drag":              "Drag cards between columns",
		"board.detail.aria":                 "Ticket detail",
		"board.empty.detail_title":          "No ticket selected",
		"board.empty.detail_body":           "Create a ticket or pick one from the board.",
		"board.empty.column_title":          "No tickets",
		"board.empty.column_done":           "Completed tickets appear here.",
		"board.empty.column_other":          "Drag or create a ticket to get started.",
		"board.assigned_to":                 "Assigned to agent:%s",
		"board.new.eyeline":                 "New ticket",
		"board.new.title":                   "Create tracked work",
		"board.new.title_placeholder":       "Ship the web board",
		"board.new.acceptance_placeholder":  "One item per line",
		"board.new.create":                  "Create ticket",
		"board.tabs.aria":                   "Ticket detail sections",
		"board.tabs.overview":               "Overview",
		"board.tabs.activity":               "Notes & activity",
		"board.tabs.gates":                  "Policy/Gates",
		"board.tabs.evidence":               "Evidence",
		"board.section.description":         "Description",
		"board.section.no_description":      "No description yet.",
		"board.section.acceptance":          "Acceptance criteria",
		"board.section.no_acceptance":       "No acceptance criteria yet.",
		"board.section.edit":                "Edit ticket",
		"board.section.save":                "Save changes",
		"board.section.comments":            "Notes & comments",
		"board.section.no_comments":         "No comments yet.",
		"board.section.comment_placeholder": "Add a comment",
		"board.section.add_comment":         "Add comment",
		"board.section.recent_history":      "Recent history",
		"board.section.no_history":          "No history yet.",
		"board.section.policy":              "Policy",
		"board.section.completion":          "Completion",
		"board.section.required_reviewer":   "Required reviewer",
		"board.section.open_gates":          "Open gates",
		"board.section.blockers":            "Blockers",
		"board.section.no_blockers":         "No unresolved blockers recorded.",
		"board.section.evidence_runs":       "Evidence / Runs",
		"board.section.no_run":              "No run yet",
		"board.summary.changes":             "Changes: %d",
		"board.summary.checks":              "Checks: %d",
		"board.summary.gates":               "Gates: %d",
		"board.action.request_review":       "Request review",
		"board.action.approve":              "Approve",
		"board.action.complete":             "Complete",
		"board.action.more":                 "More",
		"board.action.move":                 "Move",
		"board.preview.unknown_status":      "Unknown status",
		"board.preview.unassigned":          "Unassigned",
		"board.count.blocker.one":           "blocker",
		"board.count.blocker.other":         "blockers",
		"board.count.gate.one":              "gate",
		"board.count.gate.other":            "gates",
		"board.count.comment.one":           "comment",
		"board.count.comment.other":         "comments",
		"board.flash.session_expired":       "Session expired — run `tracker web serve --open` and use the new session URL",
		"board.flash.stale":                 "Board may be out of date — could not reach the server",
		"board.flash.move_failed_status":    "Move failed with {status}",
		"board.flash.move_failed":           "Move failed",
		"board.flash.updated":               "updated {id}",
	},
	"es": {
		"common.automatic":                  "Automático",
		"common.by":                         "por",
		"common.cancel":                     "Cancelar",
		"common.close":                      "Cerrar",
		"common.language":                   "Idioma",
		"common.none":                       "Ninguno",
		"common.not_set":                    "Sin configurar",
		"common.uncolored":                  "Sin color",
		"language.english":                  "English",
		"language.indonesian":               "Bahasa Indonesia",
		"language.spanish":                  "Español",
		"language.switcher.aria":            "Idioma",
		"page.board.title":                  "Tablero web de Atlas Tasker",
		"page.settings.title":               "Configuración de Atlas Tasker",
		"nav.primary":                       "Principal",
		"nav.overview":                      "Resumen",
		"nav.board":                         "Tablero",
		"nav.welcome.aria":                  "Bienvenida de Atlas Tasker",
		"meta.workspace":                    "Espacio de trabajo",
		"meta.local":                        "Local",
		"meta.actor":                        "Actor",
		"mode.read_only":                    "Solo lectura",
		"mode.read_write":                   "Lectura y escritura",
		"action.search":                     "Buscar",
		"action.settings":                   "Configuración",
		"action.new_ticket":                 "Nuevo ticket",
		"welcome.kicker":                    "Resumen del espacio de trabajo",
		"welcome.title":                     "Te damos la bienvenida",
		"welcome.new_project":               "Nuevo proyecto",
		"welcome.table.project":             "Proyecto",
		"welcome.table.status":              "Estado",
		"welcome.table.blockers":            "Bloqueos",
		"welcome.rollup.active":             "activos",
		"welcome.rollup.backlog":            "pendientes",
		"welcome.rollup.done":               "finalizados",
		"welcome.rollup.blocked":            "bloqueados",
		"welcome.empty.projects":            "Aún no hay proyectos. Crea uno aquí o ejecuta",
		"welcome.activity.kicker":           "Actividad",
		"welcome.activity.title":            "Cambios recientes",
		"welcome.activity.empty":            "Los cambios en los tickets aparecerán aquí.",
		"welcome.settings.aria":             "Configuración web",
		"welcome.project.kicker":            "Espacio de trabajo",
		"welcome.project.title":             "Nuevo proyecto",
		"welcome.project.confirm":           "¿Crear este proyecto?",
		"welcome.project.key":               "Clave del proyecto",
		"welcome.project.name":              "Nombre del proyecto",
		"welcome.project.name_placeholder":  "Proyecto de la app",
		"welcome.project.key_help":          "Las claves admiten mayúsculas, números, guiones bajos y guiones.",
		"welcome.project.create":            "Crear proyecto",
		"welcome.event.created":             "%s se creó",
		"welcome.event.moved":               "%s pasó de %s a %s",
		"welcome.event.commented":           "%s recibió un comentario",
		"welcome.event.updated":             "%s se actualizó",
		"settings.kicker":                   "Preferencias web locales",
		"settings.title":                    "Configuración",
		"settings.back":                     "Volver al resumen",
		"settings.identity":                 "Identidad",
		"settings.owner_name":               "Nombre del propietario",
		"settings.default_actor":            "Actor predeterminado",
		"settings.web_language":             "Idioma de la web",
		"settings.agent_colors":             "Colores de los agentes",
		"settings.agents":                   "Agentes",
		"settings.note":                     "Estas preferencias son de solo lectura en el navegador. Cámbialas con:",
		"board.aria":                        "Tablero Kanban",
		"board.filters.title":               "Filtros",
		"board.field.project":               "Proyecto",
		"board.field.view":                  "Vista",
		"board.field.assignee":              "Responsable",
		"board.field.reviewer":              "Revisor",
		"board.field.label":                 "Etiqueta",
		"board.field.labels":                "Etiquetas",
		"board.field.search":                "Buscar",
		"board.field.priority":              "Prioridad",
		"board.field.type":                  "Tipo",
		"board.field.title":                 "Título",
		"board.field.description":           "Descripción",
		"board.field.acceptance":            "Aceptación",
		"board.field.acceptance_criteria":   "Criterios de aceptación",
		"board.field.reason":                "Motivo",
		"board.search.placeholder":          "Buscar tickets...",
		"board.filter.search_placeholder":   "entrega, versión, acceso",
		"board.option.any":                  "Cualquiera",
		"board.priority.critical":           "Crítica",
		"board.priority.high":               "Alta",
		"board.priority.medium":             "Media",
		"board.priority.low":                "Baja",
		"board.type.task":                   "Tarea",
		"board.type.bug":                    "Error",
		"board.type.epic":                   "Épica",
		"board.type.subtask":                "Subtarea",
		"board.filters.apply":               "Aplicar",
		"board.columns.aria":                "Columnas del tablero",
		"board.column.backlog":              "Pendiente",
		"board.column.ready":                "Listo",
		"board.column.in_progress":          "En curso",
		"board.column.in_review":            "En revisión",
		"board.column.blocked":              "Bloqueado",
		"board.column.done":                 "Finalizado",
		"board.shortcuts.aria":              "Atajos de teclado",
		"board.shortcuts.new_ticket":        "n Nuevo ticket",
		"board.shortcuts.search":            "/ Buscar",
		"board.shortcuts.drag":              "Arrastra tarjetas entre columnas",
		"board.detail.aria":                 "Detalle del ticket",
		"board.empty.detail_title":          "Ningún ticket seleccionado",
		"board.empty.detail_body":           "Crea un ticket o elige uno del tablero.",
		"board.empty.column_title":          "No hay tickets",
		"board.empty.column_done":           "Los tickets finalizados aparecerán aquí.",
		"board.empty.column_other":          "Arrastra o crea un ticket para empezar.",
		"board.assigned_to":                 "Asignado al agente: %s",
		"board.new.eyeline":                 "Nuevo ticket",
		"board.new.title":                   "Crear trabajo con seguimiento",
		"board.new.title_placeholder":       "Publicar el tablero web",
		"board.new.acceptance_placeholder":  "Un elemento por línea",
		"board.new.create":                  "Crear ticket",
		"board.tabs.aria":                   "Secciones del detalle del ticket",
		"board.tabs.overview":               "Resumen",
		"board.tabs.activity":               "Notas y actividad",
		"board.tabs.gates":                  "Política/Controles",
		"board.tabs.evidence":               "Evidencia",
		"board.section.description":         "Descripción",
		"board.section.no_description":      "Aún no hay descripción.",
		"board.section.acceptance":          "Criterios de aceptación",
		"board.section.no_acceptance":       "Aún no hay criterios de aceptación.",
		"board.section.edit":                "Editar ticket",
		"board.section.save":                "Guardar cambios",
		"board.section.comments":            "Notas y comentarios",
		"board.section.no_comments":         "Aún no hay comentarios.",
		"board.section.comment_placeholder": "Añadir un comentario",
		"board.section.add_comment":         "Añadir comentario",
		"board.section.recent_history":      "Historial reciente",
		"board.section.no_history":          "Aún no hay historial.",
		"board.section.policy":              "Política",
		"board.section.completion":          "Finalización",
		"board.section.required_reviewer":   "Revisor obligatorio",
		"board.section.open_gates":          "Controles abiertos",
		"board.section.blockers":            "Bloqueos",
		"board.section.no_blockers":         "No hay bloqueos pendientes.",
		"board.section.evidence_runs":       "Evidencia / Ejecuciones",
		"board.section.no_run":              "Aún no hay ejecuciones",
		"board.summary.changes":             "Cambios: %d",
		"board.summary.checks":              "Comprobaciones: %d",
		"board.summary.gates":               "Controles: %d",
		"board.action.request_review":       "Solicitar revisión",
		"board.action.approve":              "Aprobar",
		"board.action.complete":             "Completar",
		"board.action.more":                 "Más",
		"board.action.move":                 "Mover",
		"board.preview.unknown_status":      "Estado desconocido",
		"board.preview.unassigned":          "Sin responsable",
		"board.count.blocker.one":           "bloqueo",
		"board.count.blocker.other":         "bloqueos",
		"board.count.gate.one":              "control",
		"board.count.gate.other":            "controles",
		"board.count.comment.one":           "comentario",
		"board.count.comment.other":         "comentarios",
		"board.flash.session_expired":       "La sesión caducó. Ejecuta `tracker web serve --open` y usa la nueva URL de sesión",
		"board.flash.stale":                 "El tablero podría estar desactualizado: no se pudo contactar con el servidor",
		"board.flash.move_failed_status":    "No se pudo mover: error {status}",
		"board.flash.move_failed":           "No se pudo mover",
		"board.flash.updated":               "{id} actualizado",
	},
	"id": {
		"common.automatic":                  "Otomatis",
		"common.by":                         "oleh",
		"common.cancel":                     "Batal",
		"common.close":                      "Tutup",
		"common.language":                   "Bahasa",
		"common.none":                       "Tidak ada",
		"common.not_set":                    "Belum diatur",
		"common.uncolored":                  "Tanpa warna",
		"language.english":                  "English",
		"language.indonesian":               "Bahasa Indonesia",
		"language.spanish":                  "Español",
		"language.switcher.aria":            "Bahasa",
		"page.board.title":                  "Papan web Atlas Tasker",
		"page.settings.title":               "Pengaturan Atlas Tasker",
		"nav.primary":                       "Utama",
		"nav.overview":                      "Ringkasan",
		"nav.board":                         "Papan",
		"nav.welcome.aria":                  "Sambutan Atlas Tasker",
		"meta.workspace":                    "Ruang kerja",
		"meta.local":                        "Lokal",
		"meta.actor":                        "Aktor",
		"mode.read_only":                    "Hanya baca",
		"mode.read_write":                   "Baca/tulis",
		"action.search":                     "Cari",
		"action.settings":                   "Pengaturan",
		"action.new_ticket":                 "Tiket baru",
		"welcome.kicker":                    "Ringkasan ruang kerja",
		"welcome.title":                     "Selamat datang",
		"welcome.new_project":               "Proyek baru",
		"welcome.table.project":             "Proyek",
		"welcome.table.status":              "Status",
		"welcome.table.blockers":            "Penghambat",
		"welcome.rollup.active":             "aktif",
		"welcome.rollup.backlog":            "antrean",
		"welcome.rollup.done":               "selesai",
		"welcome.rollup.blocked":            "terhambat",
		"welcome.empty.projects":            "Belum ada proyek. Buat proyek di sini atau jalankan",
		"welcome.activity.kicker":           "Aktivitas",
		"welcome.activity.title":            "Perubahan terbaru",
		"welcome.activity.empty":            "Perubahan tiket akan muncul di sini.",
		"welcome.settings.aria":             "Pengaturan web",
		"welcome.project.kicker":            "Ruang kerja",
		"welcome.project.title":             "Proyek baru",
		"welcome.project.confirm":           "Buat proyek ini?",
		"welcome.project.key":               "Kunci proyek",
		"welcome.project.name":              "Nama proyek",
		"welcome.project.name_placeholder":  "Proyek aplikasi",
		"welcome.project.key_help":          "Kunci dapat memakai huruf kapital, angka, garis bawah, atau tanda hubung.",
		"welcome.project.create":            "Buat proyek",
		"welcome.event.created":             "%s dibuat",
		"welcome.event.moved":               "%s dipindahkan dari %s ke %s",
		"welcome.event.commented":           "%s mendapat komentar",
		"welcome.event.updated":             "%s diperbarui",
		"settings.kicker":                   "Preferensi web lokal",
		"settings.title":                    "Pengaturan",
		"settings.back":                     "Kembali ke ringkasan",
		"settings.identity":                 "Identitas",
		"settings.owner_name":               "Nama pemilik",
		"settings.default_actor":            "Aktor bawaan",
		"settings.web_language":             "Bahasa web",
		"settings.agent_colors":             "Warna agen",
		"settings.agents":                   "Agen",
		"settings.note":                     "Preferensi ini hanya dapat dibaca di browser. Ubah dengan:",
		"board.aria":                        "Papan Kanban",
		"board.filters.title":               "Filter",
		"board.field.project":               "Proyek",
		"board.field.view":                  "Tampilan",
		"board.field.assignee":              "Pelaksana",
		"board.field.reviewer":              "Peninjau",
		"board.field.label":                 "Label",
		"board.field.labels":                "Label",
		"board.field.search":                "Cari",
		"board.field.priority":              "Prioritas",
		"board.field.type":                  "Jenis",
		"board.field.title":                 "Judul",
		"board.field.description":           "Deskripsi",
		"board.field.acceptance":            "Penerimaan",
		"board.field.acceptance_criteria":   "Kriteria penerimaan",
		"board.field.reason":                "Alasan",
		"board.search.placeholder":          "Cari tiket...",
		"board.filter.search_placeholder":   "serah terima, rilis, masuk",
		"board.option.any":                  "Semua",
		"board.priority.critical":           "Kritis",
		"board.priority.high":               "Tinggi",
		"board.priority.medium":             "Sedang",
		"board.priority.low":                "Rendah",
		"board.type.task":                   "Tugas",
		"board.type.bug":                    "Bug",
		"board.type.epic":                   "Epik",
		"board.type.subtask":                "Subtugas",
		"board.filters.apply":               "Terapkan",
		"board.columns.aria":                "Kolom papan",
		"board.column.backlog":              "Antrean",
		"board.column.ready":                "Siap",
		"board.column.in_progress":          "Dikerjakan",
		"board.column.in_review":            "Ditinjau",
		"board.column.blocked":              "Terhambat",
		"board.column.done":                 "Selesai",
		"board.shortcuts.aria":              "Pintasan keyboard",
		"board.shortcuts.new_ticket":        "n Tiket baru",
		"board.shortcuts.search":            "/ Cari",
		"board.shortcuts.drag":              "Tarik kartu antarkolom",
		"board.detail.aria":                 "Detail tiket",
		"board.empty.detail_title":          "Belum ada tiket yang dipilih",
		"board.empty.detail_body":           "Buat tiket atau pilih satu dari papan.",
		"board.empty.column_title":          "Belum ada tiket",
		"board.empty.column_done":           "Tiket yang selesai akan muncul di sini.",
		"board.empty.column_other":          "Tarik atau buat tiket untuk mulai.",
		"board.assigned_to":                 "Ditugaskan kepada agen: %s",
		"board.new.eyeline":                 "Tiket baru",
		"board.new.title":                   "Buat pekerjaan terlacak",
		"board.new.title_placeholder":       "Rilis papan web",
		"board.new.acceptance_placeholder":  "Satu butir per baris",
		"board.new.create":                  "Buat tiket",
		"board.tabs.aria":                   "Bagian detail tiket",
		"board.tabs.overview":               "Ringkasan",
		"board.tabs.activity":               "Catatan & aktivitas",
		"board.tabs.gates":                  "Kebijakan/Gate",
		"board.tabs.evidence":               "Bukti",
		"board.section.description":         "Deskripsi",
		"board.section.no_description":      "Belum ada deskripsi.",
		"board.section.acceptance":          "Kriteria penerimaan",
		"board.section.no_acceptance":       "Belum ada kriteria penerimaan.",
		"board.section.edit":                "Edit tiket",
		"board.section.save":                "Simpan perubahan",
		"board.section.comments":            "Catatan & komentar",
		"board.section.no_comments":         "Belum ada komentar.",
		"board.section.comment_placeholder": "Tambahkan komentar",
		"board.section.add_comment":         "Tambah komentar",
		"board.section.recent_history":      "Riwayat terbaru",
		"board.section.no_history":          "Belum ada riwayat.",
		"board.section.policy":              "Kebijakan",
		"board.section.completion":          "Penyelesaian",
		"board.section.required_reviewer":   "Peninjau wajib",
		"board.section.open_gates":          "Gate terbuka",
		"board.section.blockers":            "Penghambat",
		"board.section.no_blockers":         "Tidak ada penghambat yang belum selesai.",
		"board.section.evidence_runs":       "Bukti / Eksekusi",
		"board.section.no_run":              "Belum ada eksekusi",
		"board.summary.changes":             "Perubahan: %d",
		"board.summary.checks":              "Pemeriksaan: %d",
		"board.summary.gates":               "Gate: %d",
		"board.action.request_review":       "Minta peninjauan",
		"board.action.approve":              "Setujui",
		"board.action.complete":             "Selesaikan",
		"board.action.more":                 "Lainnya",
		"board.action.move":                 "Pindahkan",
		"board.preview.unknown_status":      "Status tidak dikenal",
		"board.preview.unassigned":          "Belum ditugaskan",
		"board.count.blocker.one":           "penghambat",
		"board.count.blocker.other":         "penghambat",
		"board.count.gate.one":              "gate",
		"board.count.gate.other":            "gate",
		"board.count.comment.one":           "komentar",
		"board.count.comment.other":         "komentar",
		"board.flash.session_expired":       "Sesi berakhir. Jalankan `tracker web serve --open` dan gunakan URL sesi yang baru",
		"board.flash.stale":                 "Papan mungkin sudah tidak mutakhir: server tidak dapat dijangkau",
		"board.flash.move_failed_status":    "Gagal memindahkan: error {status}",
		"board.flash.move_failed":           "Gagal memindahkan",
		"board.flash.updated":               "{id} diperbarui",
	},
}

func (s *Server) requestLanguage(r *http.Request) string {
	configured := ""
	if cfg, err := config.Load(s.cfg.Root); err == nil {
		configured = cfg.Web.Lang
	}
	return resolveLanguage(r.URL.Query().Get("lang"), configured, r.Header.Get("Accept-Language"))
}

func resolveLanguage(query, configured, accepted string) string {
	if lang := normalizeLanguage(query); lang != "" {
		return lang
	}
	if lang := normalizeLanguage(configured); lang != "" {
		return lang
	}
	if lang := preferredAcceptedLanguage(accepted); lang != "" {
		return lang
	}
	return defaultLanguage
}

func normalizeLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "*" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", "-")
	base, _, _ := strings.Cut(value, "-")
	if _, ok := messageCatalogs[base]; ok {
		return base
	}
	return ""
}

func preferredAcceptedLanguage(header string) string {
	bestLanguage := ""
	bestQuality := -1.0
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		lang := normalizeLanguage(fields[0])
		if lang == "" {
			continue
		}
		quality := 1.0
		for _, field := range fields[1:] {
			name, raw, ok := strings.Cut(strings.TrimSpace(field), "=")
			if !ok || !strings.EqualFold(name, "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				quality = 0
			} else {
				quality = parsed
			}
		}
		if quality > bestQuality && quality > 0 {
			bestLanguage = lang
			bestQuality = quality
		}
	}
	return bestLanguage
}

func translator(lang string) func(string, ...any) string {
	return func(key string, args ...any) string {
		message, ok := messageCatalogs[lang][key]
		if !ok {
			message, ok = messageCatalogs[defaultLanguage][key]
		}
		if !ok {
			return key
		}
		if len(args) > 0 {
			return fmt.Sprintf(message, args...)
		}
		return message
	}
}

func languageURL(current *url.URL, lang string) string {
	next := *current
	next.Scheme = ""
	next.Host = ""
	query := next.Query()
	query.Set("lang", normalizeLanguage(lang))
	next.RawQuery = query.Encode()
	return next.String()
}

func statusMessageKey(status contracts.Status) string {
	switch status {
	case contracts.StatusBacklog:
		return "board.column.backlog"
	case contracts.StatusReady:
		return "board.column.ready"
	case contracts.StatusInProgress:
		return "board.column.in_progress"
	case contracts.StatusInReview:
		return "board.column.in_review"
	case contracts.StatusBlocked:
		return "board.column.blocked"
	case contracts.StatusDone:
		return "board.column.done"
	default:
		return ""
	}
}

func priorityMessageKey(priority contracts.Priority) string {
	switch priority {
	case contracts.PriorityCritical:
		return "board.priority.critical"
	case contracts.PriorityHigh:
		return "board.priority.high"
	case contracts.PriorityMedium:
		return "board.priority.medium"
	case contracts.PriorityLow:
		return "board.priority.low"
	default:
		return ""
	}
}

func recentDescription(t func(string, ...any) string, change RecentChange) string {
	switch change.Verb {
	case "created":
		return t("welcome.event.created", change.TicketID)
	case "moved":
		return t("welcome.event.moved", change.TicketID, translatedStatus(t, change.From), translatedStatus(t, change.To))
	case "commented":
		return t("welcome.event.commented", change.TicketID)
	case "updated":
		return t("welcome.event.updated", change.TicketID)
	default:
		return strings.TrimSpace(change.TicketID + " " + change.Verb)
	}
}

func translatedStatus(t func(string, ...any) string, value string) string {
	key := statusMessageKey(contracts.Status(strings.TrimSpace(value)))
	if key == "" {
		return strings.TrimSpace(value)
	}
	return t(key)
}
