# VS Code - Automatisch Go Version Instellen

## Methode 1: settings.json (Aanbevolen)

1. Open VS Code **Settings** (Ctrl+,)
2. Search for: `go.useLanguageServer`
3. Zorg ervoor dat Language Server ingeschakeld is

Voeg dit toe aan je workspace `.vscode/settings.json`:

```json
{
  "go.lintTool": "golangci-lint",
  "go.useLanguageServer": true,
  "[go]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": "explicit"
    }
  }
}
```

## Methode 2: Automatisch Detecteren via go.work

Als je multiple modules hebt, maak `go.work`:

```bash
cd c:\Users\t.hetterscheid\Repo\fenix
go work init
go work use .
go work use ./cmd/csv2fhir
```

## Methode 3: Go-installatie Instellen

1. Open PowerShell als Administrator
2. Check beschikbare Go-versies:
```powershell
go version
go env GOROOT
```

3. Download nieuwere versie via:
```powershell
# Check what's installed
go install golang.org/dl/go1.23@latest
~/go/bin/go1.23.exe download
```

## Methode 4: Environment Variables

Zet deze in Windows Environment Variables:

1. **System Properties** → **Environment Variables**
2. Add `GOPATH`: `C:\Users\t.hetterscheid\go`
3. Add `GOROOT`: `C:\Program Files\Go`
4. Restart VS Code

## Methode 5: VS Code Go Extension Configuration

Maak/edit `.vscode/settings.json` in project root:

```json
{
  "go.gopath": "C:/Users/t.hetterscheid/go",
  "go.goroot": "C:/Program Files/Go",
  "go.lintOnSave": "package",
  "go.buildOnSave": "off",
  "go.useLanguageServer": true,
  "[go]": {
    "editor.formatOnSave": true,
    "editor.defaultFormatter": "golang.Go"
  }
}
```

## Methode 6: go.mod Version Locking

In `go.mod` kan je de minimum versie vastleggen (al gedaan):

```
go 1.19
```

Dit zorgt ervoor dat alle imports compatibel zijn met 1.19+.

## Build Settings voor CGO Problemen

Voeg toe aan `.vscode/settings.json`:

```json
{
  "go.buildFlags": [],
  "go.lintTool": "golangci-lint",
  "terminal.integrated.env.windows": {
    "CGO_ENABLED": "0"
  }
}
```

## Verificatie

Check of alles goed ingesteld is:

```bash
# In VS Code Terminal
go version
go env GOROOT
go env GOPATH
go mod tidy
go build ./cmd/csv2fhir
```

Expected output:
```
go version go1.19 ...
GOROOT=C:\Program Files\Go
GOPATH=C:\Users\t.hetterscheid\go
```

## Troubleshooting

### "undefined: unsafe.SliceData"
- Caused by Go 1.19 trying to use 1.25+ features
- **Solution**: Set `CGO_ENABLED=0` in terminal/environment

### GCC Linking Errors
- **Solution**: Use pure Go SQLite driver (modernc.org/sqlite)
- Remove: `github.com/mattn/go-sqlite3`

### "Module requires Go 1.25"
- **Solution**: Downgrade problematic dependency
```bash
go get golang.org/x/sys@v0.20.0
go mod tidy
```

## .vscode/settings.json (Compleet)

```json
{
  "go.useLanguageServer": true,
  "go.lintOnSave": "package",
  "[go]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": "explicit"
    },
    "editor.defaultFormatter": "golang.Go"
  },
  "go.buildFlags": ["-v"],
  "go.buildTags": "",
  "go.testTimeout": "10s",
  "terminal.integrated.env.windows": {
    "CGO_ENABLED": "0"
  },
  "go.gopath": "C:/Users/t.hetterscheid/go",
  "go.goroot": "C:/Program Files/Go"
}
```

## Build Command Setup

Add build task in `.vscode/tasks.json`:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "go: build csv2fhir",
      "type": "shell",
      "command": "go",
      "args": ["build", "-o", "csv2fhir.exe", "./cmd/csv2fhir"],
      "cwd": "${workspaceFolder}",
      "options": {
        "env": {
          "CGO_ENABLED": "0"
        }
      },
      "group": {
        "kind": "build",
        "isDefault": true
      }
    },
    {
      "label": "go: run csv2fhir",
      "type": "shell",
      "command": "./cmd/csv2fhir/csv2fhir.exe",
      "args": ["-help"],
      "cwd": "${workspaceFolder}",
      "group": {
        "kind": "test"
      }
    }
  ]
}
```

Nu kan je via VS Code:
- **Ctrl+Shift+B**: Build
- **Ctrl+Shift+D**: Run/Debug
- **Terminal**: Automatisch Go 1.19 gebruikt met CGO_ENABLED=0

## Quick Start

```bash
# Terminal in VS Code
cd c:\Users\t.hetterscheid\Repo\fenix

# Build csv2fhir
set CGO_ENABLED=0 && go build -o ./cmd/csv2fhir/csv2fhir.exe ./cmd/csv2fhir

# Run
./cmd/csv2fhir/csv2fhir.exe -help
```
