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

## RECEIPT · Referencia de CLI + README alineado al binario real
- **when**: 2026-08-07T00:00Z
- **intent**: el README documentaba un binario que ya no existe — describía `mcp`/`version` y
  mencionaba `import` al pasar, mientras el CLI ya expone 8 subcomandos. Toda la superficie
  operativa (que es el pilar #1 del diseño: "lo operativo vive en el CLI donde cuesta cero
  contexto") no estaba documentada en ningún lado.
- **changed**:
  - docs/cli.md (new) — referencia completa: 10 comandos, flags, exit codes, resolución de
    proyecto, sección "Never edit the database directly", 6 recetas
  - README.md (+18/-3) — sección `## CLI` nueva antes de Retrieval; `## Status` actualizado
    con la medición real de M1 (1.077 bytes, 10/10 tests) y los dos entregables adelantados
- **ran**:
  - `vestigio version` → `vestigio 0.1.0`
  - `vestigio projects` → 5 proyectos, **187 memorias / 87.719 tokens**, path de la BD impreso
  - `vestigio list --project=alcubo --limit=3` → 3 filas, truncado de título correcto en no-ASCII
  - `vestigio verify` → `ok — every hash and token count matches its content`, **exit 0**
  - lectura completa de cmd/vestigio/main.go y cmd/vestigio/admin.go — la doc se escribió
    contra el código, no contra la memoria
- **cost**: opus · ~10 tool calls
- **gaps**:
  - **BUG DE USAGE ENCONTRADO, NO CORREGIDO**: `usage()` (main.go:188) y el error de
    `runImport` (main.go:82) muestran `--map old=new` / `--skip type` separados por espacio,
    pero el parser matchea el prefijo `--map=`/`--skip=`. Con espacio falla EN SILENCIO:
    `--map` no matchea ningún case y `old=new` cae en `!HasPrefix(a,"--")` → sobrescribe
    `path` con `"old=new"`. docs/cli.md documenta la forma correcta (`=`) y advierte, pero
    los strings de usage siguen mintiendo. Fix de una línea, pendiente.
  - NO se corrió `go test ./...` ni `go vet ./...` ni `go build ./...`: el cambio no toca
    una sola línea de Go. Nada en este recibo prueba que el código compile hoy.
  - `import`, `edit`, `rm` y `show` están documentados desde la lectura del código, NO
    ejecutados — son destructivos o requieren un export a mano. Los ejemplos de salida de
    `show`/`edit`/`rm`/`verify-con-drift` son RECONSTRUIDOS de los `Printf`, no capturados.
  - El commit 3020fa7 ("feat(cli): inspect and edit memories without an agent") entró SIN
    recibo. Este documenta su superficie pero no su evidencia — no hay registro de qué se
    corrió cuando ese código se escribió.
  - docs/cli.md no está enlazado desde ningún lado salvo el README. Sin CI que verifique
    que los ejemplos siguen siendo ciertos: la doc puede desincronizarse igual que se
    desincronizó el README.

## RECEIPT · Fix del parser de `import` — fallo silencioso ahora es error duro
- **when**: 2026-08-07T00:00Z
- **intent**: cerrar el bug abierto en el recibo anterior. `--map old=new` (forma que el propio
  usage recomendaba) no fallaba: `--map` no matcheaba ningún case y `old=new` caía en
  `!HasPrefix(a,"--")` sobrescribiendo el path del export. Import corría contra un archivo
  llamado "old=new" y devolvía un "not found" que no apuntaba ni cerca de la causa.
- **changed**:
  - cmd/vestigio/main.go — extraído `parseImportArgs()` desde `runImport`; el parser ya no
    ignora lo que no entiende: bare `--map`/`--skip` → error con la forma correcta, flag
    desconocido → error + usage, segundo positional → error nombrando ambos. `importUsage`
    como const única (antes el texto estaba duplicado y desincronizado entre dos sitios).
  - cmd/vestigio/main.go — `usage()` corregido a `--map=old=new,...` / `--skip=type,...`
  - cmd/vestigio/main_test.go (new) — primer test del paquete `main`: 4 funciones, 9 casos
  - docs/cli.md — el bloque que documentaba el bug reescrito para documentar el comportamiento
- **ran**:
  - `go fmt ./...` → sin cambios pendientes
  - `go vet ./...` → exit 0
  - `go test ./...` → **44 passed, 0 failed** (incl. subtests; cmd/vestigio pasa de 0 a 9)
  - `go build -o vestigio.exe ./cmd/vestigio` → exit 0
  - binario real, los 4 casos:
    `import export.json --map old=new` → `--map takes its value inline: --map=VALUE`, exit 2
    `import export.json --verbose` → `unknown flag "--verbose"` + usage, exit 2
    `import uno.json dos.json` → `unexpected argument "dos.json"…`, exit 2
    `import export.json --map=old=new --dry-run` → falla por el ARCHIVO (`read export: open
    export.json: cannot find`), exit 1 — **la prueba del fix**: se queja del archivo real,
    no importa un fantasma
- **cost**: opus · ~8 tool calls
- **gaps**:
  - `parseImportArgs` está testeado, pero `runImport` NO — un import de punta a punta contra
    un JSON real sigue sin cobertura automatizada. Los 4 casos del binario se corrieron a mano.
  - Un path de archivo que empiece con `-` ahora es rechazado como flag desconocido. Caso
    improbable, sin escape (`--`) implementado.
  - `--map=` con un par sin `=` interno (ej. `--map=basura`) se sigue descartando en silencio
    dentro del `strings.Cut`. Mismo modo de falla que este recibo cierra, un nivel más abajo.
  - `--skip` no valida contra los tipos que existen: un typo omite nada y no avisa.
  - Solo windows/amd64. Sin commitear.

## RECEIPT · CI, licencia y protección de main en el repo público
- **when**: 2026-08-07T00:00Z
- **intent**: el repo ya estaba PÚBLICO, sin licencia, sin CI y con `main` sin protección
  (`HTTP 404 Branch not protected`). El README invitaba a `go install` sobre código que sin
  licencia es "todos los derechos reservados" — legible pero no usable.
- **changed**:
  - LICENSE — Apache-2.0 (bajada de la API de GitHub, 202 líneas, no transcripta a mano)
  - .github/workflows/ci.yml — 6 jobs: test x3 SO, race, lint (gofmt+vet+mod tidy), govulncheck
  - .github/workflows/codeql.yml — CodeQL Go, security-and-quality, + cron semanal
  - .github/dependabot.yml — gomod + github-actions semanal, modernc agrupado
  - .github/CODEOWNERS, .github/pull_request_template.md (espeja el formato de recibo)
  - CONTRIBUTING.md, SECURITY.md (threat model explícito), README badges + secciones
  - go.mod — `go 1.22` -> `go 1.25`
- **ran**:
  - local pre-push: `gofmt -l .` vacío, `go vet` 0, `go test ./...` ok, `go mod tidy` sin drift
  - CI run 1 (31159627895) → **FAILURE**: `test (macos-latest)` + `govulncheck`
  - CI run 2 (31159892559) tras los fixes → **SUCCESS, 6/6 jobs verdes**
  - protección aplicada y **PROBADA como owner**:
    `git push origin main` → `remote rejected ... Changes must be made through a pull
    request` + `7 of 7 required status checks are expected`
    `git push --force origin main` → `remote rejected` igual
  - PR #3 de Dependabot: 8/8 checks `completed/success`, `mergeStateStatus: CLEAN`
    → confirma que `analyze` reporta y que la puerta no es un deadlock
- **cost**: opus · ~20 tool calls
- **gaps**:
  - **DEFECTO REAL ENCONTRADO POR CI, no del pipeline**: binarios linkeados con Go 1.22 son
    rechazados por el dyld actual en darwin/arm64 — `missing LC_UUID load command`, compilan
    y abortan al arrancar. Los 4 test binaries murieron así en macos-latest mientras Linux y
    Windows pasaban. El README prometía "cross-compiles anywhere" y en macOS era falso.
    Arreglado subiendo a Go 1.25, verificado por CI. NO se hizo bisect de la versión exacta
    que lo arregla — 1.25 tiene margen, podría andar con 1.23 o 1.24.
  - Error propio en el primer pipeline: `govulncheck` pineado al Go del módulo no podía ni
    instalarse (x/vuln pide >= 1.25). Falló sin escanear NADA — el peor rojo, parece hallazgo
    y no lo es.
  - `required_approving_review_count` quedó en **0** a propósito. Con `enforce_admins: true`
    y 1 aprobación obligatoria el repo se auto-bloquea: GitHub no deja aprobar PRs propios y
    el owner es el único con write. El "solo yo mergeo" lo da el permiso de escritura, no el
    review. Consecuencia honesta: nada obliga a que un PR sea LEÍDO antes de mergear.
  - 3 PRs de Dependabot abiertos SIN revisar: sqlite 1.34.5→1.56.0 (22 minors), checkout
    v5→v7, setup-go v6→v7. Verdes en CI, pero verde no es lo mismo que leído.
  - Sin release ni tag todavía. `go install @latest` sirve el último commit de main.
  - Secret scanning quedó activo DESPUÉS de 5 commits ya pusheados — no escaneó retroactivo.
