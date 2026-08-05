# Archery MCP API Exposure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ensure all five Archery MCP capabilities call Archery HTTP APIs, publish and expose the seeded chain, and verify the complete path.

**Architecture:** Keep `archeryclient.Client` as the only integration boundary. Schema actions use Archery resource endpoints, query uses `/query/`, and the MCP handler invokes the published rule chain without any direct business-database connection from BaboFlow. Improve errors only at the Archery client/node boundary and make the seeded chain's publication/exposure explicit and idempotent.

**Tech Stack:** Go, RuleGo, Gin, GORM, mcp-go, Docker Compose, Go tests, curl.

## Global Constraints

- BaboFlow must not connect directly to Archery-managed business databases.
- `instanceId` remains externally passable and must be tenant-safe.
- Only read-only SQL is allowed for the MCP query action.
- Existing unrelated working-tree changes must not be reverted.

---

### Task 1: Lock API-only behavior with tests

**Files:**
- Modify: `internal/biz/rulegokit/nodes/archery_node_test.go`
- Modify: `internal/biz/rulegokit/archeryclient/client_test.go`

- [ ] Add tests proving the query client sends `POST /query/` and the schema client sends `GET /instance/instance_resource/`.
- [ ] Add a regression assertion that Archery server-side datasource errors are returned with an Archery-specific context and are not converted into local database operations.
- [ ] Run the focused tests and confirm the new assertions fail before implementation changes.

### Task 2: Clarify and harden Archery API error handling

**Files:**
- Modify: `internal/biz/rulegokit/archeryclient/client.go`
- Modify: `internal/biz/rulegokit/nodes/archery_query_node.go`
- Modify: `internal/biz/rulegokit/nodes/archery_schema_node.go`

- [ ] Preserve the existing Archery API endpoints and request parameters.
- [ ] Wrap transport, HTTP, response-status, and datasource errors with operation context such as `Archery /query/` or `Archery /instance/instance_resource/`.
- [ ] Keep SQL execution delegated to Archery and retain read-only validation.
- [ ] Run focused client and node tests until green.

### Task 3: Publish and expose the seeded chain safely

**Files:**
- Modify: `internal/data/seed.go`
- Modify: `internal/biz/mcp.go`
- Modify: `internal/biz/rulechain.go`
- Modify: `internal/data/seed_test.go`
- Modify: `internal/biz/mcp_test.go` or the existing MCP test file

- [ ] Make the seeded `chain-archery-mcp-query` available to the published-chain loader.
- [ ] Add an idempotent MCP exposure named `archery_mcp_query` using the chain input schema.
- [ ] Ensure startup/reseed does not create duplicate exposures or duplicate registered MCP tools.
- [ ] Keep exposure protected by the existing MCP session/Bearer authentication.
- [ ] Add tests for publication state, exposure persistence, and duplicate prevention.

### Task 4: Verify live MCP behavior

**Files:**
- No production file changes unless a test exposes a concrete defect.

- [ ] Rebuild and start Docker Compose.
- [ ] Authenticate through `/api/v1/auth/login`.
- [ ] Verify the chain is published and `/api/v1/mcp/exposures` contains `archery_mcp_query`.
- [ ] Exercise MCP `tools/list` and `tools/call` for all five actions.
- [ ] Verify missing fields, unknown actions, unknown instances, and non-read-only SQL fail safely.
- [ ] Record Archery datasource DNS failures separately from BaboFlow implementation failures.

### Task 5: Run regression verification

- [ ] Run `go test ./...`.
- [ ] Run frontend tests and build.
- [ ] Run `openspec validate "add-archery-mcp-query-chain" --strict`.
- [ ] Run `docker compose logs --no-color --tail=200 baboflow` and confirm no startup panic or registration error.
