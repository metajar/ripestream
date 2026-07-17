---
name: conventions
description: How code is written in this project — naming, structure, patterns, and style. Load when writing new code or reviewing existing code.
triggers:
  - "convention"
  - "pattern"
  - "naming"
  - "style"
  - "how should I"
  - "what's the right way"
edges:
  - target: context/architecture.md
    condition: when a convention depends on understanding the system structure
  - target: patterns/INDEX.md
    condition: when looking for task-specific patterns that enforce conventions
last_updated: 2026-07-17
---

# Conventions

## Naming
- **Go files**: snake_case (`atlas_stream.go`, `store.go`, not `atlasStream.go`)
- **Go packages**: single word, lowercase (`package atlas`, `package graph`, not `package atlasStream`)
- **Go types**: PascalCase (`type Store struct`, `type Record struct`)
- **Go functions**: PascalCase for exported, camelCase for internal (`func Subscribe()`, `func runSubscription()`)
- **Go constants**: UPPER_SNAKE_CASE or PascalCase for exported (`const DefaultBaseURL`)
- **TypeScript files**: PascalCase for components (`OverviewPage.tsx`, `ASNDetailPage.tsx`)
- **TypeScript**: camelCase for functions/variables (`const queryClient`, `function RouteError()`)

## Structure
- **Go internal layout**: one package per directory (`internal/atlas`, `internal/store`, `internal/graph`). Each package is cohesive around its domain.
- **Go file organization**: keep interfaces near types they serve, test files alongside source (`store.go` + `store_test.go`)
- **TypeScript pages**: one page component per file in `web/src/pages/`
- **TypeScript components**: reusable UI components in `web/src/components/ui/`, feature components in `web/src/components/`
- **Embedded resources**: use `//go:embed` directive near relevant code (`//go:embed web/dist/*` in main.go)

## Patterns
- **Context propagation**: every goroutine-accepting function takes `ctx context.Context` as first parameter. Check `ctx.Err()` before blocking operations.
- **Error handling**: explicit error returns, never panic in production code. Use `errors.Is`/`errors.As` for error type checking.
- **Graceful shutdown**: use `signal.NotifyContext` for SIGINT/SIGTERM, drain buffered data before exit, use fresh context for final flushes.
- **Channel patterns**: prefer select over blocking sends, use buffered channels (8192 for ingestion paths), never close channel from receiver.
- **HTTP handlers**: use Go 1.25 ServeMux patterns (`"GET /api/overview"`), separate handler functions, recoverPanic middleware wrapper.
- **API responses**: use structured responses from `internal/api/respond.go`, consistent JSON shape.
- **React Query**: use `useQuery` for data fetching, automatic refetch on interval, `keepPreviousData` for smooth transitions.
- **Loading UI**: use `Spinner` / `LoadingState` / `FetchingOverlay` from `components/ui/states` (RJ45 cable mark). Show `FetchingOverlay` when `isFetching && isPlaceholderData` so pagination/filter changes are visible without blanking the prior page or flashing on background refetches.

## Verify Checklist
Before presenting any code:
- [ ] Context cancellation is checked before blocking operations
- [ ] All errors are explicitly handled (no silent failures or ignored errors)
- [ ] Channels are buffered appropriately for the expected volume
- [ ] HTTP handlers use Go 1.25 ServeMux pattern syntax
- [ ] React components use TanStack Query for data fetching
- [ ] No hardcoded secrets or credentials (use environment variables)
- [ ] Go files use snake_case naming, TypeScript uses PascalCase for components
