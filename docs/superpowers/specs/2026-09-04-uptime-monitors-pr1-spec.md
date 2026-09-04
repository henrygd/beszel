# SPEC — Monitors externes type Uptime Kuma pour Beszel

## PR1 : exécution via le hub uniquement (mergeable upstream)

> **Fichier** : `docs/superpowers/specs/2026-09-04-uptime-monitors-pr1-spec.md`
> **Branche** : `feat/uptime-monitors` (fork `delta-whiplash/beszel`, `origin` = fork,
> `upstream` = `henrygd/beszel`).
> **Règle d'or** : PR1 100 % hub-only. Zéro modification de `agent/`,
> `internal/entities/`, `internal/common/`, du protocole WS/SSH/CBOR.
> La PR2 (exécution distribuée via les agents) est branchée AU-DESSUS de la PR1
> et ne démarre qu'une fois la PR1 masterisée, testée, éprouvée, sécurisée,
> conforme et belle.
>
> **Docs de référence vérifiées via Context7** :
> PocketBase Go (`/websites/pocketbase_io` : `core.NewBaseCollection`,
> `m.Register` avec revert, `core.NewRecord` + `app.Save`, `AddIndex`),
> `miekg/dns` (`Client{Net, Timeout, DialTimeout}` + `Exchange(m, "IP:port")`),
> `prometheus-community/pro-bing` (`probing.NewPinger`, `SetPrivileged(false)`, `Count/Size/
> Timeout/Interval`, `RunWithContext`, `Statistics()` avec MinRtt/AvgRtt/MaxRtt/
> PacketLoss).

---

## 1. Contexte et objectif

Beszel (`henrygd/beszel`, v0.19.0) est un monorepo Go + React embarqué :
`agent/` collecte les métriques système (pull via `gatherStats`, cache 60 s),
`internal/hub/` tourne sur PocketBase v0.40.2 (SQLite embarqué),
`internal/alerts/` gère seuils + envoi (emails + webhooks shoutrrr + quiet hours +
`alerts_history`), `internal/site/` est le frontend React 19 + Vite + Tailwind.

Il n'existe aujourd'hui **aucun monitor externe** : `internal/hub/heartbeat/` est
sortant uniquement (POST vers BetterStack/Kuma/Healthchecks), `HandleStatusAlerts`
couvre la joignabilité agent, aucune collection `monitors`, aucun scheduler de
checks, aucun code ping/DNS/HTTP/SSL réutilisable.

**Objectif** : remplacer Uptime Kuma pour 4 types de checks — HTTP(s)+keyword,
certificat SSL/TLS, DNS, ping ICMP — en Go, intégré natif à Beszel : même nav,
mêmes notifs, mêmes graphs, même config déclarative, mêmes standards de code.
On ne doit pas avoir l'impression d'une autre application.

---

## 2. Périmètre

### 2.1 Dans la PR1

- 4 checkers exécutés **côté hub** : `http` (+ variante `keyword`), `tls`, `dns`, `ping`.
- Scheduler hub : worker pool, ticker par monitor, jitter, anti-overlap, timeouts.
- Persistance PocketBase : collections `monitors` + `monitor_checks`, rétention +
  downsampling, index.
- API hub : CRUD monitors + lecture historique + test manuel + résumé.
- Notifications sur **transitions** via pipeline existant (`AlertManager.SendAlert`).
  Aucun nouveau provider de notification.
- UI : page Monitors dédiée au style Beszel + résumé sur home + formulaire par type.
- Config déclarative : clé `monitors:` dans `config.yml`.
- Docs : README, CHANGELOG, guide, benchmark vs Kuma.
- Tests Go + build site vérifié. `gofmt` + `go vet` propres.

### 2.2 Hors PR1 (reportés explicitement, pas oubliés)

- Exécution depuis les agents (PR2). Le schéma réserve un champ `agent` nullable
  pour éviter toute migration cassante, mais aucun code agent en PR1.
- Monitor TCP/port générique, NTLM, proxy par monitor, mTLS client, maintenance
  windows, status pages publiques, canaux de notif par monitor (canaux globaux
  existants en PR1), fédération multi-hub.

---

## 3. Architecture PR1

```text
  UI React (internal/site/)          Hub Go (internal/hub/)
  ─────────────────────────          ──────────────────────
  components/routes/                 monitors/  (NOUVEAU package)
   monitors.tsx (liste)               manager.go    CRUD + cache + hooks PB
   monitor/[id].tsx (détail)          scheduler.go  1 goroutine/monitor
  components/monitors/                checker_http.go  std net/http
   monitor-dialog.tsx                 checker_tls.go   std crypto/tls
  lib/monitors.ts                     checker_dns.go   miekg/dns
                                      checker_ping.go  prometheus-community/pro-bing
                                      ssrf.go          garde-fou réseau
                                      api.go           routes REST
                                      models.go        types + validation
                                        │
                                        ▼
                                     internal/alerts (EXISTANT)
                                     SendAlert → emails, shoutrrr,
                                     quiet_hours, alerts_history
                                        │
                                     internal/records (ÉTENDU)
                                     rétention monitor_checks
```

Câblage : constructeur + démarrage dans `internal/hub/hub.go` (`StartHub`, à côté
de `sm.Initialize()` et du heartbeat), routes dans `internal/hub/api.go`
(`se.Router.Group("/api/beszel")`, pattern existant lignes 88-140).
Migration dédiée `internal/migrations/<timestamp>_monitors.go` via `m.Register`
(avec fonction revert, pattern Context7). Ne jamais éditer le snapshot
`0_collections_snapshot_0_19_0.go` à la main. Règles via `setCollectionAuthSettings`
dans `internal/hub/collections.go`, calquées sur `systems`.

---

## 4. Modèle de données

### 4.1 Collection `monitors` (base : `core.NewBaseCollection("monitors")`)

| Champ | Type PB | Défaut / contrainte | Notes |
|---|---|---|---|
| `name` | Text, requis, max 100 | — | Unicité par utilisateur contrôlée applicativement |
| `type` | Select, requis | valeurs `http\|keyword\|ping\|dns\|tls` | `keyword` = HTTP + recherche texte |
| `target` | Text, requis, max 500 | — | URL (http/tls) ou hostname (ping/dns), validé par type (§6) |
| `interval` | Number, requis | 60, min 20, max 86400 (s) | Période entre checks |
| `timeout` | Number, requis | 10 (s), **strictement < interval** (validation API) | Timeout global par tentative |
| `max_retries` | Number | 2, min 0, max 10 | Échecs consécutifs avant DOWN (DOWN au (max_retries+1)e échec ; 0 = DOWN immédiat) |
| `upside_down` | Bool | false | Inverse UP/DOWN (tester un blocage voulu) |
| `paused` | Bool | false | Pas de check, pas de notif, statut `paused` |
| `notify` | Bool | true | false = historisé mais silencieux |
| `resend_after` | Number | 0 (= jamais), min 0, max 1440 (min) | Renotifie un DOWN persistant tous les N minutes |
| `users` | Relation `users` multiple, requis | — | Même pattern que `systems.users[]` |
| `agent` | Relation `systems` simple, nullable | null, **RÉSERVÉ PR2, toujours null en PR1** | Évite une migration cassante en PR2 |
| `config` | JSON | `{}` | Config typée par `type` (§6), validée côté API |
| `status` | Select | `pending` ; valeurs `up\|down\|warn\|paused\|pending` | `warn` = TLS sous seuil warn mais au-dessus du seuil crit |
| `last_check` | Date | — | Dernière tentative |
| `last_latency_ms` | Number | — | Dernière latence |
| `uptime_24h` | Number | — | Cache recalculé périodiquement (§8), jamais à chaque check |
| `cert_days` | Number, nullable | — | Jours restants (http https / tls), null sinon |
| `consecutive_failures` | Number | 0, non éditable via API publique | Compteur interne persisté (reprise au boot sans renotif) |

Règles (même pattern que `systems` dans `collections.go`) :
list/view = `@request.auth.id != "" && users.id ?= @request.auth.id`
(ou `@request.auth.id != ""` si `SHARE_ALL_SYSTEMS=true`) ;
create/update/delete = règle de lecture + `&& @request.auth.role != "readonly"`.

### 4.2 Collection `monitor_checks` (base, écriture serveur uniquement)

| Champ | Type | Notes |
|---|---|---|
| `monitor` | Relation `monitors`, requis, `CascadeDelete: true` | Suppression monitor → purge historique |
| `status` | Select `up\|down\|warn`, requis | Après application de `upside_down` |
| `latency_ms` | Number | RTT total de la tentative |
| `code` | Number, nullable | Status HTTP / RCODE DNS, null pour ping/tls |
| `message` | Text, max 500 | Résumé d'erreur ou d'état |
| `details` | JSON, nullable | Tronqué par type (§6) |
| `cert_days` | Number, nullable | Recopié pour les graphs d'expiry |
| `created` | Autodate onCreate | Index `(monitor, created)` via `AddIndex` (pattern Context7) |

Règles : list/view = membre du monitor parent (via `monitor.users.id ?=
@request.auth.id`) ou SHARE_ALL ; create/update/delete = null (jamais via API
publique, écriture serveur avec `SaveNoValidate` dans la transaction du cycle).

---

## 5. Scheduler

Fichiers `internal/hub/monitors/manager.go`, `scheduler.go`. Standards repris de
`systems/system_manager.go` et `systems/system.go` (`StartUpdater`, ticker 60 s,
jitter `getJitter`, stagger `delta = min(interval/n, 2000ms)`, `waitForContext`).

- `Manager` : construit dans `NewHub`-like, `Initialize()` charge les monitors non
  pausés (`status != 'paused'`), cache mémoire (`store.Store[string, *Monitor]`
  comme `SystemManager`), hooks PB `monitors` create/update/delete → start/stop/
  reconfigure sans restart. `OnTerminate` → cancel + waitgroup.
- 1 goroutine par monitor : jitter initial aléatoire 0–5 s + stagger au boot,
  `time.Ticker(interval)`, check immédiat au (re)démarrage si non pausé.
- Anti-overlap : flag atomique par monitor ; si un run est en cours au tick, skip
  + compteur `skipped_runs` en log debug (pas d'historique).
- Chaque run : `ctx, cancel := context.WithTimeout(timeout)`, `recover()` sur panic
  checker → échec `checker panic: …`. `timeout < interval` garanti par validation.
- `retry_interval` : en PR1 égal à `interval` (pas de ticker accéléré en état
  indécis — choix assumé, borne la charge, documenté).
- Concurrence globale bornée : sémaphore (défaut 10 runs simultanés, env
  `MONITORS_MAX_CONCURRENT`, plafond 50) pour protéger le hub et SQLite.
- Tous les paramètres ticker/env documentés dans le guide.

---

## 6. Checkers — spec par type

Champs communs (cf. §4.1) : `interval` (60, ≥20), `timeout` (10, < interval),
`max_retries` (2), `upside_down`, `paused`, `notify`, `resend_after`.

### 6.1 HTTP(s) + variante Keyword — `checker_http.go`, std uniquement

Config `config` JSON :

| Clé | Défaut | Validation / notes |
|---|---|---|
| `method` | `GET` | Enum GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS |
| `accepted_status_codes` | `200-299` | Syntaxe Kuma : `200-299,401`, `200,201,204`. Parser maison + tests table-driven (plages, listes, espaces, ordre indifférent, `200-299` inclusif) |
| `follow_redirects` / `max_redirects` | true / 10 (1–20) | `CheckRedirect` custom : cap + re-validation SSRF à chaque hop (§10), stocke `final_url` |
| `headers` | `{}` | Map ≤20 entrées, clés ≤100 car. ; interdites : `Host`, `Content-Length` ; `User-Agent: Beszel-Monitor` par défaut |
| `body` / `content_type` | `""` / auto | Envoyé si méthode à corps ; body ≤1 Mo côté validation config |
| `auth_type` | `none` | `none\|basic\|bearer` ; basic = `username`+`password`, bearer = `token` ; secrets chiffrés (§10.3), jamais loggés ni renvoyés |
| `ignore_tls_errors` | false | `InsecureSkipVerify` ; si true → `tls_insecure:true` dans details + bandeau warning UI explicite |
| `keyword` / `invert_keyword` | `""` / false | Type `keyword` : UP si le corps contient (ou ne contient pas si invert) le mot-clé ; substring case-sensitive, documenté |
| `check_cert_expiry` | true si schéma https | Réutilise le checker TLS (§6.2) sur la connexion établie |
| `warn_days` / `crit_days` | 21 / 7 | Cf. §6.2 ; `crit_days < warn_days` validé |

Transport : `http.Transport{DialContext: &net.Dialer{Timeout: 5s} custom SSRF
(§10), TLSHandshakeTimeout: 5s, ResponseHeaderTimeout: timeout, MaxIdleConns: 0}` ;
`http.Client{Timeout: timeout, CheckRedirect: custom}`.
Corps lu via `io.LimitReader` 2 Mo : au-delà, tronqué + `truncated:true`
(le match keyword porte sur les 2 premiers Mo — documenté).
Succès = TCP+TLS OK **et** status accepté **et** (si keyword) match **et**
(si https + check_cert_expiry) cert au-dessus du seuil crit.
Résultat : `latency_ms`, `code`, `cert_days` si https, details `{final_url,
keyword_found?, tls_insecure?, truncated?}`.

### 6.2 TLS / certificat — `checker_tls.go`, std `crypto/tls` + `crypto/x509`

- `target` = URL ou `host[:port]` ; `host` requis (SNI = host sauf override
  `server_name`), `port` défaut 443 (1–65535).
- `tls.DialWithDialer` + `VerifyHostname(host)` complet sauf `ignore_tls_errors`
  (alors handshake seul + warning, expiry toujours évaluée).
- Statuts : UP si `days > warn_days` ; **warn** si `crit_days < days ≤ warn_days`
  (notif warn une fois à l'entrée, puis à la sortie) ; **down** si `days ≤
  crit_days`, cert expiré, hostname mismatch, chaîne invalide.
- `cert_days` float 2 décimales ; details `{not_after (RFC3339), issuer,
  dns_names[≤5], error?}`.

### 6.3 DNS — `checker_dns.go`, dep `github.com/miekg/dns`

Justification (pas de std) : `net.Resolver` couvre mal MX/NS/SOA/SRV/CAA/PTR avec
resolver:port + protocole explicites. API vérifiée Context7 :
`c := new(dns.Client); c.Net = "udp"|"tcp"; c.Timeout / DialTimeout / ReadTimeout /
WriteTimeout` puis `c.Exchange(m, "IP:port")`, `m.SetQuestion("example.com.",
dns.TypeA)`.
Config : `qtype` (défaut A ; enum A, AAAA, CNAME, MX, TXT, NS, SOA, SRV, CAA, PTR),
`resolver` (vide = resolver système ; sinon IP ou `IP:port`, port défaut 53),
`protocol` (`udp` défaut, option `tcp`), `query_timeout` (défaut 5 s, ≤ timeout
global), `expected_answer` (vide = pas de comparaison) + `match_mode`
(`contains` défaut | `exact`, insensible à la casse, point final normalisé).
Succès = RCODE NOERROR + ≥1 réponse **et** match si attendu.
Échecs typés : NXDOMAIN, SERVFAIL, timeout, mismatch (message inclut les answers
reçues tronquées). Résultat : `latency_ms` (= rtt `Exchange`), `code` = RCODE
numérique, details `{answers[≤5], qtype, resolver}`.
Validation : qtype PTR exige une IP dans target (reverse) ; resolver custom doit
être une IP (pas de hostname → pas de récursion bootstrap).

### 6.4 Ping — `checker_ping.go`, dep `github.com/prometheus-community/pro-bing`

Justification : le std n'a pas de ping ; `x/net/icmp` brut impose raw sockets
(root) + gestion manuelle echo id/seq + code Windows spécifique. API vérifiée
Context7 : `pinger, err := probing.NewPinger(host)`, `pinger.SetPrivileged(false)`
(unprivileged UDP par défaut — pas de root requis), `Count/Size/Timeout/Interval`,
`RunWithContext(ctx)`, `Statistics()` → `PacketsSent/PacketsRecv/PacketLoss/
MinRtt/AvgRtt/MaxRtt`, callbacks `OnRecv/OnFinish` disponibles.
Config : `count` (3, 1–10), `packet_size` (56, 0–65400), `packet_timeout` (2 s,
≤ timeout global), `interval_between_packets` (1 s, ≥200 ms).
Mode : `SetPrivileged(false)` d'abord ; fallback `SetPrivileged(true)` si root/
`NET_RAW` ; si aucun mode ne fonctionne (Docker sans `NET_RAW` ni
`ping_group_range`), échec **explicite**
`ping unavailable: missing NET_RAW capability (see docs)` — jamais de faux down
silencieux. Succès si ≥1 réponse (`PacketsRecv ≥ 1`).
Résultat : `latency_ms` = AvgRtt, details `{min_ms, avg_ms, max_ms, loss_pct,
received, sent}`. Docs : section Docker (`--cap-add=NET_RAW` ou
`sysctl net.ipv4.ping_group_range`) + note « ICMP filtré → préférer http ».

### 6.5 Dépendances (minimales, justifiées)

- `github.com/miekg/dns` — §6.3.
- `github.com/prometheus-community/pro-bing` — §6.4 (fork maintenu de go-ping).
- HTTP/TLS/scheduler : std uniquement (`net/http`, `crypto/tls`, `crypto/x509`,
  `net`, `time`, `context`, `sync`, `io`, `strings`, `strconv`).
  `golang.org/x/sync` (déjà en transitif) passé en direct si `errgroup` utilisé.
  Pas de lib cron, pas de client HTTP tiers.

---

## 7. API

Fichier `internal/hub/monitors/api.go`, monté dans `api.go` (pattern lignes 88-140 :
`apiAuth := se.Router.Group("/api/beszel")` + `apis.RequireAuth()` ;
écriture + `excludeReadOnlyRole`). Accès : membre du monitor (`users` contient
`@request.auth.id`) ou SHARE_ALL ; helper `monitorHasUser` calqué sur
`userHasSystem` (`alerts_api.go`) et `system.HasUser`.

| Route | Méthode | Notes |
|---|---|---|
| `/monitors` | GET | Liste (filtre `?paused=`, tri `name`), statut + uptime_24h + cert_days inclus |
| `/monitors` | POST | Validation stricte (§4.1 + §6 + timeout<interval + ports + target par type) ; 400 typées par champ |
| `/monitors/:id` | GET | Détail + config, **secrets redacted** (`password:"••••••"`, `token:"••••••"`) |
| `/monitors/:id` | PATCH | Partiel, revalidation ; reschedule si interval/timeout changés ; reset `consecutive_failures` si target/config changée |
| `/monitors/:id` | DELETE | Cascade `monitor_checks` via règle PB |
| `/monitors/:id/checks` | GET | `?range=24h\|30d&limit=` (défaut 200, max 1000), ordre antéchronologique, pour graphs |
| `/monitors/:id/test` | POST | Run manuel immédiat ; rate-limit 1/10 s par monitor + 10/min par user ; **n'écrit pas** l'historique, ne touche pas aux compteurs (documenté) |
| `/monitors/summary` | GET | Compteurs up/down/warn/paused + liste des down courants (pour home) |

---

## 8. États, transitions, notifications, uptime

- Compteur `consecutive_failures` (mémoire + persisté à chaque cycle pour reprise
  au boot sans renotifier un DOWN déjà connu). Succès → 0. Échec → +1.
  Passage DOWN quand `failures > max_retries` (défaut 2 : DOWN au 3e échec).
  `upside_down` appliqué **avant** comparaison (succès brut → échec logique).
- Transitions notifiées si `notify=true`, via `AlertManager.SendAlert`
  (emails + webhooks shoutrrr + quiet hours via `IsNotificationSilenced`) :
  UP→DOWN (`🔴 Monitor down: <name>` — target, dernier message d'erreur, latence,
  lien `MakeLink("monitors", id)`), DOWN→UP (`🟢 Monitor recovered: <name>` —
  durée du down + uptime 24h), entrée/sortie WARN TLS (`🟡 TLS cert expiring:
  <name> (<days> days)`). `resend_after` (0 = jamais) : renotifie un DOWN persistant.
- Historique : insert `alerts_history` avec `alert_id = "monitor:<id>"`
  (même colonnes `user,system,alert_id,name,value,created,resolved` — `system`
  vide pour les monitors, documenté) ; résolution à la suppression/pause
  (pattern `resolveHistoryOnAlertDelete`).
- Uptime % = succès/(succès+échecs) sur 24 h (checks `test` exclus), recalculé
  toutes les 10 min en étendant le cron `create longer records` (`records.go`) —
  **pas de nouveau cron**.
- `paused=true` → statut `paused`, compteur reset, pas de notif, scheduler stoppé
  pour ce monitor.

---

## 9. SQLite / PocketBase — durcissement (risque n°1)

Rappel du volume : 50 monitors @60 s ≈ 72 000 lignes/jour dans `monitor_checks`.

1. **Une seule transaction par cycle** : `RunInTransaction` (insert
   `monitor_checks` + update `monitors` ensemble). Jamais de save par champ.
2. **Bornes** : interval min 20 s (validation API + contrainte migration),
   timeout < interval, concurrence globale ≤10 (`MONITORS_MAX_CONCURRENT`, max 50),
   skip overlap, `retry_interval = interval`.
3. **Rétention** : brut 30 j ; downsample 10 m au-delà de 12 h, 2 h au-delà de 7 j,
   en étendant `DeleteOldRecords` + `CreateLongerRecords` (`internal/records/`,
   même style que `deleteOldSystemStats` dans `records_deletion.go:60-100`) ;
   cap de sécurité : si >500 k lignes, purge les plus anciennes + log warn.
4. **Index** : `(monitor, created)` + `created` seul, créés dans la migration via
   `AddIndex` (pattern Context7).
5. **Realtime sobre** : subscribe PB sur `monitors` (statuts) uniquement ; jamais
   sur chaque `monitor_checks` ; historique chargé à la demande, paginé.
6. **Boot** : stagger + jitter (§5) ; `consecutive_failures` relu depuis DB.
7. **Preuve exigée** : test de charge — 50 monitors @60 s sur 24 h simulées
   (ou 200 @20 s en accéléré) : taille DB, zéro `database is locked`, p99
   scheduler < 1 s. Chiffres repris dans la description de PR. Ne pas toucher à
   la config SQLite de PocketBase (WAL etc.) sauf preuve mesurée.

---

## 10. Sécurité

### 10.1 SSRF (critique sur instance multi-utilisateurs) — `ssrf.go`

- `allow_private_network=false` par défaut ; override admin-only via env
  `MONITORS_ALLOW_PRIVATE_NETWORK=true` (labo uniquement, log warn au boot).
- Bloqués par défaut : loopback (127/8, ::1), 10/8, 172.16/12, 192.168/16,
  169.254/16 (metadata cloud), link-local et unique-local IPv6, `0.0.0.0`.
- HTTP : résolution du host **avant** dial + `DialContext` custom qui vérifie
  **chaque** IP ; re-vérification à chaque redirect (même si l'initial est public) ;
  on épingle la première résolution par tentative (anti DNS-rebinding, TTL courte
  ignorée).
- DNS/ping/TLS : même vérification d'IP avant connexion quand `allow_private=false`.
- Schémas `http(s)` uniquement pour le type `http` ; `file:`, `gopher:`, `ftp:`,
  etc. rejetés à la validation.
- Tests : matrice SSRF (loopback, 10/8, redirect public→privé, DNS-rebinding
  simulé, IPv6 link-local).

### 10.2 Timeouts et limites

Timeout global < interval (validation) ; Dial 5 s ; TLS handshake 5 s ;
header timeout = timeout ; corps réponse 2 Mo max ; body config 1 Mo max ;
`max_redirects` 1–20 ; headers ≤20 entrées ; test manuel rate-limité (§7).

### 10.3 Secrets

`password`/`token` (basic/bearer) et valeurs de headers sensibles : stockage
chiffré au repos (mécanisme existant des tokens — à défaut, champ PB à masquage
+ redaction systématique, **jamais en clair lisible**) ; redacted `•••` en lecture
API ; jamais loggés (revue de chaque log ajouté + helper de masquage façon
`sanitizeHeartbeatURL` dans `heartbeat.go:297`) ; PATCH sans secret = conserve
l'existant ; grep de contrôle `password|token|secret` sur le package avant PR.

---

## 11. Frontend — intégré Beszel, pas Kuma repeint

Fichiers (patterns : routes dans `components/routes/*.tsx` montées en lazy dans
`main.tsx:30-33`, formulaires valibot comme `settings/notifications.tsx`,
stores nanostores `lib/stores.ts`, subscribe `pb.collection()`, types
`types.d.ts`, i18n Lingui + `i18n.yml`) :

- `components/routes/monitors.tsx` : liste (badge statut, uptime 24 h, latence,
  cert_days si pertinent, menu pause/test/supprimer), états vide/erreur/loading
  au style existant (skeleton, pas de spinner custom).
- `components/routes/monitor.tsx` (détail `:id`) : graphs recharts latence + uptime
  30 j (via hook façon `use-system-data.ts`), table historique paginée, panneau
  certificat pour tls/https, lien vers `alerts_history` filtrée.
- `components/monitors/monitor-dialog.tsx` : formulaire par type (select type →
  champs dynamiques), validation miroir de l'API (valibot), aide inline
  (note NET_RAW pour ping, note 2 Mo pour keyword, warning `ignore_tls_errors`).
- `lib/monitors.ts` (+ entrées `alerts.ts`-like si toggle notify réutilisé depuis
  `alerts-sheet.tsx`), types `Monitor`/`MonitorCheck` dans `types.d.ts`.
- Nav existante + carte résumé sur `home.tsx` (compteurs via `/monitors/summary`).
- i18n : nouvelles clés Lingui, FR + EN à minima.

---

## 12. Config déclarative

Étendre `internal/hub/config/config.go` (pattern `systemConfig` + `SyncSystems`) :
`Monitors []monitorConfig yaml:"monitors"` (mêmes champs §4.1/§6, `users` par email
convertis en IDs comme lignes 72-86), sync au boot (créé/MAJ par `(name,target)`,
**ne supprime jamais** ce que l'UI a créé sauf flag explicite — même prudence que
les systems). Miroir UI `config-yaml.tsx` + exemples dans le guide.

---

## 13. Tests (exigés, pas optionnels)

- Unitaires : parser `accepted_status_codes` (table-driven), machine d'état
  (transitions, retries, upside_down, warn TLS, resend_after), validation par type
  (timeout<interval, ports, target, qtype PTR…), matrice SSRF, redaction secrets,
  calcul uptime.
- Intégration (pattern `hub_test_helpers.go` / `systems_test_helpers.go`) :
  serveurs `httptest` (status/redirect/keyword/timeout/troncation), serveur DNS
  fake (`miekg/dns` côté serveur en test), TLS avec CA de test, ping 127.0.0.1
  (skip si pas de capa, unprivileged d'abord).
- Charge SQLite : `testing.Short`-gated (skip en CI courte), chiffres repris en PR.
- Frontend : `vite build` OK (bun ou npm selon l'env) ; pas d'e2e exigé en PR1
  (follow-up documenté).
- Commandes : `gofmt -l .`, `go vet ./...`,
  `go test ./internal/hub/monitors/... ./internal/records/... ./internal/alerts/...`.

---

## 14. Docs et PR upstream

- `readme.md` (section Monitors), `supplemental/CHANGELOG.md` (entrée),
  `supplemental/guides/monitors.md` (champs par type, Docker ping, SSRF, exemples
  YAML, FAQ ICMP filtré).
- Titre : `feat(hub): add external uptime monitors (http, tls, dns, ping)`.
  Corps : motivation (remplace Kuma), screenshots, spec champs par type, choix Go
  (stdlib + 2 deps justifiées), perfs (RSS + p99 @50/200 monitors vs Kuma), sécu
  (SSRF, secrets), tests, migration PB, breaking = aucun (nouvelles collections
  uniquement). Zéro code Kuma (inspiration seulement).
- Workflow : petits commits conventionnels (1 checker/commit), rebase sur
  `upstream/main` avant push, `gh pr create --repo henrygd/beszel`.

---

## 15. Critères de sortie PR1 (avant PR2 — tous requis)

1. `gofmt`, `go vet`, tests unitaires + intégration verts.
2. Zéro régression polling systèmes/alertes existants (`go test ./internal/...`).
3. Charge SQLite : 50 monitors @60 s, 24 h simulées, zéro lock, DB bornée, chiffres
   publiés.
4. Matrice SSRF verte + secrets jamais en clair (grep + revue).
5. UI au style Beszel + FR/EN + screenshots dans la PR.
6. PR upstream ouverte, retours adressés.
7. Alors seulement : design PR2 (agents) sur la base du champ `agent` réservé.

---

## 16. Risques résiduels assumés

Ping filtré par certains réseaux (documenté, fallback http conseillé) ; débat
`warn` vs `down` TLS (seuils configurables, défauts 21/7) ; volume
`monitor_checks` (contraint §9, validé par test de charge) ; canaux par monitor
reportés (notifs globales en PR1) ; pas d'e2e frontend en PR1.
