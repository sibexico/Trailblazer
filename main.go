package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type task struct {
	ID       string
	ParentID string
	Version  string
	Type     string
	Status   string
	Title    string
	Expanded bool
	Children []*task
}

type taskRow struct {
	ID       string
	ParentID string
	Version  string
	Type     string
	Status   string
	Title    string
}

type visibleTask struct {
	Node   *task
	Prefix string
}

type mode int

const (
	modeNormal mode = iota
	modeInput
)

const versionPollInterval = 2 * time.Second

type inputAction int

const (
	actionNone inputAction = iota
	actionAddRoot
	actionAddChild
	actionNewVersion
	actionSetVersion
	actionInitProjectName
	actionInitCurrentVersion
)

type saveDoneMsg struct {
	err error
}

type exportDoneMsg struct {
	path string
	err  error
}

type versionFilePollMsg struct {
	version string
	err     error
}

type versionFileWriteDoneMsg struct {
	version string
	err     error
}

type model struct {
	projectName     string
	csvPath         string
	tasksByID       map[string]*task
	roots           []*task
	visible         []visibleTask
	cursor          int
	currentVersion  string
	lastVersionFile string
	versions        []string
	mode            mode
	inputAction     inputAction
	inputTargetID   string
	input           textinput.Model
	status          string
	width           int
	height          int
}

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7FDBFF"))
	faintStyle  = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D084")).Bold(true)
	openStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#DDDDDD"))
	doneStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8A8A8A")).Strikethrough(true)
	featureTag  = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	bugfixTag   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0055")).Bold(true)
	improveTag  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Bold(true)
	missedTag   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4D4D")).Bold(true)
	versionRe   = regexp.MustCompile(`(?i)\bv(?:\.)?(\d+\.\d+\.\d+)\b|\b(\d+\.\d+\.\d+)\b`)
)

func main() {
	pathArg := ""
	if len(os.Args) > 1 {
		pathArg = strings.TrimSpace(os.Args[1])
	}
	path, err := resolveCSVPath(pathArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "path error: %v\n", err)
		os.Exit(1)
	}

	m, err := newModel(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init error: %v\n", err)
		os.Exit(1)
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}

func newModel(csvPath string) (model, error) {
	// Build the state from CSV and optional VERSION file.
	_, statErr := os.Stat(csvPath)
	isNewProject := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !isNewProject {
		return model{}, statErr
	}

	roots, byID, err := loadCSV(csvPath)
	if err != nil {
		return model{}, err
	}
	in := textinput.New()
	in.Prompt = "> "
	in.CharLimit = 512
	in.Width = 60
	m := model{
		projectName: strings.TrimSuffix(filepath.Base(csvPath), filepath.Ext(csvPath)),
		csvPath:     csvPath,
		tasksByID:   byID,
		roots:       roots,
		input:       in,
		status:      "ready",
	}
	m.rebuildVersions()
	versionFromFile, err := readVersionFile(filepath.Dir(csvPath))
	if err != nil {
		return model{}, err
	}
	if versionFromFile != "" {
		m.setCurrentVersion(versionFromFile)
	}
	m.lastVersionFile = versionFromFile
	m.rebuildVisible()
	if isNewProject {
		m.status = "new project setup"
		m.startInput(actionInitProjectName, "project name", m.projectName)
	}
	return m, nil
}

func (m model) Init() tea.Cmd { return m.pollVersionCmd() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleRuntimeMsg(msg); handled {
		return next, cmd
	}

	if m.mode == modeInput {
		return m.updateInput(msg)
	}

	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	return m.handleKey(k)
}

func (m model) handleRuntimeMsg(msg tea.Msg) (model, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil, true
	case saveDoneMsg:
		if msg.err != nil {
			m.status = "save failed: " + msg.err.Error()
		} else {
			m.status = "saved " + filepath.Base(m.csvPath)
		}
		return m, nil, true
	case exportDoneMsg:
		if msg.err != nil {
			m.status = "export failed: " + msg.err.Error()
		} else {
			m.status = "exported " + filepath.Base(msg.path)
		}
		return m, nil, true
	case versionFileWriteDoneMsg:
		if msg.err != nil {
			m.status = "VERSION write failed: " + msg.err.Error()
			return m, nil, true
		}
		m.lastVersionFile = msg.version
		return m, nil, true
	case versionFilePollMsg:
		if msg.err != nil {
			return m, m.pollVersionCmd(), true
		}
		if msg.version == "" {
			m.lastVersionFile = ""
			return m, m.pollVersionCmd(), true
		}
		if msg.version != m.lastVersionFile {
			m.lastVersionFile = msg.version
			if m.currentVersion != msg.version {
				m.setCurrentVersion(msg.version)
				m.rebuildVisible()
				m.status = "version updated from VERSION: " + msg.version
			}
		}
		return m, m.pollVersionCmd(), true
	default:
		return m, nil, false
	}
}

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "left", "h":
		m.setSelectedExpanded(false)
		return m, nil
	case "right", "l", "enter":
		m.setSelectedExpanded(true)
		return m, nil
	case "space", " ":
		return m, m.toggleSelectedDoneCmd()
	case "a":
		m.startAddTask(true)
		return m, nil
	case "A":
		m.startAddTask(false)
		return m, nil
	case "d", "x":
		t := m.selected()
		if t == nil {
			return m, nil
		}
		m.deleteCascade(t)
		m.rebuildVersions()
		m.rebuildVisible()
		m.status = "deleted " + t.ID
		return m, m.saveCmd()
	case "n":
		m.startInput(actionNewVersion, "new version", "")
		return m, nil
	case "r":
		t := m.selected()
		if t == nil {
			return m, nil
		}
		m.inputTargetID = t.ID
		m.startInput(actionSetVersion, "set version (empty clears)", t.Version)
		return m, nil
	case "]":
		m.cycleVersion(1)
		m.rebuildVisible()
		return m, nil
	case "[":
		m.cycleVersion(-1)
		m.rebuildVisible()
		return m, nil
	case "0":
		m.setCurrentVersion("")
		m.rebuildVisible()
		return m, nil
	case "e":
		if err := m.syncCurrentVersionFromFile(); err != nil {
			m.status = "version read failed: " + err.Error()
			return m, nil
		}
		return m, m.exportCmd(false)
	case "E":
		if err := m.syncCurrentVersionFromFile(); err != nil {
			m.status = "version read failed: " + err.Error()
			return m, nil
		}
		return m, m.exportCmd(true)
	}

	return m, nil
}

func (m model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch k.String() {
		case "esc":
			m.mode = modeNormal
			m.inputAction = actionNone
			m.inputTargetID = ""
			m.input.Blur()
			m.status = "cancelled"
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			cmd, closeInput := m.commitInput(value)
			if closeInput {
				m.mode = modeNormal
				m.input.Blur()
				m.inputAction = actionNone
				m.inputTargetID = ""
				m.input.SetValue("")
			}
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) commitInput(value string) (tea.Cmd, bool) {
	switch m.inputAction {
	case actionAddRoot, actionAddChild:
		return m.commitAddTask(value)
	case actionNewVersion:
		return m.commitNewVersion(value)
	case actionSetVersion:
		return m.commitSetTaskVersion(value)
	case actionInitProjectName:
		return m.commitInitProjectName(value)
	case actionInitCurrentVersion:
		return m.commitInitCurrentVersion(value)
	}
	return nil, true
}

func (m *model) commitAddTask(value string) (tea.Cmd, bool) {
	if value == "" {
		m.status = "empty title"
		return nil, true
	}
	typeName, title := parseTaskInput(value)
	if title == "" {
		m.status = "empty title"
		return nil, true
	}

	parentID := ""
	if m.inputAction == actionAddChild {
		parentID = m.inputTargetID
	}
	id := newTaskID()
	t := &task{
		ID:       id,
		ParentID: parentID,
		Version:  m.currentVersion,
		Type:     typeName,
		Status:   "open",
		Title:    title,
		Expanded: true,
	}
	m.tasksByID[id] = t
	if parentID == "" {
		m.roots = append(m.roots, t)
	} else if p := m.tasksByID[parentID]; p != nil {
		p.Children = append(p.Children, t)
	} else {
		t.ParentID = ""
		m.roots = append(m.roots, t)
	}

	m.rebuildVersions()
	m.rebuildVisible()
	m.cursorToID(id)
	m.status = "added " + id
	return m.saveCmd(), true
}

func (m *model) commitNewVersion(value string) (tea.Cmd, bool) {
	if value == "" {
		m.status = "empty version"
		return nil, true
	}
	value = parseVersionValue(value)
	if value == "" {
		m.status = "invalid version format (use x.y.z)"
		return nil, false
	}
	m.setCurrentVersion(value)
	m.rebuildVisible()
	m.status = "version " + value
	return m.writeVersionFileCmd(value), true
}

func (m *model) commitSetTaskVersion(value string) (tea.Cmd, bool) {
	t := m.tasksByID[m.inputTargetID]
	if t == nil {
		m.status = "task not found"
		return nil, true
	}
	t.Version = value
	if value != "" && !contains(m.versions, value) {
		m.versions = append(m.versions, value)
		sort.Strings(m.versions)
	}
	m.rebuildVersions()
	m.rebuildVisible()
	m.cursorToID(t.ID)
	m.status = "updated version"
	return m.saveCmd(), true
}

func (m *model) commitInitProjectName(value string) (tea.Cmd, bool) {
	if value == "" {
		m.status = "project name required"
		return nil, false
	}
	m.projectName = value
	m.status = "set current version"
	m.startInput(actionInitCurrentVersion, "current version (optional)", m.currentVersion)
	return nil, false
}

func (m *model) commitInitCurrentVersion(value string) (tea.Cmd, bool) {
	if value != "" {
		value = parseVersionValue(value)
		if value == "" {
			m.status = "invalid version format (use x.y.z)"
			return nil, false
		}
		m.setCurrentVersion(value)
		m.rebuildVisible()
		m.status = "project initialized"
		return tea.Batch(m.saveCmd(), m.writeVersionFileCmd(value)), true
	}
	m.rebuildVisible()
	m.status = "project initialized"
	return m.saveCmd(), true
}

func (m *model) startInput(a inputAction, prompt string, initial string) {
	m.mode = modeInput
	m.inputAction = a
	m.input.Prompt = prompt + " > "
	m.input.SetValue(initial)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *model) selected() *task {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.visible[m.cursor].Node
}

func (m *model) moveCursor(delta int) {
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) setSelectedExpanded(expanded bool) {
	t := m.selected()
	if t == nil {
		return
	}
	t.Expanded = expanded
	m.rebuildVisible()
}

func (m *model) toggleSelectedDoneCmd() tea.Cmd {
	t := m.selected()
	if t == nil {
		return nil
	}
	if t.Status == "done" {
		t.Status = "open"
	} else {
		t.Status = "done"
	}
	m.rebuildVisible()
	return m.saveCmd()
}

func (m *model) startAddTask(asChild bool) {
	if asChild {
		m.startInput(actionAddChild, "new task (f:/b:/i: prefix)", "")
		sel := m.selected()
		if sel != nil {
			m.inputTargetID = sel.ID
			return
		}
		m.inputAction = actionAddRoot
		return
	}
	m.startInput(actionAddRoot, "new root task (f:/b:/i: prefix)", "")
}

func (m *model) cycleVersion(delta int) {
	options := append([]string{""}, m.versions...)
	if len(options) <= 1 {
		m.currentVersion = ""
		return
	}
	idx := 0
	for i, v := range options {
		if v == m.currentVersion {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(options)
	if idx < 0 {
		idx += len(options)
	}
	m.currentVersion = options[idx]
}

func (m *model) rebuildVersions() {
	seen := map[string]struct{}{}
	for _, t := range m.tasksByID {
		if t.Version != "" {
			seen[t.Version] = struct{}{}
		}
	}
	if m.currentVersion != "" {
		seen[m.currentVersion] = struct{}{}
	}
	m.versions = m.versions[:0]
	for v := range seen {
		m.versions = append(m.versions, v)
	}
	sort.Strings(m.versions)
}

func (m *model) rebuildVisible() {
	// Rebuild the visible list used for cursor-based navigation.
	selectedID := ""
	if sel := m.selected(); sel != nil {
		selectedID = sel.ID
	}

	out := make([]visibleTask, 0, len(m.tasksByID))
	var walk func(nodes []*task, ancestorsLast []bool)
	walk = func(nodes []*task, ancestorsLast []bool) {
		for i, t := range nodes {
			last := i == len(nodes)-1
			prefix := ""
			for _, wasLast := range ancestorsLast {
				if wasLast {
					prefix += "  "
				} else {
					prefix += "│ "
				}
			}
			if len(ancestorsLast) > 0 {
				if last {
					prefix += "└─"
				} else {
					prefix += "├─"
				}
			}
			out = append(out, visibleTask{Node: t, Prefix: prefix})
			if t.Expanded {
				ancestorsLast = append(ancestorsLast, last)
				walk(t.Children, ancestorsLast)
				ancestorsLast = ancestorsLast[:len(ancestorsLast)-1]
			}
		}
	}
	walk(m.roots, nil)
	m.visible = out
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}
	if selectedID != "" {
		for i, v := range m.visible {
			if v.Node.ID == selectedID {
				m.cursor = i
				return
			}
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) cursorToID(id string) {
	for i, v := range m.visible {
		if v.Node.ID == id {
			m.cursor = i
			return
		}
	}
}

func (m *model) deleteCascade(t *task) {
	// Remove selected node and all children from tree.
	if t.ParentID == "" {
		m.roots = removeTaskFromSlice(m.roots, t.ID)
	} else if p := m.tasksByID[t.ParentID]; p != nil {
		p.Children = removeTaskFromSlice(p.Children, t.ID)
	}
	var drop func(*task)
	drop = func(n *task) {
		for _, c := range n.Children {
			drop(c)
		}
		delete(m.tasksByID, n.ID)
	}
	drop(t)
}

func (m *model) syncCurrentVersionFromFile() error {
	// Refresh current version from VERSION file if it was edited.
	v, err := readVersionFile(filepath.Dir(m.csvPath))
	if err != nil {
		return err
	}
	if v != "" {
		m.lastVersionFile = v
	}
	if v == "" || v == m.currentVersion {
		return nil
	}
	m.setCurrentVersion(v)
	m.rebuildVisible()
	m.status = "version updated from VERSION: " + v
	return nil
}

func (m *model) setCurrentVersion(v string) {
	v = strings.TrimSpace(v)
	m.currentVersion = v
	if v == "" || contains(m.versions, v) {
		return
	}
	m.versions = append(m.versions, v)
	sort.Strings(m.versions)
}

func (m model) pollVersionCmd() tea.Cmd {
	// Periodically check whether VERSION changed on disk.
	dir := filepath.Dir(m.csvPath)
	return tea.Tick(versionPollInterval, func(time.Time) tea.Msg {
		v, err := readVersionFile(dir)
		return versionFilePollMsg{version: v, err: err}
	})
}

func (m model) writeVersionFileCmd(version string) tea.Cmd {
	// Persist project current version to VERSION file.
	dir := filepath.Dir(m.csvPath)
	return func() tea.Msg {
		return versionFileWriteDoneMsg{version: version, err: writeVersionFile(dir, version)}
	}
}

func (m model) saveCmd() tea.Cmd {
	// Persist full in-memory tree.
	rows := flattenRows(m.roots)
	path := m.csvPath
	return func() tea.Msg {
		return saveDoneMsg{err: writeRows(path, rows)}
	}
}

func (m model) exportCmd(includeSubtasks bool) tea.Cmd {
	// Export roadmap to Markdown: parents-only or full tree.
	path := filepath.Join(filepath.Dir(m.csvPath), "trailblazer.md")
	roots := m.roots
	currentVersion := m.currentVersion
	return func() tea.Msg {
		err := writeMarkdown(path, roots, currentVersion, includeSubtasks)
		return exportDoneMsg{path: path, err: err}
	}
}

func (m model) View() string {
	header := headerStyle.Render(fmt.Sprintf("Project: %s | CSV: %s | Current Version: %s", m.projectName, filepath.Base(m.csvPath), showVersion(m.currentVersion)))
	body := m.renderBody()
	footer := faintStyle.Render("q quit | j/k move | h/l collapse/expand | a child | A root | d delete | space done | n new version | r set task version | [/ ] cycle version | 0 all | e export-parents | E export-full")
	status := openStyle.Render("status: " + m.status)
	if m.mode == modeInput {
		status = cursorStyle.Render("input: ") + m.input.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer, status)
}

func (m model) renderBody() string {
	if len(m.visible) == 0 {
		return "\n(no tasks; press A for root task)\n"
	}
	lines := make([]string, 0, len(m.visible)+2)
	for i, v := range m.visible {
		t := v.Node
		check := "[ ]"
		if t.Status == "done" {
			check = "[x]"
		}
		title := t.Title
		if t.Status == "done" {
			title = doneStyle.Render(title)
		}
		version := ""
		if t.Version != "" {
			version = faintStyle.Render("(" + t.Version + ")")
		}
		missed := ""
		if m.isMissed(t) {
			missed = " " + missedTag.Render("[missed]")
		}
		row := fmt.Sprintf("%s%s %s %s %s%s", v.Prefix, check, typeTag(t.Type), title, version, missed)
		if i == m.cursor {
			row = cursorStyle.Render("> ") + row
		} else {
			row = "  " + row
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

func typeTag(t string) string {
	switch t {
	case "bugfix":
		return bugfixTag.Render("[B]")
	case "improvement":
		return improveTag.Render("[I]")
	default:
		return featureTag.Render("[F]")
	}
}

func parseTaskInput(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "feature", ""
	}
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "f:"):
		return "feature", strings.TrimSpace(s[2:])
	case strings.HasPrefix(lower, "b:"):
		return "bugfix", strings.TrimSpace(s[2:])
	case strings.HasPrefix(lower, "i:"):
		return "improvement", strings.TrimSpace(s[2:])
	default:
		return "feature", s
	}
}

func newTaskID() string {
	return strings.ToUpper(fmt.Sprintf("T%X", time.Now().UnixNano()))
}

func resolveCSVPath(input string) (string, error) {
	// Accept file or directory.
	if input == "" {
		input = "trailblazer.csv"
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err == nil {
		if info.IsDir() {
			return filepath.Join(abs, "trailblazer.csv"), nil
		}
		return abs, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if filepath.Ext(abs) == "" {
		return filepath.Join(abs, "trailblazer.csv"), nil
	}
	return abs, nil
}

func readVersionFile(dir string) (string, error) {
	// Read semantic version from VERSION file if present.
	path := filepath.Join(dir, "VERSION")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return parseVersionValue(string(b)), nil
}

func writeVersionFile(dir string, version string) error {
	path := filepath.Join(dir, "VERSION")
	return os.WriteFile(path, []byte(strings.TrimSpace(version)+"\n"), 0o644)
}

func parseVersionValue(s string) string {
	// Keep only x.y.z from other formats like v1.2.3 or v.1.2.3.
	m := versionRe.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) == 0 {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	if m[2] != "" {
		return m[2]
	}
	return ""
}

func loadCSV(path string) ([]*task, map[string]*task, error) {
	// Create nodes, then connect parent-child links.
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []*task{}, map[string]*task{}, nil
		}
		return nil, nil, err
	}
	defer file.Close()

	r := csv.NewReader(file)
	recs := make([][]string, 0, 128)
	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, nil, err
		}
		recs = append(recs, rec)
	}
	if len(recs) == 0 {
		return []*task{}, map[string]*task{}, nil
	}
	start := 0
	if len(recs[0]) >= 1 && strings.EqualFold(strings.TrimSpace(recs[0][0]), "ID") {
		start = 1
	}
	rows := make([]taskRow, 0, len(recs)-start)
	for i := start; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) < 6 {
			continue
		}
		rows = append(rows, taskRow{
			ID:       strings.TrimSpace(rec[0]),
			ParentID: strings.TrimSpace(rec[1]),
			Version:  strings.TrimSpace(rec[2]),
			Type:     normalizeType(strings.TrimSpace(rec[3])),
			Status:   normalizeStatus(strings.TrimSpace(rec[4])),
			Title:    strings.TrimSpace(rec[5]),
		})
	}

	byID := make(map[string]*task, len(rows))
	for _, row := range rows {
		if row.ID == "" {
			continue
		}
		byID[row.ID] = &task{
			ID:       row.ID,
			ParentID: row.ParentID,
			Version:  row.Version,
			Type:     row.Type,
			Status:   row.Status,
			Title:    row.Title,
			Expanded: true,
		}
	}
	roots := make([]*task, 0)
	for _, row := range rows {
		n := byID[row.ID]
		if n == nil {
			continue
		}
		if n.ParentID == "" {
			roots = append(roots, n)
			continue
		}
		if p := byID[n.ParentID]; p != nil {
			p.Children = append(p.Children, n)
		} else {
			n.ParentID = ""
			roots = append(roots, n)
		}
	}
	return roots, byID, nil
}

func flattenRows(roots []*task) []taskRow {
	rows := make([]taskRow, 0, 64)
	var walk func(*task)
	walk = func(t *task) {
		rows = append(rows, taskRow{
			ID:       t.ID,
			ParentID: t.ParentID,
			Version:  t.Version,
			Type:     normalizeType(t.Type),
			Status:   normalizeStatus(t.Status),
			Title:    t.Title,
		})
		for _, c := range t.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return rows
}

func writeRows(path string, rows []taskRow) error {
	// Atomic write to avoid half-written files.
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if err := w.Write([]string{"ID", "ParentID", "Version", "Type", "Status", "Title"}); err != nil {
		f.Close()
		return err
	}
	for _, r := range rows {
		if err := w.Write([]string{r.ID, r.ParentID, r.Version, r.Type, r.Status, r.Title}); err != nil {
			f.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			return err2
		}
	}
	return nil
}

func writeMarkdown(path string, roots []*task, currentVersion string, includeSubtasks bool) error {
	// Render nested tasks as checkbox markdown.
	var b strings.Builder
	b.WriteString("# Roadmap\n\n")
	reportVersion := strings.TrimSpace(currentVersion)
	if reportVersion == "" {
		reportVersion = "unknown"
	}
	b.WriteString("Current Version: " + reportVersion + "\n\n")

	var walk func(*task, int)
	walk = func(t *task, depth int) {
		indent := strings.Repeat("  ", depth)
		check := "[ ]"
		if t.Status == "done" {
			check = "[x]"
		}
		typeText := "Feature"
		if t.Type == "bugfix" {
			typeText = "Bugfix"
		}
		if t.Type == "improvement" {
			typeText = "Improvement"
		}
		line := fmt.Sprintf("%s- %s **[%s]** %s", indent, check, typeText, t.Title)
		if t.Version != "" {
			line += fmt.Sprintf(" *(%s)*", t.Version)
		}
		b.WriteString(line + "\n")
		if includeSubtasks {
			for _, c := range t.Children {
				walk(c, depth+1)
			}
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "bugfix", "bug", "b":
		return "bugfix"
	case "improvement", "improve", "i":
		return "improvement"
	default:
		return "feature"
	}
}

func normalizeStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "done" || s == "closed" || s == "x" {
		return "done"
	}
	return "open"
}

func removeTaskFromSlice(tasks []*task, id string) []*task {
	out := tasks[:0]
	for _, t := range tasks {
		if t.ID != id {
			out = append(out, t)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func versionIndex(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

func (m model) isMissed(t *task) bool {
	// A task is missed if it's still open from an earlier version.
	if m.currentVersion == "" || t == nil || t.Status == "done" || t.Version == "" {
		return false
	}
	cur := versionIndex(m.versions, m.currentVersion)
	taskVer := versionIndex(m.versions, t.Version)
	if cur < 0 || taskVer < 0 {
		return false
	}
	return taskVer < cur
}

func showVersion(v string) string {
	if v == "" {
		return "all"
	}
	return v
}
