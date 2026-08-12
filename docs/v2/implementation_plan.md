# Go++ Version 2 Platform Implementation Plan

- **Status:** Draft for architecture review
- **Program:** Go++ v2.0–v2.x
- **Last updated:** 2026-08-11
- **Audience:** Framework maintainers, module authors, application teams, security engineers, operators, and technical writers

## 1. Executive Summary

Go++ v2 will evolve `goplusplus` from a broad HTTP framework into a modular application platform capable of supporting content/news, commerce, marketplaces, accounting, ERP, real estate, marketing, compensation networks, mobility, delivery, warehouse management, contractor management, and multinational deployments.

The v2 program has four non-negotiable product goals:

1. **Trustworthy:** behavior is explicit, production adapters are real, failures are visible, migrations are safe, and compatibility is governed.
2. **Secure by default:** authentication, authorization, isolation, validation, quotas, audit, and secret handling are enforced by shared platform contracts.
3. **Scalable without application rewrites:** applications start as modular monoliths and can move workloads to distributed infrastructure through adapter and deployment changes.
4. **Small learning curve:** developers learn five primary concepts—`Entity`, `Capability`, `Module`, `Policy`, and `Event`—then progressively adopt advanced features.

V2 is a **program**, not a single big-bang merge. The v2.0 release establishes stable kernel and platform contracts. Official domain modules then ship across compatible v2.x releases behind independent production-readiness gates. No module may call itself production-ready while depending on simulated infrastructure.

## 2. Scope and Non-Goals

### 2.1 In scope

- A minimal, stable framework kernel.
- A declarative entity/capability platform that generates APIs, validation, migrations, admin metadata, policies, search mappings, events, and SDKs.
- Production infrastructure contracts and certified adapters.
- Durable workflows, events, jobs, streaming, execution budgets, rate limiting, metering, and subscription entitlements.
- Reusable business foundations: parties, organizations, money, units, documents, activities, accounting, tax, compensation, geography, and operations.
- Official modules and presets for the domains discussed in this plan.
- Multinational organization, accounting, tax, locale, timezone, currency, residency, and localization support.
- A controlled v1-to-v2 migration path.

### 2.2 Explicit non-goals

- Placing all business functionality in the root `gpp` package.
- Storing every domain record in one universal `objects` or EAV table.
- Executing arbitrary tenant-provided scripts inside financial or security-sensitive paths.
- Building payment networks, map providers, banks, email delivery networks, e-signature services, or tax authorities inside Go++; these remain adapters.
- Claiming automatic worldwide legal, tax, payroll, or accounting compliance. Localization packs provide reviewed mechanisms and fixtures but still require jurisdictional validation.
- Claiming “fastest” without public, reproducible benchmarks.

## 3. Architectural Decisions

### 3.1 Layered ecosystem

Dependencies must point inward only:

```text
Applications and presets
        ↓
Domain modules and integration bridges
        ↓
Business foundations
        ↓
Application platform
        ↓
Core framework
```

- **Core** owns HTTP execution, lifecycle, errors, configuration contracts, observability hooks, and module loading.
- **Platform** owns entities, capabilities, policies, workflows, events, generated APIs, admin metadata, and schema evolution.
- **Foundations** own strong cross-domain invariants such as exact money, immutable accounting, durable jobs, organizations, parties, and compensation.
- **Domain modules** compose foundations without weakening their invariants.
- **Integration bridges** translate events between modules; modules do not write directly into each other's tables.
- **Presets** assemble modules but own no domain logic.

### 3.2 Universal model, typed storage

The universal abstraction is an `EntityDefinition`, not a universal storage table.

- Common metadata, custom fields, forms, policies, workflows, and APIs are declarative.
- Critical relationships, financial records, inventory movements, genealogy placements, and state machines remain strongly typed.
- JSONB is allowed for validated sparse extensions, not as a replacement for relational constraints.
- `EntityRef` supports cross-cutting attachments such as notes, tags, activities, documents, and audit events; core financial relationships retain typed foreign keys.
- Composition through capabilities is preferred over inheritance.

Example capability composition:

```text
Product  = Offering + Priceable + Stockable + Taxable
Service  = Offering + Priceable + Schedulable
Property = Asset + Locatable + Listable + Sellable/Rentable
Vehicle  = Asset + Registrable + Maintainable + Dispatchable
```

### 3.3 Correctness rules

- Money, rates, quantities, and exchange values never use binary floating point.
- Posted accounting entries and stock movements are append-only; corrections use reversals.
- Events are delivered at least once; consumers are idempotent.
- Database writes and event publication use transactional outbox/inbox patterns.
- Financial, inventory, placement, settlement, and permission invariants are enforced server-side and in the database where possible.
- Country and plan rules are effective-dated and versioned.
- Large work is streamed, partitioned, or moved to durable jobs; it is not accumulated in process memory.
- Hard customer CPU/memory isolation is provided by processes or containers, not promised per request inside a shared Go heap.

## 4. Target Repository and Package Topology

The first implementation may remain in one repository for coordinated development, but package ownership and dependency checks are mandatory.

```text
core/
    app/
    context/
    errors/
    lifecycle/
    module/
    observability/

platform/
    schema/
    entity/
    capability/
    policy/
    workflow/
    events/
    jobs/
    execution/
    stream/
    admin/
    api/
    sdk/

frontend/
    contract/
    sdk/
    auth/
    ui/
    admin/
    blocks/
    forms/
    preview/
    cache/
    runtime/
    presets/

foundation/
    party/
    organization/
    money/
    units/
    location/
    activity/
    document/
    media/
    accounting/
    tax/
    compensation/
    genealogy/
    operations/

modules/
    content/
    news/
    commerce/
    marketplace/
    realestate/
    marketing/
    crm/
    procurement/
    inventory/
    manufacturing/
    hr/
    payroll/
    mobility/
    delivery/
    shipping/
    warehouse/
    contractor/
    fieldservice/

bridges/
    commerceaccounting/
    commercecompensation/
    realestateaccounting/
    payrollaccounting/
    operationsaccounting/

adapters/
    postgres/
    redis/
    kafka/
    s3/
    search/
    payments/
    routing/
    messaging/

localization/
    india/
    uae/
    uk/
    eu/
    us/

presets/
    news/
    commerce/
    realestate/
    erp/
    mobility/
    lastmile/
    warehouse/
```

Large adapter and domain packages may become separate Go modules after the v2 contracts stabilize. Prematurely splitting repositories is avoided because it would make coordinated schema and API changes unnecessarily expensive.

## 5. Stable Module Contract

Every official or third-party module must implement a versioned contract equivalent to:

```go
type Module interface {
	Name() string
	Version() Version
	Dependencies() []Dependency

	Entities() []EntityDefinition
	Migrations() []Migration
	Policies() []Policy
	Workflows() []WorkflowDefinition
	Events() []EventDefinition
	Jobs() []JobDefinition
	AdminExtensions() []AdminExtension
	HealthChecks() []HealthCheck
	Capabilities() CapabilityManifest
}
```

The capability manifest declares:

- Tables and migrations owned.
- Events produced and consumed.
- Permissions and secrets required.
- External network access.
- PII and financial data classifications.
- Storage, queue, search, and cache dependencies.
- Data residency constraints.
- Production adapter requirements.
- Upgrade, disable, and uninstall behavior.

Startup fails with an actionable diagnostic when dependencies, versions, migrations, adapters, or permissions are invalid.

## 6. V2 Developer Experience

### 6.1 Golden path

```go
app := gpp.NewPlatform()

app.Install(
	party.Module(),
	commerce.Module(),
	accounting.Module(),
	compensation.Module(),
	localization.India(),
)

if err := app.Run(ctx); err != nil {
	log.Fatal(err)
}
```

### 6.2 Progressive disclosure

1. **Preset:** `gpp new my-store --preset commerce`.
2. **Configuration:** configure currencies, providers, and enabled capabilities.
3. **Entity extension:** add validated fields and relationships.
4. **Domain code:** implement explicit services, policies, and event handlers.
5. **Infrastructure:** replace providers or isolate workloads without rewriting business logic.

### 6.3 Required CLI surface

```text
gpp new <app> --preset <preset>
gpp new <app> --frontend <none|embedded|astro|next>
gpp dev
gpp doctor
gpp module graph
gpp module add/remove
gpp schema validate/diff
gpp migrate plan/apply/status
gpp policies explain
gpp routes
gpp events trace/replay
gpp jobs inspect/retry
gpp search reindex
gpp generate sdk
gpp upgrade v2
```

Diagnostics must include stable error codes, source locations, remediation steps, and documentation links.

## 7. Platform Workstreams

### 7.1 Core reliability and error contract

**Deliverables**

- Typed error taxonomy with stable codes and RFC 9457 problem responses.
- Clear distinction between validation, authentication, authorization, conflict, quota, deadline, dependency, and internal failures.
- Panic recovery only at process boundaries, with secret-safe logging and request/trace IDs.
- End-to-end `context.Context` cancellation and deadlines.
- Graceful startup, shutdown, readiness, draining, and lifecycle ownership.
- No unmanaged background goroutines.
- Configuration validation before listening for traffic.
- Development versus production readiness classification for all providers.

**Exit criteria**

- No production configuration can silently fall back to in-memory behavior.
- Every public failure path has a stable error contract and tests.
- Cancellation reaches database, network, stream, and job boundaries.

### 7.2 Production adapters

**Deliverables**

- Real PostgreSQL primary/replica pools, transactions, timeouts, query naming, and health checks.
- Redis cache, sessions, distributed locks, idempotency, quota, and rate-limit providers.
- Durable queue providers with retry, delay, priority, checkpointing, dead-letter queues, and draining.
- Real S3-compatible multipart/resumable storage with signed URLs.
- Search adapters with zero-downtime reindex and authorization filters.
- Provider conformance test kits.
- Failure modes and fallback behavior documented per adapter.

**Exit criteria**

- Each production adapter passes conformance, recovery, load, cancellation, and fault-injection tests.
- Development adapters are labeled and rejected in production unless an explicit unsafe override is recorded.

### 7.3 Entity, schema, and capability platform

**Deliverables**

- Typed field registry: text, rich text, number, decimal, money, boolean, date/time, enum, JSON, relation, polymorphic relation, upload, array, group, blocks, geo, computed, and localized fields.
- Required/default/unique/check constraints and server-side validation.
- One-to-one, one-to-many, many-to-many, hierarchical, and bounded polymorphic relationships.
- Capability composition with conflict detection.
- Custom fields backed by validated JSONB plus targeted index declarations.
- Safe schema diff and expand/contract migration planner.
- Entity, field, relationship, and event schema versions.
- Temporal/effective-dated and optional bitemporal records.

**Exit criteria**

- One definition generates consistent database, validation, REST, GraphQL, admin, search, audit, and SDK metadata.
- Destructive schema changes require an explicit reviewed migration plan.
- Entity customization cannot bypass domain invariants.

### 7.4 Policy, identity, tenant, and organization platform

**Deliverables**

- Row-, field-, action-, relationship-, and transition-level policies.
- Deny-by-default authorization with explainable decisions.
- Tenant, enterprise group, legal entity, establishment, branch, business unit, department, cost center, and warehouse boundaries.
- OIDC, SAML, SCIM, passkeys/WebAuthn, service accounts, scoped API keys, session/device management, MFA, and step-up authentication.
- Delegated administration, segregation of duties, emergency access, and audited impersonation.
- Database-enforced tenant/legal-entity scoping where supported.

**Exit criteria**

- Generated APIs and admin UI use the same policy engine.
- Cross-tenant and cross-legal-entity isolation tests pass for every repository adapter.
- Authentication credentials never determine authorization without explicit policy evaluation.

### 7.5 Events, workflows, jobs, and automation

**Deliverables**

- Versioned event envelope and schema registry.
- Transactional outbox and idempotent consumer inbox.
- Durable jobs with retry, jitter, delay, priority, uniqueness, cancellation, checkpoint/resume, progress, and dead-letter handling.
- State-machine workflows with guards, authorized transitions, timers, approval tasks, history, and compensation.
- Long-running orchestration with wait-for-event, escalation, pause/resume, and manual intervention.
- Constrained, typed rules DSL with validation, simulation, execution budgets, and no arbitrary system access.

**Exit criteria**

- Process crashes cannot lose committed domain events.
- Replaying an event or job does not duplicate financial or inventory effects.
- Workflow versions preserve the behavior applied to historical instances.

### 7.6 Streaming, execution governance, and subscriptions

**Deliverables**

- Bounded JSON, file, CSV, upload, SSE, and WebSocket streaming.
- Backpressure, slow-consumer detection, heartbeats, deadlines, byte limits, and resume/checkpoint semantics.
- Per-request execution budgets for time, body size, response size, buffer size, rows, query cost, concurrency, streams, and jobs.
- Workload classes: interactive, streaming, background, batch, financial, realtime, and analytics.
- Hierarchical admission control, load shedding, weighted fair scheduling, and dedicated workload pools.
- Distributed rate limiting by tenant, principal, API key, route, action, and weighted cost.
- Subscription entitlements, trials, add-ons, metering, proration, credits, dunning, plan history, and customer billing portal contracts.
- Immutable, idempotent usage ledger separate from observability metrics.

**Exit criteria**

- Large uploads, exports, and queries have bounded process memory.
- Lower plans receive the same correctness and security; higher plans receive larger limits, reserved capacity, isolation, and SLA options.
- Hard memory/CPU guarantees use separate processes or containers.

### 7.7 Generated admin, API, and SDK platform

**Deliverables**

- Schema-driven admin application with lists, forms, relationships, bulk actions, saved filters, dashboards, media, revisions, workflow actions, and role-aware navigation.
- Custom fields, pages, widgets, and dashboard extension API.
- REST, GraphQL, in-process Go API, OpenAPI, AsyncAPI, and typed Go/TypeScript SDK generation.
- API lifecycle: versioning, deprecation, changelog, compatibility checks, sunset policy, sandbox, developer portal, and usage visibility.
- Accessibility, keyboard navigation, screen-reader semantics, RTL, theming, multi-brand, and custom domains.

**Exit criteria**

- A new entity can be administered and accessed without handwritten CRUD code.
- Generated clients and server schemas pass compatibility tests.
- Admin UI cannot expose actions or fields denied by policy.

### 7.8 Shared application services

**Deliverables**

- Master data: deduplication, merge, canonical records, external IDs, stewardship, quality rules, and provenance.
- Documents: templates, PDFs, numbering, versions, comparison, OCR adapters, e-sign adapters, retention, watermarking, and legal hold.
- Communications: email, SMS, WhatsApp, push, in-app inbox, localized templates, preferences, consent, delivery receipts, and inbound replies.
- Integrations: OAuth vault, polling, webhooks, CDC, mapping, transformation, sync cursors, conflict handling, backfill, and reconciliation.
- Reporting: semantic metrics, dashboards, report builder, scheduled reports, row-level security, drill-down, exports, and analytics warehouse adapters.
- Mobile/offline: delta sync, encrypted local storage, offline commands, conflict resolution, device registration, push, and low-bandwidth operation.
- Feature flags: tenant/user cohorts, percentage rollout, kill switches, approval, history, and expiry.
- Fraud/risk: velocity rules, duplicate accounts, promotion abuse, account takeover, GPS spoofing signals, manual review, and KYC/KYB provider contracts.

**Exit criteria**

- Shared services attach to any authorized entity without weakening typed domain relationships.
- Reconciliation and replay are available for every external integration.
- Offline commands retain idempotency and original business time.

### 7.9 Observability, resilience, and testability

**Deliverables**

- OpenTelemetry-compatible traces, metrics, and structured logs with request, tenant, module, event, and job correlation.
- Configurable sampling and mandatory redaction of secrets, credentials, tokens, and classified fields.
- Module-level health, saturation, queue lag, dependency, error-rate, and latency signals.
- SLO definitions, burn-rate alerts, operational dashboards, continuous profiling hooks, and slow-query diagnostics.
- Explicit retry budgets, circuit breakers, bulkheads, dependency timeouts, fallback policies, and chaos/failure injection.
- Deterministic clocks, ID generators, random sources, test factories, fake providers, local infrastructure presets, and record/replay fixtures.
- Contract, integration, end-to-end, load, soak, fuzz, property, migration, recovery, and compatibility test harnesses.
- Audit events stored separately from operational logs, with tamper-evident persistence and controlled access.

**Exit criteria**

- Every official module ships dashboards, alerts, health checks, runbooks, and test factories.
- Operators can trace one request through database work, events, jobs, external calls, and resulting ledger records.
- Resilience policies cannot create unbounded retries or retry non-idempotent side effects unsafely.

### 7.10 Optional frontend platform

Go++ remains headless-capable. When enabled, the frontend platform provides a generated admin and a customizable public application over the same schema, policy, workflow, and SDK contracts.

**Deliverables**

- Framework-neutral frontend contract and generated TypeScript SDK, query builders, validation schemas, permission capabilities, event types, and workflow action types.
- Official React/TypeScript admin with schema-driven forms, lists, relationships, revisions, media, workflow actions, dashboards, role-aware navigation, accessibility, RTL, theming, and extension points.
- Embedded React/Vite mode compiled and served by Go for the smallest operational footprint.
- Official Astro preset for content-heavy SEO applications and Next.js preset for dynamic commerce/customer applications; neither is required by the backend.
- Public-site primitives for routing, layouts, blocks, forms, authentication, localization, SEO, structured data, analytics consent, images, errors, loading states, and cache tags.
- Draft preview and live preview with short-lived tokens, trusted origins, tenant/locale-aware URLs, responsive breakpoints, and secure cache bypass.
- Entity-aware frontend/CDN invalidation and preview-deployment integration.
- Domain starters for news, commerce, real estate, marketing, ERP portals, and operations dashboards.

**Exit criteria**

- `--frontend none` produces a complete headless backend without frontend runtime dependencies.
- Embedded, Astro, and Next.js reference applications consume the same generated contract and pass API compatibility tests.
- Frontend hiding never substitutes for backend authorization, and denied fields/actions are absent from generated responses.
- Custom components and complete pages can be replaced without forking the generated admin or SDK.

## 8. Business Foundation Workstreams

### 8.1 Party and common business model

Define reusable, extensible archetypes:

- `Party`: person, organization, customer, supplier, employee, seller, agent, broker, affiliate, and team.
- `Offering`: product, service, subscription, rental, course, and listing.
- `Asset`: property, building, unit, vehicle, machine, equipment, device, and intellectual property.
- `Agreement`: contract, lease, sale, employment, supplier, service-level, and subscription agreements.
- `BusinessDocument`: lead, opportunity, quote, order, invoice, receipt, payment, shipment, return, and credit note.
- `Activity`: task, call, meeting, visit, message, note, approval, reminder, and notification.

Common capabilities include `Ownable`, `Assignable`, `Addressable`, `Locatable`, `Searchable`, `Auditable`, `Versioned`, `Approvable`, `Schedulable`, `Priceable`, `Taxable`, `Stockable`, `Commissionable`, `Maintainable`, and `PIIProtected`.

### 8.2 Accounting and finance

**Core**

- Exact money/decimal, currencies, rates, units, rounding, and allocation.
- Immutable double-entry journal and general ledger.
- Chart of accounts, accounting dimensions, fiscal calendars, periods, locks, reversals, recurring entries, and audit.
- Versioned posting-rule engine driven by domain events.
- AR, AP, cash/banking, reconciliation, inventory valuation, fixed assets, payroll, commissions, leases, loans, and revenue-recognition subledgers.
- Trial balance, P&L, balance sheet, cash flow, consolidation, and statutory reporting adapters.
- Multi-book, multi-company, multi-currency, intercompany, transfer-pricing metadata, eliminations, and group reporting.

**Invariants**

- Debits equal credits per posted journal.
- Posted entries are immutable.
- Source event plus posting rule is idempotent.
- Operational subledgers reconcile to the general ledger.
- Accounting recognition is policy-driven and not inferred merely from order or payment creation.

### 8.3 Tax and multinational localization

- Jurisdiction, registration, place-of-supply, tax category, exemption, withholding, inclusive/exclusive tax, and recoverability models.
- Country packs for chart templates, tax rules, invoice requirements, numbering, e-invoicing, payroll, reports, and retention.
- Functional, transaction, settlement, and presentation currencies with historical rate snapshots and realized/unrealized differences.
- Locale-aware names, addresses, numbers, currency, calendars, translations, collation, RTL, and IANA timezones.
- Residency region, data classification, encryption-key region, replication, retention, export, anonymization, legal hold, and data-subject workflows.

### 8.4 Compensation, incentives, genealogy, and settlement

**Commission/incentive engine**

- Flat, fixed, combined, slab, marginal, retroactive, volume, margin, target, recurring, rank, split, cap, floor, bonus, and clawback rules.
- Plan assignment, priority, exclusivity, effective dates, immutable plan versions, simulation, historical comparison, maker-checker approval, and explainable results.
- Transaction, team, KPI, quota, scorecard, territory, collections, and performance measurements.
- Accrual, pending, payable, paid, reversal, recovery, dispute, and settlement lifecycle.
- Immutable commission ledger and accounting reconciliation.

**Genealogy/network engine**

- Sponsor, placement, binary, unilevel, fixed matrix, N-ary, leadership, and multiple simultaneous trees.
- Atomic placement, cycle prevention, effective-dated edges, bounded compensated ancestry, binary leg volume, carry-forward, flush, ratios, rank qualification, and compression.
- Crore-scale event-driven volume projections; no recursive full-tree traversal during payout runs and no unbounded closure table.
- Partitioned, checkpointed, idempotent commission runs with replay and reconciliation.

### 8.5 Media processing and delivery

Media processing is a shared foundation for content, commerce, real estate, marketing, identity documents, support, and field operations.

**Asset model**

- Immutable original asset, content hash, ownership, classification, visibility, retention, usage references, and metadata.
- Deterministic variant specifications and IDs so identical transformations are idempotent and CDN-cacheable.
- Processing job, variant, video/audio track, caption, thumbnail, moderation, and failure records.
- Public, private, tenant-scoped, expiring, and signed-delivery policies.

**Upload and safety pipeline**

```text
Initiate → Direct Multipart Upload → Quarantine → Verify Type/Size
         → Malware/Decompression Checks → Extract Metadata
         → Async Transform → Validate Output → Publish → CDN
```

- Stream directly to object storage; never buffer complete media in application memory.
- Inspect actual media/container signatures rather than trusting filename or `Content-Type`.
- Enforce byte, pixel, frame, duration, track, codec, archive, and decompression limits.
- Sandbox processors with CPU, memory, disk, process, network, and wall-clock limits.
- Strip or explicitly preserve sensitive EXIF/GPS metadata according to policy.

**Image processing**

- Resize, crop, contain, cover, pad, rotate/orient, focal point, quality, colorspace, watermark, metadata policy, and animated-image handling.
- JPEG, PNG, WebP, AVIF, and configurable source-preservation profiles.
- Responsive `srcset` variants, aspect-ratio presets, thumbnails, dominant color, and compact placeholder generation.
- Local low-memory processor adapter and managed image-CDN adapters.

**Video and audio processing**

- Probe containers, codecs, resolution, frame rate, bitrate, duration, rotation, audio, caption, and metadata tracks.
- Transcode/remux into approved codec/container profiles; avoid lossy transcoding when stream copy is sufficient.
- Resolution, bitrate, frame-rate, aspect-ratio, crop, watermark, normalization, and audio-resampling profiles.
- Adaptive bitrate HLS, optional DASH/CMAF adapters, progressive MP4, posters, thumbnails, preview clips, and timeline sprite sheets.
- Multiple audio tracks, subtitles/captions, transcript adapters, and accessibility metadata.
- Encryption/DRM provider contracts; keys and licenses never live in public asset metadata.
- Prerecorded media is in the initial scope; live ingest and low-latency broadcast are a separate optional module over the same asset and delivery contracts.

**Execution and subscriptions**

- Processing runs only in durable media-worker queues, never synchronously in upload requests.
- Workload classes for image, video, audio, OCR, and moderation with checkpoint, retry, cancellation, and dead-letter behavior.
- Subscription limits for upload bytes, duration, source resolution, generated variants, concurrent transcodes, processing minutes, storage, retention, and delivery bandwidth.
- Dedicated worker pools or managed transcoding providers for customers requiring hard performance isolation.

**Exit criteria**

- Processor crashes or retries cannot duplicate or publish incomplete variants.
- Originals remain recoverable and immutable; variants are reproducible from versioned specifications.
- Image and video stress tests prove bounded gateway memory and enforce worker resource limits.
- Private media, signed delivery, preview access, metadata redaction, and tenant isolation pass security tests.

## 9. Official Domain Modules

Each module below ships only after its foundations and bridges pass production gates.

### 9.1 Content and news

- Collections, blocks, rich text, media, authors, desks, categories, tags, pages, menus, drafts, revisions, preview, approvals, scheduled publication, archives, and redirects.
- Breaking news, live blogs, related content, SEO, social metadata, sitemaps, news sitemaps, RSS, syndication, newsletters, comments/moderation, paywall hooks, localization, and CDN invalidation.
- Media pipeline: signed/resumable upload, MIME/size validation, malware-scan hooks, transformations, WebP/AVIF, responsive variants, metadata, copyright, usage tracking, quotas, and private assets.

### 9.2 Commerce and marketplace

- Catalog, variants, attributes, categories, bundles, physical/digital/service items, channels, and import/export.
- Exact pricing, price lists, currencies, customer groups, promotions, coupons, tax, historical snapshots, and conflict rules.
- Warehouses, stock movements, reservations, backorders, available-to-promise, atomic allocation, and overselling protection.
- Guest/authenticated carts, checkout, shipping/tax quotes, idempotent order creation, orders, fulfillment, cancellation, returns, refunds, invoices, and customer timelines.
- Payment adapters with authorization, capture, webhook verification, deduplication, reconciliation, refunds, and provider-independent state.
- Marketplace sellers, onboarding, verification, offers, commissions, split settlements, order routing, moderation, disputes, seller performance, and payout adapters.

### 9.3 Real estate and marketing

**Real estate**

- Developers, agencies, owners, brokers, projects, buildings, floors, units, plots, listings, availability, pricing, documents, geographic search, leads, visits, offers, booking, deposits, agreements, registration, possession, leases, rent schedules, maintenance, owner statements, and brokerage commissions.

**Marketing**

- Contacts, consent, lead capture, sources, segmentation, audiences, campaigns, budgets, journeys, templates, channels, scoring, UTM, attribution, landing forms, frequency caps, suppression, experiments, referrals, affiliates, and ROI.
- Large audiences are materialized asynchronously and never evaluated synchronously in request handlers.

### 9.4 ERP modules

- CRM and sales.
- Procurement: requisition, RFQ, quote comparison, approval, purchase order, receipt, supplier invoice, and three-way match.
- Inventory and warehouse.
- Manufacturing: BOM, work centers, routings, production orders, MRP, capacity, costing, quality, scrap, and subcontracting.
- HR and payroll: employee lifecycle, recruitment, attendance, leave, shifts, benefits, payroll, expenses, performance, and localization packs.
- Projects and professional services: tasks, resources, time, expenses, milestones, utilization, billing, and profitability.
- Customer support: tickets, conversations, SLA, escalation, knowledge base, warranty, service history, and satisfaction.
- Asset/fleet management: registry, assignment, depreciation, inspection, fuel, telematics, maintenance, warranty, and replacement.
- Treasury: cash position, liquidity forecast, payment factory, bank accounts, intercompany netting, FX exposure, and approvals.
- Contract lifecycle: templates, clauses, negotiation, approvals, signing, obligations, renewals, escalation, and termination.

### 9.5 Operations, mobility, delivery, warehouse, and contractor modules

**Shared operations lifecycle**

```text
Request → Quote → Capacity Reservation → Assignment → Execution
        → Tracking → Completion Proof → Billing → Settlement → Rating
```

**Shared primitives**

- Service requests, jobs, resources, skills, availability, capacity, reservations, assignments, routes, stops, tracking sessions, SLAs, proofs, exceptions, ratings, disputes, and settlements.
- PostGIS/H3-style geo indexes, geofences, service areas, nearest-resource search, route/ETA provider contracts, and location-event ingestion.
- Separate transactional, realtime-presence, durable-event, and analytics planes.

**Mobility**

- Riders, drivers, vehicles, driver sessions, fare quotes, matching, expiring offers, atomic assignment, pickup/dropoff, multi-stop trips, live tracking, dynamic pricing, safety, payments, driver earnings, incentives, wallets, ratings, and disputes.

**Shipping and last-mile delivery**

- Shipments, packages, rates, labels, manifests, tracking, delivery attempts, route batching, windows, capacity, OTP/photo/signature proof, COD, redelivery, claims, and return-to-origin.

**Warehouse management**

- Warehouse, zone, aisle, rack, bin, SKU, lot, serial, handling unit, pallet, receipt, quality, putaway, allocation, wave, pick, pack, manifest, replenishment, cycle count, transfer, cross-dock, kitting, barcode/QR, cold-chain metadata, FIFO/FEFO, and immutable stock ledger.

**Contractor/field service**

- Contractor organizations, individuals, skills, licenses, territories, availability, rate cards, estimates, work orders, scheduling, assignments, checklists, time, materials, expenses, inspection, milestones, change orders, warranty callbacks, invoice, payable, incentive, rating, and dispute.

### 9.6 Optional AI, recommendation, and decision-support modules

AI is an optional platform consumer, not a privileged path around policies or domain services.

- Model-provider gateway with tenant isolation, timeouts, cost budgets, rate limits, and provider fallback policy.
- Prompt, tool, model, retrieval-index, and output-schema versioning.
- PII redaction, consent, retention, residency, safety filters, and immutable invocation audit.
- Evaluation datasets, regression gates, confidence thresholds, human approval, and deterministic fallbacks.
- Document extraction, natural-language search, support assistance, report explanation, entity/form drafting, anomaly explanation, and recommendation candidates.
- Recommendations retain explainable inputs, policy filtering, experimentation controls, and feedback measurement.
- AI output cannot directly post accounting, move inventory, approve payments, change genealogy, or grant permissions; it submits validated commands through ordinary workflows.

## 10. Operational Control Plane

Provide a separately permissioned operations console for:

- Health, dependency, capacity, and SLO status.
- Failed jobs, dead letters, retries, checkpoints, and cancellation.
- Event tracing, replay, and reconciliation.
- Webhook delivery, replay, secret rotation, and endpoint health.
- Backfills, reindexing, migrations, and tenant rollout.
- Tenant suspension, plan changes, quota overrides, and maintenance mode.
- Feature flags and kill switches.
- Audited support impersonation and access review.
- Data export, retention, anonymization, legal hold, and incident investigation.

Privileged actions require explicit permissions, reason codes, immutable audit records, and optional dual approval.

## 11. Security and Supply-Chain Program

- Threat models per core subsystem and official module.
- Secure defaults for headers, CORS, CSRF, request sizes, timeouts, uploads, sessions, cookies, redirects, and error redaction.
- Input validation and output encoding at all boundaries.
- Tenant/legal-entity isolation and authorization fuzz/property tests.
- KMS-backed encryption, envelope encryption for sensitive fields, secret rotation, and no credentials in configuration logs.
- Signed releases, dependency pinning, SBOM, provenance, vulnerability scanning, and coordinated disclosure process.
- Plugin manifests declare permissions, data classes, secrets, and network access.
- Abuse resistance: bot, credential, OTP, payment, coupon, GPS, commission, and export controls.
- Evidence exports to support—not falsely claim—SOC 2, ISO 27001, PCI DSS, privacy, or jurisdictional audits.

## 12. Performance and Scale Program

### 12.1 Workload profiles

Maintain reproducible profiles for:

- Small single-node application.
- Multi-instance SaaS application.
- High-volume content and commerce.
- Crore-scale member/genealogy/commission platform.
- Realtime mobility/delivery location workload.
- Billion-line accounting, usage, inventory, and event workloads.
- Multinational multi-region deployment.

### 12.2 Required techniques

- Stateless request execution and bounded concurrency.
- Cursor pagination and query-cost admission.
- Partitioned/time-partitioned event and ledger tables.
- Precomputed read models and incremental aggregates.
- Bounded ancestry projections and volume buckets.
- Cache tags and explicit invalidation.
- Streaming and object-storage exports.
- Backpressure, load shedding, fair scheduling, and tenant isolation.
- Logical shards and online rebalance strategy.
- Regional data placement and failover.

### 12.3 Performance governance

- Publish benchmark source, hardware, datasets, and commands.
- Track latency, throughput, allocation, memory, and tail behavior.
- Gate unexplained benchmark regressions.
- Use profiling and evidence before optimization.
- Reflection may run during registration; hot paths use compiled metadata.
- Define SLOs after Phase 0 baselines rather than inventing unsupported claims.

## 13. Data Governance, Backup, and Recovery

- Data classification, lineage, provenance, ownership, quality, and master-record management.
- Retention, archive, anonymization, erasure, legal hold, residency, and export policies.
- Automated full/incremental backup and point-in-time recovery.
- Restore verification in isolated environments.
- Tenant-level portability and offboarding.
- Projection and search-index rebuild from authoritative records/events.
- RPO/RTO targets per workload class and subscription tier.
- Disaster-recovery and regional-failover exercises before GA.

## 14. Version 1 Migration and Compatibility

### 14.1 Release policy

- Maintain a supported v1 line for critical fixes during the v2 adoption window.
- Publish v2 module compatibility matrices.
- Deprecations include replacements, diagnostics, and a documented removal release.
- Event and persisted schema compatibility is treated separately from Go API compatibility.

### 14.2 Migration tooling

`gpp upgrade v2` should:

- Inventory imports, middleware, configuration, database usage, and adapters.
- Flag simulated/development adapters.
- Produce a migration report without modifying files by default.
- Generate safe mechanical edits where unambiguous.
- Create schema and data migration plans.
- Identify behavior changes requiring human decisions.
- Run v1/v2 compatibility tests and API contract comparisons.

### 14.3 Compatibility layer

- Provide narrowly scoped `compat/v1` shims only where they do not weaken v2 guarantees.
- Do not preserve unsafe implicit behavior merely for source compatibility.
- Every shim has telemetry and a removal plan.

## 15. Phased Delivery Roadmap

Each phase produces a usable, tested increment. Work from later phases may be designed earlier, but it cannot bypass prerequisite exit gates.

The intended release train is:

| Release family | Planned scope |
| --- | --- |
| v2.0 | Kernel, production adapters, entity/policy/workflow/execution platform, optional frontend/admin/SDK foundation, media pipeline, and news reference vertical |
| v2.1 | Party/organization, accounting, tax, compensation ledger, and initial multinational localization |
| v2.2 | Commerce, marketplace, payment bridges, incentives, genealogy/network, and settlement |
| v2.3 | Real estate, marketing, geo/operations, mobility, delivery, shipping, WMS, contractor, and field service |
| v2.4+ | Expanded ERP, localization, analytics, offline, intelligence, and ecosystem maturity |

Minor-version placement may change after measurement, but v2.0 must not be delayed until every domain pack is complete. All v2.x modules must honor the v2.0 contracts and their own production-readiness gates.

### Phase 0 — Truth, baselines, and architecture freeze

**Deliver**

- Inventory every existing package and classify behavior as production, development, experimental, or deprecated.
- Correct documentation that overstates simulated capabilities.
- Capture public API, benchmark, security, dependency, test, and migration baselines.
- Approve v2 layering, naming, module ownership, event envelope, error contract, and release policy through ADRs.
- Establish the v2 branch/release workflow without disrupting v1 maintenance.

**Exit gate**

- No unknown adapter or API status.
- Architecture decisions and ownership are approved.
- Baseline test and benchmark commands are reproducible.

### Phase 1 — V2 kernel and production infrastructure

**Deliver**

- Core lifecycle, error, configuration, module, observability, and context contracts.
- PostgreSQL, Redis, queue, storage, search, and idempotency provider contracts plus initial production adapters.
- Transactional outbox/inbox.
- Conformance kits and production startup validation.

**Exit gate**

- A minimal API runs in multi-instance mode without in-memory correctness dependencies.
- Crash/retry tests prove event and idempotency behavior.
- Production adapters pass their conformance suites.

### Phase 2 — Entity, policy, workflow, and execution platform

**Deliver**

- Schema/entity/capability registry.
- Migration planner and schema versioning.
- Policy engine and organization/tenant boundaries.
- Events, durable jobs, workflows, typed rules, streaming, budgets, rate limiting, quotas, and workload classes.

**Exit gate**

- One sample entity is generated end-to-end with secure APIs, migrations, admin metadata, search, events, and tests.
- Cross-tenant, replay, cancellation, and memory-bound tests pass.

### Phase 3 — Admin, SDK, integrations, and shared services

**Deliver**

- Generated admin application and optional frontend platform with embedded, Astro, and Next.js reference modes.
- Go and TypeScript SDKs and API lifecycle tooling.
- Media processing, documents, notifications, webhook platform, integration SDK, master data, feature flags, reporting foundation, offline-sync protocol, and operational console.

**Exit gate**

- A developer can create, administer, integrate, operate, and optionally render a custom entity without handwritten CRUD.
- Accessibility, authorization, upgrade, and failure-path tests pass.

### Phase 4 — News vertical slice

**Deliver**

- Content, image/video media, editorial workflow, scheduled publication, search, SEO, revisions, live preview, public frontend, CDN invalidation, audit, and news preset.

**Exit gate**

- `gpp new newsroom --preset news --frontend astro` produces a secure deployable admin, backend, and public application; `--frontend none` produces the equivalent headless backend.
- Load, media, publication recovery, migration, and editor-permission tests pass.

### Phase 5 — Financial and multinational foundations

**Deliver**

- Money/decimal, party/organization, accounting, tax, multi-company, multi-book, multi-currency, intercompany, consolidation contracts, compensation ledger, and first localization pack.

**Exit gate**

- Accounting property tests guarantee balanced, immutable, idempotent postings.
- AR/AP/cash and commission subledgers reconcile to GL.
- Currency and effective-date fixtures pass independent accounting review.

### Phase 6 — Commerce, marketplace, compensation, and network

**Deliver**

- Catalog, pricing, inventory, checkout, orders, payment bridges, fulfillment, returns, seller marketplace, commission/incentive plans, binary/unilevel/matrix/multi-tree genealogy, and settlement.

**Exit gate**

- Full sale-to-ledger-to-settlement and refund/clawback journeys pass.
- Concurrency tests prove no overselling, duplicate orders, duplicate commissions, or duplicate payouts.
- Crore-scale synthetic genealogy and commission runs meet the Phase 0 SLOs.

### Phase 7 — Real estate, marketing, and operations

**Deliver**

- Real estate and marketing modules.
- Shared geo, capacity, booking, dispatch, routing, tracking, work-order, proof, SLA, and settlement foundations.
- Mobility, delivery, shipping, WMS, contractor, and field-service presets.

**Exit gate**

- Property booking-to-accounting, campaign-to-lead, ride-to-driver-settlement, shipment-to-proof, warehouse receipt-to-shipment, and contractor-job-to-payable journeys pass.
- Realtime, offline, geospatial, privacy, and load tests pass.

### Phase 8 — ERP and expanded localization

**Deliver**

- Procurement, manufacturing, HR/payroll, projects, support, assets/fleet, treasury, and contract lifecycle modules.
- Additional country packs and data-residency deployment controls.

**Exit gate**

- Cross-module procure-to-pay, order-to-cash, hire-to-payroll, manufacture-to-stock, and consolidate-to-report journeys reconcile operational and accounting records.
- Localization packs pass versioned fixtures and jurisdictional review.

### Phase 9 — V2 program maturity and recurring GA hardening

**Deliver**

- Repeatable security review, performance publication, disaster-recovery exercises, migration rehearsals, compatibility reports, operational runbooks, upgrade guides, and support policy for each GA release in the v2 train.

**Exit gate**

- No open critical security or data-integrity defects.
- Restore, replay, reindex, backfill, regional failover, and v1 migration drills succeed.
- Documentation and examples use only GA APIs and production adapters.

## 16. Quality and Verification Matrix

Every workstream must include tests for the following where applicable:

| Area | Required verification |
| --- | --- |
| Business rules | Unit, table-driven, property, boundary, and historical-version tests |
| Database | Integration, concurrency, isolation, migration, rollback, and query-plan tests |
| Authorization | Deny-by-default, cross-tenant, field, action, transition, and fuzz tests |
| Events/jobs | Crash, duplicate, reorder, delay, replay, dead-letter, and checkpoint tests |
| Financial | Balance, idempotency, rounding, reversal, reconciliation, close, and currency tests |
| Streaming | Backpressure, slow consumer, cancellation, byte limit, reconnect, and leak tests |
| Scale | Reproducible load, soak, tail-latency, hot-key, shard, and recovery tests |
| Security | Threat model, static scan, dependency scan, secret scan, fuzz, and abuse tests |
| Adapters | Shared conformance suite plus provider-specific failure tests |
| UI/accessibility | Policy parity, keyboard, screen reader, RTL, visual, and browser tests |
| Compatibility | Go API, generated API, event schema, persisted schema, and SDK tests |

Release CI must include formatting, linting, unit/integration tests, the race detector, fuzz smoke tests, vulnerability scans, migration tests, adapter conformance, and benchmark regression checks. Expensive scale and recovery suites may run on scheduled or release pipelines but must gate GA.

## 17. Documentation and ADR Deliverables

Before implementation proceeds beyond Phase 0, create ADRs for:

1. Layering and dependency direction.
2. Module and capability contracts.
3. Entity storage and custom-field strategy.
4. Migration and schema-version policy.
5. Event envelope, outbox/inbox, and delivery semantics.
6. Error taxonomy and API problem contract.
7. Tenant, organization, legal-entity, and residency boundaries.
8. Policy model and deny-by-default behavior.
9. Exact money, accounting, and ledger invariants.
10. Job, streaming, execution budget, quota, and rate-limit model.
11. Production adapter readiness classification.
12. V1 compatibility and deprecation policy.

Documentation sets must include quick starts, production guides, security implications, scaling behavior, failure behavior, extension contracts, migration guides, and operational runbooks. Every documented example must be executable in CI.

## 18. Program Risks and Controls

| Risk | Control |
| --- | --- |
| Scope becomes unfinishable | Deliver vertical slices and independent module readiness; do not make all modules a v2.0 GA blocker |
| Core becomes a kitchen sink | Enforce dependency direction and package ownership in CI |
| Generic model weakens domain integrity | Keep financial, inventory, placement, tax, and workflow invariants typed |
| Simulated capabilities create false confidence | Readiness classification and production startup rejection |
| Distributed complexity harms the learning curve | Preserve one local developer model and move complexity behind providers |
| Country rules pollute the core | Versioned localization packs and stable contracts |
| Plugin ecosystem becomes unsafe | Signed manifests, permissions, compatibility, conformance, and isolation |
| Performance claims are not credible | Reproducible public benchmarks and regression gates |
| Crore-scale design creates premature complexity | Start with logical boundaries, validate profiles, and introduce physical distribution only where measured |
| Financial errors become irreversible | Immutable ledgers, reversals, approval, reconciliation, and accountant-reviewed fixtures |
| V1 users cannot migrate safely | Supported v1 line, inventory tool, compatibility report, shims, and rehearsed migration |

## 19. Definition of Done for V2 Components

A v2 component is complete only when:

- Its boundary, owner, dependencies, public API, and data ownership are explicit.
- Its secure defaults and failure modes are documented.
- It validates all untrusted input and propagates cancellation.
- It has no unbounded memory, concurrency, retry, or queue behavior.
- Its production adapters are real and pass conformance tests.
- Its migrations are online-safe or explicitly identify downtime.
- Its events and jobs are versioned and idempotent.
- Its authorization and tenant/legal-entity isolation are tested.
- Its observability, health, metrics, tracing, and operational procedures exist.
- Its performance is benchmarked against an agreed workload.
- Its upgrade, rollback, disable, and recovery procedures are tested.
- Its documentation and examples compile and run in CI.
- No critical security, correctness, reconciliation, or data-loss defect remains open.

## 20. First Implementation Backlog

The first issues should be created in this order:

1. Create Phase 0 package/readiness inventory and correct misleading capability labels.
2. Approve ADR for v2 layering, module contract, and repository topology.
3. Define production/development adapter readiness interfaces and startup enforcement.
4. Define v2 typed error and RFC problem contract.
5. Define lifecycle, cancellation, configuration validation, and graceful shutdown contracts.
6. Implement PostgreSQL provider interface and conformance suite.
7. Define transactional outbox/inbox event envelope and tests.
8. Define module manifest, dependency validation, and migration ownership.
9. Define entity/field/capability metadata and compile-time/runtime validation.
10. Implement safe schema diff with a read-only `gpp schema plan` command.
11. Define policy engine and tenant/legal-entity scope contracts.
12. Define bounded streaming and execution-budget contracts.
13. Replace the existing in-process rate limiter with provider-based memory and Redis implementations, retaining memory as development-only.
14. Implement durable job contract and one production provider.
15. Build the first end-to-end `Article` vertical slice before expanding the module catalog.

This backlog intentionally prioritizes truthful foundations over feature count. Once the first article vertical slice proves the platform contracts, subsequent modules should be delivered as compositions of those contracts rather than parallel bespoke frameworks.
