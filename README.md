![Windows](https://img.shields.io/badge/Windows-Supported-blue?labelColor=gray&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI0ZGRiIgZD0iTTAgMGgxMXYxMUgwek0xMyAwaDExdjExSDEzek0wIDEzaDExdjExSDB6TTEzIDEzaDExdjExSDEzeiIvPjwvc3ZnPg%3D%3D)
 ![Linux](https://img.shields.io/badge/Linux-Supported-yellow?labelColor=gray&logo=linux)
 ![MacOS](https://img.shields.io/badge/MasOS-Supported-white?labelColor=gray&logo=apple)
 ![FreeBSD](https://img.shields.io/badge/FreeBSD-Supported-red?labelColor=gray&logo=freebsd)

[![Tests passed](https://img.shields.io/badge/Tests-Failed-red?labelColor=gray&logo=github)](https://github.com/sibexico/Trailblazer/actions)
[![Tests coverage](https://img.shields.io/badge/Tests%20Coverage-71.3%25-green?labelColor=gray&logo=gitextensions)](https://github.com/sibexico/Trailblazer/actions)

![Go Version](https://img.shields.io/badge/Go-1.26.1-blue?labelColor=gray&logo=go)
 [![Go Report Card](https://goreportcard.com/badge/github.com/sibexico/Trailblazer)](https://goreportcard.com/report/github.com/sibexico/Trailblazer)
 [![Support Me](https://img.shields.io/badge/Support-Me-darkgreen?labelColor=black&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI0ZGRiIgZmlsbC1ydWxlPSJldmVub2RkIiBjbGlwLXJ1bGU9ImV2ZW5vZGQiIGQ9Ik0xMiAxQzUuOTI1IDEgMSA1LjkyNSAxIDEyczQuOTI1IDExIDExIDExIDExLTQuOTI1IDExLTExUzE4LjA3NSAxIDEyIDF6bTAgNGwyLjUgNi41SDIxbC01LjUgNCAyIDYuNUwxMiAxNy41IDYgMjJsMi02LjUtNS41LTRoNi41TDEyIDV6Ii8%2BPC9zdmc%2B)](https://sibexi.co/support)


# Trailblazer

Trailblazer is an easy terminal roadmap planner written in Go. I use it daily to keep releases and tasks clear. Very usable for solo dev projects.

## Install

```bash
go install github.com/sibexico/Trailblazer@latest
```

### In Windows:

```powershell
winget sibexico.Trailblazer
```


## Quick start

Run against a directory (uses directory/trailblazer.csv):

```bash
trailblazer /path/to/project
```

Generate Markdown:

```bash
trailblazer -e /path/to/project   # parents only (same as key e)
trailblazer -E /path/to/project   # full tree (same as key E)
```

Show CLI help:

```bash
trailblazer -h
```


## Handy keys

- q: quit
- h: show in-app key hints (press any key to close)
- arrows / j / k: move
- left / l / enter: collapse / expand
- a / A: add child / root task
- space: toggle done
- d / x: delete selected task (press twice to confirm)
- u: undo last delete
- t: edit selected task description (Ctrl+S save, Esc cancel)
- n: create version
- w: write current version to VERSION (with Yes/No confirmation)
- r: set selected task version
- v: pick filter version from menu
- [ / ]: switch filter version
- 0: clear filter (show all versions)
- e / E: export parents-only markdown / export full tree markdown

## Task CSV format

Trailblazer reads and writes one CSV file (default: trailblazer.csv) with this header:

```csv
ID,ParentID,Version,Type,Status,Title,Description
```

Columns:

- ID: unique task identifier. App-generated IDs look like T18AF00CB1BFBF474.
- ParentID: parent task ID. Leave empty for root tasks.
- Version: semantic version attached to the task, for example 1.2.3.
- Type: task type. Accepted values: feature, bugfix, improvement.
- Status: open or done.
- Title: short task title shown in the list.
- Description: optional multi-line details (saved in one CSV cell).

Notes:

- Header row is optional but recommended.
- Rows with fewer than 6 columns are ignored.
- If ParentID points to a missing task, Trailblazer promotes that row to a root task.
- Type aliases are normalized on load: b/bug -> bugfix, i/improve -> improvement, everything else -> feature.
- Status aliases are normalized on load: closed/x -> done, everything else -> open.

Example:

```csv
ID,ParentID,Version,Type,Status,Title,Description
T1,,1.0.0,feature,open,Release dashboard,Main task v1
T2,T1,1.0.0,bugfix,done,Fix login redirect,Resolved bug
T3,T1,1.1.0,improvement,open,Reduce build time,"This is wery important task
with description."
```
