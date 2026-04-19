![Windows](https://img.shields.io/badge/Windows-Supported-blue?labelColor=gray&logo=data:image/svg%2Bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI%2BPHBhdGggZmlsbD0iI0ZGRiIgZD0iTTAgMGgxMXYxMUgwek0xMyAwaDExdjExSDEzek0wIDEzaDExdjExSDB6TTEzIDEzaDExdjExSDEzeiIvPjwvc3ZnPg%3D%3D)
 ![Linux](https://img.shields.io/badge/Linux-Supported-yellow?labelColor=gray&logo=linux)
 ![MacOS](https://img.shields.io/badge/MasOS-Supported-white?labelColor=gray&logo=apple)
 ![FreeBSD](https://img.shields.io/badge/FreeBSD-Supported-red?labelColor=gray&logo=freebsd)


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
- ?: show in-app key hints
- arrows / j / k: move
- h / l / enter: collapse / expand
- a / A: add child / root task
- space: toggle done
- d / x: delete task
- n: create version
- r: set selected task version
- [ / ]: switch filter version
- 0: clear filter (show all versions)
- e / E: export parents-only markdown / export full tree markdown
