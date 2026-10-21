---
title: Cline en gentle-ai — soporte total para Cline
date: 2026-10-21
status: implementado y verificado en sandbox aislado
scope: soporte completo de Cline en gentle-ai (adapter, ODD, RDD, hooks, assets, telemetría)
repo: XaviCode1000/gentle-ai-cline-support
base_commit: upstream origin/main 2a3129aa
sandbox: ../../fusion_cline_gentle-ai/sandbox (HOME aislado dentro del workspace)
---

# Cline en gentle-ai — soporte total

## TL;DR

Se añade soporte de **Cline** a `gentle-ai` como agente de tier `full`, con la misma
profundidad de integración que el piloto de CodeBuddy: no es solo un adapter, es la
participación de Cline en cada superficie por agente que el framework decide en runtime.

**Hecho y verificado:** los ocho commits ODD v4 migrados sobre el upstream actual, el
orchestrator de Cline generado desde el canónico genérico (con su *Cline Delegation
Boundary*), el hook de skill-registry como archivo ejecutable —que es lo que el runtime
de Cline realmente ejecuta—, los agentes configurados, la clase RDD nativa (transporte
de revisor, capacidad anunciada, assets de review, stop hook y verbo de telemetría), el
comando estable de Engram, el esquema de telemetría corregido, la decisión documentada
sobre el overlay de permisos, y el teardown con backup. Todo probado contra un sandbox
aislado: el `~/.cline` real queda byte a byte idéntico.

**Decisión de seguridad:** Cline no recibe overlay de permisos. Su único interruptor
inyectable es el booleano global `toolAutoApprove`, que auto-aprueba toda llamada sin
deny-list por ruta; gentle-ai no puede expresar la mitad "permisivo + deny de secretos"
que ofrece a los demás runtimes, así que Cline se suma al grupo de no-ops deliberados
(Codex, Cursor, Antigravity, Hermes) con el motivo escrito en el código.

**Diferido a propósito:** el sandbox es el único destino verificado. El home real del
usuario no se toca, por instrucción explícita.

---

## Contexto

`gentle-ai` es un configurador (Go CLI + TUI) de agentes de IA. Cada agente se integra
por cuatro capas, y Cline participa en las cuatro:

| Capa | Paquete | Qué aporta Cline |
|---|---|---|
| Modelo | `internal/model/types.go` | `AgentCline = "cline"` |
| Adapter | `internal/agents/cline/` | rutas, detección, estrategias |
| Manifest | `internal/agents/capabilitymanifest/manifest.go` | feature claims + exposición de review |
| Registro/Catálogo | `agents/factory.go`, `catalog/agents.go` | alta del agente |

Y por componentes que inyectan contenido: `agentguidance` (orchestrator ODD), `persona`,
`permissions`, `skills`, `engram`, `mcp`, `communitytool`, `reviewassets`, `agenthooks`,
`telemetryruntime`, `uninstall`.

### Mapa Cline ↔ Claude Code (deltas que importan)

| gentle-ai | Claude Code | Cline |
|---|---|---|
| Config root | `~/.claude` | **`~/.cline`** |
| System prompt | `CLAUDE.md` | **`~/.cline/rules/gentle-ai.md`** (secciones markdown) |
| MCP | `~/.claude/mcp/*.json` + `~/.claude.json` | **`~/.cline/data/settings/cline_mcp_settings.json`** (shape `mcpServers`) |
| Hooks | merge JSON en `settings.json` | **archivos ejecutables** en `~/.cline/hooks/<Evento>[.ext]` |
| Delegación | tool `Task` | **`spawn_agent`** (sin MCP en los subagentes) |
| Agentes de archivo | `~/.claude/agents/*.md` | **`~/.cline/agents/*.yaml`** (frontmatter `name`, `description`, `tools`, `skills`) |
| Subagentes | heredan herramientas | **sin servidores MCP**: la persistencia queda en el lead |

---

## Alcance

### Hecho

**1. Adapter + wiring** — `internal/agents/cline/{adapter.go,paths.go}`: root `~/.cline`,
reglas en `~/.cline/rules`, skills en `~/.cline/skills`, agentes en `~/.cline/agents`,
MCP en `~/.cline/data/settings/cline_mcp_settings.json`, settings en
`~/.cline/data/settings/global-settings.json`, detección por raíz de configuración o
binario en PATH, comando de instalación vía npm.

**2. Orchestrator ODD generado** — `internal/assets/cline/orchestrator.md` **se genera**
desde el asset canónico genérico más un overlay de Cline (título, binding line,
*Cline Delegation Boundary* y las líneas de memoria que nombran el contrato de
delegados sin MCP). El generador falla cerrado si el canón mueve un ancla, y un test de
deriva falla si el asset commiteado deja de coincidir con la fuente. Se regeneró el
baseline del módulo y los goldens de routing.

**3. Hook de skill-registry** — el runtime de Cline ejecuta *archivos*, no un documento
de settings, así que el hook se escribe como `~/.cline/hooks/UserPromptSubmit.sh` con el
mismo comando de refresco que los demás runtimes. El contenido gestionado es también la
marca de propiedad: un hook del usuario nunca se sobrescribe ni se borra, y el teardown
solo elimina los bytes gestionados.

**4. Superficies por agente** — comando estable de Engram (Cline entra en la cohorte
estándar), esquema de telemetría con `cline` en ambas copias y digest re-fijado, y
overlay de permisos como no-op documentado.

**5. Clase RDD nativa** — Cline es runtime con revisión por recibos: adaptador de
transporte que invoca el CLI headless (`cline "<prompt>" --json --system <texto>
--auto-approve false`) en un área de trabajo temporal y aislada —raíces de configuración,
datos y home propias, sin herramientas—, que falla cerrado cuando no puede producir un
veredicto; capacidad anunciada en el manifest y en `model.rdd.go`; assets de review
lenses y Judgment Day; stop hook; verbo de telemetría.

### Fuera de alcance

- El home real del usuario: el sandbox es el único destino verificado (S1).
- Superficies que Cline no puede expresar: overlay de permisos (decisión documentada) y
  el slug de setup de Engram (el binario de Engram no expone `cline`).

---

## Cambios por archivo

### Nuevos
| Archivo | Contenido |
|---|---|
| `internal/agents/cline/adapter.go` | Implementa `agents.Adapter` + `CapabilityManifest()` |
| `internal/components/clineagents/` | Proyección de los agentes configurados de Cline + generador del orchestrator |
| `internal/components/agenthooks/cline_skill_registry.go` | Hook de skill-registry como archivo, con marca de propiedad |
| `internal/reviewerprovider/cline_*.go` | Transporte de revisor headless, entorno aislado y parsing de salida |
| `internal/assets/cline/agents/*.yaml` | Agentes de review lenses y Judgment Day en el esquema de Cline |
| `internal/assets/cline/orchestrator.md` | Generado desde el asset canónico genérico |

### Modificados
| Archivo | Cambio |
|---|---|
| `internal/model/{types.go,rdd.go}` | `AgentCline` y la clase RDD |
| `internal/catalog/agents.go`, `internal/agents/factory.go` | Alta del agente |
| `internal/agents/capabilitymanifest/manifest.go` | Feature claims y exposición de review |
| `internal/components/agentguidance/{orchestrator.go,routing.go}` | Selección del asset y routing de Cline |
| `internal/components/permissions/inject.go` | No-op deliberado con el motivo escrito |
| `internal/components/engram/inject.go` | Cline en la cohorte de comando estable |
| `internal/components/agenthooks/skill_registry.go` | Rama de Cline antes de los escritores JSON |
| `internal/components/reviewassets/install.go` | Manifest de agentes nativos de Cline |
| `internal/components/uninstall/service.go` | Teardown del hook gestionado |
| `internal/cli/{run.go,sync.go}` | Backup del hook y de los agentes |
| `internal/telemetrycollector/schema/event.schema.json`, `contracts/telemetry/v1/schemas/event.schema.json` | `cline` en el enum, capacidad y digest re-fijados |
| `docs/{agents.md,components.md,platforms.md}`, `README.md` | Fila y badge |

> **Nota de mantenimiento:** los tests de conteo de agentes, manifests y tablas
> exhaustivas son *ratchets* deliberados: obligan a considerar cada agente nuevo. Se
> actualizaron de forma explícita, no silenciosa.

---

## Validación

Entorno: sandbox dentro del workspace, con `HOME`, `CLINE_DIR` y las variables XDG
apuntando a `sandbox/home` y con `sandbox/bin` primero en el PATH. El `~/.cline` real
**no se escribe**.

- `go build ./...` — OK.
- Suites focales de permisos, Engram, telemetría, agenthooks, uninstall, agentguidance,
  assets y clineagents — verdes.
- `gentle-ai install --agent cline --scope global` — controles de verificación en verde y
  segunda instalación sin cambios.
- `gentle-ai doctor` en el sandbox — estado de Cline instalado y versión de assets
  coincidente.
- Reporte del propio runtime (`cline config rules|skills|agents|mcp|hooks --json`): la
  regla `gentle-ai`, las skills gestionadas, los agentes configurados y los dos servidores
  MCP.
- Evidencia de aislamiento: `~/.cline` (362 entradas, 245 digests), `~/.gentle-ai`
  (40 entradas, 26 digests), el binario global de gentle-ai y el install global de Cline
  byte-idénticos antes y después.
- Pruebas del CLI de Cline compiladas desde el workspace (`bun run build:sdk` con Bun
  1.3.13 de mise), no desde el install global.

Detalle completo en `sandbox/evidence/sandbox-verification.md` (18 controles, 0 fallos).

### Fallo de test preexistente y ambiental (NO relacionado)

| Test | Causa |
|---|---|
| `internal/cli/TestEngramPathGuidanceDefault` | El shell exporta `GOBIN` apuntando al Go de mise, así que la guía sugiere ese directorio. Con `env -u GOBIN` el test pasa. Reproducible sin este cambio. |

---

## Cómo reproducir (sandbox)

```bash
# binarios aislados: el wrapper fija HOME, CLINE_DIR y PATH dentro del workspace
sandbox/bin/sb gentle-ai --version
sandbox/bin/sb cline version
sandbox/bin/sb gentle-ai install --agent cline --scope global
sandbox/bin/sb gentle-ai doctor
sandbox/bin/sb cline config agents --json
```

` sandbox/scripts/build.sh ` reconstruye ambos binarios desde las fuentes del workspace.
`sandbox/scripts/build.sh gentle-ai` compila el binario Go; `sandbox/scripts/build.sh
cline` instala las dependencias con el Bun de mise y compila el SDK.

---

## Decisiones de diseño

1. **El orchestrator de Cline se genera, no se copia a mano.** Copiar el asset genérico
   fue exactamente lo que dejó al prompt de Cline con la variante *Small Model* y con las
   reglas de escritura y verificación anteriores a 4.2. El generador más el guard de
   deriva eliminan esa clase de defecto.
2. **El hook de Cline es un archivo, no un merge JSON.** Los helpers compartidos
   (`decodeHookSettings`/`encodeHookSettings`) habrían corrupto el archivo del hook.
3. **Cline no recibe overlay de permisos.** El contrato de gentle-ai es "modo permisivo
   con deny-list de secretos"; Cline solo puede aprobar todo sin excepción. Preferimos
   un no-op explícito a quitarle al usuario la puerta de aprobación.
4. **El transporte RDD corre en un área temporal aislada.** El revisor ve la evidencia
   del candidato y nada más: sin reglas del usuario, sin reglas del proyecto, sin
   herramientas, sin escribir el `~/.cline` real.
5. **El estado de Cline no es RDD por accidente.** `model.rdd.go` y el manifest se mueven
   juntos, porque un test de paridad lo exige: un runtime sin RDD no puede recibir
   lenguaje de review.

---

## Riesgos y preguntas abiertas

- **La revisión nativa de Cline no se ha ejecutado en vivo.** El transporte está
  construido, aislado y probado por contrato, pero una sesión real de revisión con
  Cline necesita una credencial del provider y no se ejecutó en este entorno.
- **`--auto-approve false` es la frontera de herramientas en modo no interactivo.** Si
  una versión futura de Cline cambiara ese comportamiento por defecto, el revisor
  podría alcanzar herramientas; el área temporal aislada sigue siendo la barrera.
- **El overlay de permisos queda para una decisión de producto.** Si Cline sumara un
  deny-list por ruta, el overlay se podría reevaluar sin tocar el resto.
- **El slug `cline` en el setup de Engram no existe**: la configuración MCP se escribe
  directamente, sin pasar por `engram setup`.

---

## Borrador de PR (inglés)

**Title:** `feat(agent): add Cline support`

**Body:**

```markdown
## Summary

Adds Cline as a full-tier agent with the same surface coverage as the other
full-tier runtimes: adapter and registrations, an ODD orchestrator generated
from the canonical generic asset, the skill-registry hook as the executable
file the Cline runtime actually runs, the native receipt-driven-development
review class (headless reviewer transport, advertised capability, review
assets, stop hook, telemetry verb), the stable Engram command, a corrected
telemetry schema, and a documented no-op for the permission overlay.

## What's included

- `internal/agents/cline/`: config root `~/.cline`, rules `~/.cline/rules`,
  skills `~/.cline/skills`, configured agents `~/.cline/agents`, MCP
  `~/.cline/data/settings/cline_mcp_settings.json`.
- `internal/assets/cline/orchestrator.md`: generated from the canonical generic
  asset plus a Cline overlay (runtime title, binding line, Cline Delegation
  Boundary, memory lines). A fail-closed generator and a drift guard keep it in
  sync with the canon.
- `internal/components/agenthooks/cline_skill_registry.go`: the hook is a file
  in `~/.cline/hooks`, because that is what the runtime executes; the managed
  bytes are the ownership marker, so a user hook is never touched.
- `internal/reviewerprovider/cline_*.go`: the RDD reviewer transport, running
  the headless CLI in an isolated scratch area and failing closed when no
  verdict can be captured.
- Permission overlay: a deliberate documented no-op. Cline exposes only a
  global `toolAutoApprove` boolean with no path-scoped deny, so gentle-ai's
  "permissive plus secrets deny" contract cannot be honoured.

## Not included (deliberate)

- No permission overlay (see above).
- No real-home install: this work is verified in an isolated sandbox only.

## Testing

- `go build ./...`
- Focused suites: permissions, engram, telemetry, agenthooks, uninstall,
  agentguidance, assets, clineagents.
- Sandbox: `gentle-ai install --agent cline --scope global` (verification
  checks green, second run unchanged), `gentle-ai doctor`, and the Cline
  runtime's own report of rules, skills, agents and MCP.
- Isolation proof: the real `~/.cline`, `~/.gentle-ai` and the global
  binaries are byte-identical before and after the run.
- One pre-existing, environment-only failure remains:
  `cli/TestEngramPathGuidanceDefault` (the shell exports `GOBIN`); it passes
  with `env -u GOBIN`.

## Notes

- Agent-count and manifest ratchets were updated explicitly.
- The orchestrator module baseline and the routing goldens were regenerated.
```
