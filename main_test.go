package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseCLIArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		exportMode int
		help       bool
		wantErr    bool
	}{
		{name: "default", args: nil, path: "", exportMode: 0, help: false, wantErr: false},
		{name: "path only", args: []string{"proj"}, path: "proj", exportMode: 0, help: false, wantErr: false},
		{name: "export parents", args: []string{"-e", "proj"}, path: "proj", exportMode: 1, help: false, wantErr: false},
		{name: "export full", args: []string{"-E", "proj"}, path: "proj", exportMode: 2, help: false, wantErr: false},
		{name: "help literal as path", args: []string{"help"}, path: "help", exportMode: 0, help: false, wantErr: false},
		{name: "help short", args: []string{"-h"}, path: "", exportMode: 0, help: true, wantErr: false},
		{name: "help long", args: []string{"--help"}, path: "", exportMode: 0, help: true, wantErr: false},
		{name: "unknown flag", args: []string{"-z"}, wantErr: true},
		{name: "conflicting export flags", args: []string{"-e", "-E"}, wantErr: true},
		{name: "multiple paths", args: []string{"a", "b"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, exportMode, help, err := parseCLIArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if path != tt.path || exportMode != tt.exportMode || help != tt.help {
				t.Fatalf("unexpected parse result: got (%q, %d, %v)", path, exportMode, help)
			}
		})
	}
}

func TestUsageText(t *testing.T) {
	text := usageText()
	if !strings.Contains(text, "Usage:") {
		t.Fatalf("usage text missing Usage section")
	}
	if !strings.Contains(text, "-h, --help") {
		t.Fatalf("usage text missing help flag")
	}
}

func TestShortHelpText(t *testing.T) {
	text := shortHelpText()
	if !strings.Contains(text, "add task") || !strings.Contains(text, "export") {
		t.Fatalf("shortHelpText missing key guidance: %q", text)
	}
}

func TestParseVersionValue(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "1.2.3", want: "1.2.3"},
		{in: "v1.2.3", want: "1.2.3"},
		{in: "v.1.2.3", want: "1.2.3"},
		{in: "release 2.3.4 now", want: "2.3.4"},
		{in: "no version", want: ""},
	}
	for _, tt := range tests {
		if got := parseVersionValue(tt.in); got != tt.want {
			t.Fatalf("parseVersionValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseTaskInput(t *testing.T) {
	tests := []struct {
		in        string
		wantType  string
		wantTitle string
	}{
		{in: "f: add auth", wantType: "feature", wantTitle: "add auth"},
		{in: "b: fix crash", wantType: "bugfix", wantTitle: "fix crash"},
		{in: "i: faster load", wantType: "improvement", wantTitle: "faster load"},
		{in: "plain title", wantType: "feature", wantTitle: "plain title"},
		{in: "", wantType: "feature", wantTitle: ""},
	}
	for _, tt := range tests {
		gotType, gotTitle := parseTaskInput(tt.in)
		if gotType != tt.wantType || gotTitle != tt.wantTitle {
			t.Fatalf("parseTaskInput(%q) = (%q,%q), want (%q,%q)", tt.in, gotType, gotTitle, tt.wantType, tt.wantTitle)
		}
	}
}

func TestParseTaskRowsBuildTreeAndFlatten(t *testing.T) {
	recs := [][]string{
		{"ID", "ParentID", "Version", "Type", "Status", "Title"},
		{"T1", "", "1.0.0", "f", "open", "root"},
		{"T2", "T1", "1.0.0", "b", "x", "child"},
		{"T3", "MISSING", "", "i", "closed", "orphan"},
		{"bad"},
	}
	rows := parseTaskRows(recs)
	if len(rows) != 3 {
		t.Fatalf("expected 3 valid rows, got %d", len(rows))
	}
	byID := buildTaskIndex(rows)
	roots := buildTaskTree(rows, byID)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}
	if got := byID["T2"].Status; got != "done" {
		t.Fatalf("expected T2 status done, got %q", got)
	}
	flat := flattenRows(roots)
	if len(flat) != 3 {
		t.Fatalf("expected 3 flattened rows, got %d", len(flat))
	}
}

func TestResolveCSVPath(t *testing.T) {
	tmp := t.TempDir()

	dirPath, err := resolveCSVPath(tmp)
	if err != nil {
		t.Fatalf("resolveCSVPath(dir) error: %v", err)
	}
	if got, want := filepath.Base(dirPath), "trailblazer.csv"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	extless := filepath.Join(tmp, "project")
	p, err := resolveCSVPath(extless)
	if err != nil {
		t.Fatalf("resolveCSVPath(extless) error: %v", err)
	}
	if got, want := filepath.Base(p), "trailblazer.csv"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	filePath := filepath.Join(tmp, "roadmap.csv")
	if err := os.WriteFile(filePath, []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	p2, err := resolveCSVPath(filePath)
	if err != nil {
		t.Fatalf("resolveCSVPath(file) error: %v", err)
	}
	if p2 != filePath {
		t.Fatalf("expected exact file path, got %q", p2)
	}
}

func TestWriteAndReadVersionFile(t *testing.T) {
	tmp := t.TempDir()
	if err := writeVersionFile(tmp, " 1.2.3 "); err != nil {
		t.Fatalf("writeVersionFile error: %v", err)
	}
	got, err := readVersionFile(tmp)
	if err != nil {
		t.Fatalf("readVersionFile error: %v", err)
	}
	if got != "1.2.3" {
		t.Fatalf("expected 1.2.3, got %q", got)
	}
}

func TestWriteRowsAndLoadCSVRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "trailblazer.csv")

	rows1 := []taskRow{
		{ID: "T1", ParentID: "", Version: "1.0.0", Type: "feature", Status: "open", Title: "root"},
		{ID: "T2", ParentID: "T1", Version: "1.0.0", Type: "bugfix", Status: "done", Title: "child"},
	}
	if err := writeRows(path, rows1); err != nil {
		t.Fatalf("writeRows rows1 error: %v", err)
	}

	// Overwrite to exercise replace path and keep CSV consistent.
	rows2 := []taskRow{{ID: "T3", ParentID: "", Version: "2.0.0", Type: "improvement", Status: "open", Title: "new root"}}
	if err := writeRows(path, rows2); err != nil {
		t.Fatalf("writeRows rows2 error: %v", err)
	}

	roots, byID, err := loadCSV(path)
	if err != nil {
		t.Fatalf("loadCSV error: %v", err)
	}
	if len(roots) != 1 || len(byID) != 1 {
		t.Fatalf("expected one root/task after overwrite, got roots=%d byID=%d", len(roots), len(byID))
	}
	if roots[0].ID != "T3" {
		t.Fatalf("expected root T3, got %q", roots[0].ID)
	}
}

func TestWriteMarkdown(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "trailblazer.md")
	roots := []*task{
		{
			ID:      "T1",
			Type:    "feature",
			Status:  "open",
			Title:   "Root",
			Version: "1.0.0",
			Children: []*task{
				{ID: "T2", Type: "bugfix", Status: "done", Title: "Child", Version: "1.0.1"},
			},
		},
	}

	if err := writeMarkdown(path, roots, "1.0.1", false); err != nil {
		t.Fatalf("writeMarkdown parents-only error: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown error: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "Child") {
		t.Fatalf("did not expect child task in parents-only export")
	}
	if !strings.Contains(out, "Current Version: 1.0.1") {
		t.Fatalf("missing current version in markdown")
	}

	if err := writeMarkdown(path, roots, "", true); err != nil {
		t.Fatalf("writeMarkdown full-tree error: %v", err)
	}
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown error: %v", err)
	}
	out = string(b)
	if !strings.Contains(out, "Child") {
		t.Fatalf("expected child task in full export")
	}
	if !strings.Contains(out, "Current Version: unknown") {
		t.Fatalf("expected unknown version when current version is empty")
	}
}

func TestModelVersionAndFilterBehavior(t *testing.T) {
	t1 := &task{ID: "T1", Version: "1.0.0", Status: "open", Title: "one", Expanded: true}
	t2 := &task{ID: "T2", Version: "1.0.1", Status: "open", Title: "two", Expanded: true}
	m := model{
		tasksByID:      map[string]*task{"T1": t1, "T2": t2},
		roots:          []*task{t1, t2},
		currentVersion: "1.0.1",
		versions:       []string{"1.0.0", "1.0.1"},
	}

	m.filterVersion = ""
	m.cycleFilter(1)
	if m.filterVersion == "" {
		t.Fatalf("expected filter to move off 'all'")
	}

	m.setCurrentVersion("1.0.2")
	if !contains(m.versions, "1.0.2") {
		t.Fatalf("expected new current version added to versions list")
	}

	if !m.isMissed(t1) {
		t.Fatalf("expected older open task to be marked missed")
	}
	if m.isMissed(&task{ID: "T3", Version: "1.0.2", Status: "open"}) {
		t.Fatalf("task in current version must not be missed")
	}
}

func TestCommitSetTaskVersionValidation(t *testing.T) {
	taskNode := &task{ID: "T1", Version: "1.0.0", Status: "open", Title: "task", Expanded: true}
	m := model{
		tasksByID:     map[string]*task{"T1": taskNode},
		roots:         []*task{taskNode},
		inputTargetID: "T1",
	}
	m.rebuildVersions()
	m.rebuildVisible()

	cmd, closeInput := m.commitSetTaskVersion("invalid")
	if closeInput {
		t.Fatalf("invalid version should keep input open")
	}
	if cmd != nil {
		t.Fatalf("invalid version should not return save cmd")
	}
	if taskNode.Version != "1.0.0" {
		t.Fatalf("task version changed on invalid input")
	}

	cmd, closeInput = m.commitSetTaskVersion("v2.3.4")
	if !closeInput {
		t.Fatalf("valid version should close input")
	}
	if cmd == nil {
		t.Fatalf("valid version should return save cmd")
	}
	if taskNode.Version != "2.3.4" {
		t.Fatalf("expected normalized version 2.3.4, got %q", taskNode.Version)
	}

	cmd, closeInput = m.commitSetTaskVersion("   ")
	if !closeInput || cmd == nil {
		t.Fatalf("clearing version should close input and return save cmd")
	}
	if taskNode.Version != "" {
		t.Fatalf("expected cleared task version")
	}
}

func TestCommitAddTaskEmptyKeepsInputOpen(t *testing.T) {
	m := model{tasksByID: map[string]*task{}, roots: []*task{}, inputAction: actionAddRoot}
	cmd, closeInput := m.commitAddTask("")
	if closeInput {
		t.Fatalf("empty title should keep input open")
	}
	if cmd != nil {
		t.Fatalf("empty title should not return save cmd")
	}
	if !strings.Contains(m.status, "empty title") {
		t.Fatalf("expected empty-title status, got %q", m.status)
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	child := &task{ID: "T2", ParentID: "T1", Status: "open", Title: "child", Expanded: true}
	root := &task{ID: "T1", Status: "open", Title: "root", Expanded: true, Children: []*task{child}}
	m := model{tasksByID: map[string]*task{"T1": root, "T2": child}, roots: []*task{root}}
	m.rebuildVisible()

	m1, cmd, handled := m.handleTaskEditingKey("d")
	if !handled {
		t.Fatalf("delete key should be handled")
	}
	if cmd != nil {
		t.Fatalf("first delete press must not save")
	}
	if m1.pendingDeleteID != "T1" {
		t.Fatalf("expected pending delete for T1, got %q", m1.pendingDeleteID)
	}
	if len(m1.tasksByID) != 2 {
		t.Fatalf("tasks should remain until confirmation")
	}

	m2, cmd, handled := m1.handleTaskEditingKey("d")
	if !handled {
		t.Fatalf("second delete key should be handled")
	}
	if cmd == nil {
		t.Fatalf("second delete press must return save cmd")
	}
	if len(m2.tasksByID) != 0 || len(m2.roots) != 0 {
		t.Fatalf("expected cascade delete after confirmation")
	}
}

func TestUndoDeleteRestoresTree(t *testing.T) {
	child := &task{ID: "T2", ParentID: "T1", Status: "open", Title: "child", Expanded: true}
	root := &task{ID: "T1", Status: "open", Title: "root", Expanded: true, Children: []*task{child}}
	m := model{tasksByID: map[string]*task{"T1": root, "T2": child}, roots: []*task{root}}
	m.rebuildVisible()

	m1, _, _ := m.handleTaskEditingKey("d")
	m2, _, _ := m1.handleTaskEditingKey("d")
	if len(m2.tasksByID) != 0 {
		t.Fatalf("expected deleted tree before undo")
	}

	m3, cmd, handled := m2.handleTaskEditingKey("u")
	if !handled {
		t.Fatalf("undo key should be handled")
	}
	if cmd == nil {
		t.Fatalf("undo should trigger save")
	}
	if len(m3.tasksByID) != 2 || len(m3.roots) != 1 {
		t.Fatalf("expected original tree restored on undo")
	}
	if m3.undoRows != nil {
		t.Fatalf("undo buffer should be cleared after undo")
	}
}

func TestUndoWithoutDeleteShowsStatus(t *testing.T) {
	m := model{tasksByID: map[string]*task{}, roots: []*task{}}
	next, cmd, handled := m.handleTaskEditingKey("u")
	if !handled {
		t.Fatalf("undo key should be handled")
	}
	if cmd != nil {
		t.Fatalf("undo without snapshot should not save")
	}
	nm := next
	if !strings.Contains(nm.status, "nothing to undo") {
		t.Fatalf("expected nothing-to-undo status, got %q", nm.status)
	}
}

func TestHandleKeyClearsPendingDeleteOnOtherKey(t *testing.T) {
	root := &task{ID: "T1", Status: "open", Title: "root", Expanded: true}
	m := model{tasksByID: map[string]*task{"T1": root}, roots: []*task{root}, pendingDeleteID: "T1"}
	m.rebuildVisible()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	nm := next.(model)
	if nm.pendingDeleteID != "" {
		t.Fatalf("non-delete key should clear pending delete")
	}
}

func TestHelpToggleAndClose(t *testing.T) {
	m := model{}
	m1, _, handled := m.handleNavigationKey("?")
	if !handled || !m1.showHelp {
		t.Fatalf("expected help to open on ?")
	}
	m2, _, handled := m1.handleNavigationKey("esc")
	if !handled || m2.showHelp {
		t.Fatalf("expected help to close on esc")
	}
}

func TestFooterTextByModeAndUndo(t *testing.T) {
	m := model{}
	if got := m.footerText(); !strings.Contains(got, "d delete(confirm)") {
		t.Fatalf("normal footer missing delete hint: %q", got)
	}

	m.mode = modeInput
	if got := m.footerText(); !strings.Contains(got, "enter submit") {
		t.Fatalf("input footer missing submit hint: %q", got)
	}

	m.mode = modeNormal
	m.showHelp = true
	if got := m.footerText(); !strings.Contains(got, "close help") {
		t.Fatalf("help footer missing close hint: %q", got)
	}

	m.showHelp = false
	m.mode = modePicker
	if got := m.footerText(); !strings.Contains(got, "enter apply") {
		t.Fatalf("picker footer missing apply hint: %q", got)
	}

	m.showHelp = false
	m.mode = modeNormal
	m.undoRows = []taskRow{{ID: "T1"}}
	if got := m.footerText(); !strings.Contains(got, "u undo") {
		t.Fatalf("footer should include undo when snapshot exists: %q", got)
	}
}

func TestVersionPickerOpenNavigateApply(t *testing.T) {
	root1 := &task{ID: "T1", Version: "1.0.0", Status: "open", Title: "one", Expanded: true}
	root2 := &task{ID: "T2", Version: "1.1.0", Status: "open", Title: "two", Expanded: true}
	m := model{tasksByID: map[string]*task{"T1": root1, "T2": root2}, roots: []*task{root1, root2}, versions: []string{"1.0.0", "1.1.0"}}
	m.rebuildVisible()

	m1, _, handled := m.handleTaskEditingKey("v")
	if !handled {
		t.Fatalf("v should open picker")
	}
	if m1.mode != modePicker {
		t.Fatalf("expected picker mode after v")
	}
	if len(m1.pickerOptions) != 3 || m1.pickerOptions[0] != "all" {
		t.Fatalf("unexpected picker options: %#v", m1.pickerOptions)
	}

	next, _ := m1.updatePicker(tea.KeyMsg{Type: tea.KeyDown})
	m2 := next.(model)
	if m2.pickerIndex != 1 {
		t.Fatalf("expected picker index 1 after down, got %d", m2.pickerIndex)
	}

	next, _ = m2.updatePicker(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := next.(model)
	if m3.mode != modeNormal {
		t.Fatalf("expected return to normal mode after apply")
	}
	if m3.filterVersion != "1.0.0" {
		t.Fatalf("expected picked filter 1.0.0, got %q", m3.filterVersion)
	}
}

func TestVersionPickerCancel(t *testing.T) {
	m := model{versions: []string{"1.0.0"}}
	m.startPicker(actionSetFilterVersion, "filter version", []string{"all", "1.0.0"}, "all")

	next, _ := m.updatePicker(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(model)
	if nm.mode != modeNormal {
		t.Fatalf("expected normal mode after cancel")
	}
	if nm.pickerOptions != nil {
		t.Fatalf("expected picker options cleared on cancel")
	}
}

func TestCommitSetFilterVersionValidation(t *testing.T) {
	root1 := &task{ID: "T1", Version: "1.0.0", Status: "open", Title: "one", Expanded: true}
	root2 := &task{ID: "T2", Version: "1.1.0", Status: "open", Title: "two", Expanded: true}
	m := model{tasksByID: map[string]*task{"T1": root1, "T2": root2}, roots: []*task{root1, root2}, versions: []string{"1.0.0", "1.1.0"}}
	m.rebuildVisible()

	cmd, closeInput := m.commitSetFilterVersion("all")
	if cmd != nil || !closeInput || m.filterVersion != "" {
		t.Fatalf("expected all filter to clear and close")
	}

	cmd, closeInput = m.commitSetFilterVersion("invalid")
	if cmd != nil || closeInput {
		t.Fatalf("invalid filter should keep input open and not save")
	}

	cmd, closeInput = m.commitSetFilterVersion("2.0.0")
	if cmd != nil || closeInput {
		t.Fatalf("unknown version should keep input open and not save")
	}

	cmd, closeInput = m.commitSetFilterVersion("v1.1.0")
	if cmd != nil || !closeInput || m.filterVersion != "1.1.0" {
		t.Fatalf("expected normalized known version to be applied")
	}
}

func TestBuildVisibleRowsAndCursorRestore(t *testing.T) {
	child := &task{ID: "T2", ParentID: "T1", Version: "1.0.0", Type: "feature", Status: "open", Title: "child", Expanded: true}
	root := &task{ID: "T1", Version: "1.0.0", Type: "feature", Status: "open", Title: "root", Expanded: true, Children: []*task{child}}
	m := model{
		tasksByID: map[string]*task{"T1": root, "T2": child},
		roots:     []*task{root},
	}

	m.rebuildVisible()
	if len(m.visible) != 2 {
		t.Fatalf("expected 2 visible rows, got %d", len(m.visible))
	}

	m.cursorToID("T2")
	if m.selectedID() != "T2" {
		t.Fatalf("expected cursor on T2")
	}

	root.Expanded = false
	m.rebuildVisible()
	if got := len(m.visible); got != 1 {
		t.Fatalf("expected only root visible when collapsed, got %d", got)
	}
	if m.selectedID() != "T1" {
		t.Fatalf("cursor should restore to visible root when child hidden")
	}
}

func TestNormalizeHelpers(t *testing.T) {
	if normalizeType("B") != "bugfix" {
		t.Fatalf("expected type alias B -> bugfix")
	}
	if normalizeType("i") != "improvement" {
		t.Fatalf("expected type alias i -> improvement")
	}
	if normalizeStatus("closed") != "done" {
		t.Fatalf("expected closed -> done")
	}
	if normalizeStatus("todo") != "open" {
		t.Fatalf("expected unknown status -> open")
	}

	if showCurrentVersion("") != "unknown" {
		t.Fatalf("expected empty current version to render unknown")
	}
	if showFilterVersion("") != "all" {
		t.Fatalf("expected empty filter version to render all")
	}
}

func TestRemoveTaskAndContainsHelpers(t *testing.T) {
	a := &task{ID: "A"}
	b := &task{ID: "B"}
	in := []*task{a, b}
	out := removeTaskFromSlice(in, "A")
	if len(out) != 1 || out[0].ID != "B" {
		t.Fatalf("unexpected removeTaskFromSlice result: %+v", out)
	}

	list := []string{"1.0.0", "1.0.1"}
	if !contains(list, "1.0.1") || contains(list, "2.0.0") {
		t.Fatalf("contains helper returned unexpected values")
	}
	if idx := versionIndex(list, "1.0.0"); idx != 0 {
		t.Fatalf("expected index 0, got %d", idx)
	}
}

func TestReadCSVRecordsMissingFile(t *testing.T) {
	_, err := readCSVRecords(filepath.Join(t.TempDir(), "missing.csv"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestLoadCSVMissingFileReturnsEmpty(t *testing.T) {
	roots, byID, err := loadCSV(filepath.Join(t.TempDir(), "missing.csv"))
	if err != nil {
		t.Fatalf("loadCSV should not fail on missing file: %v", err)
	}
	if len(roots) != 0 || len(byID) != 0 {
		t.Fatalf("expected empty data for missing CSV")
	}
}

func TestFlattenRowsOrder(t *testing.T) {
	root := &task{ID: "R", Title: "root", Type: "feature", Status: "open"}
	c1 := &task{ID: "C1", ParentID: "R", Title: "c1", Type: "feature", Status: "open"}
	c2 := &task{ID: "C2", ParentID: "R", Title: "c2", Type: "feature", Status: "open"}
	root.Children = []*task{c1, c2}

	rows := flattenRows([]*task{root})
	ids := []string{rows[0].ID, rows[1].ID, rows[2].ID}
	if !reflect.DeepEqual(ids, []string{"R", "C1", "C2"}) {
		t.Fatalf("unexpected flatten order: %v", ids)
	}
}
