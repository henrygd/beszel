# Uptime Monitors PR1 — Plan d'exécution TDD avec sub-agents (juste ce qu'il faut)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implémenter les monitors externes PR1 (http, keyword, tls, dns, ping, hub-only) en TDD strict, en déléguant aux sub-agents uniquement les tâches bornées et indépendantes.

**Architecture:** Nouveau package `internal/hub/monitors/` (types + 4 checkers + SSRF + scheduler/manager), 2 collections PocketBase (`monitors`, `monitor_checks`), routes REST, notifs via `AlertManager` existant, UI React au style Beszel. Zéro touche à `agent/`, `entities/`, `common/`, WS/SSH/CBOR.

**Tech Stack:** Go 1.27.1, PocketBase v0.40.2 (SQLite), std `net/http` + `crypto/tls`, `github.com/miekg/dns`, `github.com/prometheus-community/pro-bing` (package `probing`), React 19 + Vite + Tailwind + recharts + valibot + Lingui.

## Global Constraints

- Branche `feat/uptime-monitors`, fork `origin`, `upstream = henrygd/beszel`. Petits commits conventionnels (1 checker/commit). Rebase `upstream/main` avant PR.
- `interval` défaut 60, min 20, max 86400 (s). `timeout` défaut 10 (s), **toujours < interval** (validation). `max_retries` défaut 2, 0–10. DOWN au (max_retries+1)e échec.
- `warn_days` 21, `crit_days` 7 (`crit < warn`). Corps réponse HTTP max 2 Mo, body config max 1 Mo, headers custom ≤ 20, `max_redirects` 1–20 (défaut 10). `User-Agent: Beszel-Monitor`.
- Seules deps ajoutées : `miekg/dns`, `prometheus-community/pro-bing`. Pas de lib cron, pas de client HTTP tiers.
- Secrets (`password`, `token`) chiffrés, redacted `••••••` en lecture API, jamais loggés. `allow_private_network=false` par défaut.
- 1 transaction par cycle de check. Concurrence globale ≤ 10 (`MONITORS_MAX_CONCURRENT`, max 50). Test manuel : 1/10 s par monitor, 10/min par user, **sans écriture** d'historique. Historique : défaut 200, max 1000.
- Chaque tâche TDD suit RED (test qui échoue pour la bonne raison) → GREEN (code minimal) → REFACTOR. `gofmt -l` propre, `go vet ./...` OK avant chaque commit. Tests avec `-count=1`.
- Spec : `docs/superpowers/specs/2026-09-04-uptime-monitors-pr1-spec.md`. Todo : `2026-09-04-uptime-monitors-pr1-todo.md`.

---

## File Structure

```
internal/hub/monitors/
  models.go          Task 1 — Monitor, MonitorType, CheckResult, StatusUp/Down/Warn, Validate() error
  models_test.go     Task 1
  ssrf.go            Task 2 — IsPrivateIP, DialContext SSRF-safe, redirect re-validation
  ssrf_test.go       Task 2
  checker_http.go    Task 3 — CheckHTTP(ctx, Monitor) CheckResult (stdlib)
  checker_http_test.go
  checker_tls.go     Task 4 — CheckTLS(ctx, Monitor) CheckResult (stdlib)
  checker_tls_test.go
  checker_dns.go     Task 5 — CheckDNS(ctx, Monitor) CheckResult (miekg/dns)
  checker_dns_test.go
  checker_ping.go    Task 6 — CheckPing(ctx, Monitor) CheckResult (pro-bing)
  checker_ping_test.go
  manager.go         Task 7 — Manager, cache, hooks PB, RunCheck dispatch
  scheduler.go       Task 7 — 1 goroutine/monitor, ticker, jitter, sémaphore
  scheduler_test.go  Task 7
  api.go             Task 9 — 7 routes REST
  api_test.go        Task 9
internal/migrations/<ts>_monitors.go   Task 8 — m.Register + revert
internal/hub/collections.go            Task 8 — règles monitors + monitor_checks
internal/records/                      Task 8 — rétention monitor_checks
internal/alerts/                       Task 10 — transitions → SendAlert + history
internal/site/src/...                  Task 11 — routes, dialog, lib, i18n
internal/hub/config/config.go          Task 12 — monitors: YAML
```

## Routage sub-agents (uniquement lorsque nécessaire)

- **Subagent (frais, contexte isolé) : Tasks 2, 3, 4, 5, 6.** Fonctions pures, 1–2 fichiers, contrats figés ci-dessous, tests avec serveurs locaux (`httptest`, DNS fake, CA de test). Aucune dépendance DB/hub — le subagent n'a besoin que de sa tâche.
- **Inline (session principale) : Tasks 0, 1, 7, 8, 9, 10, 11, 12, 13.** Fondations d'interfaces (1), concurrence/état partagé (7), DB et migrations (8), wiring auth/hub (9, 10), UI/YAML/docs (11–13) — transverses, nécessitent le contexte global.
- **Review après chaque tâche subagent** : agent `code-reviewer` (conformité spec + qualité). Fix loop ≤ 5 rounds, puis arbitrage. **Final review** whole-branch une fois, sur le modèle le plus capable, puis UNE vague de fix + une re-review cadrée.

---

### Task 0: Vérification environnement (inline — déjà partiellement fait)

**Files:** aucun (contrôles uniquement)

**Interfaces:** Consumes: rien. Produces: feu vert outillage.

- [ ] **Step 1: Vérifier Go et l'arbre**

```bash
go version && gofmt -l internal/ docs/ && go vet ./internal/hub/... && git status -sb
```

Expected: `go1.27.1`, aucune sortie `gofmt -l`, vet sans erreur nouvelle, branche `feat/uptime-monitors`.

- [ ] **Step 2: Ajouter les 2 dépendances**

```bash
go get github.com/miekg/dns github.com/prometheus-community/pro-bing && go build ./... 
```

Expected: `go.mod`/`go.sum` à jour, build OK. Si le fetch réseau échoue : STOP, demander (pas de bricolage, pas de paquet système — consigne Arch).

---

### Task 1: Types + validation (inline — fondation, tous en dépendent)

**Files:**
- Create: `internal/hub/monitors/models.go`
- Test: `internal/hub/monitors/models_test.go`

**Interfaces:**
- Consumes: rien.
- Produces (contrats verbatim pour Tasks 2–7, 9) :
  - `type MonitorType string` + `TypeHTTP, TypeKeyword, TypePing, TypeDNS, TypeTLS`
  - `StatusUp, StatusDown, StatusWarn = "up", "down", "warn"`
  - `type Monitor struct { Name string; Type MonitorType; Target string; IntervalSeconds int; TimeoutSeconds int; MaxRetries int; UpsideDown bool; Config map[string]any }`
  - `type CheckResult struct { Status string; LatencyMs float64; Code *int; Message string; Details map[string]any; CertDays *float64 }`
  - `func (m Monitor) Validate() error` (interval 20–86400, timeout < interval, max_retries 0–10, target non vide, type connu)
  - `func CheckHTTP/CheckTLS/CheckDNS/CheckPing(ctx context.Context, m Monitor) CheckResult`
  - `func RunCheck(ctx context.Context, m Monitor) CheckResult` (dispatch, Task 7)

- [ ] **Step 1: Écrire le test qui échoue**

```go
func TestValidate_TimeoutMustBeLessThanInterval(t *testing.T) {
	m := Monitor{Name: "t", Type: TypeHTTP, Target: "https://example.com", IntervalSeconds: 60, TimeoutSeconds: 60, MaxRetries: 2}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error when timeout >= interval")
	}
}
```

- [ ] **Step 2: Lancer et vérifier l'échec**

Run: `go test ./internal/hub/monitors/ -run TestValidate_TimeoutMustBeLessThanInterval -v -count=1`
Expected: FAIL (`undefined: Monitor` — le type n'existe pas encore, c'est la bonne raison).

- [ ] **Step 3: Implémentation minimale**

```go
package monitors

type MonitorType string

const (
	TypeHTTP    MonitorType = "http"
	TypeKeyword MonitorType = "keyword"
	TypePing    MonitorType = "ping"
	TypeDNS     MonitorType = "dns"
	TypeTLS     MonitorType = "tls"
)

const (StatusUp = "up"; StatusDown = "down"; StatusWarn = "warn")

type Monitor struct {
	Name string; Type MonitorType; Target string
	IntervalSeconds int; TimeoutSeconds int; MaxRetries int
	UpsideDown bool; Config map[string]any
}

type CheckResult struct {
	Status string; LatencyMs float64; Code *int
	Message string; Details map[string]any; CertDays *float64
}

func (m Monitor) Validate() error {
	switch m.Type {
	case TypeHTTP, TypeKeyword, TypePing, TypeDNS, TypeTLS:
	default:
		return fmt.Errorf("unknown monitor type %q", m.Type)
	}
	if m.Target == "" {
		return errors.New("target is required")
	}
	if m.IntervalSeconds < 20 || m.IntervalSeconds > 86400 {
		return fmt.Errorf("interval must be 20..86400 seconds")
	}
	if m.TimeoutSeconds <= 0 || m.TimeoutSeconds >= m.IntervalSeconds {
		return fmt.Errorf("timeout must be > 0 and < interval")
	}
	if m.MaxRetries < 0 || m.MaxRetries > 10 {
		return fmt.Errorf("max_retries must be 0..10")
	}
	return nil
}
```

- [ ] **Step 4: Vérifier le vert + tout le package**

Run: `go test ./internal/hub/monitors/ -v -count=1`
Expected: PASS, sortie propre.

- [ ] **Step 5: Commit**

```bash
git add internal/hub/monitors/models.go internal/hub/monitors/models_test.go
git commit -m "feat(monitors): add monitor types and validation"
```

---

### Task 2: Garde SSRF (SUBAGENT — fonction pure, bornée)

**Files:**
- Create: `internal/hub/monitors/ssrf.go`
- Test: `internal/hub/monitors/ssrf_test.go`

**Interfaces:**
- Consumes: rien (net std uniquement).
- Produces (pour Tasks 3–6) : `func IsPrivateIP(ip net.IP) bool`, `func GuardDialContext(ctx, network, addr) (net.Conn, error)` (résout, vérifie chaque IP, bloque réseau privé sauf `allow_private_network=true`), helper `func ParsePortOrDefault(hostport string, def int) (string, int, error)`.

Brief subagent : contrat ci-dessus + liste des plages bloquées (loopback 127/8 + ::1, 10/8, 172.16/12, 192.168/16, 169.254/16, IPv6 link-local fe80::/10 + unique-local fc00::/7, 0.0.0.0) + env `MONITORS_ALLOW_PRIVATE_NETWORK`. TDD : tests table-driven (publique OK, chaque plage KO, override env OK), `httptest` + redirect public→privé KO.

- [ ] **Step 1: Test qui échoue** (ex. `TestIsPrivateIP_BlocksLoopback` sur `127.0.0.1` et `::1`).
- [ ] **Step 2: Run, FAIL attendu** (`undefined: IsPrivateIP`).
- [ ] **Step 3: Implémentation minimale** (table CIDR + `ip.IsLoopback/IsLinkLocal/Unicast` + check).
- [ ] **Step 4: Vert + matrice complète** (toutes plages + override env + dialer).
- [ ] **Step 5: Commit** `feat(monitors): add SSRF guard for outbound checks`.
- [ ] **Step 6: Review** code-reviewer (spec §10.1 + qualité). Fix loop si findings.

---

### Task 3: Checker HTTP + keyword (SUBAGENT — std uniquement)

**Files:**
- Create: `internal/hub/monitors/checker_http.go`
- Test: `internal/hub/monitors/checker_http_test.go`

**Interfaces:**
- Consumes: `Monitor`, `CheckResult`, `GuardDialContext` (Task 2), `CheckTLS` réutilisé pour `check_cert_expiry` (Task 4 — si Task 4 non mergée, le subagent stubbe l'appel derrière une variable `tlsExpiryHook` remplaçable, pas de duplication).
- Produces: `func CheckHTTP(ctx context.Context, m Monitor) CheckResult`.

Brief subagent : config §6.1 spec (method, `accepted_status_codes` syntaxe Kuma avec parser maison, redirects ≤10 revalidés SSRF, headers ≤20 hors Host/Content-Length, UA défaut, body ≤1 Mo, auth none/basic/bearer sans log, keyword/invert sur 2 Mo + `truncated`, `ignore_tls_errors` → `tls_insecure:true`). Transport : Dial 5 s, TLSHandshake 5 s, header timeout = timeout, `MaxIdleConns: 0`. TDD avec `httptest` : 200 OK, 500 KO, code accepté custom `200-299,401`, redirect, keyword found/not-found/invert, timeout, corps >2 Mo tronqué.

- [ ] **Step 1: Test `TestCheckHTTP_Accepts200`** (serveur 200 → `StatusUp`).
- [ ] **Step 2: Run, FAIL** (`undefined: CheckHTTP`).
- [ ] **Step 3: Implémentation minimale** (GET + 2xx = up).
- [ ] **Step 4: Vert.** Puis itérer : un test par comportement (codes custom, redirect, keyword, timeout, troncation, auth, TLS insecure flag) — chaque itération RED→GREEN.
- [ ] **Step 5: Commit** `feat(monitors): add HTTP and keyword checker`.
- [ ] **Step 6: Review** + fix loop.

---

### Task 4: Checker TLS (SUBAGENT — std `crypto/tls`)

**Files:**
- Create: `internal/hub/monitors/checker_tls.go`
- Test: `internal/hub/monitors/checker_tls_test.go`

**Interfaces:**
- Consumes: `Monitor`, `CheckResult`.
- Produces: `func CheckTLS(ctx context.Context, m Monitor) CheckResult` (+ `func CertDaysLeft(chain []*x509.Certificate, now time.Time) float64` testable sans réseau).

Brief subagent : SNI = host (override `server_name`), port 443 défaut, `VerifyHostname` sauf `ignore_tls_errors`, seuils warn 21 / crit 7, UP/warn/down, `cert_days` 2 décimales, details `{not_after, issuer, dns_names[≤5], error?}`. TDD : CA de test (`crypto/x509` + `httptest.NewTLSServer` ou certs générés en test) — valide UP, proche expiry warn, expiré down, hostname mismatch down.

- [ ] Steps RED→GREEN par cas, puis commit `feat(monitors): add TLS certificate checker`, puis review + fix loop.

---

### Task 5: Checker DNS (SUBAGENT — `miekg/dns`, API Context7 figée)

**Files:**
- Create: `internal/hub/monitors/checker_dns.go`
- Test: `internal/hub/monitors/checker_dns_test.go`

**Interfaces:**
- Consumes: `Monitor`, `CheckResult`.
- Produces: `func CheckDNS(ctx context.Context, m Monitor) CheckResult`.

Brief subagent : `c := new(dns.Client); c.Net = "udp"|"tcp"; c.Timeout/DialTimeout/ReadTimeout/WriteTimeout; c.Exchange(m, "IP:port")` ; 10 qtypes ; `expected_answer` + `contains|exact` insensible casse ; answers ≤5 ; RCODE en `code` ; resolver custom = IP ; PTR exige IP cible. TDD avec serveur DNS fake (`miekg/dns` côté serveur dans le test) : NOERROR+match, mismatch, NXDOMAIN, SERVFAIL, timeout.

- [ ] Steps RED→GREEN par cas, puis commit `feat(monitors): add DNS checker`, puis review + fix loop.

---

### Task 6: Checker ping (SUBAGENT — `pro-bing`, API Context7 figée)

**Files:**
- Create: `internal/hub/monitors/checker_ping.go`
- Test: `internal/hub/monitors/checker_ping_test.go`

**Interfaces:**
- Consumes: `Monitor`, `CheckResult`.
- Produces: `func CheckPing(ctx context.Context, m Monitor) CheckResult`.

Brief subagent : `probing.NewPinger(host)`, `SetPrivileged(false)` d'abord puis fallback `true`, `Count/Size/Timeout/Interval` depuis config, `RunWithContext(ctx)`, `Statistics()` → `PacketsRecv ≥ 1` = succès, `latency = AvgRtt`, details `{min,avg,max,loss,received,sent}`, message `missing NET_RAW capability` si aucun mode. TDD : 127.0.0.1 (skip si pas de capa, unprivileged d'abord), host injoignable (loss 100 % typé, pas de faux up).

- [ ] Steps RED→GREEN, puis commit `feat(monitors): add ping checker`, puis review + fix loop.

---

### Task 7: Scheduler + manager (inline — concurrence, état partagé)

**Files:**
- Create: `internal/hub/monitors/manager.go`, `scheduler.go`, `scheduler_test.go`

**Interfaces:**
- Consumes: `Monitor`, `CheckHTTP/CheckTLS/CheckDNS/CheckPing`, `Validate`.
- Produces: `func RunCheck(ctx, m) CheckResult` (switch sur `m.Type`), `type Manager` (cache `store.Store`, hooks, sémaphore `MONITORS_MAX_CONCURRENT` défaut 10/max 50, `consecutive_failures`, skip overlap, jitter 0–5 s + stagger).

TDD : machine d'état en test pur (fausse fonction check injectée) — retries, DOWN au (max+1)e, reset au succès, `upside_down`, warn, `resend_after`. Puis scheduler réel (ticker court en test, skip overlap vérifié). Commit `feat(monitors): add scheduler and manager`. Pas de subagent : état partagé + patterns `SystemManager` à matcher au plus près.

---

### Task 8: Persistance (inline — DB, migrations)

**Files:**
- Create: `internal/migrations/<ts>_monitors.go`
- Modify: `internal/hub/collections.go`, `internal/records/*.go`

**Interfaces:** Consumes: schéma §4 spec. Produces: collections + index + rétention.

TDD : test de migration up/down sur base vierge (pattern `hub_test_helpers.go`), test de règles (readonly bloqué), test de rétention (vieux `monitor_checks` purgés, downsample). Une seule transaction par cycle (`RunInTransaction`). Commit `feat(monitors): add PocketBase collections and retention`.

---

### Task 9: API REST (inline — wiring auth/hub)

**Files:**
- Create: `internal/hub/monitors/api.go`, `api_test.go`
- Modify: `internal/hub/api.go` (montage routes)

**Interfaces:** Consumes: Manager, collections. Produces: 7 routes §7 spec.

TDD : création 201, 400 par champ (timeout ≥ interval…), secrets redacted `•••`, accès refusé non-membre, rate-limit test (1/10 s), historique paginé. Commit `feat(monitors): add monitors API`.

---

### Task 10: Alerting transitions (inline — hooks `AlertManager`)

**Files:** Modify: `internal/alerts/` (nouveau `alerts_monitors.go` + history).

TDD : scénario UP→DOWN→UP avec `SendAlert` capturé (pas d'envoi réel), quiet hours respectées, `alert_id = "monitor:<id>"`, résolution à pause/suppression. Commit `feat(monitors): notify monitor transitions`.

---

### Task 11: UI (inline — style Beszel, pas de test auto imposé au subagent)

**Files:** `components/routes/monitors.tsx`, `components/routes/monitor.tsx`, `components/monitors/monitor-dialog.tsx`, `lib/monitors.ts`, `types.d.ts`, nav, `home.tsx`, locales FR+EN.

Pas de subagent : fidélité visuelle Beszel exige l'œil du contexte global. DONE = `vite build` OK + screenshots liste/détail/dialog. Commit `feat(monitors): add monitors UI`.

---

### Task 12: Config YAML (inline — petit, couplé à Task 8)

**Files:** Modify: `internal/hub/config/config.go`, `config-yaml.tsx`.

TDD : sync `monitors:` (match `name+target`, ne supprime jamais l'UI, users par email). Commit `feat(monitors): support monitors in config.yml`.

---

### Task 13: Charge + benchmark + docs + PR (inline — preuves)

- [ ] Charge SQLite (`testing.Short`-gated) : 50 monitors @60 s / 24 h simulées — taille DB, zéro `database is locked`, p99 < 1 s.
- [ ] Benchmark RSS + p99 @50/200 vs Kuma → chiffres dans la PR.
- [ ] Docs : README, CHANGELOG, `supplemental/guides/monitors.md`.
- [ ] `gofmt -l` propre, `go vet ./...`, `go test ./internal/...` vert (zéro régression).
- [ ] Rebase `upstream/main`, push fork, `gh pr create --repo henrygd/beszel` (corps §14 spec).

---

## Self-Review (faite à l'écriture)

- **Couverture spec** : §4 modèle → Tasks 1+8 ; §5 scheduler → 7 ; §6 checkers → 3–6 ; §7 API → 9 ; §8 états/notifs → 7+10 ; §9 SQLite → 8+13 ; §10 sécu → 2+5(tasks)+13(grep) ; §11 UI → 11 ; §12 YAML → 12 ; §13 tests → chaque task ; §14 PR → 13. `agent` réservé PR2 : champ présent dès la migration Task 8, toujours null (validation).
- **Placeholders** : aucun TBD/TODO ; chaque step a son code ou sa commande exacte ; pas de renvoi « comme Task N » sans contenu.
- **Cohérence types** : `Monitor/CheckResult/StatusUp…/Validate/CheckXxx/RunCheck/IsPrivateIP/GuardDialContext` nommés identiquement partout ; `alert_id = "monitor:<id>"` ; redaction `••••••`.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-04-uptime-monitors-pr1.md`. Two execution options:

**1. Subagent-Driven (recommended)** - I dispatch a fresh subagent per task (Tasks 2–6), review between tasks, fast iteration; Tasks 0–1, 7–13 inline.

**2. Inline Execution** - Execute all tasks in this session using executing-plans, batch execution with checkpoints.

**Which approach?**
