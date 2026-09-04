# TODO — Monitors externes Beszel, PR1 hub-only

> **Règle d'or (anti-Keynit)** : aucune case cochée sans preuve — log de test,
> screenshot, mesure ou sortie de commande citée dans le commit ou la PR.
> Les standards applicables sont rappelés par phase ; en cas de conflit,
> `spec.md` fait foi.
> **Spec** : `2026-09-04-uptime-monitors-pr1-spec.md` (même dossier).

## Phase 0 — Setup et cadrage

- [x] Fork `henrygd/beszel` → `delta-whiplash/beszel` (`origin` = fork, `upstream` = upstream), branche `feat/uptime-monitors` créée depuis `main` à jour.
- [ ] `go version` ≥ 1.27 OK, `gofmt -l` propre sur l'arbre existant, `go vet ./internal/hub/...` sans erreur nouvelle. Preuve : sortie de commandes.
- [ ] Requêtes Context7 effectuées et APIs figées : PB (`NewBaseCollection`, `m.Register` + revert, `NewRecord` + `Save`, `AddIndex`), `miekg/dns` (`Client` + `Exchange`), `prometheus-community/pro-bing` (`NewPinger`, `SetPrivileged`, `RunWithContext`, `Statistics`).
- [ ] Fichiers charnières relus : `internal/hub/hub.go` (câblage), `internal/hub/api.go:88-140` (routes), `internal/hub/collections.go` (règles), `internal/hub/systems/system_manager.go` + `systems/system.go:69-122` (ticker/jitter/stagger), `internal/alerts/alerts.go` (`SendAlert`), `internal/records/records_deletion.go:60-100` (rétention), `internal/hub/config/config.go` (sync YAML), `internal/site/src/main.tsx:30-33` (routes lazy).
- [ ] `go get github.com/miekg/dns github.com/prometheus-community/pro-bing` passe (sinon : demander, ne pas bricoler). `go.mod`/`go.sum` à jour, `go build ./...` OK.

## Phase 1 — Checker HTTP + keyword (`checker_http.go`, std uniquement)

- [ ] Transport custom : Dial 5 s, TLSHandshake 5 s, header timeout = timeout, `MaxIdleConns: 0`, `CheckRedirect` max `max_redirects` (1–20). Fichier : `internal/hub/monitors/checker_http.go`.
- [ ] Parser `accepted_status_codes` syntaxe Kuma (`200-299,401`, listes, espaces) + tests table-driven. Standard : erreurs typées par champ.
- [ ] Headers custom (≤20, `Host`/`Content-Length` interdits, UA `Beszel-Monitor` défaut), body ≤1 Mo, auth none/basic/bearer (secrets jamais loggés).
- [ ] Variante keyword : `keyword`/`invert_keyword`, corps limité 2 Mo (`LimitReader` + `truncated:true`), match substring case-sensitive documenté.
- [ ] `check_cert_expiry` sur https via checker TLS (§Phase 2) ; `ignore_tls_errors` → `tls_insecure:true` + warning.
- [ ] Tests `httptest` : status OK/KO, redirect (dont public→privé, cf. Phase 5), keyword found/not-found/invert, timeout, corps >2 Mo, `final_url` stockée. DONE = `go test ./internal/hub/monitors/ -run TestHTTP -v` vert.

## Phase 2 — Checker TLS (`checker_tls.go`, std `crypto/tls`/`x509`)

- [ ] `tls.DialWithDialer` + SNI = host (override `server_name`), port défaut 443 (1–65535), `VerifyHostname` sauf `ignore_tls_errors` (warning).
- [ ] Seuils `warn_days` 21 / `crit_days` 7 (`crit < warn` validé) ; statuts UP/warn/down (§8 spec) ; `cert_days` 2 décimales ; details `{not_after RFC3339, issuer, dns_names[≤5], error?}`.
- [ ] Tests avec CA de test : valide, expiring (warn), expiré (down), hostname mismatch, chaîne invalide. DONE = tests verts.

## Phase 3 — Checker DNS (`checker_dns.go`, `miekg/dns`)

- [ ] Client `dns.Client{Net: udp|tcp, Timeout/DialTimeout/ReadTimeout/WriteTimeout}`, `SetQuestion`, `Exchange(m, "IP:port")`, resolver vide = système, port 53 défaut.
- [ ] 10 qtypes (A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, CAA, PTR) ; `expected_answer` + `contains|exact` (case-insensible, point final normalisé) ; answers ≤5 ; RCODE numérique en `code`.
- [ ] Validation : PTR exige IP cible ; resolver custom = IP (pas de hostname).
- [ ] Tests avec serveur DNS fake (`miekg/dns` côté serveur) : NOERROR+match, mismatch, NXDOMAIN, SERVFAIL, timeout, tcp. DONE = tests verts.

## Phase 4 — Checker ping (`checker_ping.go`, `prometheus-community/pro-bing`)

- [ ] `NewPinger` + `SetPrivileged(false)` d'abord, fallback `true` si capa ; `Count`/`Size`/`Timeout`/`Interval` depuis config (count 1–10, size 0–65400, packet_timeout ≤ timeout global, inter-paquets ≥200 ms) ; `RunWithContext`.
- [ ] Succès si `PacketsRecv ≥ 1` ; `latency_ms` = AvgRtt ; details `{min,avg,max,loss,received,sent}` ; échec explicite `missing NET_RAW capability` si aucun mode (jamais de faux down silencieux).
- [ ] Tests : 127.0.0.1 unprivileged (skip si pas de capa), host injoignable (loss 100 % typé). DONE = tests verts ou skip justifié en log.

## Phase 5 — SSRF + secrets (`ssrf.go`, validation)

- [ ] `allow_private_network=false` défaut, override env admin-only + log warn au boot ; blocage loopback/RFC1918/169.254/IPv6 local/`0.0.0.0` ; schémas http(s) seuls.
- [ ] `DialContext` custom vérifiant chaque IP + re-vérif par redirect + pin de la première résolution (anti DNS-rebinding) ; même garde pour DNS/ping/TLS.
- [ ] Matrice de tests SSRF : loopback, 10/8, redirect public→privé, rebinding simulé, IPv6 link-local. DONE = matrice verte.
- [ ] Secrets : stockage chiffré, redacted `•••` en lecture API, PATCH sans secret = conserve, grep `password|token|secret` propre sur le package + revue de chaque log ajouté.

## Phase 6 — Scheduler + manager (`manager.go`, `scheduler.go`, `models.go`)

- [ ] Types + validation centralisée (`timeout < interval`, interval 20–86400, ports, target par type, qtype PTR, `crit < warn`).
- [ ] 1 goroutine/monitor (ticker + jitter 0–5 s + stagger `min(interval/n,2000ms)`), check immédiat au démarrage, skip overlap (atomique + compteur debug), `context.WithTimeout`, `recover` panics, `retry_interval = interval` documenté.
- [ ] Sémaphore global ≤10 (`MONITORS_MAX_CONCURRENT`, max 50) ; `OnTerminate` propre ; hooks PB create/update/delete → reschedule sans restart ; `consecutive_failures` persisté et relu au boot.
- [ ] Machine d'état testée : retries, DOWN au (max+1)e échec, reset au succès, `upside_down`, entrée/sortie warn, `resend_after`. DONE = tests verts.

## Phase 7 — Persistance (migration + règles + rétention)

- [ ] Migration `internal/migrations/<ts>_monitors.go` (`m.Register` + revert) : collections §4 spec, index `(monitor,created)` + `created` via `AddIndex`, `CascadeDelete` monitor→checks, `monitor_checks` sans droits d'écriture publique.
- [ ] Règles dans `setCollectionAuthSettings` : `monitors` calquées sur `systems` (membre ou SHARE_ALL, écriture bloque readonly) ; `monitor_checks` list/view membre-via-parent, écriture null.
- [ ] Cycle d'écriture en **1 transaction** (`RunInTransaction` : insert check + update monitor) ; `uptime_24h` recalculé via cron `records` existant (pas de nouveau cron).
- [ ] Rétention : étendre `DeleteOldRecords`/`CreateLongerRecords` (brut 30 j, 10 m >12 h, 2 h >7 j, cap 500 k + warn). DONE = migration up/down OK sur base vierge + testée.

## Phase 8 — API (`api.go`)

- [ ] 7 routes §7 spec (liste, POST validée 400/champ, détail redacted, PATCH revalidé + reschedule, DELETE cascade, historique paginé défaut 200/max 1000, test rate-limité 1/10 s + 10/min/user **sans écriture**, summary).
- [ ] Accès membre ou SHARE_ALL (`monitorHasUser` calqué sur `userHasSystem`), écriture bloque readonly. DONE = tests API (création, 400, redaction, accès refusé, rate-limit).

## Phase 9 — Alerting (transitions + historique)

- [ ] UP→DOWN, DOWN→UP (durée + uptime), entrée/sortie WARN via `SendAlert` + `MakeLink("monitors", id)` + quiet hours ; `resend_after` ; `notify=false` = silencieux historisé.
- [ ] `alerts_history` (`alert_id = "monitor:<id>"`, `system` vide documenté), résolution à pause/suppression. DONE = scénario UP→DOWN→UP vérifié en test avec notifs capturées.

## Phase 10 — UI (style Beszel, pas Kuma)

- [ ] `components/routes/monitors.tsx` (liste + badges + menu pause/test/supprimer + skeleton/empty/error), `components/routes/monitor.tsx` (recharts latence + uptime 30 j, table paginée, panneau cert, lien alerts_history).
- [ ] `components/monitors/monitor-dialog.tsx` (champs dynamiques par type, valibot miroir API, aides inline NET_RAW/2 Mo/TLS), `lib/monitors.ts`, types `Monitor`/`MonitorCheck`, toggle notify réutilisant `alerts-sheet.tsx`, subscribe `monitors` uniquement.
- [ ] Nav + carte `home.tsx` via `/monitors/summary` ; i18n Lingui FR+EN. DONE = `vite build` OK + screenshots liste/détail/dialog.

## Phase 11 — Config YAML

- [ ] `monitors:` dans `config.go` (users par email, match `name+target`, ne supprime jamais l'UI), miroir `config-yaml.tsx`, exemples guide. DONE = sync testée sur base de dev.

## Phase 12 — Charge + benchmark (preuves pour la PR)

- [ ] Charge SQLite : 50 monitors @60 s / 24 h simulées (ou 200 @20 s accéléré), `testing.Short`-gated : taille DB, zéro `database is locked`, p99 scheduler < 1 s.
- [ ] Benchmark vs Kuma : RSS hub + p99 @50/200 monitors. DONE = chiffres dans la PR.

## Phase 13 — Docs + PR upstream

- [ ] `readme.md` (section Monitors), `CHANGELOG.md` (entrée), `supplemental/guides/monitors.md` (champs, Docker ping, SSRF, YAML, FAQ ICMP).
- [ ] `gofmt -l` propre, `go vet ./...` OK, `go test ./internal/...` vert (zéro régression polling/alertes), grep secrets propre.
- [ ] Commits petits (1 checker/commit), rebase `upstream/main`, push fork, `gh pr create --repo henrygd/beszel` avec corps §14 spec (motivation, screenshots, spec champs, choix Go, perfs, sécu, tests, migration, breaking = aucun, zéro code Kuma).

## Critères de sortie PR1 (tous requis avant PR2)

1. [ ] Tests unitaires + intégration verts.
2. [ ] Zéro régression (`go test ./internal/...`).
3. [ ] Charge SQLite OK, chiffres publiés.
4. [ ] SSRF verte + secrets propres (grep + revue).
5. [ ] UI Beszel FR/EN + screenshots.
6. [ ] PR upstream ouverte, retours adressés.
7. [ ] Alors seulement : design PR2 (champ `agent` déjà réservé).
