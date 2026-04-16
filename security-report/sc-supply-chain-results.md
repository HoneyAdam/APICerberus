# Supply Chain + IaC Re-Audit
**Scope:** go.mod, web/package.json, Dockerfile*, deployments/helm, deployments/kubernetes, deployments/docker, .github/workflows, Makefile
**Date:** 2026-04-16
**Baseline:** `security-report/dependency-audit.md` (2026-04-16 01:43), `security-report/DIFF-SECURITY-REPORT.md` — backup-cronjob findings (DIFF-001/002/003) intentionally **not** repeated here.

---

## Findings

### SUPPLY-001: CI/CD GitHub Actions pinned by floating tag, not full-SHA
- **Severity:** High
- **Confidence:** 95/100
- **CWE:** CWE-494 (Download of Code Without Integrity Check), CWE-829 (Inclusion of Functionality from Untrusted Control Sphere)
- **File:** `.github/workflows/ci.yml:28, 31, 40, 89, 107, 331, 346, 351, 376, 379, 382, 390, 402, 430, 468, 473`; `.github/workflows/release.yml:21, 27, 32, 43, 57, 60, 63, 71, 81, 101`
- **Description:** All third-party Actions are referenced by major-version tag (`@v3`, `@v4`, `@v5`, `@v6`), not by immutable commit SHA. Tags in GitHub Actions are **mutable** — a compromised maintainer or attacker with repository write access can retarget `v4` to a malicious commit, and every workflow run on `main` or PRs will immediately execute the new code with access to secrets (`KUBE_CONFIG_PRODUCTION`, `PRODUCTION_ADMIN_API_KEY`, `PRODUCTION_JWT_SECRET`, `GITHUB_TOKEN` with `packages:write`). This is the exact attack vector used against `tj-actions/changed-files` in March 2025 (CVE-2025-30066). Affected actions in this repo include: `aquasecurity/trivy-action@v0.35.0`, `securego/gosec@v2.22.10`, `codecov/codecov-action@v4`, `docker/login-action@v3`, `docker/setup-buildx-action@v3`, `docker/build-push-action@v6`, `azure/setup-helm@v4`, `azure/setup-kubectl@v3`, `goreleaser/goreleaser-action@v6`, `golangci/golangci-lint-action@v6`, `github/codeql-action/upload-sarif@v3`. First-party `actions/*` are lower-risk but still benefit from SHA pinning.
- **Exploit scenario:** Attacker compromises (or social-engineers) a maintainer of `aquasecurity/trivy-action`. They force-push `v0.35.0` (or publish `v0.35.1` that is pulled via the docker metadata action's automatic minor update). On the next push to `main`, the trivy step runs with `actions: read`, `contents: read`, `security-events: write` — but because earlier steps in the same job run under `${{ secrets.GITHUB_TOKEN }}` context, a malicious action can exfiltrate it. If the malicious action survives to the `deploy-production` job, it gets `KUBE_CONFIG_PRODUCTION` and production admin keys.
- **Remediation:** Pin every third-party action to a full 40-char commit SHA with the version in a trailing comment. Enable Dependabot for Actions to automate rotation:
  ```yaml
  - uses: aquasecurity/trivy-action@6e7b7d1fd3e4fef0c5fa8cce1229c54b2c9bd0d8 # v0.35.0
  ```
  Add `.github/dependabot.yml`:
  ```yaml
  version: 2
  updates:
    - package-ecosystem: "github-actions"
      directory: "/"
      schedule:
        interval: "weekly"
  ```

---

### SUPPLY-002: Production/Staging Helm secrets passed via `--set` on CLI (command-line leakage + state-file persistence)
- **Severity:** High
- **Confidence:** 90/100
- **CWE:** CWE-214 (Invocation of Process Using Visible Sensitive Information), CWE-532 (Insertion of Sensitive Information into Log File), CWE-312 (Cleartext Storage of Sensitive Information)
- **File:** `.github/workflows/ci.yml:491-492, 568-569`
- **Description:** Both `deploy-staging` and `deploy-production` pass the JWT secret and admin API key on the `helm upgrade` command line:
  ```yaml
  helm upgrade --install apicerberus deployments/helm/apicerberus \
    ...
    --set secrets.jwtSecret=${{ secrets.PRODUCTION_JWT_SECRET }} \
    --set secrets.adminApiKey=${{ secrets.PRODUCTION_ADMIN_API_KEY }} \
  ```
  Issues:
  1. Secrets appear as arguments to the `helm` process — visible in `/proc/<pid>/cmdline` to any other process on the runner (multi-tenant / malicious action risk).
  2. GitHub Actions masks the secret in *stdout*, but the value is written into the Helm release **state** (`secrets.kubernetes.io/helm.release.v1.*`) in plaintext Go-template-substituted form inside the ConfigMap/Secret. An attacker with `get secrets` in the release namespace can trivially extract them — but more importantly, Helm stores the **rendered manifest** (including substituted values) in a Secret in the release namespace. A stale release history entry therefore preserves every historical admin key that was rotated.
  3. If any future `run:` step accidentally echoes `helm get values` or `helm history`, the secret leaks into CI logs despite masking (Helm's output format can break GitHub's secret-scanning regex — e.g., when the value is rendered inside a JSON structure with special characters or line-wrapped).
- **Exploit scenario:** Compromised dependency in an earlier CI step (see SUPPLY-001) reads `/proc/*/cmdline`, recovers the prod admin API key, and uses it to exfiltrate the entire gateway configuration (users, API keys, route costs) via the Admin API.
- **Remediation:**
  - Use `--values` with a file that is created via `echo "$SECRET" > values-override.yaml` where the file path is never logged, or better:
  - Provision secrets out-of-band using SealedSecrets / External Secrets Operator / `kubectl create secret` piped from stdin and let Helm reference an **existing** Secret (the chart's `secret.yaml` already supports `helm.sh/resource-policy: keep` — leverage it). Remove the `--set secrets.*` entirely from the workflow.
  - Rotate `PRODUCTION_JWT_SECRET` and `PRODUCTION_ADMIN_API_KEY` immediately if CI logs have ever been shared externally.

---

### SUPPLY-003: CI workflow missing top-level `permissions:` block — tokens default to overly-broad scope
- **Severity:** Medium
- **Confidence:** 95/100
- **CWE:** CWE-250 (Execution with Unnecessary Privileges), CWE-732 (Incorrect Permission Assignment)
- **File:** `.github/workflows/ci.yml:1-18` (no top-level `permissions:`); `release.yml:8-10` (sets `contents: write` and `packages: write` at **workflow** level, so every job inherits write access)
- **Description:** `ci.yml` has no top-level `permissions:` key. When a repository's default token permission is "Read and write" (GitHub's historical default, still common), **every job** in `ci.yml` without its own `permissions:` block receives the legacy default of `contents: write, packages: write, issues: write, pull-requests: write, statuses: write, actions: write, checks: write, deployments: write` etc. The jobs `lint`, `test`, `web-test`, `build`, `integration-test`, `e2e-test`, `benchmark`, `helm` have no job-level `permissions:` and therefore inherit this broad scope. Only `security-scan`, `docker`, `deploy-staging`, `deploy-production` set narrower job-level permissions. A compromised test dependency in `lint`/`test`/`benchmark` can therefore push commits, create releases, invalidate Actions caches (cache poisoning), or modify workflow files.
  `release.yml` sets `contents: write` + `packages: write` at the **workflow** level, meaning `docker-release` (which only needs `packages: write`) and `helm-release` get `contents: write` when they should not.
- **Exploit scenario:** A malicious test dependency in `benchmark` job pushes a commit to `main` modifying `.github/workflows/ci.yml` to inject a secret-exfiltration step. Because CI runs on push to main, the modified workflow self-executes with full secret access.
- **Remediation:** Add to `ci.yml` top of file:
  ```yaml
  permissions:
    contents: read
  ```
  Then keep job-level `permissions:` only where elevation is required (already done for `security-scan`, `docker`, `deploy-*`). For `release.yml`, scope permissions per job:
  ```yaml
  # Remove top-level permissions
  jobs:
    goreleaser:
      permissions:
        contents: write  # tag/release creation
      # ...
    docker-release:
      permissions:
        packages: write
        contents: read
    helm-release:
      permissions:
        contents: write  # pushes to gh-pages
  ```

---

### SUPPLY-004: Docker base images pinned to `:latest` (ecosystem image lottery)
- **Severity:** Medium
- **Confidence:** 100/100
- **CWE:** CWE-1357 (Reliance on Insufficiently Trustworthy Component), CWE-494
- **File:**
  - `docker-compose.yml:80` (`prom/prometheus:latest`), `:102` (`grafana/grafana:latest`)
  - `docker-compose.prod.yml:130` (`prom/prometheus:latest`), `:159` (`prom/alertmanager:latest`), `:180` (`grafana/grafana:latest`), `:210` (`prom/node-exporter:latest`), `:235` (`gcr.io/cadvisor/cadvisor:latest`)
  - `deployments/docker/docker-compose.swarm.yml:195` (`prom/node-exporter:latest`), `:218` (`prom/prometheus:latest`), `:248` (`grafana/grafana:latest`)
  - `deployments/kubernetes/base/deployment.yaml:39` (`ghcr.io/apicerberus/apicerberus:latest`) and `statefulset.yaml:47`
  - `deployments/examples/kubernetes-deployment.yaml:121` (`apicerberus/apicerberus:latest`)
- **Description:** `:latest` tag is mutable. Every container restart/reschedule may pull a different image than the one that was tested, breaking reproducibility and exposing the cluster to upstream compromise (malicious tag retarget, rollback to a vulnerable prior version tagged latest). The own app `ghcr.io/apicerberus/apicerberus:latest` in the base k8s deployment is particularly dangerous: CI produces `sha-*`, semver, and branch tags (ci.yml:393-399) but the base manifest will still pull `:latest` in production if the overlay doesn't override the image tag. This also defeats the `imagePullPolicy: IfNotPresent` in `deployment.yaml:40` because pods on new nodes will pull the drifted `:latest`.
- **Exploit scenario:** Upstream Grafana image `latest` is retargeted during a CVE disclosure. Prometheus also rolls. Because node-exporter (prod) has extensive host mounts (`/proc`, `/sys`, `/` at `docker-compose.prod.yml:220-222`), a malicious image instantly obtains host read access.
- **Remediation:**
  - Pin by immutable digest: `prom/prometheus@sha256:<digest>` instead of `:latest`.
  - At minimum pin by specific semver: `prom/prometheus:v2.55.0`, `grafana/grafana:11.3.0`.
  - For the app's own image in `deployments/kubernetes/base/deployment.yaml` and `statefulset.yaml`, replace `:latest` with a placeholder like `IMAGE_PLACEHOLDER` that must be substituted by kustomize overlay or CI-generated manifest.
  - Run `docker pull <image> && docker inspect --format '{{index .RepoDigests 0}}' <image>` to capture digests.

---

### SUPPLY-005: Helm chart default `networkPolicy.enabled: false` — no default-deny posture
- **Severity:** Medium
- **Confidence:** 90/100
- **CWE:** CWE-306 (Missing Authentication for Critical Function), CWE-284 (Improper Access Control)
- **File:** `deployments/helm/apicerberus/values.yaml:213-217`
- **Description:** The Helm chart ships with `networkPolicy.enabled: false`. Even when enabled, the policy at `templates/networkpolicy.yaml:26-38` uses an empty-selector `- ports:` rule (no `from:` block) which matches traffic from **any pod in any namespace** for ports 8080 (gateway), admin (9876), portal (9877), and gRPC (50051). The Admin API (9876) becomes reachable from any compromised sidecar/workload in the cluster as long as the attacker knows the ClusterIP or Service DNS name. The `ci.yml:579` production deploy sets `--set networkPolicy.enabled=true`, but even enabled, the admin port has no namespace restriction. The standalone k8s base (`deployments/kubernetes/base/networkpolicy.yaml:25-35`) correctly restricts admin port to `monitoring` and `ingress-nginx` namespaces — the Helm chart is less strict than the kustomize base.
- **Exploit scenario:** An unrelated workload (CI runner, third-party operator) is compromised via a supply-chain vector. The attacker enumerates ClusterIP services, finds `apicerberus-admin:9876`, and, if the admin API key has leaked via any other channel (see SUPPLY-002), pivots to full cluster-administrative control of the gateway.
- **Remediation:**
  1. Default `networkPolicy.enabled: true` in values.yaml.
  2. In `templates/networkpolicy.yaml`, gate the admin port behind an explicit `from:` selector sourced from `.Values.networkPolicy.adminAllowedNamespaces`. Do **not** leave the admin port open to empty-from rules.
  3. Restrict egress similarly — current egress allows `0.0.0.0/0:80,443` which negates exfiltration constraints. Use `ipBlock` exclusions for RFC1918 at minimum, or require operator to set explicit `.Values.networkPolicy.egress.allowedCIDRs`.

---

### SUPPLY-006: Helm chart ServiceAccount missing `automountServiceAccountToken: false`
- **Severity:** Medium
- **Confidence:** 100/100
- **CWE:** CWE-250, CWE-732
- **File:** `deployments/helm/apicerberus/templates/serviceaccount.yaml:1-12`, `templates/deployment.yaml:34` (spec references SA but does not disable token mount)
- **Description:** The Helm ServiceAccount template has no `automountServiceAccountToken: false` directive, and the Deployment spec does not set it either. The pod therefore mounts the SA token at `/var/run/secrets/kubernetes.io/serviceaccount/token` even though APICerberus has no documented need to call the Kubernetes API. If an RCE / SSRF ever surfaces in the gateway, the attacker has immediate access to the Kubernetes API with whatever permissions the SA holds (default: list pods in namespace, but also enough to enumerate the cluster). The **kustomize** base (`deployments/kubernetes/base/serviceaccount.yaml:9`) correctly sets `automountServiceAccountToken: false` — the Helm chart lags behind.
- **Exploit scenario:** An RCE via a vulnerable WASM plugin or compromised upstream reads `/var/run/secrets/kubernetes.io/serviceaccount/token`, then calls the Kubernetes API to list Secrets in the namespace (default ServiceAccount has no Secret read — but if the operator followed the chart's `ServiceAccount → RoleBinding` guidance in any extension, this amplifies).
- **Remediation:**
  ```yaml
  # templates/serviceaccount.yaml
  apiVersion: v1
  kind: ServiceAccount
  automountServiceAccountToken: {{ .Values.serviceAccount.automountToken | default false }}
  # ...
  ```
  And at the Pod spec in `deployment.yaml`:
  ```yaml
  spec:
    automountServiceAccountToken: false
  ```
  Apply the same change to `backup-cronjob.yaml` (already recommended in DIFF-001).

---

### SUPPLY-007: PodDisruptionBudget `minAvailable: 2` but default `replicaCount: 1` — invariant violation at deploy-time
- **Severity:** Low
- **Confidence:** 95/100
- **CWE:** CWE-754 (Improper Check for Unusual or Exceptional Conditions)
- **File:** `deployments/helm/apicerberus/values.yaml:5 (replicaCount: 1)`, `:222 (podDisruptionBudget.minAvailable: 2)`
- **Description:** Default `replicaCount: 1` combined with `podDisruptionBudget.enabled: false, minAvailable: 2`: if an operator enables PDB without also scaling up (`--set podDisruptionBudget.enabled=true --set replicaCount=1`), the PDB is unsatisfiable from day one. Any voluntary disruption (node drain, cluster upgrade) is permanently blocked, leading operators to bypass the PDB with `--force`, which defeats the purpose. The ci.yml production deploy correctly sets `replicaCount=3`, but self-serve Helm installs will mis-deploy. This has a secondary effect of discouraging operators from using PDBs at all ("they never work").
- **Exploit scenario:** Not directly exploitable — operational/availability concern.
- **Remediation:** Bind PDB validity to replicaCount in chart `_helpers.tpl`:
  ```yaml
  # templates/pdb.yaml
  spec:
    {{- if and .Values.podDisruptionBudget.minAvailable (gt (int .Values.replicaCount) 1) }}
    minAvailable: {{ .Values.podDisruptionBudget.minAvailable }}
    {{- else }}
    maxUnavailable: 1
    {{- end }}
  ```
  Or add a NOTES.txt warning + `helm.sh/hook` validation.

---

### SUPPLY-008: `deployments/docker/Dockerfile` does not use distroless / multi-stage runtime hardening
- **Severity:** Low
- **Confidence:** 90/100
- **CWE:** CWE-1357, CWE-693 (Protection Mechanism Failure)
- **File:** `deployments/docker/Dockerfile:22-63`
- **Description:** The repo has two Dockerfiles. The root `Dockerfile` (reviewed earlier) uses `gcr.io/distroless/static:nonroot` — good. But `deployments/docker/Dockerfile:22` uses `alpine:3.21` as runtime, installs `ca-certificates curl`, adds a non-root user, and retains the full Alpine userland (`sh`, `wget`, `apk`, `busybox`). This increases the post-exploitation attack surface: any RCE lands in a shell with package-manager access. `curl` is installed only for the HEALTHCHECK (line 46) — the distroless image uses the binary's own `health` subcommand (root Dockerfile line 102), proving `curl` is unnecessary. The `build-docker.sh` script (line 164) builds from the **root** Dockerfile only, so `deployments/docker/Dockerfile` appears to be an orphaned/alternative build. If any ops runbook references it, it will produce a weaker image than CI does.
- **Exploit scenario:** Operator manually builds with `docker build -f deployments/docker/Dockerfile .` for a "troubleshooting" variant, deploys it, and RCE via any gateway bug now has shell + `apk add` + outbound `curl`, dramatically simplifying exfiltration.
- **Remediation:** Either delete `deployments/docker/Dockerfile` and point all references to the root `Dockerfile`, or rewrite it to use the distroless pattern. If an Alpine variant must exist (commented-out in root Dockerfile lines 116-138 is identical), keep only one copy to avoid drift. Add a note in `deployments/docker/README.md` stating which Dockerfile is canonical.

---

### SUPPLY-009: `actions/upload-artifact@v4` / coverage artifact has no explicit retention override for SARIF sensitive outputs
- **Severity:** Low
- **Confidence:** 70/100
- **CWE:** CWE-532
- **File:** `.github/workflows/ci.yml:107-113, 240-243, 309-313`
- **Description:** Coverage reports, benchmark results, and build binaries are uploaded as artifacts with `retention-days: 7` or `30`. These artifacts can contain:
  - `coverage.html` — reveals full source code paths and untested (attack-surface) branches.
  - `benchmark.txt` — reveals hot paths, potentially leaks internal routing patterns.
  The trivy/gosec SARIF files (lines 339, 351) are uploaded via `github/codeql-action/upload-sarif` — these go to the Security tab (public for open-source repos), which is normally the desired behavior, but should be double-checked for private repos where partial exposure through GitHub "Advanced Security" integrations is a concern. No severity here if the repo is public; potential exposure if private with PRs from forks.
- **Exploit scenario:** Fork PR author triggers CI, downloads `benchmark.txt` and `coverage.html` artifacts to map internal code paths for targeted vulnerability research.
- **Remediation:** For private repos, reduce `retention-days` to 1-3 for `coverage-report` and `benchmark-results`. Move SARIF output behind `if: github.event_name != 'pull_request'` if PR-from-fork is common.

---

### SUPPLY-010: NPM lockfile contains deprecated transitive `glob@10.5.0` and 5 install-script packages
- **Severity:** Low
- **Confidence:** 85/100
- **CWE:** CWE-1104 (Use of Unmaintained Third Party Components), CWE-506 (Embedded Malicious Code — postinstall vector)
- **File:** `web/package-lock.json:6843-6847` (deprecated glob); `:6665 (esbuild), :6792 (fsevents), :7580 (msw), :7853 (playwright/fsevents), :8575 (sharp)` — five packages with `"hasInstallScript": true`.
- **Description:**
  1. `glob@10.5.0` is flagged as deprecated by its maintainer with message *"Old versions of glob are not supported, and contain widely publicized security vulnerabilities"*. This is a transitive dep; direct consumers need to be upgraded to pull a non-deprecated glob (11.x+).
  2. Five packages execute arbitrary code via npm install scripts during `npm ci` in CI (`ci.yml:134, 192, 489`). This is normal for native-binary packages (`esbuild`, `fsevents`, `playwright`, `sharp`) but is an unavoidable supply-chain entrypoint: a compromised release of any of these executes arbitrary code on the GitHub Runner with access to the `GITHUB_TOKEN` and any secrets exposed to the `web-test`/`build`/`deploy-staging` jobs.
  3. `msw` (Mock Service Worker) runs its own install script; it is only used in dev/tests but `npm ci` runs all install scripts unless `--ignore-scripts` is set.
- **Exploit scenario:** Attacker publishes malicious `esbuild-0.27.8` via compromised maintainer account. Renovate/Dependabot auto-bumps; `npm ci` in `web-test` job executes the postinstall, reads `$GITHUB_TOKEN` from env, opens a PR on every repo the runner has access to, or exfiltrates secrets from the Actions runner.
- **Remediation:**
  - Run `npm ci --ignore-scripts` in CI where possible and invoke any required build-time codegen manually. For build jobs that genuinely need native binaries, isolate them to a dedicated job with minimal `permissions: contents: read` and no deploy secrets.
  - Set `"overrides": { "glob": "^11.0.0" }` in `web/package.json` to force non-deprecated glob transitively.
  - Enable GitHub Dependabot for `npm` with grouped security updates.

---

### SUPPLY-011: Helm `deploy-staging` passes JWT/Admin secrets via `--set` (same as SUPPLY-002 but pre-production)
- **Severity:** Low (duplicate class, lower impact than prod)
- **Confidence:** 90/100
- **CWE:** CWE-214, CWE-532
- **File:** `.github/workflows/ci.yml:491-492`
- **Description:** Same pattern as SUPPLY-002 applies to staging deploys, with `STAGING_JWT_SECRET` and `STAGING_ADMIN_API_KEY`. Lower severity because staging is lower-trust, but staging compromise often pivots to production (shared registries, shared Helm state, shared observability stack). Mentioned separately only because the remediation is slightly different (test keys vs live keys).
- **Remediation:** Same as SUPPLY-002. Use External Secrets / SealedSecrets; eliminate `--set secrets.*`.

---

### SUPPLY-012: Helm Chart has no `digest:` metadata and chart is not signed
- **Severity:** Low
- **Confidence:** 70/100
- **CWE:** CWE-347 (Improper Verification of Cryptographic Signature)
- **File:** `deployments/helm/apicerberus/Chart.yaml:1-21`; `release.yml:94-118`
- **Description:** The Helm release workflow packages and pushes the chart to gh-pages but never signs it with `helm package --sign`. Consumers who `helm repo add` have no integrity guarantee — they rely on HTTPS alone. For a Helm chart that defines production-critical RBAC/NetworkPolicy/Secret primitives, provenance files (`.prov`) should be published alongside. No key is configured in the Release workflow.
- **Exploit scenario:** DNS hijack of `charts.apicerberus.com` or gh-pages domain redirects consumers to a malicious chart with the same version. Without `.prov` verification (`helm verify`), nothing alerts.
- **Remediation:** Configure a GPG key in repository secrets (`HELM_SIGNING_KEY`, `HELM_SIGNING_KEY_PASSPHRASE`), export via `helm package --sign --key <id> --keyring <path>`, publish `.tgz.prov` to the chart repo, and document `helm install --verify` in README.

---

## Positive Findings

| # | Item | File |
|---|------|------|
| P1 | Root Dockerfile uses `gcr.io/distroless/static:nonroot` with non-root user, no shell/package-manager, HEALTHCHECK via app binary | `Dockerfile:69-111` |
| P2 | Multi-stage build with `CGO_ENABLED=0` static binary and build metadata injected via `-ldflags` | `Dockerfile:56-64` |
| P3 | `govulncheck` integrated into CI with pinned version | `ci.yml:356-359` |
| P4 | `trivy-action` + `gosec` SARIF uploads to GitHub Security tab | `ci.yml:330-354` |
| P5 | Helm chart secret uses `helm.sh/resource-policy: keep` and `lookup` to avoid regenerating existing secrets on upgrade | `templates/secret.yaml:7-22` |
| P6 | Kustomize base ServiceAccount disables token auto-mount | `deployments/kubernetes/base/serviceaccount.yaml:9` |
| P7 | Kustomize deployment sets `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop: [ALL]`, `runAsNonRoot: true`, fsGroup 65532 | `deployments/kubernetes/base/deployment.yaml:41-46, 32-36` |
| P8 | Production deploy gated by GitHub Environment `production` with documented review-rules requirement in comment | `ci.yml:513-527` |
| P9 | Web dashboard and Go deps audit (SECURITY-REPORT, dependency-audit.md) show no known unpatched CVEs in direct deps | `security-report/dependency-audit.md` |
| P10 | `docker-compose.yml:42-43` uses `${VAR:?error}` to require JWT_SECRET/ADMIN_API_KEY at compose-time instead of defaulting to `changeme` | `docker-compose.yml:42-43` |
| P11 | No `pull_request_target` + no `curl \| sh` patterns found in workflows or Makefile | (absence confirmed) |
| P12 | Swarm stack uses overlay networks with `encrypted: true` for raft-cluster and backend networks | `deployments/docker/docker-compose.swarm.yml:317-331` |

---

## Dependencies Requiring Attention

| Package | Version | Issue | Recommended Action |
|---------|---------|-------|--------------------|
| `glob` (transitive) | 10.5.0 | Deprecated by maintainer; unsupported, known vulnerabilities | Add `"overrides": { "glob": "^11" }` in web/package.json |
| GitHub Actions (all third-party) | tag-pinned | Mutable tag pinning (CVE-2025-30066 class) | Pin to full SHA + enable Dependabot actions group |
| `prom/prometheus` | `:latest` | Mutable tag | Pin `v2.55.0` or digest |
| `grafana/grafana` | `:latest` | Mutable tag | Pin `11.3.0` or digest |
| `prom/alertmanager` | `:latest` | Mutable tag | Pin specific version/digest |
| `prom/node-exporter` | `:latest` | Mutable tag; extensive host mounts in prod | Pin + review mount scope |
| `gcr.io/cadvisor/cadvisor` | `:latest` | Mutable tag; `/var/lib/docker:ro` mount | Pin; ensure read-only |
| `ghcr.io/apicerberus/apicerberus` | `:latest` (k8s base) | Own image unpinned in base manifest | Replace with placeholder; require overlay override |
| `redis:7-alpine` | floating minor | Redis 7.x has ongoing CVE stream | Pin `7.4.x` exact or switch to `redis:7.4.2-alpine` |
| `postgres:16-alpine` | floating minor | Postgres minor rollover | Pin `16.4-alpine` exact |
| Helm chart | v1.0.0 | Unsigned on release | Add `--sign` to `helm package` step |
| `esbuild`, `fsevents`, `msw`, `playwright`, `sharp` | various | `hasInstallScript: true` | Run `npm ci --ignore-scripts` where feasible; isolate native-build jobs |

---

## Summary

**12 findings** across Docker, Helm, Kubernetes, CI/CD, and NPM supply chain — **2 High, 4 Medium, 6 Low** — all distinct from the backup-cronjob issues in `DIFF-SECURITY-REPORT.md`. Go direct/indirect deps are clean (confirmed against baseline dependency-audit.md, no regressions). The two most urgent items are **SUPPLY-001** (pin GitHub Actions to SHA — active attack class demonstrated in tj-actions/changed-files incident) and **SUPPLY-002** (eliminate `--set secrets.*` in Helm deploys — secrets leak via process table, Helm state, and potential log-escape). **SUPPLY-003** (missing `permissions:`) amplifies the impact of both. Medium-severity **SUPPLY-004/005/006** are straightforward one-line hardening fixes that close entire categories of cluster-level pivoting. No evidence of typosquats, `pull_request_target` misuse, `curl | sh` patterns, or plaintext secrets committed to the repo. Go ecosystem remains on clean fixed versions.

*Audit generated: 2026-04-16*
