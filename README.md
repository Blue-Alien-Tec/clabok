# CLABOK

Command-Line Authoring for Blocks and Orchestration Kit.

CLI: `pecia`

Status: initial Go scaffold. The full documentation is authored with CLABOK itself; this README is
the first book.

## Checks

Local:

```powershell
./scripts/check.ps1
```

```sh
./scripts/check.sh
```

`release-ci.yml` runs on `workflow_dispatch` and on tags matching `v*.*.0`.
