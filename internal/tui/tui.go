// Package tui implements the bubbletea interactive review UI. It is the
// human-in-the-loop step between reconstruction and push: it lists the day's
// sessions and lets the user merge, split, re-describe, re-assign, or drop them
// before triggering a push. It orchestrates the sessions, template, clockify,
// and push pipeline through an injected Deps; it holds no planning logic of its
// own.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vdpeijl/clk/internal/fuzzy"
	"github.com/vdpeijl/clk/internal/sessions"
)

// Project is a Clockify project offered in the reassign picker.
type Project struct {
	ID   string
	Name string
}

// Task is a Clockify task offered after a project is picked in the reassign
// flow. The zero Task (empty ID) represents "no task".
type Task struct {
	ID   string
	Name string
}

// PushItem is one reviewed session handed to the push pipeline, carrying any
// per-session reassignment the user made. An empty ProjectID means "resolve the
// Clockify target from the session's project-token mapping" — the normal path.
type PushItem struct {
	Session   sessions.Session
	ProjectID string
	TaskID    string
}

// PushSummary reports the outcome of a push so the UI can show it. It mirrors
// the create/update/skip/warn breakdown of the push pipeline, plus the count of
// sessions skipped for lacking a Clockify mapping.
type PushSummary struct {
	Created  int
	Updated  int
	Skipped  int
	Warned   int
	Unmapped int
}

// String renders the summary for the status line.
func (s PushSummary) String() string {
	return fmt.Sprintf("%d created, %d updated, %d skipped, %d warned, %d unmapped",
		s.Created, s.Updated, s.Skipped, s.Warned, s.Unmapped)
}

// Deps are the external capabilities the review UI orchestrates. They are
// injected so the model can be driven in tests with in-memory fakes instead of
// a live Clockify workspace.
type Deps struct {
	// Projects lists the Clockify projects available for reassignment.
	Projects func() ([]Project, error)
	// Tasks lists the tasks of a project for the second reassignment step.
	Tasks func(projectID string) ([]Task, error)
	// Push renders, plans, and executes a push of the reviewed items.
	Push func([]PushItem) (PushSummary, error)
}

// mode is the current input focus of the review screen.
type mode int

const (
	modeList    mode = iota // browsing and acting on the session list
	modeEdit                // editing the current session's description
	modeSplit               // choosing a split point for the current session
	modePickProject         // fuzzy-picking a Clockify project to reassign to
	modePickTask            // picking a task within the chosen project
	modeConfirm             // confirming a push
)

// item is one reviewable session plus the edit state layered over it.
type item struct {
	session     sessions.Session
	dropped     bool   // excluded from push; never reaches Clockify
	selected    bool   // multi-select marker for merge
	projectID   string // reassignment override (empty = use token mapping)
	taskID      string
	projectName string // display label once reassigned
}

// Model is the bubbletea model for the review screen.
type Model struct {
	deps  Deps
	items []item

	cursor int
	mode   mode

	input textinput.Model

	// split state
	splitOffset time.Duration

	// reassign state
	projects   []Project
	ranked     []fuzzy.Match
	pickCursor int
	pending    Project // project chosen, awaiting task
	tasks      []Task

	status   string
	err      error
	quitting bool

	width int
}

// New builds a review model over the given sessions and dependencies.
func New(ss []sessions.Session, deps Deps) Model {
	items := make([]item, len(ss))
	for i, s := range ss {
		items[i] = item{session: s}
	}
	ti := textinput.New()
	ti.Prompt = "› "
	ti.CharLimit = 512
	return Model{
		deps:  deps,
		items: items,
		input: ti,
		width: 80,
	}
}

// Run starts the TUI and blocks until the user exits.
func Run(m Model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model. It dispatches to the active mode's handler.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modeEdit:
			return m.updateEdit(msg)
		case modeSplit:
			return m.updateSplit(msg)
		case modePickProject:
			return m.updatePickProject(msg)
		case modePickTask:
			return m.updatePickTask(msg)
		case modeConfirm:
			return m.updateConfirm(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

// updateList handles keys while browsing the session list.
func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ":
		if cur := m.current(); cur != nil {
			cur.selected = !cur.selected
		}
		m.status = ""
	case "d":
		if cur := m.current(); cur != nil {
			cur.dropped = !cur.dropped
		}
		m.status = ""
	case "m":
		return m.mergeSelected(), nil
	case "s":
		return m.enterSplit(), nil
	case "e":
		return m.enterEdit(), nil
	case "r":
		return m.enterPickProject(), nil
	case "p":
		return m.enterConfirm(), nil
	}
	return m, nil
}

// current returns a pointer to the item under the cursor, or nil when the list
// is empty.
func (m *Model) current() *item {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	return &m.items[m.cursor]
}

// mergeSelected collapses the multi-selected sessions into one, placing the
// merged session at the position of the earliest and removing the rest. With
// fewer than two selected it falls back to a status hint.
func (m Model) mergeSelected() Model {
	var picked []int
	for i := range m.items {
		if m.items[i].selected {
			picked = append(picked, i)
		}
	}
	if len(picked) < 2 {
		m.status = "select two or more sessions with <space> before merging"
		return m
	}

	ss := make([]sessions.Session, len(picked))
	for i, idx := range picked {
		ss[i] = m.items[idx].session
	}
	merged := sessions.Merge(ss...)

	// Keep the slot of the earliest-starting selected session, then drop the
	// others. Rebuild rather than splice in place to keep the logic obvious.
	earliest := picked[0]
	for _, idx := range picked {
		if m.items[idx].session.Start.Before(m.items[earliest].session.Start) {
			earliest = idx
		}
	}
	drop := make(map[int]bool, len(picked))
	for _, idx := range picked {
		drop[idx] = true
	}

	next := make([]item, 0, len(m.items)-len(picked)+1)
	for i := range m.items {
		switch {
		case i == earliest:
			next = append(next, item{session: merged})
		case drop[i]:
			// removed
		default:
			next = append(next, m.items[i])
		}
	}
	m.items = next
	m.clampCursor()
	m.status = fmt.Sprintf("merged %d sessions", len(picked))
	return m
}

// enterSplit opens the split-point picker for the current session, defaulting
// the split to the session midpoint.
func (m Model) enterSplit() Model {
	cur := m.current()
	if cur == nil {
		return m
	}
	if cur.session.Duration() < 2*time.Minute {
		m.status = "session too short to split"
		return m
	}
	m.mode = modeSplit
	m.splitOffset = (cur.session.Duration() / 2).Round(time.Minute)
	m.status = ""
	return m
}

// updateSplit handles keys in the split-point picker.
func (m Model) updateSplit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cur := m.current()
	if cur == nil {
		m.mode = modeList
		return m, nil
	}
	dur := cur.session.Duration()
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
	case "left", "h":
		m.splitOffset = clampOffset(m.splitOffset-time.Minute, dur)
	case "right", "l":
		m.splitOffset = clampOffset(m.splitOffset+time.Minute, dur)
	case "H", "shift+left":
		m.splitOffset = clampOffset(m.splitOffset-5*time.Minute, dur)
	case "L", "shift+right":
		m.splitOffset = clampOffset(m.splitOffset+5*time.Minute, dur)
	case "enter":
		return m.applySplit(), nil
	}
	return m, nil
}

// clampOffset keeps a split offset strictly inside the (0, dur) interval, on a
// whole-minute grid so the picker lands on displayable times.
func clampOffset(off, dur time.Duration) time.Duration {
	if off < time.Minute {
		off = time.Minute
	}
	if off > dur-time.Minute {
		off = dur - time.Minute
	}
	return off.Round(time.Minute)
}

// applySplit divides the current session at the chosen point and replaces it
// with the two halves.
func (m Model) applySplit() Model {
	cur := m.current()
	if cur == nil {
		m.mode = modeList
		return m
	}
	at := cur.session.Start.Add(m.splitOffset)
	early, late, ok := sessions.SplitAt(cur.session, at)
	if !ok {
		m.mode = modeList
		m.status = "split point out of range"
		return m
	}

	next := make([]item, 0, len(m.items)+1)
	next = append(next, m.items[:m.cursor]...)
	next = append(next, item{session: early}, item{session: late})
	next = append(next, m.items[m.cursor+1:]...)
	m.items = next
	m.mode = modeList
	m.status = fmt.Sprintf("split at %s", at.Format("15:04"))
	return m
}

// enterEdit opens the description editor for the current session.
func (m Model) enterEdit() Model {
	cur := m.current()
	if cur == nil {
		return m
	}
	m.mode = modeEdit
	m.input.SetValue(cur.session.Description)
	m.input.CursorEnd()
	m.input.Focus()
	m.status = ""
	return m
}

// updateEdit handles keys while editing a description.
func (m Model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "enter":
		if cur := m.current(); cur != nil {
			cur.session.Description = strings.TrimSpace(m.input.Value())
		}
		m.mode = modeList
		m.input.Blur()
		m.status = "description updated"
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// enterPickProject loads the Clockify projects and opens the reassign picker.
func (m Model) enterPickProject() Model {
	if m.current() == nil {
		return m
	}
	if m.deps.Projects == nil {
		m.status = "reassignment unavailable"
		return m
	}
	projects, err := m.deps.Projects()
	if err != nil {
		m.err = err
		m.status = "could not load projects: " + err.Error()
		return m
	}
	if len(projects) == 0 {
		m.status = "no Clockify projects to reassign to"
		return m
	}
	m.projects = projects
	m.mode = modePickProject
	m.pickCursor = 0
	m.input.SetValue("")
	m.input.Focus()
	m.ranked = fuzzy.Rank("", projectNames(projects))
	m.status = ""
	return m
}

// updatePickProject handles keys in the project picker: typing filters, the
// arrows move the highlight, enter selects.
func (m Model) updatePickProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		m.input.Blur()
		return m, nil
	case "up", "ctrl+p":
		if m.pickCursor > 0 {
			m.pickCursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.pickCursor < len(m.ranked)-1 {
			m.pickCursor++
		}
		return m, nil
	case "enter":
		return m.selectProject()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.ranked = fuzzy.Rank(strings.TrimSpace(m.input.Value()), projectNames(m.projects))
	if m.pickCursor >= len(m.ranked) {
		m.pickCursor = 0
	}
	return m, cmd
}

// selectProject records the highlighted project and advances to task selection,
// loading the project's tasks.
func (m Model) selectProject() (tea.Model, tea.Cmd) {
	if len(m.ranked) == 0 {
		m.status = "no project matches the filter"
		return m, nil
	}
	m.pending = m.projects[m.ranked[m.pickCursor].Index]
	m.input.Blur()

	var tasks []Task
	if m.deps.Tasks != nil {
		t, err := m.deps.Tasks(m.pending.ID)
		if err != nil {
			m.err = err
			m.status = "could not load tasks: " + err.Error()
			m.mode = modeList
			return m, nil
		}
		tasks = t
	}
	m.tasks = tasks
	m.pickCursor = 0
	m.mode = modePickTask
	return m, nil
}

// updatePickTask handles keys in the task picker. The first row is always the
// "no task" option, so the cursor ranges over len(tasks)+1 rows.
func (m Model) updatePickTask(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "up", "k", "ctrl+p":
		if m.pickCursor > 0 {
			m.pickCursor--
		}
	case "down", "j", "ctrl+n":
		if m.pickCursor < len(m.tasks) {
			m.pickCursor++
		}
	case "enter":
		return m.applyReassign(), nil
	}
	return m, nil
}

// applyReassign writes the chosen project/task onto the current session.
func (m Model) applyReassign() Model {
	cur := m.current()
	if cur == nil {
		m.mode = modeList
		return m
	}
	cur.projectID = m.pending.ID
	cur.projectName = m.pending.Name
	if m.pickCursor == 0 {
		cur.taskID = ""
	} else {
		cur.taskID = m.tasks[m.pickCursor-1].ID
	}
	m.mode = modeList
	m.status = "reassigned to " + m.pending.Name
	return m
}

// enterConfirm opens the push confirmation, unless every session is dropped.
func (m Model) enterConfirm() Model {
	if len(m.pushItems()) == 0 {
		m.status = "nothing to push — every session is dropped"
		return m
	}
	m.mode = modeConfirm
	m.status = ""
	return m
}

// updateConfirm handles the push confirmation prompt.
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		return m.doPush(), nil
	case "n", "N", "esc", "q":
		m.mode = modeList
		m.status = "push cancelled"
		return m, nil
	}
	return m, nil
}

// doPush runs the injected push and reports the result on the status line.
func (m Model) doPush() Model {
	m.mode = modeList
	if m.deps.Push == nil {
		m.status = "push unavailable"
		return m
	}
	summary, err := m.deps.Push(m.pushItems())
	if err != nil {
		m.err = err
		m.status = "push failed: " + err.Error()
		return m
	}
	m.status = "pushed: " + summary.String()
	return m
}

// pushItems returns the non-dropped sessions with their reassignments applied.
func (m Model) pushItems() []PushItem {
	var out []PushItem
	for _, it := range m.items {
		if it.dropped {
			continue
		}
		out = append(out, PushItem{
			Session:   it.session,
			ProjectID: it.projectID,
			TaskID:    it.taskID,
		})
	}
	return out
}

// clampCursor keeps the cursor inside the current item bounds.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// projectNames extracts project names index-aligned with the input.
func projectNames(projects []Project) []string {
	names := make([]string, len(projects))
	for i, p := range projects {
		names[i] = p.Name
	}
	return names
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	cursorStyle   = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	droppedStyle  = lipgloss.NewStyle().Faint(true).Strikethrough(true)
	helpStyle     = lipgloss.NewStyle().Faint(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	switch m.mode {
	case modeEdit:
		return m.viewEdit()
	case modeSplit:
		return m.viewSplit()
	case modePickProject:
		return m.viewPickProject()
	case modePickTask:
		return m.viewPickTask()
	case modeConfirm:
		return m.viewConfirm()
	default:
		return m.viewList()
	}
}

// viewList renders the session table with the cursor, selection, and drop
// markers, followed by the status line and key help.
func (m Model) viewList() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("clk review — today's sessions"))
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString("No sessions to review.\n\n")
	}
	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = cursorStyle.Render("➤ ")
		}
		mark := "[ ]"
		if it.selected {
			mark = selectedStyle.Render("[x]")
		}
		line := fmt.Sprintf("%s%s  %s", cursor, mark, m.rowText(it))
		if it.dropped {
			line = cursor + droppedStyle.Render(fmt.Sprintf("%s  %s", "[ ]", m.rowText(it)))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.status != "" {
		b.WriteString(statusStyle.Render(m.status))
		b.WriteString("\n")
	}
	b.WriteString(helpStyle.Render(
		"↑/↓ move · <space> select · m merge · s split · e edit · r reassign · d drop · p push · q quit"))
	b.WriteString("\n")
	return b.String()
}

// rowText renders a session's one-line summary for the list.
func (m Model) rowText(it item) string {
	s := it.session
	target := dash(s.ProjectToken)
	if it.projectName != "" {
		target = "→ " + it.projectName
	}
	desc := s.Description
	if desc == "" {
		desc = "(no description)"
	}
	return fmt.Sprintf("%s–%s  %-6s  %-16s  %s",
		s.Start.Format("15:04"),
		s.End.Format("15:04"),
		formatDuration(s.Duration()),
		target,
		desc,
	)
}

func (m Model) viewEdit() string {
	cur := m.current()
	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit description"))
	b.WriteString("\n\n")
	if cur != nil {
		b.WriteString(helpStyle.Render(m.rowText(*cur)))
		b.WriteString("\n\n")
	}
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter save · esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewSplit() string {
	cur := m.current()
	var b strings.Builder
	b.WriteString(titleStyle.Render("Split session"))
	b.WriteString("\n\n")
	if cur != nil {
		at := cur.session.Start.Add(m.splitOffset)
		b.WriteString(fmt.Sprintf("  %s  →  %s + %s\n\n",
			m.rowText(*cur),
			fmt.Sprintf("%s–%s", cur.session.Start.Format("15:04"), at.Format("15:04")),
			fmt.Sprintf("%s–%s", at.Format("15:04"), cur.session.End.Format("15:04")),
		))
		b.WriteString(fmt.Sprintf("  split at %s\n\n", cursorStyle.Render(at.Format("15:04"))))
	}
	b.WriteString(helpStyle.Render("←/→ ±1m · H/L ±5m · enter split · esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewPickProject() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Reassign — pick a project"))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	shown := m.ranked
	const maxRows = 10
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for i, mt := range shown {
		cursor := "  "
		if i == m.pickCursor {
			cursor = cursorStyle.Render("➤ ")
		}
		b.WriteString(cursor + m.projects[mt.Index].Name + "\n")
	}
	if len(m.ranked) == 0 {
		b.WriteString(helpStyle.Render("  no match\n"))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("type to filter · ↑/↓ move · enter select · esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewPickTask() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Reassign — pick a task in " + m.pending.Name))
	b.WriteString("\n\n")
	rows := append([]string{"(no task)"}, taskNames(m.tasks)...)
	for i, name := range rows {
		cursor := "  "
		if i == m.pickCursor {
			cursor = cursorStyle.Render("➤ ")
		}
		b.WriteString(cursor + name + "\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ move · enter select · esc cancel"))
	b.WriteString("\n")
	return b.String()
}

func (m Model) viewConfirm() string {
	n := len(m.pushItems())
	var b strings.Builder
	b.WriteString(titleStyle.Render("Push to Clockify"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Push %d session%s to Clockify?\n\n", n, plural(n)))
	b.WriteString(helpStyle.Render("y confirm · n cancel"))
	b.WriteString("\n")
	return b.String()
}

func taskNames(tasks []Task) []string {
	names := make([]string, len(tasks))
	for i, t := range tasks {
		names[i] = t.Name
	}
	return names
}

// dash renders empty values as "-" for readability.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// formatDuration renders a duration rounded to whole minutes, e.g. "28m" or
// "1h05m".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d / time.Hour)
	mins := int((d % time.Hour) / time.Minute)
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
