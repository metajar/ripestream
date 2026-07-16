---
name: add-graph-query
description: Adding a new graph query feature or topology analysis to the FalkorDB graph system. Use when extending topology exploration, path finding, or ASN analysis features.
triggers:
  - "graph query"
  - "topology"
  - "path"
  - "transit"
  - "cypher"
edges:
  - target: context/graph.md
    condition: when understanding the graph model, node/edge types, and query patterns
  - target: context/stack.md
    condition: when working with FalkorDB-specific features and configuration
  - target: context/conventions.md
    condition: when writing Go code for graph operations
  - target: patterns/add-api-endpoint.md
    condition: when creating an API endpoint for the graph query
  - target: context/alerts.md
    condition: when the query is used for alert evaluation
last_updated: 2025-01-16
---

# Add Graph Query Feature

## Context
FalkorDB stores IP/ASN topology with nodes (:Probe, :IP, :AS) and edges (:NEXT_HOP, :TRANSITS, :IN_AS, etc.). Queries use Cypher syntax. Reader interface in `internal/graph/` defines read operations.

## Steps

### 1. Define Reader Interface Method
Add method to `internal/graph/` Reader interface (usually in `queries_read.go`):

```go
type Reader interface {
    // ... existing methods ...

    // MyFeature queries the graph for ...
    MyFeature(ctx context.Context, param string) ([]MyFeatureResult, error)
}
```

### 2. Implement Graph Query
In `internal/graph/queries_read.go` or new file:

```go
func (s *Store) MyFeature(ctx context.Context, param string) ([]MyFeatureResult, error) {
    if err := ctx.Err(); err != nil {
        return nil, err
    }

    q := `
MATCH path = (start:AS {asn: $asn})-[:TRANSITS*]->(:AS)
RETURN path
LIMIT 100
`

    params := map[string]any{"asn": param}
    res, err := s.graph.Query(q, params, nil)
    if err != nil {
        return nil, fmt.Errorf("myfeature query: %w", err)
    }
    defer res.Close()

    var results []MyFeatureResult
    for res.Next() {
        var r MyFeatureResult
        if err := res.Scan(&r); err != nil {
            return nil, fmt.Errorf("scan: %w", err)
        }
        results = append(results, r)
    }
    if err := res.Err(); err != nil {
        return nil, fmt.Errorf("iterate: %w", err)
    }
    return results, nil
}
```

### 3. Define Result Type
Add struct for query results:

```go
type MyFeatureResult struct {
    ASN     int64  `db:"asn"`
    Org     string `db:"org"`
    // ... other fields
}
```

### 4. Add API Handler
Create handler in `internal/api/handlers_myfeature.go`:

```go
func (s *Server) myFeature(w http.ResponseWriter, r *http.Request) {
    if s.graph == nil {
        respond.Unavailable(w, "graph not enabled")
        return
    }

    param := r.PathValue("param")

    results, err := s.graph.MyFeature(r.Context(), param)
    if err != nil {
        respond.Error(w, "query failed", http.StatusInternalServerError, err)
        return
    }

    respond.JSON(w, results)
}
```

### 5. Register Route and Test
Add to `api/server.go` register() method and test with curl:
```bash
curl http://localhost:8080/api/myfeature/123
```

## Gotchas
- **Read timeout**: Complex queries can take 2-4s. Ensure ReadTimeout >= 10s in `graph/store.go`
- **Indexes required**: MERGE requires indexes on Probe.id, IP.addr, AS.asn. Create via EnsureIndexes()
- **Query limits**: Always LIMIT Cypher queries to prevent unbounded results
- **Context cancellation**: Check ctx.Err() before long-running queries
- **Result scanning**: Use proper struct tags and Scan() method. Field names must match query return
- **Path variable syntax**: Use `$param` syntax in Cypher, pass via params map

## Common Query Patterns

**AS-to-AS transit paths:**
```cypher
MATCH path = (a:AS {asn: $asnA})-[:TRANSITS*1..5]->(b:AS {asn: $asnB})
RETURN [n in nodes(path) | n.asn] AS as_path
```

**IPs with ASN enrichment:**
```cypher
MATCH (ip:IP {addr: $addr})
OPTIONAL MATCH (ip)-[:IN_AS]->(as:AS)
RETURN ip.addr, ip.asn, as.asn, as.org
```

**Hot hops by RTT:**
```cypher
MATCH (a:IP)-[e:NEXT_HOP]->(b:IP)
WHERE e.last_rtt_ms > $threshold
RETURN a.addr, b.addr, e.last_rtt_ms
ORDER BY e.last_rtt_ms DESC
LIMIT 100
```

## Verify
Before considering the graph query complete:
- [ ] Query uses proper index-backed filters (ASN, IP.addr, Probe.id)
- [ ] Cypher query has LIMIT clause
- [ ] Context cancellation checked before query execution
- [ ] Results properly scanned into struct with correct field tags
- [ ] Handler returns 503 when graph.Reader is nil
- [ ] Error handling uses fmt.Errorf with context
- [ ] Query tested with FalkorDB Browser UI (port 3000) first

## Debug
If graph queries fail:
- **Timeout errors**: Check ReadTimeout in graph/store.go (should be >= 10s)
- **No results**: Verify indexes exist with `CALL db.indexes()` in FalkorDB Browser
- **Syntax errors**: Test query in FalkorDB Browser UI (port 3000) first
- **Slow queries**: Use EXPLAIN in browser UI to check query plan
- **Connection errors**: Verify FalkorDB container is running (`docker compose ps falkordb`)
- **Scan errors**: Check struct field names match Cypher RETURN columns exactly

## Update Scaffold
- [ ] Update `.mex/ROUTER.md` "Current Project State" if new query type is working
- [ ] Update `.mex/context/graph.md` if adding new query patterns to common patterns section
- [ ] If this task recurs without a pattern, create one in `.mex/patterns/`
