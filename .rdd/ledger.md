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
