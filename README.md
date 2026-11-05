# CLABOK

Command-Line Authoring for Blocks and Orchestration Kit.

CLI: `pecia`

Status: initial Go scaffold. The full documentation is authored with CLABOK itself; this README is
the first book.

## Checks

Free, local:

```powershell
./scripts/check.ps1
```

or

```sh
./scripts/check.sh
```

The cloud workflow `release-ci.yml` runs only on manual dispatch or a `vX.Y.0` tag.
