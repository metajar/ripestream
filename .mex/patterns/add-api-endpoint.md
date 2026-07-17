---
name: add-api-endpoint
description: Adding a new API endpoint with Go handler and corresponding React route/page. Use when extending the API with new data views or features.
triggers:
  - "add endpoint"
  - "add route"
  - "add api"
  - "new handler"
edges:
  - target: context/conventions.md
    condition: when reviewing Go/TypeScript naming and structure conventions
  - target: context/architecture.md
    condition: when understanding how the API layer connects to graph/store readers
  - target: context/stack.md
    condition: when working with TanStack Query or React Router
  - target: context/graph.md
    condition: when the endpoint needs to query the FalkorDB graph
  - target: context/alerts.md
    condition: when the endpoint needs to serve alert data
last_updated: 2026-07-17
---

# Add API Endpoint

## Context
API endpoints follow Go 1.25 ServeMux pattern syntax. Handlers live in `internal/api/`. React pages live in `web/src/pages/`. Routes registered in `api/server.go` and `web/src/main.tsx`.

## Steps

### 1. Add Go Handler
Create or update handler file in `internal/api/`:

```go
// internal/api/handlers_myfeature.go
package api

import (
    "log/slog"
    "net/http"
    "strconv"
)

type MyFeatureResponse struct {
    // Define response structure
}

func (s *Server) myFeature(w http.ResponseWriter, r *http.Request) {
    // Parse path parameters if needed
    id := r.PathValue("id") // Go 1.25 ServeMux syntax

    // Query graph.Reader or store.Reader
    data, err := s.graph.QuerySomething(r.Context(), id)
    if err != nil {
        respond.Error(w, "query failed", http.StatusInternalServerError, err)
        return
    }

    respond.JSON(w, MyFeatureResponse{...})
}
```

### 2. Register Route in Server
Add to `s.register()` in `internal/api/server.go`:

```go
func (s *Server) register() {
    m := s.mux

    // ... existing routes ...

    // My feature (UCX)
    m.HandleFunc("GET /api/myfeature", s.myFeature)
    m.HandleFunc("GET /api/myfeature/{id}", s.myFeatureDetail)
}
```

### 3. Create React Page
Create `web/src/pages/MyFeaturePage.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";

async function fetchMyFeature(id?: string) {
  const url = id
    ? `/api/myfeature/${id}`
    : `/api/myfeature`;
  const res = await fetch(url);
  if (!res.ok) throw new Error("Failed to fetch");
  return res.json();
}

export function MyFeaturePage() {
  const { id } = useParams();
  const { data, isLoading, error } = useQuery({
    queryKey: ["myfeature", id],
    queryFn: () => fetchMyFeature(id),
  });

  if (isLoading) return <div>Loading...</div>;
  if (error) return <div>Error: {error.message}</div>;

  return <div>{/* render data */}</div>;
}
```

### 4. Register React Route
Add to router in `web/src/main.tsx`:

```tsx
import { MyFeaturePage } from "@/pages/MyFeaturePage";

const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    errorElement: <RouteError />,
    children: [
      // ... existing routes ...
      { path: "myfeature", element: <MyFeaturePage /> },
      { path: "myfeature/:id", element: <MyFeaturePage /> },
    ],
  },
]);
```

### 5. Add Navigation Link (optional)
Add link in `web/src/components/AppShell.tsx` navigation if needed.

## Gotchas
- **Path parameter syntax**: Use `r.PathValue("id")` for Go 1.25 ServeMux, not chi/mux patterns
- **Error responses**: Use `respond.Error()` from `internal/api/respond.go`, never manual status codes
- **Query keys**: React Query keys should be stable and include parameters (e.g., `["myfeature", id]`)
- **503 handling**: Handlers should return 503 when graph.Reader is nil (graph disabled), use `respond.Unavailable()`
- **Deployment-safe deep links**: Keep small, operationally important page shells eager when users commonly open them directly in fresh tabs. Lazy-load the heavy visualization/library inside the page instead; this avoids a new-index/old-chunk deployment race while preserving bundle splitting.
- **Error boundaries**: Distinguish a real router 404 from a lazy-module or render failure. Runtime failures should offer a reload action so the browser can pick up the latest deployed asset set.
- **Context cancellation**: Always pass `r.Context()` to read operations

## Verify
Before considering this endpoint complete:
- [ ] Route uses Go 1.25 ServeMux pattern syntax
- [ ] Handler checks for nil dependencies and returns 503 when unavailable
- [ ] Errors use `respond.Error()` with proper status codes
- [ ] React component uses TanStack Query for data fetching
- [ ] Route is registered in both Go server and React router
- [ ] Navigation link works (if added to AppShell)
- [ ] 404 path tested (invalid ID returns error, not panic)

## Debug
If the endpoint doesn't work:
- Check route registration in `api/server.go` — handler name must match method signature
- Verify path parameter names match between Go (`{id}`) and React (`:id`)
- Test with curl: `curl http://localhost:8080/api/myfeature/123`
- Check browser console for React Query errors
- Verify Go handler is exported (capitalized function name)
- Check CORS if calling from different origin

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" if what's working/not built has changed
- [ ] Update `.mex/context/architecture.md` if this adds new component types
- [ ] If this task recurs without a pattern, create one in `.mex/patterns/`
