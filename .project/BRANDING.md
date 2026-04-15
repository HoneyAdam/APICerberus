# API Cerberus — Branding Guide

## BRANDING.md

> Visual identity, messaging, and content creation guide for API Cerberus.

---

## 1. Brand Identity

### 1.1 Name & Variations

| Context | Usage |
|---------|-------|
| Full name | **API Cerberus** |
| Short name | **Cerberus** (only in context where "API" is already established) |
| CLI binary | `apicerberus` |
| Package/module | `APICerberus` |
| GitHub org | `APICerberus` |
| Domain | `apicerberus.com` |
| Hashtag | `#APICerberus` |
| Environment prefix | `APICERBERUS_` |
| MCP tools prefix | `apicerberus_` |

### 1.2 Taglines

| Type | Text |
|------|------|
| Primary | Three-headed guardian for your APIs — Minimal dependencies, single binary, full control. |
| Short | Guard your APIs. Minimal deps. One binary. |
| Technical | Full-stack API Gateway & Management Platform in pure Go. |
| Mythological | Three heads. One binary. No API passes unguarded. |
| Commercial | API Gateway + User Management + Credit Billing — all in one binary. |
| Turkish | Three-headed guardian for your APIs — Minimal dependencies, single binary, full control. |

### 1.3 Brand Story

Cerberus — In Greek mythology, the three-headed dog guarding the gates of the underworld. No soul passes uncontrolled.

API Cerberus's three heads:
- **HTTP/HTTPS** — Web API traffic
- **gRPC** — Microservice communication
- **GraphQL** — Federated data queries

No API request passes uncontrolled. Auth, rate limiting, credit billing, audit logging — all in a single binary, with minimal, curated external dependencies.

Against Kong's 200+ dependencies: 22 curated Go modules. Against Tyk's Redis requirement: embedded SQLite. Against KrakenD's missing UI: full Admin Panel + User Portal.

---

## 2. Color Palette

### 2.1 Primary Colors

```
┌──────────────────────────────────────────────────┐
│  API CERBERUS COLOR PALETTE                      │
├──────────────────────────────────────────────────┤
│                                                  │
│  PRIMARY                                         │
│  ██████  Deep Purple     #6B21A8  rgb(107,33,168)│
│  ██████  Purple          #7C3AED  rgb(124,58,237)│
│  ██████  Purple Light    #A855F7  rgb(168,85,247)│
│                                                  │
│  ACCENT                                          │
│  ██████  Crimson         #DC2626  rgb(220,38,38) │
│  ██████  Crimson Light   #EF4444  rgb(239,68,68) │
│                                                  │
│  STATUS                                          │
│  ██████  Emerald         #059669  rgb(5,150,105) │
│  ██████  Amber           #D97706  rgb(217,119,6) │
│  ██████  Sky             #0284C7  rgb(2,132,199) │
│                                                  │
│  NEUTRAL                                         │
│  ██████  Slate 950       #0F172A  rgb(15,23,42)  │
│  ██████  Slate 900       #1E293B  rgb(30,41,59)  │
│  ██████  Slate 800       #334155  rgb(51,65,85)  │
│  ██████  Slate 700       #475569  rgb(71,85,105) │
│  ██████  Slate 400       #94A3B8  rgb(148,163,184│
│  ██████  Slate 200       #E2E8F0  rgb(226,232,240│
│  ██████  Slate 50        #F8FAFC  rgb(248,250,252│
│  ██████  White           #FFFFFF  rgb(255,255,255│
│                                                  │
└──────────────────────────────────────────────────┘
```

### 2.2 Usage Rules

| Color | Usage |
|-------|-------|
| Deep Purple #6B21A8 | Primary brand color, logo, buttons, links, active states |
| Purple #7C3AED | Hover states, secondary accents, gradient midpoint |
| Crimson #DC2626 | Error states, destructive actions, "killer" emphasis |
| Emerald #059669 | Success, healthy status, positive metrics |
| Amber #D97706 | Warning, degraded status, pending states |
| Sky #0284C7 | Info, links, secondary actions |
| Slate 950 #0F172A | Dark mode background, primary text (light mode) |
| White #FFFFFF | Light mode background, primary text (dark mode) |

### 2.3 Gradients

```
Primary Gradient:     #6B21A8 → #7C3AED → #A855F7  (left to right)
Hero Gradient:        #0F172A → #1E1338 → #2D1B69  (dark bg with purple tint)
Danger Gradient:      #DC2626 → #B91C1C             (crimson depth)
Card Glow (dark):     box-shadow: 0 0 40px rgba(124, 58, 237, 0.15)
```

---

## 3. Typography

### 3.1 Fonts

| Usage | Font | Weight | Fallback |
|-------|------|--------|----------|
| Headings | Inter Variable | 600 (Semibold), 700 (Bold) | system-ui, sans-serif |
| Body | Inter Variable | 400 (Regular), 500 (Medium) | system-ui, sans-serif |
| Code / Mono | JetBrains Mono Variable | 400, 500 | Fira Code, monospace |
| Logo wordmark | Inter Variable | 700 (Bold) | — |

### 3.2 Type Scale (Dashboard)

```
text-xs:   12px / 16px  — Badges, captions, timestamps
text-sm:   14px / 20px  — Table cells, secondary text, form labels
text-base: 16px / 24px  — Body text, descriptions
text-lg:   18px / 28px  — Subheadings, card titles
text-xl:   20px / 28px  — Section headings
text-2xl:  24px / 32px  — Page titles
text-3xl:  30px / 36px  — KPI values, hero numbers
text-4xl:  36px / 40px  — Landing page hero
```

---

## 4. Logo

### 4.1 Logo Concept

The API Cerberus logo is a modern, geometric three-headed dog silhouette formed from network/data flow lines. The three heads represent the three protocol pillars: HTTP, gRPC, GraphQL.

**Design Elements:**
- Three dog head silhouettes arranged in a trident/triangular formation
- Heads connected by circuit-like lines (representing data flow)
- Geometric, angular style — not cartoonish
- Negative space forming a shield or gateway shape
- Minimal detail — works at 16px favicon and 512px hero sizes

**Logo Variants:**
- **Icon only**: Three-headed silhouette (square, for favicon, app icon)
- **Horizontal**: Icon + "API Cerberus" wordmark (for header, navbar)
- **Stacked**: Icon above "API Cerberus" wordmark (for loading screen, about page)
- **Monochrome**: Single color version (for dark/light backgrounds)

### 4.2 Logo Colors

| Variant | Background | Icon | Wordmark |
|---------|-----------|------|----------|
| On dark | Transparent / #0F172A | #A855F7 (Purple Light) | #FFFFFF |
| On light | Transparent / #FFFFFF | #6B21A8 (Deep Purple) | #0F172A |
| Monochrome dark | Transparent | #FFFFFF | #FFFFFF |
| Monochrome light | Transparent | #0F172A | #0F172A |

### 4.3 Logo Prompt (Nano Banana 2 / AI Image Gen)

```
LOGO PROMPT — Icon Only (1:1):
A minimal geometric logo icon of a three-headed dog (Cerberus) formed from
circuit board traces and data flow lines. Modern tech aesthetic, angular
geometric shapes, deep purple (#6B21A8) and light purple (#A855F7) on
pure black (#0F172A) background. Three dog head silhouettes arranged in
triangular formation, connected by glowing network lines. Clean vector
style, no text, suitable for app icon and favicon. Minimalist, premium,
tech brand feel.

LOGO PROMPT — Full Logo (16:9):
Modern tech brand logo "API Cerberus" featuring a geometric three-headed
dog icon on the left, with "API Cerberus" wordmark in clean bold sans-serif
font on the right. Icon made of circuit traces and network flow lines forming
three angular dog head silhouettes. Color scheme: deep purple icon, white
text, on dark slate (#0F172A) background. Premium SaaS brand aesthetic,
clean and professional. Subtle purple glow effect around the icon.
```

### 4.4 Favicon

- 16×16, 32×32, 48×48: Simplified single or triple head outline in purple
- 192×192, 512×512: Full icon with detail
- Format: SVG preferred, PNG fallback
- Apple Touch Icon: 180×180 with purple background, white icon

---

## 5. Social Media & Marketing Assets

### 5.1 Asset Dimensions

| Platform | Format | Size |
|----------|--------|------|
| GitHub README hero | 16:9 | 1280×720 |
| GitHub social preview | og:image | 1280×640 |
| X (Twitter) header | 3:1 | 1500×500 |
| X post image | 16:9 | 1200×675 |
| X post square | 1:1 | 1080×1080 |
| LinkedIn post | 1.91:1 | 1200×628 |
| Blog header | 16:9 | 1200×675 |
| Favicon | 1:1 | 32×32 / 512×512 |

### 5.2 Nano Banana 2 — Project Infographic Prompts

#### 16:9 Feature Overview Infographic

```
PROMPT (16:9 — 1200×675):
A modern dark-themed tech infographic for "API Cerberus" — an open-source
API Gateway & Management Platform. Dark slate (#0F172A) background with
subtle purple gradient glow.

CENTER: A stylized three-headed dog guardian (geometric, minimal, purple
glowing) standing at a gateway/portal made of circuit lines.

LEFT COLUMN (3 items with icons):
🔐 Auth & Rate Limiting (lock icon)
💳 Credit-Based Billing (coin icon)
📊 Analytics Dashboard (chart icon)

RIGHT COLUMN (3 items with icons):
⚡ Minimal Dependencies (lightning icon)
🔄 10 Load Balancers (arrows icon)
🤖 MCP Server (robot icon)

BOTTOM: "API Cerberus — Three-headed guardian for your APIs" in clean
sans-serif. "github.com/APICerberus/APICerberus" in smaller text.

Style: Editorial illustration meets tech diagram. Deep purple (#6B21A8),
light purple (#A855F7), crimson (#DC2626) accents on dark background.
Clean, premium, no clutter.
```

#### 1:1 Square Social Post

```
PROMPT (1:1 — 1080×1080):
Modern dark tech infographic square format for "API Cerberus". Dark
background (#0F172A) with purple glow.

TOP: "API Cerberus" logo text in bold white with purple accent line below.

CENTER: Isometric or 3D-style illustration of a gateway/portal with three
streams of data flowing through it (HTTP in blue, gRPC in green, GraphQL
in purple). A geometric Cerberus guardian silhouette watches over the streams.

BOTTOM GRID (2×3):
"Minimal Deps" | "Single Binary"
"Credit System" | "User Portal"
"Audit Logs" | "10 LB Algos"

Each item in a small rounded card with subtle border glow.

Footer: "apicerberus.com" | "MIT License" | "Pure Go"

Style: Cinematic tech poster, premium SaaS aesthetic.
```

#### GitHub README Hero Banner

```
PROMPT (16:9 — 1280×720):
Wide banner for GitHub README of "API Cerberus" project. Dark gradient
background (#0F172A → #1E1338).

LEFT SIDE: Large geometric three-headed dog icon, glowing purple lines,
circuit-board aesthetic. Slightly tilted for dynamism.

RIGHT SIDE: Clean text layout:
Line 1: "API Cerberus" (large, bold, white)
Line 2: "API Gateway & Management Platform" (medium, purple #A855F7)
Line 3: "Minimal deps • Single binary • Full control" (small, slate #94A3B8)

BOTTOM: Subtle horizontal line of floating tech badges/icons representing
features: lock, chart, globe, credit card, terminal, plug — in muted
purple outlines.

Style: Premium open-source project banner. Clean, minimal, professional.
No cartoonish elements. Dark theme that looks great on GitHub dark mode.
```

#### Architecture Diagram (Cinematic)

```
PROMPT (16:9 — 1200×675):
Cinematic tech architecture diagram for "API Cerberus" API Gateway.
Dark background (#0F172A).

FLOW (left to right):
LEFT: Multiple client icons (mobile, browser, server) sending requests
→ CENTER: A large glowing purple gateway portal with Cerberus guardian
silhouette. Inside the portal, visible layers:
  - Auth (lock icon)
  - Rate Limit (gauge icon)
  - Transform (arrows icon)
  - Credit Check (coin icon)
→ RIGHT: Multiple upstream API servers (cloud icons) receiving requests

BELOW THE FLOW: Two panels:
- "Admin Panel" (dashboard UI mockup silhouette)
- "User Portal" (user dashboard mockup silhouette)

COLOR: Purple gateway glow, blue client arrows, green upstream arrows,
red for blocked requests bouncing off the gateway.

Style: Isometric tech diagram meets editorial illustration. Professional,
not childish. Shows the architecture story at a glance.
```

#### Comparison Post (Kong Killer)

```
PROMPT (1:1 — 1080×1080):
Dark comparison infographic "API Cerberus vs The Rest".
Dark background (#0F172A).

TOP: "API Cerberus vs Kong vs Tyk vs KrakenD" in bold white.

CENTER: Four columns comparison table with visual indicators:
Column 1 (API Cerberus — highlighted purple glow):
✅ 22 Curated Dependencies
✅ Built-in UI
✅ Credit System
✅ User Portal
✅ MCP Server

Column 2 (Kong — dimmed):
❌ 200+ deps
❌ Enterprise only UI
❌ No billing
❌ No portal
❌ No MCP

Column 3 (Tyk — dimmed):
❌ 50+ deps / Redis
✅ Dashboard
❌ No billing
❌ Enterprise portal
❌ No MCP

Column 4 (KrakenD — dimmed):
❌ 20+ deps
❌ No UI
❌ No billing
❌ No portal
❌ No MCP

BOTTOM: "One binary to rule them all." + API Cerberus logo.

Style: Clean dark data comparison, purple highlight on API Cerberus column.
Factual, not aggressive. Let the data speak.
```

### 5.3 Nano Banana 2 — Article Header Prompts

#### "What is API Cerberus?" Article

```
PROMPT (16:9 — 1200×675):
Editorial illustration header for a tech article about API Cerberus.
A majestic three-headed dog made of purple glowing circuit lines and
data streams, standing guard at a massive digital gateway. The gateway
is formed by two towering server rack pillars with streams of API requests
(small glowing data packets) flowing through. Some packets are blocked
(red), most pass through (green/blue). Dark atmospheric background with
subtle purple fog/mist. Style: Cinematic editorial illustration, moody
lighting, tech noir aesthetic. NOT cartoonish — think Blade Runner meets
network diagram.
```

#### "Zero to API Gateway" Tutorial

```
PROMPT (16:9 — 1200×675):
Tech tutorial header illustration. A developer's desk (dark themed) with
a glowing terminal showing "apicerberus start" command. From the terminal,
purple light rays expand outward forming a network of connected API
endpoints. Small icons float around: lock (auth), speedometer (rate limit),
chart (analytics), credit card (billing). Above the desk, a subtle
Cerberus guardian silhouette watches over the scene.
Style: Atmospheric workspace illustration, cinematic lighting, developer
culture aesthetic. Dark purple and blue tones.
```

---

## 6. README.md Badges & Shields

```markdown
<!-- Badges for README.md -->
![Version](https://img.shields.io/badge/version-v1.0.0-purple?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)
![Minimal Deps](https://img.shields.io/badge/dependencies-22-brightgreen?style=flat-square)
![Single Binary](https://img.shields.io/badge/deploy-single%20binary-purple?style=flat-square)
![React](https://img.shields.io/badge/UI-React%2019-61DAFB?style=flat-square&logo=react)
![Tailwind](https://img.shields.io/badge/CSS-Tailwind%204.1-06B6D4?style=flat-square&logo=tailwindcss)
![MCP](https://img.shields.io/badge/MCP-enabled-purple?style=flat-square)
```

---

## 7. X (Twitter) Post Templates

### 7.1 Launch Post

```
🔱 API Cerberus v1.0.0 is live!

Against Kong's 200+ dependencies: 22 curated Go modules.
Against Tyk's Redis: embedded SQLite.
Against KrakenD's missing UI: full dashboard.

Three-headed API Gateway:
⚡ HTTP + gRPC + GraphQL
🔐 Auth + Rate Limiting + Credits
📊 Admin Panel + User Portal
🤖 MCP Server (Claude Code ready)

Single binary. Minimal dependencies. Pure Go.

github.com/APICerberus/APICerberus

#APICerberus #OpenSource #APIGateway #GoLang
```

### 7.2 Feature Highlight — Credit System

```
You want to sell your APIs but don't want to write a billing system.

Everything is built-in in API Cerberus:

💳 Credit-based API access
📊 Per-route pricing
🧪 Test key (ck_test_) — doesn't consume credits
⚠️ Automatic 402 when balance depleted
📈 Revenue dashboard

No Stripe needed for MVP.

github.com/APICerberus/APICerberus

How would you make an API paid?

#APICerberus #APIMonetization
```

### 7.3 Feature Highlight — User Portal

```
Kong Enterprise sells Dev Portal for $35K/year:

Free and built-in in API Cerberus:

🔑 Generate/manage API keys
🧪 API Playground (like Postman)
📊 Usage analytics
📋 Request logs (full req/res)
💳 Credit balance tracking
🔒 IP whitelist management

Inside a single binary, zero extra setup.

This is neither Postman nor Swagger UI — above both.

#APICerberus #DevPortal #APIManagement
```

### 7.4 Feature Highlight — Minimal Dependencies

```
How many lines in your go.mod?

Kong: 200+ dependency
Tyk: 50+ dependency
KrakenD: 20+ dependency
Traefik: 30+ dependency
API Cerberus: 0

Zero. Only Go standard library.

YAML parser? We wrote our own.
JWT validator? crypto/rsa + crypto/hmac.
SQLite? Bundled amalgamation.
Web UI? embed.FS.

When you want to escape dependency hell:

github.com/APICerberus/APICerberus

#APICerberus #MinimalDeps #NOFORKANYMORE
```

### 7.5 Architecture Post

```
API Cerberus architecture 👇

Request arrives →
1. 🔐 Auth (API Key / JWT)
2. 🚦 Rate Limit (4 algorithms)
3. ✅ Permission check
4. 💳 Credit check
5. 🔄 Request transform
6. ⚡ Proxy to upstream (10 LB algorithms)
7. 🔄 Response transform
8. 📝 Audit log (full req/res)
9. 📊 Analytics metrics
10. 💰 Credit deduction

All this pipeline in a single binary, <1ms overhead.

50K+ req/sec throughput.

Where else do you find this?

#APICerberus #APIGateway
```

### 7.6 MCP Post

```
"Claude, add a new route to API Cerberus and set rate limit"

Claude Code directly from terminal:

apicerberus mcp start

→ claude: apicerberus_create_route
→ claude: apicerberus_enable_plugin rate-limit
→ claude: apicerberus_create_user
→ claude: apicerberus_topup_credits

API Gateway management is now in natural language.

MCP tools: 40+
Resources: 13

This isn't just an API Gateway, it's AI-native infrastructure.

#APICerberus #MCP #ClaudeCode #AI
```

### 7.7 Thread Starter — "Why Another API Gateway?"

```
🧵 Why would the world want another API Gateway?

1/ All existing solutions have the same problem:
Either 200+ dependencies (Kong)
Or Redis required (Tyk)
Or no UI (KrakenD)
Or all of the above.

2/ Want API monetization? Buy Enterprise plan.
Want Dev portal? Buy Enterprise plan.
Want audit logs? Write a plugin.

3/ API Cerberus is different:
- 22 dependencies
- Built-in billing (credit system)
- Built-in dev portal + playground
- Built-in audit logging
- Single binary
- MIT license
- Free

4/ Plus:
- 10 load balancing algorithms
- 4 rate limiting algorithms
- 25+ built-in plugins
- Raft clustering
- MCP server (AI-native)
- React 19 + shadcn/ui dashboard

5/ "But is it production-ready?"

50K+ req/sec, <1ms overhead, Raft HA.

Does everything Kong can do.
Does things Kong can't do.
And it's free.

github.com/APICerberus/APICerberus

#APICerberus #OpenSource
```

---

## 8. Documentation Site Design

### 8.1 docs.apicerberus.com

```
Color scheme: Same as dashboard (purple primary, dark background)
Font: Inter Variable + JetBrains Mono Variable
Framework: VitePress or Starlight (Astro)

Sections:
├── Getting Started
│   ├── Installation
│   ├── Quick Start (5 min tutorial)
│   ├── Configuration Guide
│   └── Docker Deployment
├── Core Concepts
│   ├── Services, Routes, Upstreams
│   ├── Plugin Pipeline
│   ├── Authentication
│   ├── Rate Limiting
│   └── Load Balancing
├── User Management
│   ├── Users & Roles
│   ├── API Keys
│   ├── Permissions
│   └── IP Whitelist
├── Billing
│   ├── Credit System
│   ├── Route Pricing
│   ├── Self-Purchase
│   └── Revenue Reports
├── Audit & Analytics
│   ├── Audit Logging
│   ├── Analytics Dashboard
│   ├── Log Retention
│   └── Export & Archive
├── Protocols
│   ├── HTTP/HTTPS
│   ├── gRPC
│   ├── GraphQL
│   └── WebSocket
├── Clustering
│   ├── Raft Consensus
│   ├── Multi-Node Setup
│   └── Distributed State
├── Integrations
│   ├── MCP Server
│   ├── Prometheus
│   ├── OpenTelemetry
│   └── Webhooks
├── API Reference
│   ├── Admin API
│   ├── Portal API
│   └── CLI Reference
├── Migration Guides
│   ├── From Kong
│   ├── From Tyk
│   └── From KrakenD
└── Contributing
    ├── Development Setup
    ├── Architecture Overview
    └── Plugin Development
```

---

## 9. GitHub Repository Structure

### 9.1 README.md Template

```markdown
<div align="center">

<!-- Hero banner image here -->

# API Cerberus

### Three-headed guardian for your APIs

**Minimal dependencies • Single binary • Full control**

[![Version](badge)](#) [![Go](badge)](#) [![License](badge)](#) [![Minimal Deps](badge)](#)

[Documentation](https://docs.apicerberus.com) •
[Quick Start](#quick-start) •
[Download](https://github.com/APICerberus/APICerberus/releases) •
[Discord](#)

</div>

---

## What is API Cerberus?

API Cerberus is a full-stack **API Gateway, API Management, and API
Monetization Platform** written in pure Go with minimal, curated external dependencies.

One binary gives you:

- 🔐 **Authentication** — API Key + JWT (RS256/HS256)
- 🚦 **Rate Limiting** — 4 algorithms (Token Bucket, Sliding Window, Fixed Window, Leaky Bucket)
- ⚡ **Load Balancing** — 10 algorithms (Round Robin to Geo-aware)
- 💳 **Credit System** — Sell API access with per-route pricing
- 👥 **Multi-Tenant** — User management with per-endpoint permissions
- 📊 **Analytics** — Real-time dashboard with traffic, latency, errors
- 📝 **Audit Logging** — Full request/response capture with masking
- 🧪 **API Playground** — Built-in API tester for your users
- 🔌 **25+ Plugins** — Transform, cache, CORS, compression, and more
- 🤖 **MCP Server** — AI-native management via Claude Code
- 🏗️ **Raft Clustering** — Built-in high availability
- 🌐 **3 Protocols** — HTTP/HTTPS, gRPC, GraphQL federation

<!-- Screenshot of dashboard here -->

## Quick Start

...
```

### 9.2 Repository Files

```
.github/
├── ISSUE_TEMPLATE/
│   ├── bug_report.md
│   ├── feature_request.md
│   └── config.yml
├── PULL_REQUEST_TEMPLATE.md
├── workflows/
│   ├── ci.yml              # Build + test on push
│   ├── release.yml          # Build binaries on tag
│   └── docker.yml           # Build + push Docker image
└── FUNDING.yml              # GitHub Sponsors

assets/
├── logo/
│   ├── icon-dark.svg
│   ├── icon-light.svg
│   ├── logo-horizontal-dark.svg
│   ├── logo-horizontal-light.svg
│   ├── logo-stacked-dark.svg
│   ├── logo-stacked-light.svg
│   ├── favicon.ico
│   ├── favicon-32x32.png
│   ├── apple-touch-icon.png
│   └── og-image.png          # 1280×640 social preview
├── screenshots/
│   ├── dashboard.png
│   ├── portal.png
│   ├── playground.png
│   ├── analytics.png
│   ├── audit-logs.png
│   ├── cluster-topology.png
│   └── dark-mode.png
└── infographics/
    ├── architecture-16x9.png
    ├── features-16x9.png
    ├── comparison-1x1.png
    └── hero-banner.png

CONTRIBUTING.md
CODE_OF_CONDUCT.md
SECURITY.md
CHANGELOG.md
```

---

## 10. Presentation Slide Deck Theme

### 10.1 Slide Template

```
Background:      #0F172A (dark) with subtle radial purple glow at top-center
Title font:      Inter Variable Bold, #FFFFFF
Body font:       Inter Variable Regular, #CBD5E1 (Slate 300)
Accent elements: #6B21A8 lines, #A855F7 highlights
Code blocks:     JetBrains Mono Variable, #A855F7 on #1E293B rounded card
Slide numbers:   Bottom-right, JetBrains Mono Variable, #475569 (Slate 700)

Title Slide:
- Logo (centered)
- "API Cerberus" (large)
- Tagline (small, #94A3B8)
- Purple gradient line separator

Content Slide:
- Title (top-left, large)
- Content (body text or bullet points)
- Visual (right side or full-width)
- No more than 3-4 bullet points per slide

Code Slide:
- Dark code block (rounded corners, purple border)
- Syntax highlighted (purple keywords, green strings, white operators)
- File path badge top-left of code block

Comparison Slide:
- Two columns
- Purple highlight on API Cerberus column
- Dimmed/gray for competitors
```

---

## 11. Email & Communication

### 11.1 Email Signature

```
─────────────────────────────
API Cerberus — Three-headed guardian for your APIs
🌐 apicerberus.com
📦 github.com/APICerberus/APICerberus
─────────────────────────────
```

### 11.2 Tone of Voice

| Do | Don't |
|-----|-------|
| Confident, factual | Aggressive, trash-talking |
| Technical depth when needed | Oversimplify or dumb down |
| Show, don't tell (benchmarks, code) | Vague marketing claims |
| Acknowledge competitors' strengths | Pretend competitors don't exist |
| Use data (16 deps, 50K req/sec) | Use subjective superlatives |
| Turkish + English mix naturally | Force one language |
| Reference mythology naturally | Overdo mythological metaphors |

---

## 12. Content Calendar Template

### 12.1 Launch Week

| Day | Content | Platform |
|-----|---------|----------|
| Day 1 | Launch announcement + hero image | X, LinkedIn, Reddit |
| Day 2 | Architecture infographic | X |
| Day 3 | "Kong vs API Cerberus" comparison post | X |
| Day 4 | Video: 5 min quickstart demo | X, YouTube |
| Day 5 | Credit system feature post | X |
| Day 6 | User Portal + Playground feature post | X |
| Day 7 | "Neden Yeni Bir API Gateway?" thread (TR) | X |

### 12.2 Ongoing Content Ideas

```
Weekly:
- Feature highlight post (1 feature deep dive per week)
- Code snippet sharing (interesting Go patterns from the codebase)
- User/community question answers

Bi-weekly:
- Blog post on apicerberus.com (tutorial, comparison, architecture)
- Infographic update for new features

On each release:
- Release notes post with changelog highlights
- Updated comparison table
- Updated infographic
```
