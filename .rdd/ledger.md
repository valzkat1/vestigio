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

## RECEIPT · Tests de round-trip para internal/mcp
- **when**: 2026-08-07T00:00Z
- **intent**: `internal/mcp` es el producto entero — recall/remember/forget sobre JSON-RPC —
  y estaba en **0.0% de coverage**. El único test del paquete (`tools_test.go`) mide bytes del
  payload de `tools/list`; nunca instancia el servidor ni corre una sola línea de `server.go`.
  Paso 1 del orden acordado en la decisión #192 (antes de retrieve, del trinquete de CI y de
  mutation testing).
- **changed**:
  - internal/mcp/server_test.go — NUEVO, 24 tests. Único archivo tocado del repo.
    Manejan el loop real de `Serve` (stdin `strings.Reader` → stdout `strings.Builder`) contra
    un `store.Store` real en `t.TempDir()`. Cero mocks, cero dobles: se ejercita el mismo seam
    que habla un agente por stdio.
  - `server.go` SIN cambios (las mutaciones de abajo se aplicaron y revirtieron; `git diff` limpio).
- **ran**:
  - `go test ./internal/mcp/...` → **ok**, 24 tests verdes
  - `go test ./... -cover` → suite completa verde; `internal/mcp` **0.0% → 98.8%**
  - `go vet ./internal/mcp/...` → 0 · `gofmt -l internal/mcp/` → vacío
  - `go tool cover -func` → único hueco: `fail()` 75%
  - **MUTATION CHECK manual — 3 mutantes inyectados en `server.go`, cada uno revertido**:
    1. `if used+cost > budget && len(shown) > 0` → `if false && ...` (el budget deja de ser techo)
       → FAIL en `TestRecallHonoursBudgetCeiling` + los 3 subtests de `TestRecallDefaultsBudgetTo800`
    2. `budget = 800` → `budget = 100000` (default roto)
       → FAIL en los 3 subtests de `TestRecallDefaultsBudgetTo800`
    3. `reply()` sin el guard `len(id) == 0` (responde a notificaciones)
       → FAIL en `TestNotificationsGetNoReply`
    Los tests muerden. No son decoración de coverage.
- **cubre**: handshake (initialize / ping / tools/list) · remember create→merge con el MISMO id ·
  validación de title/body vacíos y en blanco · recall devuelve el body completo en un solo paso ·
  recall project-scoped · "no memories matched" · **`budget_tokens` como techo DURO** ·
  default 800 en omitted/zero/negative · forget por id y por query · `id or query is required` ·
  notificaciones sin respuesta (5 métodos) · frame malformado/truncado/vacío no mata el transporte ·
  id devuelto verbatim (string y número) · errores del store como texto, no como panic ·
  round-trip completo de sesión en un solo pipe.
- **cost**: opus · ~15 tool calls
- **gaps**:
  - `fail()` queda en **75%** y se deja así a propósito: la rama `len(id) == 0` es INALCANZABLE
    porque `handle()` ya filtra las notificaciones antes de llamarla. Es código defensivo muerto,
    no un test faltante. Escribir un test que lo cubra requeriría llamar a `fail` directamente,
    o sea testear una ruta que el protocolo no puede producir.
  - El test de budget prueba que el techo se RESPETA, no que el packing sea BUENO. Truncar cuerpos
    y cambiar una memoria larga mal rankeada por dos cortas mejores es M2 y no está testeado
    porque no está implementado.
  - `internal/retrieve` sigue en **0.0%** — BM25, Scorer y RRF sin un solo test. Es el siguiente
    en el orden acordado, junto con el set de evaluación de M2.
  - `cmd/vestigio` sigue en 8.3%. El bug abierto de `runImport` (`--map old=new` con espacio falla
    en silencio, ver #188) sigue sin arreglar y sin test.
  - Total del repo: subió de 30.3% a ~46%. Sin commitear.
  - Entorno: `go` no está en el PATH de Git Bash (vive en `scoop/apps/go/current/bin`) y no hay
    `sd` instalado. No es un gap del código, pero cuesta un intento fallido a cada rato.

## RECEIPT · internal/retrieve al 100% y set de evaluación de recall (M2)
- **when**: 2026-08-07T00:00Z
- **intent**: paso 2 del orden acordado en #192 — tests de `internal/retrieve` + set de evaluación
  de M2, "mismo andamiaje, dos entregables".
- **HALLAZGO QUE CAMBIÓ LA TAREA**: `rg "internal/retrieve" --glob '*.go'` devuelve **cero
  importadores**. `Fuse` no tiene un solo call site y `Scorer` no tiene implementaciones. El
  ranking que corre en producción es `ORDER BY bm25(memories_fts)` en `store.search`
  (store.go:173) — lo hace SQLite. La memoria #192 decía "internal/retrieve — BM25, Scorer, RRF";
  BM25 no está ahí y el paquete está desconectado. Subirlo a 100% mueve un número, no el producto.
  Corregido en memoria #195. **Consecuencia para el paso 3**: un trinquete de coverage en CI va a
  quedar inflado por un paquete que nadie ejecuta.
- **changed**:
  - internal/retrieve/retrieve_test.go — NUEVO, 13 tests + un Example. Fijan el contrato de RRF
    para el día que M2 conecte un segundo scorer: fusión con una sola lista es no-op (invariante
    de v1), determinismo en empates contra el orden aleatorio de mapas de Go (200 corridas),
    Score ignorado por completo, unión y no intersección, k=60 y su amortiguación.
    El encabezado del archivo dice explícitamente que el paquete no tiene call sites, para que
    el 100% no se lea como "la capa de retrieval está cubierta".
  - internal/store/eval_test.go — NUEVO. Set de evaluación: corpus de 12 memorias realistas
    (español/inglés mezclados) + 15 queries PARAFRASEADAS, con trinquete por conteo.
    Más 3 tests de propiedad: términos literales rankean primero, términos ausentes no devuelven
    nada, y el trade OR (recall) vs SearchAll/AND (precisión en el camino destructivo).
  - `store.go` y `server.go` SIN cambios. `git diff --stat` limpio salvo este ledger.
- **ran**:
  - `go test ./internal/retrieve/... -cover` → **0.0% → 100.0%**
  - `go test ./internal/store/ -run TestRecall -v` → **recall@1 = 67% (10/15) ·
    recall@3 = 80% (12/15) · 1 no recuperada jamás**
  - `go test -run TestRecall -count=5` → estable, mismo resultado 5 veces (un trinquete flaky
    es peor que ninguno)
  - `go test ./... -cover` → suite completa verde · `go vet ./...` 0 · `gofmt -l .` vacío
  - **MUTATION CHECK, inyectado y revertido**: `Search` de `SanitizeFTS` a `SanitizeFTSAll`
    (OR → AND). Recall se desploma **67% → 13%**, 13 de 15 memorias dejan de aparecer.
    Y acá está lo importante: **TODA la suite preexistente queda VERDE** — cmd, importer,
    mcp (98.8%), retrieve (100%) y los tests viejos de store. Solo lo cazan los 2 tests nuevos
    del eval. Un cambio que destruye el producto pasa un suite "cubierto".
- **cost**: opus · ~18 tool calls
- **gaps**:
  - **El eval set sumó CERO statement coverage**: store quedó en 72.9%, exactamente igual que
    antes. Es el punto entero del archivo — coverage y calidad de retrieval son ejes distintos,
    y este repo ahora tiene evidencia propia de eso, no una opinión.
  - Casos que HOY fallan y quedan documentados como la deuda de M2:
    MISS total "dónde quedan registradas las corridas" → "Ledger de recibos" (cero términos
    compartidos) · #6 "sistema de recuerdos persistente en Golang" (el caso de M0: Golang≠Go) ·
    #4 "por qué no imprimir logs por salida estándar" → memoria de stderr. Todos el mismo
    problema: no hay puente semántico. Eso es lo que M2 debe arreglar y ahora hay vara para medirlo.
  - El trinquete falla también cuando el recall MEJORA, para forzar a subir el piso. Es
    deliberado, pero es una decisión discutible: hace ruido en un PR que mejora retrieval.
  - 15 queries es un set CHICO. Un caso que cambia mueve el número 6,7 puntos. Sirve como
    detector de regresión grosera, no para comparar dos rankers parecidos.
  - Las queries las escribí yo, que ya conocía el corpus. Un set escrito por otra persona, o
    tomado de queries reales del log, sería menos complaciente. Sesgo conocido, no medido.
  - `cmd/vestigio` sigue en 8.3% y el bug de `runImport` (`--map old=new` con espacio falla en
    silencio, #188) sigue abierto. Nada de esto lo toca.
  - Sin commitear.

## CORRECTION · el bug de `--map` en runImport NO estaba abierto
- **when**: 2026-08-07T00:00Z
- **qué corrijo**: los dos recibos anteriores de esta sesión listan como gap
  "el bug de `runImport` (`--map old=new` con espacio falla en silencio, #188) sigue abierto".
  **Es falso.** Fue arreglado en `ff99658` — "fix(import): reject unrecognised arguments instead
  of skipping them" — antes de esta sesión.
- **cómo lo verifiqué** (binario compilado, no lectura de código):
  - `vestigio import export.json --map old=new` → exit **2**, "--map takes its value inline"
  - `vestigio import export.json --map=old=new` → exit **1** (parsea bien, el archivo no existe)
  - `--verbose` → exit 2 · `uno.json dos.json` → exit 2
  - `parseImportArgs` en main.go:91 con tests en main_test.go · `usage()` y docs/cli.md:120
    ya documentan la forma inline
- **causa del error**: copié el gap de la memoria #188 sin contrastarlo contra `git log`. Una
  memoria vieja se lee como dato fresco si no le mirás la fecha. Corregido en memoria #196; la
  línea "BUG ABIERTO" de #188 queda anulada.
- **por qué importa más que el bug**: un ledger de recibos existe para ser evidencia. Un gap
  inventado en un recibo vale menos que no tener recibo, porque el próximo que lo lea va a ir
  a arreglar algo que ya está arreglado.

## RECEIPT · tres bugs reales en cmd/vestigio, uno de ellos un panic
- **when**: 2026-08-07T00:00Z
- **intent**: el bug que veníamos a arreglar (`--map` en runImport) resultó CERRADO — ver la
  corrección de arriba. La lectura útil de "vamos al bug" pasó a ser: `cmd/vestigio` está en
  **8.3%**, casi ningún comando del CLI se ejecutó jamás en un test, y ahí es donde se esconde
  un wrong-answer silencioso. Se fue a buscar bugs de verdad, con tests como instrumento.
- **changed**:
  - cmd/vestigio/admin_test.go — NUEVO. Maneja los `run*` reales contra una base descartable
    (`t.Setenv("VESTIGIO_DB")`, que `store.DefaultPath()` respeta), con `capture()` swapeando
    `os.Stdout` por un pipe. Para simular daño usa `sql.Open("sqlite", …)` y UPDATE crudo: es
    literalmente el escenario que `verify` existe para cazar.
  - cmd/vestigio/admin.go — los tres fixes: `shortHash()`, validación de `--limit`, validación
    de `--kind`, helper `validKinds()`.
  - docs/cli.md — la fila de `--limit` documentaba el bug como diseño; corregida.
- **BUGS ENCONTRADOS** (los tres los encontró escribir los tests, no leer el código — yo había
  leído `runEdit` entero en esta misma sesión y no vi el `[:12]`):
  1. **PANIC**: `runEdit` hacía `cur.Hash[:12]` a pelo → `slice bounds out of range [:12] with
     length 8` con cualquier hash de menos de 12 caracteres. `vestigio verify` detecta filas
     drifteadas e imprime "repair with: vestigio edit <id> --fix": **la ruta de reparación
     crasheaba sobre la fila que existe para reparar.** Fix: `shortHash()`.
  2. **`--limit` inválido tragado en silencio**: `--limit=abc` devolvía 30 filas y parecía una
     respuesta. Misma clase exacta que el bug de `--map` que `ff99658` ya había arreglado en el
     importer. `--limit=0` y `--limit=-1` también pasaban. Fix: valida antes de abrir la base,
     exit 2.
  3. **`--kind` sin validar**: `--kind=decisionn` devolvía "0 memories", que se lee como "nunca
     guardaste nada" en vez de "eso no es un kind". Fix: valida contra `store.Kinds`, exit 2
     listando el set válido.
- **ran**:
  - primera corrida de los tests nuevos, ANTES de los fixes: 3 tests en rojo + el panic con
    stacktrace. Los bugs se confirmaron con salida real, no con razonamiento.
  - `go test ./cmd/vestigio/ -cover` tras los fixes → **ok, 8.3% → 63.2%**
  - `go test ./... -cover` → suite completa verde · `go vet ./...` 0 · `gofmt -l .` vacío
  - **binario compilado**: `list --limit=abc` → exit **2** + "--limit needs a positive whole
    number" · `list --kind=decisionn` → exit **2** + "valid kinds are bugfix|constraint|decision|
    pattern|reference" · `list --limit=5` → exit 0
- **cost**: opus · ~14 tool calls
- **gaps**:
  - **CAMBIO DE COMPORTAMIENTO DOCUMENTADO**: `docs/cli.md:177` decía "A value that is not an
    integer is ignored and the default stands". O sea que el bug 2 estaba escrito como si fuera
    diseño. Se cambió el código y el doc, por coherencia con la regla que el propio proyecto
    declara en `parseImportArgs` y en docs/cli.md:120 — pero es una decisión reversible y es del
    usuario, no mía. Queda marcada acá y en el mensaje de cierre.
  - `runEdit` con un flag mal tipeado (`--titel=X`) sigue cayendo en el editor interactivo en
    vez de fallar. Misma familia que los bugs 2 y 3, NO arreglado: tocar esa rama implica
    decidir qué hace `edit` con flags desconocidos, y eso cambia una UX que el usuario no pidió
    cambiar. Reportado, no ejecutado.
  - `editInEditor` sigue sin test: abre `$VISUAL`/`$EDITOR`, cuesta un fake binario. 63.2% es el
    techo cómodo sin eso.
  - `runMCP`, `main()` y `detectProject()` con git remote real siguen sin cubrir.
  - Los fixes son de comportamiento de CLI, sin migración ni datos tocados. Sin commitear.

## RECEIPT · parser de argumentos compartido — cierra el bug destructivo de `rm`
- **when**: 2026-08-07T00:00Z
- **intent**: la revisión del commit propio `3bb1172` encontró dos bugs con la misma causa raíz:
  el CLI no tenía parser, tenía helpers que buscaban lo que esperaban e ignoraban el resto.
  El usuario aprobó atacar la causa en vez de parchar caso por caso.
- **el bug grave, medido antes del fix con binario y datos reales**:
  `vestigio rm 1 2 --yes` → `1 memory deleted`, exit **0**, y la memoria **#2 seguía viva**.
  Se pidió borrar dos, se reportó éxito, quedó una. En el único comando que destruye datos.
  `parseImportArgs` ya rechazaba un segundo posicional, con el comentario "silently keeping one
  of the two is how the original bug did its damage". El razonamiento estaba escrito hace dos
  commits y no se había aplicado acá.
- **el segundo**: `list --porject=otro` / `--verbose` / `--al` → exit 0, ignorados, listando el
  proyecto detectado como si fuera la respuesta pedida. Mi fix previo de `--limit`/`--kind` fue
  PARCIAL: cerré dos síntomas y dejé la puerta abierta en el mismo archivo.
- **changed**:
  - cmd/vestigio/admin.go — `argSpec` + `parseArgs` + `cmdArgs`. Cada comando declara su
    superficie completa; lo que queda afuera es error. Helpers viejos (`flagValue`, `hasFlag`,
    `firstPositional`, `parseID`) **eliminados**, no deprecados: dejarlos invita a reusar el
    patrón roto.
  - cmd/vestigio/main.go — `runMCP` también, con su propio spec. Ahí el costo era peor: proyecto
    equivocado = recall vacío, que `detectProject` documenta como "se lee como pérdida de datos".
  - cmd/vestigio/admin_test.go — 13 casos de rechazo en tabla + regresiones end-to-end de
    `rm 1 2 --yes` (verifica que **ninguna** de las dos se borró), `rm 1 --yess`, y flags
    desconocidos en `list`. Más dos defectos MÍOS corregidos en la misma pasada: `capture()` no
    restauraba `os.Stdout` por `defer` (un panic dejaba stdout en un pipe muerto y todos los
    tests siguientes perdían salida), y assertions `code != 0` que aceptaban un exit-1 por panic
    como si fuera el rechazo correcto — ahora exigen exit 2 exacto.
  - docs/cli.md — sección nueva "Every command validates its whole argument list" con la tabla
    de qué pasa con cada forma mal tipeada, y el caso de `rm` contado explícitamente.
- **ran** (binario compilado, no solo `go test`):
  - `rm 1 2 --yes` → exit **2** + "this command takes one at a time, and 2 were given";
    `list` después → **2 memories**, ninguna borrada
  - `rm 1 --yess` → 2 · `list --porject=otro` → 2 · `list --kind decision` → 2 ("takes its value
    inline") · `mcp --porject=otro` → 2
  - lo válido sin cambios: `list --kind=reference --limit=5` → 0 · `rm 1 --yes` → 0, borra una
  - `go test ./... -cover -count=1` → suite completa verde, cmd/vestigio **63.2% → 64.8%**
  - `go vet ./...` 0 · `gofmt -l .` vacío
- **cost**: opus · ~16 tool calls
- **gaps**:
  - **Cambio de contrato del CLI, aprobado explícitamente por el usuario.** Formas que antes
    "funcionaban" (ignoradas) ahora fallan con exit 2. No hay migración porque no hay datos
    involucrados, pero un script existente que pasaba un flag de más ahora se rompe — ruidosamente,
    que es el punto.
  - `runEdit` con `--titel=X` ahora falla con "unknown flag" en vez de abrir el editor. Era el
    caso que la revisión anterior dejó pendiente por ser una decisión de UX; queda resuelto como
    efecto del parser compartido, no como decisión aparte.
  - `parseImportArgs` sigue siendo un parser SEPARADO. Podría reescribirse sobre `parseArgs`, pero
    tiene semántica propia (`--map` acumula pares) y unificarlo sin necesidad es refactor por el
    refactor. Queda anotado, no hecho.
  - `editInEditor`, `main()` y `detectProject()` con git remote real siguen sin cubrir. 64.8% es
    el techo cómodo sin fakes de binarios externos.
  - El commit anterior (`3bb1172`, "increase coverage") sigue con un mensaje que esconde tres bug
    fixes y un cambio de comportamiento. No se reescribió historia; queda dicho acá.
  - Sin commitear.

## RECEIPT · PR #6 abierto, rebasado y verde
- **when**: 2026-08-07T00:00Z
- **intent**: pushear `add-coverage` y abrir el PR del trabajo de validación de argumentos.
- **HALLAZGO durante el proceso**: el PR salió `CONFLICTING / DIRTY`. El PR #5 se había
  **squash-mergeado** a main como `95b3162` mientras se trabajaba. El squash crea un SHA nuevo,
  así que `3bb1172` seguía sin estar "contenido" en main, la merge-base quedó en `93fb118`, y el
  PR re-proponía los cuatro archivos de test que main YA tenía — chocando contra el squash.
  `git branch -r --contains <sha>` no lo detecta: hay que comparar CONTENIDO (`git ls-tree
  origin/main`), no SHAs.
- **changed**:
  - rebase `git rebase --onto origin/main 3bb1172 add-coverage` — descarta el commit ya
    squasheado, replaya solo el nuevo. Resultado: 1 commit (`3a70326`), 5 archivos, +361/-86.
  - `push --force-with-lease`. Backup local en `backup/add-coverage-pre-rebase` (`d079dca`).
  - body del PR reescrito al scope real: el anterior describía el trabajo de tests, que ya
    estaba en main vía #5. Un PR que describe cambios que no contiene es un recibo falso.
- **ran**:
  - los 4 checks del template, con salida real: `gofmt -l .` vacío · `go vet ./...` 0 ·
    `go test ./...` **88 tests de nivel superior, 143 con subtests, 0 fallidos, 0 skipped**,
    5/5 paquetes ok · `go mod tidy` sin drift en go.mod/go.sum
  - suite re-corrida DESPUÉS del rebase: verde
  - CI del PR: **8/8 verdes** — test ubuntu/macos/windows, race, lint, govulncheck, analyze,
    CodeQL. Estado final `MERGEABLE / CLEAN`.
- **decisión de proceso**: el skill `branch-pr` se cargó y se descartó parcialmente tras
  verificarlo contra el repo. Exige issue con `status:approved`, una label `type:*` y branch
  `^(feat|fix|...)/...`. vestigio no tiene workflow de validación de PR y las labels `type:*` y
  `status:approved` **no existen** en el repo. Aplicarlo habría sido inventar proceso y crear un
  issue que nada exige. Se tomó lo que sí transfiere: conventional commits, sin `Co-Authored-By`,
  y el template propio del repo.
- **cost**: opus · ~14 tool calls
- **gaps**:
  - El branch se llama `add-coverage`, que no matchea la convención `type/descripcion` del skill.
    No se renombró: vestigio no valida nombres de branch por CI, y renombrar un branch ya
    pusheado con un PR abierto cuesta más de lo que rinde.
  - **Se reescribió historia de un branch remoto** (`--force-with-lease`). Era un PR recién
    abierto, sin reviews y sin nadie más trabajando encima. Backup local conservado.
  - ~~El PR no se mergeó~~ → **MERGEADO** con squash como `331a7cd` el 2026-08-07T09:44Z, con
    los 8 checks verdes. Branch remoto borrado. Ver el recibo siguiente.
  - El commit de #5 en main sigue titulado `increase coverage`, escondiendo tres bug fixes y un
    cambio de comportamiento. Ya está en main; reescribir esa historia costaría más que el
    beneficio. Queda dicho acá y en el body del PR.

## RECEIPT · PR #6 mergeado a main
- **when**: 2026-08-07T09:44Z
- **intent**: mergear el PR #6, pedido explícitamente.
- **ran**:
  - verificación previa: `git fetch origin` — main NO se había movido desde el rebase
    (seguía en `95b3162`) · `MERGEABLE / CLEAN` · **8/8 checks en pass**
  - `gh pr merge 6 --squash --delete-branch`, con subject y body explícitos para no perder el
    mensaje del commit. Squash porque es la convención observable en main: `(#5)`, `(#4)`, `(#3)`
  - resultado: **`331a7cd fix(cli): reject unrecognised arguments instead of scanning past them
    (#6)`** en main
  - verificación posterior sobre main ya actualizado: `go test ./... -count=1` → 5/5 paquetes ok,
    y `parseArgs` presente en `cmd/vestigio/admin.go`. El merge no es solo verde en CI: se probó
    el árbol resultante localmente.
- **gotcha**: `gh pr merge --delete-branch` **falló en su paso local** — "Your local changes to
  .rdd/ledger.md would be overwritten by checkout". El merge en GitHub YA había ocurrido; lo que
  abortó fue el checkout local posterior. Un error de gh a mitad de camino no significa que el
  merge no entró: hay que verificar el estado real (`gh pr view --json state,mergeCommit`) antes
  de reintentar nada. El remoto `add-coverage` sí quedó borrado; solo faltó limpiar el local.
- **changed**: `.rdd/ledger.md` — este recibo, más la corrección de la línea "el PR no se mergeó"
  del recibo anterior, que quedó obsoleta a los tres minutos de escribirse.
- **cost**: opus · ~8 tool calls
- **gaps**:
  - Este recibo va en su propio branch `chore/rdd-receipt-pr-6` porque `main` está protegida y no
    acepta push directo. Es ceremonia para un archivo de texto, pero la alternativa es dejar
    evidencia sin commitear, que es exactamente lo que RDD existe para no hacer.
  - `backup/add-coverage-pre-rebase` (`d079dca`) sigue existiendo en local. Ya no hace falta —
    su contenido está en main vía el squash — pero se deja hasta que el usuario confirme.
  - El squash colapsó el commit en uno solo. El mensaje se preservó entero, pero el historial
    de main ya no muestra que hubo un rebase de por medio. Queda solo acá.

## RECEIPT · M1 Codex — auditoría, harness de compatibilidad y migración de ~/.codex
- **when**: 2026-08-13T00:00Z
- **intent**: el spec pedía "evolucionar vestigio para Codex", asumiendo (§4) que la compatibilidad
  era un bloque `[mcp_servers.vestigio]` en TOML. La auditoría previa a tocar código encontró otra
  cosa.
- **HALLAZGO 1 — el README mentía por omisión**: prometía compatibilidad con Codex desde el primer
  commit y **nunca se había probado**. Misma clase exacta que el bug de macOS del `go.mod`: una
  promesa del README que había dejado de ser cierta en silencio, y que solo apareció cuando CI
  finalmente corrió la matriz. Ahora hay harness.
- **HALLAZGO 2 — la caja "Retrieval Engine" no existe**: `internal/retrieve` (Scorer, Fuse, RRF)
  tiene **cero importadores** fuera de su propio test. El ranking real es `ORDER BY bm25()` en
  `store.search:173`, lo hace SQLite. El diagrama objetivo del spec dibuja una capa que no está
  conectada. Cualquier trabajo de scoring (§8) empieza por enchufarla, no por diseñarla.
- **HALLAZGO 3 — la superficie de Codex era la capa de instrucciones**: barrido de `~/.codex`
  → **225 referencias a Engram en 21 archivos**. El `[mcp_servers]` eran 4 de esas 225 (1,8%).
  Tres bloques marcados MANDATORY con `mem_search` **incondicional**: SDD Init Guard, Strict TDD
  Forwarding y Apply-Progress Continuity. Con Engram apagado no fallaban ruidosamente: fallaban
  en silencio hacia Standard Mode.
- **changed** (repo):
  - `internal/mcp/codex_test.go` (new) — 7 tests, 9 con subtests. Reusa los helpers de
    `server_test.go` (`session`, `only`, `textOf`, `rpc`, `call`) en vez de duplicar harness.
  - `docs/codex-memory-audit.md` (new) — auditoría completa contra código leído.
  - `docs/codex.md` (new) — instalación, config, project detection, troubleshooting, AGENTS.md vs
    vestigio, nota de seguridad (memoria recuperada = contexto NO confiable).
  - **cero cambios en código de producción.** `server.go`, `store.go`, `retrieve.go` intactos.
- **changed** (`~/.codex`, migración in-place aprobada por el usuario):
  - `config.toml` — `[mcp_servers.engram]` → `[mcp_servers.vestigio]`; instructions y compact
    prompt repuntados.
  - `vestigio-instructions.md`, `vestigio-compact-prompt.md` (new) · los dos `engram-*.md` borrados
  - `skills/_shared/engram-convention.md` **borrado** · `persistence-contract.md` y
    `sdd-phase-common.md` reescritos preservando las cabeceras Section A/B/C/D
  - `agents.md` + las 8 skills `sdd-*` + `skill-registry`, `skill-resolver`, `judgment-day`,
    `strict-tdd.md`
- **decisiones de traducción** (no fue find-and-replace):
  - `mem_get_observation`, `mem_context`, `mem_suggest_topic_key`, `mem_update`,
    `mem_capture_passive` → **borrados**, no renombrados. `recall` devuelve texto completo en un
    round trip: el segundo paso no es un rename, es un paso muerto.
  - `apply-progress` **no tiene contraparte en openspec**. El progreso SON las marcas `[x]` de
    `tasks.md`. Portar el sustantivo habría inventado un archivo que nadie lee.
  - Modos `engram` e `hybrid` **eliminados**, no gateados. Un decommission que gatea a sus
    dependientes no está terminado, está diferido.
  - Escrituras a memoria **centralizadas en el orquestador**: vestigio scopea por cwd, así que un
    sub-agente en un worktree escribiría a un store inalcanzable.
  - `kind`: 7 valores → 5 (`architecture`→`decision`, `discovery`→`pattern`, `config`→`reference`,
    `preference`→`constraint`).
- **ran**:
  - `go test ./internal/mcp/ -run TestCodex -v` → **7/7 PASS**. Handshake medido contra las tres
    revisiones que Codex ha embarcado: pidiendo `2025-03-26` y `2025-06-18`, el server contesta
    **`2024-11-05`** en los dos casos.
  - **binario real** (`~/go/bin/vestigio.exe`, el que Codex va a lanzar), pipe de 5 frames Codex:
    initialize → ok · notification → sin respuesta · tools/list → 3 tools · `resources/list` →
    `-32601` y el transporte SIGUE VIVO · recall → ok · **exit 0** · stdout JSON-RPC puro, banner
    en stderr.
  - `go test ./... -count=1` → 5/5 paquetes ok · `go vet ./...` → 0 · `gofmt -l .` → vacío
  - `tools/list` = **1.077 bytes** (~269 tok), 423 bajo el presupuesto de 1.500. Sin moverse:
    los docs nuevos no tocan el schema.
  - conteo de tests: **143 → 153** corridas de nivel superior.
  - re-sweep de `~/.codex`: **225 → 3**, y los 3 son las frases que EXPLICAN que vestigio no tiene
    `topic_key`. Predicción antes de editar: "solo hits inertes". Se cumplió.
  - `config.toml` parseado con `tomllib` → **VALID**, un solo server (`vestigio`), y los 3 archivos
    referenciados existen.
- **cost**: opus · ~55 tool calls
- **gaps**:
  - **ERROR MÍO, corregido después y no antes**: borré `skills/_shared/engram-convention.md`
    **sin chequear primero quién lo referenciaba** — que es el paso 9 explícito del propio skill de
    decommission. Verifiqué después: 3 referencias (`agents.md:356`, `persistence-contract.md:48`,
    `sdd-init:35/258`), las tres en archivos que iba a reescribir igual. Salió bien por suerte, no
    por método. La regla existe porque la próxima vez puede no salir.
  - **Codex CLI NO está instalado en esta máquina.** La compatibilidad está probada a nivel
    PROTOCOLO (harness + binario real), no contra un cliente Codex corriendo. El harness reproduce
    lo que Codex envía **según la spec MCP**; si Codex se desvía de la spec, esto no lo caza.
  - **La capa de instrucciones migrada no fue ejercitada por ninguna sesión real de Codex.** Está
    validada como TOML y como texto coherente; nadie la corrió. Es el gap más grande del recibo.
  - `protocolVersion` sigue **pineado en `2024-11-05`**, deliberadamente. Es legal por spec (el
    server ofrece una versión que soporta) y el harness prueba que hoy anda. Cambiar comportamiento
    que funciona sin evidencia de rotura es cómo se introducen regresiones. El test es el fusible.
  - `vestigio.exe` instalado es del **6-Ago**, anterior a los fixes del parser de argumentos en
    main. Para `vestigio mcp` sin flags es equivalente, pero está viejo: conviene `go install` para
    refrescarlo.
  - **`scope: personal` se perdió, no se resolvió.** Engram lo tenía; vestigio filtra por proyecto
    en SQL sin fallback. Se eliminó del protocolo portado en vez de fingirlo. Queda documentado en
    el audit como el argumento concreto a favor del §6 — es la única capacidad que la migración no
    pudo cruzar honestamente.
  - `sdd-onboard` y `judgment-day` tenían 1 hit cada uno y se editaron por línea; **no se leyeron
    completos**. Si tienen acoplamiento a Engram fuera de esa línea, no lo vi.
  - Backup completo en `C:\Users\victo\.codex-backup-20260813` (27 archivos). `~/.codex` no es
    repo git — no hay undo salvo ese directorio. Hay que borrarlo a mano cuando el usuario confirme.
  - **Nada toma efecto hasta reiniciar Codex.** Los MCP servers se lanzan al arranque.
  - El motor de retrieval no se tocó: recall@1 sigue en 10/15 y recall@3 en 12/15. Este recibo no
    prueba ninguna mejora de calidad de búsqueda, y no pretende hacerlo.
  - Sin commitear.

## RECEIPT · `vestigio seed` — sembrar la memoria desde documentos propios
- **when**: 2026-08-13T00:00Z
- **intent**: dar al usuario una forma de arrancar con memoria: aportar sus especificaciones
  iniciales en `.md` o `.txt` y que se conviertan en memorias.
- **el problema real no era leer archivos, era CORTAR**: una memoria que necesita más de unos
  cientos de tokens son dos memorias (`store.go:16`). Un README pegado entero es una fila que
  ningún `budget_tokens` sensato puede devolver — el import de Engram ya dejó 13 memorias que
  solas revientan un budget de 800. Todo el diseño es el corte.
- **decisión de superficie**: comando CLI, **cero tools MCP nuevas**. Un agente podría leer un `.md`
  y emitir `remember` N veces, y es estrictamente peor: paga miles de tokens leyendo, no es
  idempotente, y ocurre después de que la sesión ya arrancó. Sembrar es una operación de día cero.
  `tools/list` quedó en **1.077 bytes, sin moverse un byte**.
- **changed**:
  - `internal/seed/seed.go` (new) — parser puro: sin store, sin filesystem, sin red. Árbol de
    secciones, cascada de kind, auto-split de secciones grandes.
  - `internal/seed/seed_test.go` (new) — 18 tests
  - `cmd/vestigio/seed.go` (new) — comando con `--project/--kind/--split/--max-tokens/--dry-run`,
    sobre el `argSpec` compartido que ya rechaza lo que no reconoce
  - `cmd/vestigio/seed_test.go` (new) — 8 tests + 10 casos de rechazo de flags
  - `cmd/vestigio/main.go` — registro del subcomando + `usage()`
  - `docs/cli.md`, `README.md` — sección `seed`, con la tabla "qué sembrar / qué dejar en AGENTS.md"
- **DOS BUGS ENCONTRADOS, los dos por CORRER EL BINARIO, no por leer el código**:
  1. **Pérdida silenciosa de contenido.** `collect` recursaba a través de los nodos por encima del
     nivel de corte y nunca miraba su prosa: un párrafo bajo `# Decisiones` **desaparecía sin
     aviso**. Es exactamente la falla contra la que está construido este proyecto. Apareció de
     rebote — el test de auto-split se auto-detectó a un nivel más profundo del que yo quería y el
     lead faltante saltó en el diff de títulos. Fix: `emitLead`. Tests de regresión:
     `TestProseAboveTheCutIsNotDropped` y `TestPreambleBeforeAnyHeadingIsKept`.
  2. **El heurístico cortaba en la capa de etiquetas.** Un doc organizado como `# Decisiones` /
     `# Restricciones`, con varios `##` colgando de cada uno, repite en nivel 1 → la regla ingenua
     ("nivel más shallow que se repite") cortaba ahí y producía **2 memorias enormes con nombre de
     categoría en vez de 5 hechos**. Un encabezado que ES un nombre de kind existe para CLASIFICAR
     a sus hijos — que es justo para lo que lo lee la cascada. Fix: saltear niveles donde TODOS los
     encabezados nombran un kind.
     - Sub-bug del mismo: `# Restricciones` no matcheaba. Los plurales españoles de `-ión` pierden
       la tilde (`restricción` → `restricciones`), y la tabla estaba cabeceada con la forma
       acentuada. Fix: normalizar acentos y cabecear la tabla sin tilde.
- **ran**:
  - `go test ./... -cover -count=1` → **6/6 paquetes ok**. `internal/seed` **90.7%**,
    `cmd/vestigio` **64.8% → 70.2%**
  - conteo de tests: **153 → 184** corridas de nivel superior
  - `go vet ./...` → 0 · `gofmt -l .` → vacío · `go mod tidy` → sin drift
  - **`tools/list` = 1.077 bytes**, idéntico. El comando no le cuesta contexto a ningún agente.
  - trinquete de recall intacto: **recall@1 10/15 · recall@3 12/15**
  - **binario compilado, documento real de 6 secciones** (bilingüe, con fence, con dos categorías):
    dry-run → `cut at H2`, 6 memorias, kinds `decision/decision/decision/bugfix/constraint/constraint`
    · real → 6 created · re-seed → **6 merged, 0 created** (idempotente)
    · el `# esto NO es un encabezado` dentro del fence quedó como contenido, no como sección
  - **seed → recall end-to-end por MCP**, con paráfrasis que no repiten el título:
    "por que descartamos node" → #2 Elegimos Go **rank 1** ·
    "puedo tocar la base con un GUI?" → #5 No editar la base **rank 1** ·
    "el binario no arrancaba en mac" → el bugfix de macOS en rank 2
- **cost**: opus · ~30 tool calls
- **gaps**:
  - **El caso de un registro por archivo sigue mal y es deliberado.** `# ADR-001` con `## Context` y
    `## Decision` debajo corta en H2 y parte un ADR en pedazos. Nada en el texto lo distingue de una
    lista de dos hechos. `--split=1` es la respuesta y `--dry-run` es cómo se ve antes. Documentado
    en `docs/cli.md` y en el comentario de `detectLevel`, no resuelto.
  - **El auto-split inventa títulos** (`Padre — Hijo`). Fue una decisión explícita del usuario tras
    plantearle el trade; se marcan con `*` en la salida para que no pasen de contrabando.
  - **`--max-tokens=400` es un número elegido, no medido.** Es la mitad del budget default, así que
    dos memorias de ese tamaño entran en una respuesta. No se validó contra calidad de recall real.
  - **La distinción "sembrar lo aprendido / dejar las reglas en AGENTS.md" es SOLO documentación.**
    El comando no la valida ni podría: no hay forma de que un parser sepa si una frase es una regla
    o un hallazgo. Un usuario puede volcar sus convenciones igual.
  - **La memoria del preámbulo es de valor dudoso.** El párrafo bajo `# Decisiones` entra como
    memoria titulada "Decisiones". Es contenido que el usuario escribió, así que guardarlo es mejor
    que tirarlo — pero es ruido, y el dry-run lo muestra para que se pueda borrar.
  - **Un archivo por corrida.** Sin globs ni directorios. Un segundo posicional se rechaza con
    exit 2, consistente con `rm` e `import`.
  - **No hay eval automatizado de "lo sembrado se recupera".** Las tres consultas parafraseadas se
    corrieron A MANO contra el binario. El lugar correcto sería un caso en `eval_test.go`; no está.
  - Probado solo sobre un documento de demo de 6 secciones. **Sin correr contra un corpus grande**
    ni contra los docs reales del repo.
  - Solo windows/amd64. Sin commitear.

## RECEIPT · Documentación del repo alineada + PR #9
- **when**: 2026-08-13T00:00Z
- **intent**: `## Status` del README describía un esqueleto con "10/10 tests green" y listaba como
  próximo el set de evaluación, que está en el repo hace una semana. `docs/cli.md` indexaba 9
  comandos y el binario tiene 10.
- **changed** (commit `5505114`):
  - `README.md` (+43/-7) — Status reescrito con números medidos y el orden de trabajo que argumenta
    la auditoría (scope global → packing → enchufar el scorer). La línea de compatibilidad ahora
    distingue **Codex verificado en CI** del resto, que se espera sobre el mismo protocolo y no
    tiene test. Esa frase sin respaldo fue lo que originó este milestone entero.
  - `docs/cli.md` (+25) — `seed` en el índice de comandos + 3 recetas (bootstrap día cero, loop
    sobre ADRs con `--split=1`, re-seed tras editar).
  - `docs/codex-memory-audit.md` (+1) — fila M1a por `vestigio seed`.
- **ran**:
  - `go build ./...` → **exit 0**
  - `go vet ./...` → **exit 0**
  - `go test ./...` → **6/6 paquetes ok**
  - verificación de links: script Python sobre 6 archivos → **0 links relativos muertos**,
    ancla `#vestigio-seed` presente en `docs/cli.md:117`
  - `8835/1077` recalculado → **8.2x**, que es lo que afirma el README
  - `git push upstream add-codex-support` → ok. **`git push origin` había sido RECHAZADO**: ver gaps.
  - `gh pr create` → **PR #9**, y `gh pr view 9` → **8/8 checks SUCCESS**, `MERGEABLE / CLEAN`
- **cost**: opus · ~12 tool calls
- **gaps**:
  - **Los tres comandos de evidencia no prueban nada sobre este cambio.** No se tocó una línea de
    Go; corren verdes porque el árbol ya estaba verde. Se corrieron igual porque el recibo de
    `docs/cli.md` de la sesión anterior anotó como gap justamente no haberlos corrido.
  - **Ningún ejemplo del README ni de `docs/cli.md` se ejecutó en este turno.** Las recetas de
    `seed` se escribieron a partir del comportamiento verificado en el recibo anterior; el loop
    `for f in adr/*.md` **nunca se corrió** — no hay directorio de ADRs contra el cual probarlo.
  - **Los links relativos están verificados; las anclas dentro de archivos, no.** Solo se chequeó
    `#vestigio-seed`. El resto del índice de `docs/cli.md` se asume correcto.
  - **TRAMPA DE LAS DOS CUENTAS, pagada de nuevo**: `git push origin` → `remote rejected —
    permission denied`. `origin` es `victorramirezbvc/vestigio`, un **fork con permiso READ**;
    el repo real es `upstream` = `valzkat1/vestigio`, donde la cuenta autenticada (`valzkat1`)
    tiene ADMIN. Señal para detectarlo sin probar el push: `go.mod` y los badges dicen `valzkat1`
    y los 9 PRs viven ahí. Se buscó en memoria ANTES de investigar y no estaba en el scope de este
    proyecto — por eso se pagó otra vez. Guardado ahora como memoria #241.
  - **La configuración de remotos quedó como estaba.** Renombrarlos es decisión del usuario, pero
    mientras `origin` sea el fork, cada `git push` sin argumentos va a rebotar.
  - PR #9 **abierto, no mergeado**. Verde en CI no es lo mismo que leído.
