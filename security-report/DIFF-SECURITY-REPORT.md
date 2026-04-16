# APICerebrus Security Diff Report

**Branch:** main
**Base:** 90dff02 (last full scan snapshot: 2026-04-16 12:20)
**Head:** 3c487f9 (HEAD)
**Scan Date:** 2026-04-16
**Scope:** 14 commits since previous `SECURITY-REPORT.md` (full scan remains valid for untouched code)
**Mode:** Diff / incremental — delta-only analysis

---

## Scope Inventory

| File | Change | Security-Relevant |
|------|--------|--------|
| `internal/migrations/migrations.go` | Modified (+88 / -5) | Yes (rollback added) |
| `internal/migrations/migrations_test.go` | Added (+179) | Test only |
| `internal/cli/cmd_db.go` | Modified (+52 / -1) | Yes (CLI rollback subcommand) |
| `internal/cli/cmd_db_test.go` | Modified (+7) | Test only |
| `internal/store/store.go` | Modified (+16) | Yes (wrappers) |
| `deployments/helm/apicerberus/templates/backup-cronjob.yaml` | Added (+90) | Yes (new workload) |
| `deployments/helm/apicerberus/templates/deployment.yaml` | Modified (+4) | Minor (GOMEMLIMIT env) |
| `deployments/helm/apicerberus/values.yaml` | Modified (+47) | Yes (backup defaults) |
| `web/src/hooks/useWebVitals.ts` | Added (+181) | Minor (telemetry hook) |
| `web/src/hooks/useWebVitals.test.ts` | Added (+34) | Test only |
| `.project/PRODUCTIONREADY.md` / `ROADMAP.md` | Docs | Skipped (docs) |

**Files scanned:** 8 (docs & lock files filtered out).

---

## Summary

| Severity | New | Existing (in touched files) | Total |
|----------|-----|-----------------------------|-------|
| Critical | 0   | 0 | 0 |
| High     | 0   | 0 | 0 |
| Medium   | 1   | 0 | 1 |
| Low      | 2   | 0 | 2 |
| Info     | 2   | 0 | 2 |

## Verdict

**WARN** — No Critical or High regressions. One Medium hardening gap + two Low risk findings introduced by the new backup CronJob. Two operational/data-integrity Info items worth addressing before the next release.

---

## New Findings (Introduced by This Change)

### DIFF-001: Backup CronJob Pod Missing Container-Level `securityContext`

- **Severity:** Medium
- **Confidence:** 90/100
- **Classification:** NEW
- **CWE:** CWE-276 (Incorrect Default Permissions), CWE-250 (Execution with Unnecessary Privileges)
- **File:** `deployments/helm/apicerberus/templates/backup-cronjob.yaml:31-34`
- **Diff Context:**
```yaml
          containers:
          - name: backup
            image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
            imagePullPolicy: {{ .Values.image.pullPolicy }}
            command:
            - /bin/sh
```
- **Description:** The main application Deployment applies `.Values.securityContext` (readOnlyRootFilesystem, drop ALL capabilities, allowPrivilegeEscalation=false) at the container level. The new backup CronJob container **does not** apply these. The backup pod therefore runs with a writable root filesystem, full default Linux capabilities, and auto-mounts the ServiceAccount token even though no Kubernetes API access is needed. If a malicious container image, compromised registry, or a vulnerable `sqlite3` binary is ever introduced, the blast radius inside the cluster is larger than necessary.
- **Remediation:**
  ```yaml
  containers:
  - name: backup
    securityContext:
      {{- toYaml .Values.securityContext | nindent 14 }}
    # ...
  ```
  Also add `automountServiceAccountToken: false` on the Pod spec. Consider a dedicated `ServiceAccount` for the CronJob (least privilege).

---

### DIFF-002: Backup Archives Contain Sensitive Data with No Encryption at Rest

- **Severity:** Low
- **Confidence:** 80/100
- **Classification:** NEW
- **CWE:** CWE-312 (Cleartext Storage of Sensitive Information), CWE-311 (Missing Encryption)
- **File:** `deployments/helm/apicerberus/templates/backup-cronjob.yaml:52-60`
- **Diff Context:**
```yaml
              timeout 30 sqlite3 "${DATA_DIR}/apicerberus.db" ".backup '${BACKUP_DIR}/apicerberus_${TIMESTAMP}.db'" || \
              timeout 30 sqlite3 "${DATA_DIR}/apicerberus.db" "VACUUM INTO '${BACKUP_DIR}/apicerberus_${TIMESTAMP}.db'"
              # ...
              tar -czf "backup_${TIMESTAMP}.tar.gz" apicerberus_*.db 2>/dev/null || true
```
- **Description:** The backup archive is a full copy of `apicerberus.db`, which contains bcrypt `users.password_hash`, `api_keys.key_hash`, `sessions.token_hash`, hashed webhook secrets, and `audit_logs` with potentially-sensitive request/response bodies. The archive is written to a PVC (or `emptyDir` by default), plain-tar-gzipped, no symmetric encryption, no integrity signature. If the PVC is backed by shared storage (NFS, CSI volumes with `ReadWriteMany`), or a restore flow mounts the backup volume into another namespace, hashes and audit bodies leak.
- **Remediation:**
  - Encrypt archives before persistence: `tar -cz ... | age -r <pubkey> -o backup.tar.gz.age` or `gpg --symmetric --cipher-algo AES256`.
  - Use an encrypted StorageClass (AWS EBS+KMS, GCP PD+CMEK).
  - Set `fsGroup` and file-mode restrictions on the backup volume.
  - Document key rotation and restore procedure with separate KMS-managed credentials.

---

### DIFF-003: Helm Values Interpolated Unquoted into `sh -c` Block

- **Severity:** Low
- **Confidence:** 60/100
- **Classification:** NEW
- **CWE:** CWE-78 (OS Command Injection)
- **File:** `deployments/helm/apicerberus/templates/backup-cronjob.yaml:38-63`
- **Diff Context:**
```yaml
            command:
            - /bin/sh
            - -c
            - |
              echo "Starting backup..."
              {{- if .Values.backup.backupScript }}
              {{ .Values.backup.backupScript | nindent 14 }}
              {{- else }}
              BACKUP_DIR="{{ .Values.backup.storage.path }}"
              DATA_DIR="{{ .Values.backup.dataPath | default "/data" }}"
              # ...
              find "${BACKUP_DIR}" -name "backup_*.tar.gz" -mtime +{{ .Values.backup.retention.days | default 7 }} -delete
```
- **Description:** Four Helm values are spliced directly into the shell-script body without quoting or validation:
  1. `.Values.backup.backupScript` — inlined raw as script body.
  2. `.Values.backup.storage.path` — assigned to `BACKUP_DIR`; only outer double-quote wraps the expansion (no guard against embedded quotes).
  3. `.Values.backup.dataPath` — same.
  4. `.Values.backup.retention.days` — inlined as a number, but any string (e.g., `7 -path /etc/shadow -o`) is injected into `find -mtime +<X>`.

  This follows Helm's standard operator-trust model (the person running `helm install` defines values.yaml), so it is Low severity. It becomes exploitable if:
  - A CD pipeline sources chart values from PR-controlled sources (GitOps with untrusted contributors).
  - Multi-tenant chart installers allow customer-supplied value overrides.
- **Remediation:** Validate critical values in `_helpers.tpl` (e.g., `regexMatch "^/[A-Za-z0-9_/-]+$"` for paths, `int` coercion for numerics), and pipe through `quote`: `BACKUP_DIR={{ .Values.backup.storage.path | quote }}`. For `retention.days`: `{{ int .Values.backup.retention.days | default 7 }}`.

---

### DIFF-004 (Info): PostgreSQL Migration v8 Uses Unsupported `CREATE TRIGGER IF NOT EXISTS`

- **Severity:** Info (operational bug, not exploitable)
- **Confidence:** 95/100
- **Classification:** EXISTING in pre-existing code — surfaced because migration idempotency is now re-exercised by the new rollback paths
- **File:** `internal/store/store.go:253, 257`
- **Description:** PostgreSQL (through version 17) does not support `IF NOT EXISTS` on `CREATE TRIGGER`. Re-applying migration v8 against an already-migrated Postgres DB raises `syntax error at or near "NOT"`. Not a security vulnerability — but because this migration is the SQL-injection boundary for Postgres full-text search, operators who hit the failure may resort to ad-hoc manual DDL which is itself a risk vector.
- **Remediation:**
  ```sql
  DROP TRIGGER IF EXISTS audit_search_vector_insert ON audit_logs;
  CREATE TRIGGER audit_search_vector_insert BEFORE INSERT ON audit_logs ...;
  ```
  Or wrap the `CREATE TRIGGER` in a `DO $$ ... $$` block that checks `pg_trigger`.

---

### DIFF-005 (Info): Migration Rollback Does Not Enforce Reverse Order

- **Severity:** Info (data-integrity risk)
- **Confidence:** 85/100
- **Classification:** NEW
- **CWE:** CWE-665 (Improper Initialization) — data-integrity gap
- **File:** `internal/migrations/migrations.go:82-140`, `internal/store/store.go:378`
- **Description:** `Rollback(version)` allows rolling back *any* applied migration, including out-of-order (e.g., rolling back v3 while v4/v5/v6 remain applied). Later migrations often depend on earlier ones' state (v5 may reference columns created in v3). Out-of-order rollback leaves the database in a state where subsequent migrations reference dropped tables/columns — typical failure mode is silent data loss followed by noisy errors on restart.

  The CLI's `db migrate rollback --version N` exposes this directly to operators. No safety check warns "migration v5 is still applied and depends on v3".
- **Remediation:**
  - Default behavior: require rollback in strict reverse order (reject if any `applied_version > target`).
  - Provide an explicit `--force` escape hatch for dev environments.
  - Optionally track dependency ranges in the `Migration` struct.

---

## Verified Non-Issues (Candidates Cleared)

The following patterns in the diff were examined and determined **not** to be vulnerabilities:

| Candidate | File | Why Safe |
|-----------|------|----------|
| SQL injection in rollback `tx.Exec(stmt)` | `migrations.go:124` | `stmt` is sourced from `Migration.Rollback[]`, hardcoded in `store.go`'s `migrationsList`. No user-tainted input. |
| CLI rollback missing auth | `cmd_db.go:126-172` | Matches existing trust model — all `apicerberus <cmd>` CLI operations are local-root-equivalent. No new attack surface. |
| `useWebVitals` leaks URL to remote | `useWebVitals.ts:66` | `window.location.href` passed only to caller-supplied `onReport` callback. No automatic network sink. Admin session uses httpOnly cookies, so URLs do not contain tokens. |
| `buffered: true` on PerformanceObserver | `useWebVitals.ts:128-132` | Standard Web Vitals pattern. No cross-origin leak. |
| GOMEMLIMIT env from values.yaml | `deployment.yaml:88-91` | Plain string substitution of operator-controlled value, no shell context. |
| `find -delete` in backup script | `backup-cronjob.yaml:63` | Bounded by `$BACKUP_DIR` and name glob `backup_*.tar.gz`. Cannot escape unless `BACKUP_DIR` is weaponized (see DIFF-003). |
| `ServiceAccount` shared between app and backup | `backup-cronjob.yaml:25` | Default chart defines no Role/RoleBinding, so the SA only has default namespace permissions. Low-risk sharing, but dedicated SA still recommended (see DIFF-001). |

Also noted (**not security**, but worth fixing):
- `useWebVitals.ts:146-150` — `visibilitychange` listener added with inline arrow function; the `removeEventListener(..., handleUnload)` on line 159 references a different function object → **event-handler leak on unmount**. Not a vuln.
- `cmd_db.go:71` — `"%spending"` should be `"%s\tpending"` (missing tab). Cosmetic.

---

## Existing Findings Still Open (Context)

All pre-existing findings from the most recent full scan remain tracked in `security-report/verified-findings.md`. The diff did **not** regress any of the prior fixes. Key outstanding items, unchanged:

- Finding 29 (Medium) — Portal sessionStorage auth flag readable by XSS (documented; acceptable for current deployment).
- Finding 31 (Low) — Kafka `InsecureSkipVerify` remains admin-configurable (validation rejects in prod).
- Findings 33, 34 (Low) — Minor hardening items documented in the previous scan.

No regressions detected.

---

## Dependency Changes

| Package | Change | Risk |
|---------|--------|------|
| — | No `go.mod`, `go.sum`, `package.json`, or `package-lock.json` changes in this diff range | No new supply-chain surface |

---

## Changed Files Not Scanned

- `.project/PRODUCTIONREADY.md`, `.project/ROADMAP.md` — Documentation, no security impact.
- `internal/migrations/migrations_test.go`, `internal/cli/cmd_db_test.go`, `web/src/hooks/useWebVitals.test.ts`, `internal/cli/cli_full_coverage_test.go`, `internal/cli/cmd_audit_test.go`, `internal/mcp/server_additional_test.go` — Test code (scanned for obvious hardcoded secrets; none found).

---

## PR Comment Summary

```
## Security Scan Results

WARN

New findings: 5 (1 Medium, 2 Low, 2 Info)
Existing findings in touched files: 0

Top items to address:
- [MEDIUM] Backup CronJob container missing securityContext (backup-cronjob.yaml:31)
- [LOW]    Backup archives unencrypted with sensitive data (backup-cronjob.yaml:52)
- [LOW]    Helm values injected unquoted into sh -c            (backup-cronjob.yaml:38)
- [INFO]   PG migration v8 uses CREATE TRIGGER IF NOT EXISTS   (store.go:253)
- [INFO]   Migration rollback allows out-of-order invocation   (migrations.go:82)

No Critical/High regressions. Safe to merge after addressing the Medium finding.
```

---

## Remediation Priority

1. **Before next Helm release** — Apply `securityContext` + `automountServiceAccountToken: false` to the backup CronJob (DIFF-001).
2. **Before enabling backup in production** — Add at-rest encryption and document key management (DIFF-002).
3. **Opportunistic hardening** — Quote Helm value interpolations (DIFF-003).
4. **Before next Postgres-target release** — Replace `CREATE TRIGGER IF NOT EXISTS` with `DROP TRIGGER IF EXISTS` + `CREATE TRIGGER` (DIFF-004).
5. **Operational safety** — Enforce reverse-order rollback or require `--force` (DIFF-005).
