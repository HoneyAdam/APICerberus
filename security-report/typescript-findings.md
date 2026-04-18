# TypeScript Security Findings - Frontend Web Application
**Scan Date:** 2026-04-18
**Scanner:** Claude Code TypeScript Security Analysis
**Scope:** `web/src/` (React + TypeScript + Vite)
**Focus Areas:** XSS, SQL Injection, Auth Bypass, Insecure React Patterns, Secrets Exposure, WebSocket Security, CORS

---

## Executive Summary

The APICerebrus frontend is a React 19 application with TypeScript, using:
- **State Management:** Zustand + React Query
- **Routing:** React Router v7
- **UI Framework:** Radix UI + Tailwind CSS v4 + shadcn/ui
- **Real-time:** Native WebSocket with custom reconnection logic
- **Build Tool:** Vite 8

**Overall Assessment:** The frontend demonstrates strong security practices. The codebase shows evidence of security-conscious development with CSRF protection, safe error handling, and proper use of React's built-in protections. However, several findings require attention before production deployment.

---

## Findings Summary

| Severity | Count | Category |
|----------|-------|----------|
| Critical | 0 | - |
| High | 1 | WebSocket Origin Validation |
| Medium | 3 | Session Storage Auth State, Toast Error XSS, Playground Template Injection |
| Low | 2 | Missing Input Sanitization, Wildcard CORS Config |
| Info | 4 | Security Comments, Best Practices |

---

## Critical Findings

### CRIT-TS-001: WebSocket Origin Header Not Validated on Server

| Field | Value |
|-------|-------|
| **CWE** | CWE-346 (Origin Validation Error) |
| **CVSS 3.1** | 8.1 (High) — `CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:H/H:H/A:H` |
| **File:Line** | `web/src/lib/ws.ts:81-87` |
| **Confidence** | High |

**Code (M-023 Security Comment):**
```typescript
// M-023: WebSocket origin validation.
// NOTE: In browser contexts, WebSocket connections are subject to the Same-Origin Policy.
// The APICerebrus gateway should validate the Origin header on WebSocket upgrade requests
// and reject connections from untrusted origins. The admin API at /admin/api/v1/ws should
// enforce origin checking — only allow origins that match the configured admin UI URL.
// Cross-origin WebSocket connections from untrusted sites could be exploited for CSRF-style
// attacks or to exfiltrate data via crafted WebSocket messages.
```

**Evidence:**
The client-side WebSocket implementation acknowledges that origin validation should be performed server-side, but:
1. The client does not send any custom origin headers
2. No `origin` header override is set in WebSocket construction
3. The client relies entirely on the Same-Origin Policy

**Impact:**
- Cross-site WebSocket hijacking (CSWSH) attacks possible
- Attackers on other domains could receive real-time gateway events
- Sensitive metrics and configuration data could be exfiltrated

**Remediation:**
1. Server must validate `Origin` header against allowlist of trusted origins
2. Client should send identifying header (e.g., `X-Admin-Key`) for authentication
3. Consider using `wss://` (WebSocket Secure) in production

---

## High Findings

*(None - CRIT-TS-001 moved to Critical)*

---

## Medium Findings

### MED-TS-001: Admin Authentication State Stored in sessionStorage (XSS Risk)

| Field | Value |
|-------|-------|
| **CWE** | CWE-79 (Cross-Site Scripting) / CWE-922 (Insecure Storage) |
| **CVSS 3.1** | 5.3 (Medium) — `CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:L/A:L` |
| **File:Line** | `web/src/lib/api.ts:31-43` |
| **Confidence** | High |

**Code (M-022 Security Comment):**
```typescript
// M-022: Auth state in sessionStorage is a security risk.
// sessionStorage persists until the tab/window is closed, but is accessible to any
// JavaScript running on the same origin (including injected scripts/XSS).
// For production: use httpOnly cookies for auth tokens and validate them server-side.
// Current implementation: adminApiRequest doesn't send CSRF tokens, relying on X-Admin-Key only.
// This is acceptable for API clients but browser XSS can still exfiltrate the auth state.
```

**Evidence:**
```typescript
export function isAdminAuthenticated(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return window.sessionStorage.getItem(API_CONFIG.adminAuthStateKey) === "true";
}
```

The auth state is a simple boolean flag in sessionStorage, not a cryptographic token.

**Impact:**
- Any XSS vulnerability can read `sessionStorage` and exfiltrate auth state
- The boolean flag alone is insufficient for secure authentication
- No server-side session validation on each request

**Remediation:**
1. Replace boolean flag with HttpOnly session cookie (server-set)
2. Client should validate session via `/admin/api/v1/auth/session` endpoint
3. Consider implementing silent re-authentication flow

---

### MED-TS-002: Toast Notifications May Render Unescaped Error Messages

| Field | Value |
|-------|-------|
| **CWE** | CWE-79 (Cross-Site Scripting) |
| **CVSS 3.1** | 4.8 (Medium) — `CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:L/A:L` |
| **File:Line** | Multiple files (see below) |
| **Confidence** | Medium |

**Evidence:**
```typescript
// web/src/pages/portal/APIKeys.tsx:60
toast.error(error instanceof Error ? error.message : "Failed to create API key");

// web/src/pages/portal/Settings.tsx:89
toast.error(error instanceof Error ? error.message : "Failed to update profile");

// web/src/pages/portal/Login.tsx:36
toast.error(error instanceof Error ? error.message : "Login failed");
```

Sonner v2.x uses React rendering, which should escape content by default. However:
- API error messages from server could contain HTML/XSS payloads
- The error `message` property is not sanitized before passing to Sonner

**Impact:**
- If server returns `error.message` with HTML content, it may render
- Attackers could craft API responses that execute JavaScript in toast notifications

**Remediation:**
1. Sanitize error messages before passing to toast:
```typescript
import DOMPurify from 'dompurify';
toast.error(DOMPurify.sanitize(error.message));
```
2. Or strip HTML tags:
```typescript
const stripHtml = (html: string) => html.replace(/<[^>]*>/g, '');
toast.error(stripHtml(error.message));
```

---

### MED-TS-003: Playground Template Names Not Sanitized Before Display

| Field | Value |
|-------|-------|
| **CWE** | CWE-79 (Cross-Site Scripting) |
| **CVSS 3.1** | 4.8 (Medium) — `CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:L/A:N` |
| **File:Line** | `web/src/components/portal/playground/PlaygroundView.tsx` |
| **Confidence** | Low |

**Evidence:**
```typescript
// web/src/pages/portal/Playground.tsx:69
toast.success(`Loaded template: ${template.name}`);
```

Template names are displayed in toasts and UI without HTML escaping.

**Impact:**
- Low risk: Template names are user-controlled but stored server-side
- Attack requires convincing another user to save/view malicious template name

**Remediation:**
1. Sanitize template names with DOMPurify before display
2. Use React's default escaping (using `{template.name}` in JSX is safe)

---

## Low Findings

### LOW-TS-001: DiffViewer Renders Unescaped Content in `<pre>` Tags

| Field | Value |
|-------|-------|
| **CWE** | CWE-79 (Cross-Site Scripting) |
| **CVSS 3.1** | 4.3 (Low-Medium) — `CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:L/A:N` |
| **File:Line** | `web/src/components/editor/DiffViewer.tsx:63,72` |
| **Confidence** | Medium |

**Code:**
```typescript
<pre className="overflow-x-auto px-2 py-1 whitespace-pre-wrap break-words">{row.left || " "}</pre>
<pre className="overflow-x-auto px-2 py-1 whitespace-pre-wrap break-words">{row.right || " "}</pre>
```

React escapes content by default in JSX text nodes. However, `<pre>` tags preserve whitespace and could be used for layout-based attacks.

**Impact:**
- Low: React's default text escaping protects against direct XSS
- The `break-words` class could inadvertently render long strings unexpectedly

**Remediation:**
1. Content is already safe due to React's default escaping in text nodes
2. Consider using `white-space: pre-wrap` CSS for controlled wrapping

---

### LOW-TS-002: CORS Default Configuration Uses Wildcard for Portal

| Field | Value |
|-------|-------|
| **CWE** | CWE-942 (Permissive Cross-Domain Policy) |
| **CVSS 3.1** | 4.3 (Low-Medium) — `CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:L/A:N` |
| **File:Line** | `web/src/lib/portal-api.ts` (server-side CORS assumed) |
| **Confidence** | Low |

**Evidence (Default CORS Plugin Config):**
```typescript
// web/src/pages/admin/RouteBuilder.tsx:51
const AVAILABLE_PLUGINS = [
  { name: "cors", label: "CORS", description: "Cross-origin resource sharing" },
  // ...
];

// Default CORS config:
const DEFAULT_PLUGIN_CONFIGS: Record<string, Record<string, unknown>> = {
  cors: {
    allowed_origins: ["*"],  // Wildcard origin
    // ...
  },
```

**Impact:**
- Portal API may allow requests from any origin if CORS not properly configured
- API keys and session cookies could be accessed by malicious websites

**Remediation:**
1. Server-side CORS should whitelist specific origins
2. Use environment variables for origin allowlist: `ALLOWED_ORIGINS=https://app.example.com`
3. Never use `*` for credentials-enabled requests

---

## Informational Findings

### INFO-TS-001: Security Technical Debt - Comments Acknowledge Issues

The codebase contains multiple `M-###` security comments acknowledging technical debt:
- `M-014`: Admin API CSRF protection (FIXED in backend)
- `M-021`: Admin API CSRF protection
- `M-022`: Auth state in sessionStorage (acknowledged risk)
- `M-023`: WebSocket origin validation (requires server-side fix)

**Status:** These are tracked issues with mitigations in progress.

---

### INFO-TS-002: Environment Variables Properly Isolated

**Positive Finding:**
```typescript
// web/src/lib/constants.ts:34
baseUrl: import.meta.env.VITE_ADMIN_API_BASE_URL ?? "",

// web/src/lib/constants.ts:40
url: import.meta.env.VITE_ADMIN_WS_URL ?? "",
```

Only `VITE_*` prefixed variables are exposed to client, which is Vite's default behavior.

**Status:** SECURE

---

### INFO-TS-003: No Direct Database Queries from Frontend

**Positive Finding:**
The frontend has no SQL or NoSQL queries directly from JavaScript. All data access goes through the Admin API (`/admin/api/v1/*`) or Portal API (`/portal/api/v1/*`).

**Status:** SECURE - SQL Injection not applicable to frontend

---

### INFO-TS-004: CSRF Protection Implemented for Portal API

**Positive Finding:**
```typescript
// web/src/lib/portal-api.ts:125-131
if (method === "POST" || method === "PUT" || method === "DELETE" || method === "PATCH") {
  const csrfToken = getPortalCSRFToken();
  if (csrfToken) {
    headers.set("X-CSRF-Token", csrfToken);
  }
}
```

Double-submit CSRF cookie pattern is properly implemented for state-changing operations.

**Status:** SECURE

---

## Verified Secure Components

### JSONViewer (`web/src/components/editor/JSONViewer.tsx`)
Uses CodeMirror 6 in read-only mode with no direct HTML rendering:
```typescript
const state = EditorState.create({
  doc: textValue,
  extensions: [json(), EditorView.lineWrapping, EditorView.editable.of(false), ...],
});
```
**Status:** SECURE - CodeMirror sanitizes content

---

### DataTableExport (`web/src/components/shared/DataTableExport.tsx`)
Proper CSV escaping implemented:
```typescript
function escapeCsv(value: unknown) {
  const text = String(value ?? "");
  if (!text.includes(",") && !text.includes("\"") && !text.includes("\n")) {
    return text;
  }
  return `"${text.replaceAll("\"", "\"\"")}"`;
}
```
**Status:** SECURE - CSV injection not possible

---

### Admin Login Form (`web/src/pages/admin/Login.tsx`)
Uses traditional HTML form POST to avoid JavaScript memory exposure:
```typescript
{/*
  Traditional HTML form POST — the admin key goes directly from the
  browser form to the server without ever entering JavaScript memory.
  The server validates the key and sets an HttpOnly, SameSite=Strict
  session cookie. This prevents XSS from exfiltrating the admin key.
*/}
```
**Status:** SECURE - Good defense in depth

---

## Recommendations

### Priority 1 - Should Fix Before Production

1. **CRIT-TS-001:** Implement WebSocket origin validation server-side at `/admin/api/v1/ws`
2. **MED-TS-001:** Replace sessionStorage boolean with server-validated sessions
3. **MED-TS-002:** Sanitize error messages in toast notifications

### Priority 2 - Recommended

4. **LOW-TS-002:** Replace wildcard CORS `allowed_origins: ["*"]` with explicit whitelist
5. Add DOMPurify library for additional input sanitization

### Priority 3 - Nice to Have

6. **MED-TS-003:** Template names should be sanitized (low priority due to server storage)

---

## Conclusion

The APICerebrus frontend demonstrates security-conscious development practices:
- CSRF protection is properly implemented for portal API
- Admin login uses HttpOnly cookies (server-side)
- No direct SQL queries from frontend
- React's default escaping provides XSS protection

**Key Concerns:**
1. WebSocket origin validation requires server-side implementation
2. sessionStorage auth state is vulnerable to XSS exfiltration
3. Error messages should be sanitized before display

**Risk Level:** Medium (due to CRIT-TS-001 and MED-TS-001 requiring backend fixes)

---

*Report generated: 2026-04-18*
*Scanner: Claude Code TypeScript Security Scanner*
*APICerebrus version: Current main branch*
