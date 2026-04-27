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
	Match  bool
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
	actionSetFilterVersion
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
	filterVersion   string
	lastVersionFile string
	versions        []string
	mode            mode
	inputAction     inputAction
	inputTargetID   string
	input           textinput.Model
	showHelp        bool
	pendingDeleteID string
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
	pathArg, exportMode, showHelp, err := parseCLIArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "args error: %v\n", err)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, usageText())
		os.Exit(1)
	}
	if showHelp {
		fmt.Print(usageText())
		return
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

	if exportMode != 0 {
		if err := m.syncCurrentVersionFromFile(); err != nil {
			fmt.Fprintf(os.Stderr, "version read failed: %v\n", err)
			os.Exit(1)
		}
		if err := m.exportNow(exportMode == 2); err != nil {
			fmt.Fprintf(os.Stderr, "export error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "runtime error: %v\n", err)
		os.Exit(1)
	}
}

func parseCLIArgs(args []string) (string, int, bool, error) {
	pathArg := ""
	exportMode := 0
	showHelp := false
	for _, raw := range args {
		a := strings.TrimSpace(raw)
		if a == "" {
			continue
		}
		switch a {
		case "-h", "--help":
			showHelp = true
			continue
		case "-e":
			if exportMode != 0 {
				return "", 0, false, fmt.Errorf("use only one export flag: -e or -E")
			}
			exportMode = 1
			continue
		case "-E":
			if exportMode != 0 {
				return "", 0, false, fmt.Errorf("use only one export flag: -e or -E")
			}
			exportMode = 2
			continue
		}
		if strings.HasPrefix(a, "-") {
			return "", 0, false, fmt.Errorf("unknown flag: %s", a)
		}
		if pathArg != "" {
			return "", 0, false, fmt.Errorf("multiple paths provided")
		}
		pathArg = a
	}
	if showHelp {
		return "", 0, true, nil
	}
	return pathArg, exportMode, false, nil
}

func usageText() string {
	return strings.TrimSpace(`Trailblazer - terminal roadmap planner

Usage:
  trailblazer [flags] [path]

Arguments:
  path                  CSV file or directory (default: ./trailblazer.csv)

Flags:
  -e                    Export parents-only markdown and exit
  -E                    Export full markdown tree and exit
  -h, --help            Show this help

TUI keys:
  q                     Quit
  arrows/j/k            Move cursor
  h/l or Enter          Collapse/expand selected task
  a / A                 Add child/root task
  space                 Toggle done/open
  d / x                 Delete selected task with children
  n                     Set current version
  r                     Set selected task version
	v                     Set filter version (all or x.y.z)
  [ / ]                 Cycle version filter
  0                     Clear version filter
  e / E                 Export parents-only/full markdown
`) + "\n"
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
	m.filterVersion = ""
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
	key := k.String()
	if key != "d" && key != "x" {
		m.pendingDeleteID = ""
	}
	if next, cmd, handled := m.handleNavigationKey(key); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleTaskEditingKey(key); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleFilterKey(key); handled {
		return next, cmd
	}
	if next, cmd, handled := m.handleExportKey(key); handled {
		return next, cmd
	}
	return m, nil
}

func (m model) handleNavigationKey(key string) (model, tea.Cmd, bool) {
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit, true
	case "?":
		m.showHelp = !m.showHelp
		if m.showHelp {
			m.status = "help: press ? or esc to close"
		} else {
			m.status = "help closed"
		}
		return m, nil, true
	case "esc":
		if m.showHelp {
			m.showHelp = false
			m.status = "help closed"
			return m, nil, true
		}
		return m, nil, false
	case "up", "k":
		m.moveCursor(-1)
		return m, nil, true
	case "down", "j":
		m.moveCursor(1)
		return m, nil, true
	case "left", "h":
		m.setSelectedExpanded(false)
		return m, nil, true
	case "right", "l", "enter":
		m.setSelectedExpanded(true)
		return m, nil, true
	case "space", " ":
		return m, m.toggleSelectedDoneCmd(), true
	default:
		return m, nil, false
	}
}

func (m model) handleTaskEditingKey(key string) (model, tea.Cmd, bool) {
	switch key {
	case "a":
		m.startAddTask(true)
		return m, nil, true
	case "A":
		m.startAddTask(false)
		return m, nil, true
	case "d", "x":
		t := m.selected()
		if t == nil {
			return m, nil, true
		}
		if m.pendingDeleteID != t.ID {
			m.pendingDeleteID = t.ID
			children := countDescendants(t)
			m.status = fmt.Sprintf("confirm delete: press d again to remove %s (%d subtasks)", t.ID, children)
			return m, nil, true
		}
		m.pendingDeleteID = ""
		m.deleteCascade(t)
		m.rebuildVersions()
		m.rebuildVisible()
		m.status = fmt.Sprintf("deleted %s", t.ID)
		return m, m.saveCmd(), true
	case "n":
		m.startInput(actionNewVersion, "new version", "")
		return m, nil, true
	case "r":
		t := m.selected()
		if t == nil {
			return m, nil, true
		}
		m.inputTargetID = t.ID
		m.startInput(actionSetVersion, "set version (empty clears)", t.Version)
		return m, nil, true
	case "v":
		initial := m.filterVersion
		if initial == "" {
			initial = "all"
		}
		m.startInput(actionSetFilterVersion, "filter version (all or x.y.z)", initial)
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) handleFilterKey(key string) (model, tea.Cmd, bool) {
	switch key {
	case "]":
		m.cycleFilter(1)
		m.rebuildVisible()
		m.status = "filter: " + showFilterVersion(m.filterVersion)
		return m, nil, true
	case "[":
		m.cycleFilter(-1)
		m.rebuildVisible()
		m.status = "filter: " + showFilterVersion(m.filterVersion)
		return m, nil, true
	case "0":
		m.filterVersion = ""
		m.rebuildVisible()
		m.status = "filter: all"
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m model) handleExportKey(key string) (model, tea.Cmd, bool) {
	switch key {
	case "e":
		if err := m.syncCurrentVersionFromFile(); err != nil {
			m.status = "version read failed: " + err.Error()
			return m, nil, true
		}
		return m, m.exportCmd(false), true
	case "E":
		if err := m.syncCurrentVersionFromFile(); err != nil {
			m.status = "version read failed: " + err.Error()
			return m, nil, true
		}
		return m, m.exportCmd(true), true
	default:
		return m, nil, false
	}
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
	case actionSetFilterVersion:
		return m.commitSetFilterVersion(value)
	case actionInitProjectName:
		return m.commitInitProjectName(value)
	case actionInitCurrentVersion:
		return m.commitInitCurrentVersion(value)
	}
	return nil, true
}

func (m *model) commitAddTask(value string) (tea.Cmd, bool) {
	if value == "" {
		m.status = "empty title; type a task name or esc to cancel"
		return nil, false
	}
	typeName, title := parseTaskInput(value)
	if title == "" {
		m.status = "empty title; type a task name or esc to cancel"
		return nil, false
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

func (m *model) commitSetFilterVersion(value string) (tea.Cmd, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "all" {
		m.filterVersion = ""
		m.rebuildVisible()
		m.status = "filter: all versions"
		return nil, true
	}
	parsed := parseVersionValue(value)
	if parsed == "" {
		m.status = "invalid filter version (use all or x.y.z)"
		return nil, false
	}
	if !contains(m.versions, parsed) {
		m.status = "version not found in project"
		return nil, false
	}
	m.filterVersion = parsed
	m.rebuildVisible()
	m.status = "filter: " + parsed
	return nil, true
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
	value = strings.TrimSpace(value)
	if value != "" {
		parsed := parseVersionValue(value)
		if parsed == "" {
			m.status = "invalid version format (use x.y.z)"
			return nil, false
		}
		value = parsed
	}
	t.Version = value
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

func (m *model) cycleFilter(delta int) {
	options := append([]string{""}, m.versions...)
	if len(options) <= 1 {
		m.filterVersion = ""
		return
	}
	idx := 0
	for i, v := range options {
		if v == m.filterVersion {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(options)
	if idx < 0 {
		idx += len(options)
	}
	m.filterVersion = options[idx]
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
	selectedID := m.selectedID()
	matchPath, matchSelf := m.computeVisibleMatchMaps()
	m.visible = m.buildVisibleRows(matchPath, matchSelf)
	m.restoreCursor(selectedID)
}

func (m *model) selectedID() string {
	if sel := m.selected(); sel != nil {
		return sel.ID
	}
	return ""
}

func (m *model) computeVisibleMatchMaps() (map[*task]bool, map[*task]bool) {
	matchPath := map[*task]bool{}
	matchSelf := map[*task]bool{}
	var mark func(*task) bool
	mark = func(t *task) bool {
		self := m.filterVersion == "" || t.Version == m.filterVersion
		child := false
		for _, c := range t.Children {
			if mark(c) {
				child = true
			}
		}
		matchSelf[t] = self
		matchPath[t] = self || child
		return matchPath[t]
	}
	for _, r := range m.roots {
		mark(r)
	}
	return matchPath, matchSelf
}

func (m *model) buildVisibleRows(matchPath, matchSelf map[*task]bool) []visibleTask {
	out := make([]visibleTask, 0, len(m.tasksByID))
	var walk func(nodes []*task, ancestorsLast []bool)
	walk = func(nodes []*task, ancestorsLast []bool) {
		for i, t := range nodes {
			if !matchPath[t] {
				continue
			}
			last := i == len(nodes)-1
			prefix := treePrefix(ancestorsLast, last)
			out = append(out, visibleTask{Node: t, Prefix: prefix, Match: matchSelf[t]})
			if t.Expanded {
				ancestorsLast = append(ancestorsLast, last)
				walk(t.Children, ancestorsLast)
				ancestorsLast = ancestorsLast[:len(ancestorsLast)-1]
			}
		}
	}
	walk(m.roots, nil)
	return out
}

func treePrefix(ancestorsLast []bool, isLast bool) string {
	prefix := ""
	for _, wasLast := range ancestorsLast {
		if wasLast {
			prefix += "  "
		} else {
			prefix += "│ "
		}
	}
	if len(ancestorsLast) == 0 {
		return prefix
	}
	if isLast {
		return prefix + "└─"
	}
	return prefix + "├─"
}

func (m *model) restoreCursor(selectedID string) {
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}
	if selectedID != "" {
		if idx := m.visibleIndex(selectedID); idx >= 0 {
			m.cursor = idx
			return
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *model) visibleIndex(id string) int {
	for i, v := range m.visible {
		if v.Node.ID == id {
			return i
		}
	}
	return -1
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
	path := exportPathForCSV(m.csvPath)
	roots := m.roots
	currentVersion := m.currentVersion
	return func() tea.Msg {
		err := writeMarkdown(path, roots, currentVersion, includeSubtasks)
		return exportDoneMsg{path: path, err: err}
	}
}

func (m model) exportNow(includeSubtasks bool) error {
	return writeMarkdown(exportPathForCSV(m.csvPath), m.roots, m.currentVersion, includeSubtasks)
}

func exportPathForCSV(csvPath string) string {
	return filepath.Join(filepath.Dir(csvPath), "trailblazer.md")
}

func (m model) View() string {
	header := headerStyle.Render(fmt.Sprintf("Project: %s | CSV: %s | Project Version: %s | Filter: %s", m.projectName, filepath.Base(m.csvPath), showCurrentVersion(m.currentVersion), showFilterVersion(m.filterVersion)))
	body := m.renderBody()
	footer := faintStyle.Render("q quit | ? help | arrows/j/k move | a child | A root | d delete(confirm) | space done | n version | r task version | v filter | e/E export")
	if m.showHelp {
		body = m.renderHelpPanel()
	}
	status := openStyle.Render("status: " + m.status)
	if m.mode == modeInput {
		status = cursorStyle.Render("input: ") + m.input.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer, status)
}

func shortHelpText() string {
	return "keys: a/A add task, space toggle done, d delete(confirm), n/r version, v or [/] filter, e/E export"
}

func (m model) renderHelpPanel() string {
	return strings.Join([]string{
		"",
		headerStyle.Render("Help"),
		"  move: arrows / j / k",
		"  expand/collapse: h / l / enter",
		"  add: a child, A root",
		"  done/open: space",
		"  delete: d then d to confirm",
		"  versions: n set project, r set task, v set filter",
		"  filter quick cycle: [ / ]  | clear: 0",
		"  export: e parents-only, E full tree",
		"  close help: ? or esc",
		"",
	}, "\n")
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
		if m.filterVersion != "" && !v.Match {
			title = faintStyle.Render(title)
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
	recs, err := readCSVRecords(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []*task{}, map[string]*task{}, nil
		}
		return nil, nil, err
	}
	if len(recs) == 0 {
		return []*task{}, map[string]*task{}, nil
	}
	rows := parseTaskRows(recs)
	byID := buildTaskIndex(rows)
	roots := buildTaskTree(rows, byID)
	return roots, byID, nil
}

func readCSVRecords(path string) ([][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

func parseTaskRows(recs [][]string) []taskRow {
	start := 0
	if len(recs) > 0 && len(recs[0]) > 0 && strings.EqualFold(strings.TrimSpace(recs[0][0]), "ID") {
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
	return rows
}

func buildTaskIndex(rows []taskRow) map[string]*task {
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
	return byID
}

func buildTaskTree(rows []taskRow, byID map[string]*task) []*task {
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
	return roots
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
	defer func() { _ = os.Remove(tmp) }()
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
		// Windows can't rename over an existing file. Fallback to remove+rename,
		// with best-effort restoration if replacement fails.
		oldData, readErr := os.ReadFile(path)
		hadOld := readErr == nil
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			return rmErr
		}
		if err2 := os.Rename(tmp, path); err2 != nil {
			if hadOld {
				_ = os.WriteFile(path, oldData, 0o644)
			}
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

func countDescendants(t *task) int {
	if t == nil {
		return 0
	}
	total := len(t.Children)
	for _, c := range t.Children {
		total += countDescendants(c)
	}
	return total
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

func showCurrentVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func showFilterVersion(v string) string {
	if v == "" {
		return "all"
	}
	return v
}
