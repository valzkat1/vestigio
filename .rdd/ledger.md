# Ledger — vestigio

Append-only. Newest at the bottom. See the RDD receipt schema for the contract.

## RECEIPT · M1 skeleton — store, retrieval, MCP server
- **when**: 2026-08-06T00:00Z
- **intent**: M1 del plan — esqueleto Go con SQLite+FTS5 y 3 tools MCP end-to-end
- **changed**:
  - go.mod (new)
  - cmd/vestigio/main.go (new)
  - internal/store/{schema,store,store_test}.go (new ×3)
  - internal/retrieve/retrieve.go (new)
  - internal/mcp/{server,tools,version,tools_test}.go (new ×4)
  - README.md, .gitignore, .rdd/{config.json,ledger.md} (new)
- **ran**:
  - `go build ./...` → **not run** — no hay toolchain de Go en la máquina
  - `go test ./...` → **not run** — mismo motivo
  - `node measure-vestigio.js` → tools/list = 1075 chars (~269 tok), 3 tools.
    8.2x menor que Engram agent (8835), 9.7x menor que all (10477).
    Impuesto de arranque 16764 → 1075 chars.
- **cost**: opus · ~12 tool calls
- **gaps**: NADA de este código fue compilado ni ejecutado. El payload de 1075 chars
  se midió replicando el JSON en Node, no emitiéndolo desde el binario. Sin verificar:
  que modernc.org/sqlite traiga FTS5 habilitado, los triggers de sincronía FTS,
  el handshake MCP real contra un cliente. El module path dice USERNAME.

## RECEIPT · M1 verificado + fix de borrado destructivo
- **when**: 2026-08-06T00:00Z
- **intent**: cerrar los gaps del recibo anterior — toolchain Go instalado, todo ejecutado
- **changed**:
  - internal/store/store.go (+31/-9) — SearchAll, SanitizeFTSAll, ForgetQuery estricto
  - internal/store/store_test.go (+21) — TestForgetQueryRequiresAllTerms
  - go.mod / go.sum (deps de modernc.org/sqlite resueltas)
- **ran**:
  - `scoop install go` → go1.26.5 windows/amd64 (winget bloqueado por policy, exit 1625)
  - `go build ./...` → exit 0
  - `go vet ./...` → exit 0
  - `go test ./...` → **10 passed, 0 failed**
  - probe MCP contra el binario real → `tools/list` = **1077 bytes**, 3 tools
  - e2e por MCP → remember created #1/#2, re-save → `merged #1`, recall con budget=30
    omitió 1, query sin match → "no memories matched", forget → removed, recall → vacío
- **cost**: opus · ~14 tool calls
- **gaps**: FTS5 confirmado funcionando, pero el recall NO se probó contra corpus grande
  (8 memorias en total). El set de evaluación de paráfrasis es M2 y sigue pendiente.
  Sin probar en macOS/Linux — solo windows/amd64. Binario de 9.9 MB, no medido contra
  alternativas. Module path sigue en USERNAME. Sin commit todavía.

## RECEIPT · Importador de Engram (M4 adelantado)
- **when**: 2026-08-06T00:00Z
- **intent**: migrar el corpus real de Engram — M2 necesita corpus real para su set de evaluación
- **changed**:
  - internal/importer/engram.go (new) — parseo, mapeo de kinds, consolidación de proyectos
  - internal/importer/engram_test.go (new) — 7 tests
  - internal/store/store.go (+30) — Import() preservando timestamps originales
  - cmd/vestigio/main.go (+50) — comando `import` con --dry-run/--map/--skip
- **ran**:
  - `go vet ./...` → exit 0
  - `go test ./...` → **17 passed, 0 failed** (10 previos + 7 del importador)
  - `vestigio import --dry-run` → plan revisado antes de escribir
  - `vestigio import --skip=session_summary --map=...` → **179 importadas, 1 duplicada**,
    209 totales, 27 session_summary omitidas, 2 vacías
  - eval de auto-recuperación, 25 memorias de 107 en alcubo:
    título completo → 100% encontrada / 100% rankeada #1
    título degradado → 100% encontrada / 84% rankeada #1
- **cost**: opus · ~12 tool calls
- **gaps**: el eval es AUTO-RECUPERACIÓN con palabras del propio título, NO paráfrasis
  semántica — no responde la pregunta de vectores, solo da línea de base. El set real
  sigue siendo M2. Los 27 session_summary quedaron fuera por decisión del usuario
  (20% del corpus); siguen en Engram, no se borró nada. 13 memorias exceden solas
  un budget de 800 tok y el packer de M2 tendrá que truncarlas en lectura.
