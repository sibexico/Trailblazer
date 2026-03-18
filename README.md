# Trailblazer

Trailblazer is an easy terminal roadmap planner written in Go. I use it daily to keep releases and tasks clear. Very usable for solo dev projects.

## Install

```bash
go install github.com/sibexico/trailblazer@latest
```

## Quick start

Run against a directory (uses directory/trailblazer.csv):

```bash
trailblazer /path/to/project
```


## Handy keys

- j / k: move
- h / l: collapse / expand
- a / A: add child / root task
- space: toggle done
- d: delete task
- n: create version
- r: set selected task version
- [ / ]: switch current version
- 0: clear current version
- e / E: export parents-only markdown / export full tree markdown
