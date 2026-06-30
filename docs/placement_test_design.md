# Placement Algorithm Test Suite — Design

A minimal, non-redundant, reusable test suite for the Vdev chunk-placement
algorithm. It is built so that **every metric and assertion is a pure reduction
over a single source of truth** (the Chunk Placement Matrix), which makes the
checks algorithm-agnostic: any future placement algorithm can be held to the
same contract without changing a line of the validation library.

> Package: `controlplane/ctlplanefuncs/client` (Go test files, `_test.go`).
> Running the integration layer requires a live control plane — see the
> `niova-mdsvc-testing-guide` skill. The statistics + validation layers are pure
> and run with no cluster.

---

## 1. Files

| File | Layer | Cluster needed | Purpose |
|------|-------|----------------|---------|
| `placement_stats_test.go` | statistics | no | `DistStats`, `ComputeStats`, `FairnessThresholds`. Self-verified by `TestPlacementStats`. |
| `placement_model_test.go` | model | no | `HierarchyModel`, `PlacementRow` (the matrix), `CountsByLevel` reduction. |
| `placement_topology_test.go` | fixtures | no (build) / yes (register) | Declarative topology builder → identical in-memory model *or* live NISD registration. |
| `placement_asserts_test.go` | validation | no | The reusable `eval*`/`Assert*` helpers. Self-verified by `TestPlacementValidationHelpers`. |
| `placement_report_test.go` | reporting | no | The 8 required output tables, all pure functions of the matrix. |
| `placement_distribution_test.go` | integration | yes | The test matrix: `TestPlacementDistribution`, `TestPlacementRebalance`, `TestPlacementEdgeCases`. |

**Two tests prove the test infrastructure itself** (no cluster, run anywhere):

```bash
# from controlplane/ctlplanefuncs/client
RAFT_ID=dummy GOSSIP_NODES_PATH=dummy go test -run 'TestPlacementStats|TestPlacementValidationHelpers' -v
```

`TestPlacementValidationHelpers` feeds hand-built good/bad distributions into
every validator and asserts good passes and bad fails — so a green run means the
fairness math and the assertions are trustworthy before they ever touch the
algorithm.

---

## 2. Core idea: one source of truth

```
HierarchyModel (PDU > Rack > Hypervisor > Device > NISD, + capacity)
        +
[]PlacementRow   ← one row per placement = the "Chunk Placement Matrix"
        |
        |  CountsByLevel(rows, level, typeClass, eligible)   ← the only reduction
        v
   map[entity]count ──► ComputeStats ──► DistStats ──► thresholds ──► pass/fail
                    └──► reports (§5)
```

Everything — fairness, diversity, capacity, proportionality, parity, rebalance,
and all eight report tables — derives from those two structures. There is no
second code path that could disagree with the reports.

`TypeClass` selects which placements a metric counts:
`ClassData` (Data **and** Replica — the payload), `ClassParity` (EC parity only),
`ClassAll`. Folding Replica into "data" is what lets one set of helpers serve
both replicated and EC vdevs.

**Capacity model:** each placement — replica, EC data block, or EC parity block —
occupies one full 8 GB chunk (`CHUNK_SIZE`) on its NISD. Allocation = (placements
on entity) × `CHUNK_SIZE`.

---

## 3. The test matrix (smallest set, maximum coverage)

Each topology scenario runs the **standard vdev set** — all six required vdev
configurations in one pass, so the expensive hierarchy setup is shared:

`replica3`, `single-replica`, `ec4p2`, `ec6p3 + PFS`, `replica3 + PFS`,
`single-replica + PFS`.

| # | Test / sub-test | Topology | Capacity | Why it exists (unique behaviour) |
|---|-----------------|----------|----------|----------------------------------|
| 1 | `TestPlacementDistribution/balanced-equal` | balanced | equal | Control: near-perfect even spread is *achievable*; held to the **strict** bar. |
| 2 | `…/balanced-different-sized` | balanced | one 3× device/rack | Allocation must track **capacity**, not entity count (rule 17). |
| 3 | `…/unbalanced-equal` | irregular fan-out | equal | Device/NISD evenness holds while rack/HV totals scale with fan-out (rules 7–10, 21). |
| 4 | `…/unbalanced-different-sized` | irregular fan-out | skewed | Proportional allocation under **combined** structural + capacity asymmetry (hardest case). |
| 5 | `TestPlacementRebalance/balanced` | balanced + device add | equal | Minimal, targeted movement onto new capacity (rules 23–25). |
| 6 | `TestPlacementRebalance/unbalanced` | irregular + device add | equal | Rebalance correctness when the base tree is already skewed. |
| 7 | `TestPlacementEdgeCases/*` | degenerate | mixed | Boundary topologies where bias hides (see §6). |

Rows 1–4 are the four (balanced|unbalanced) × (equal|different) cells; rows 5–6
are the two device-addition cells. That is the entire required matrix in **6
parameterised cases + 1 edge-case table** — no scenario duplicates another's
unique behaviour.

### Deterministic isolation on a shared cluster

The control plane persists NISDs across tests, and the algorithm picks from the
*global* NISD pool. So each scenario:

1. registers its own topology with **fresh UUIDs**, and
2. creates every vdev with a `Filter{FD_PDU, <its PDU>}`,

confining placement to its own subtree. The eligible set is then
`model.Scoped(LevelPDU, pdu)` — fully deterministic regardless of what other
tests left behind. (Consequence: PDU-level *fairness* is not exercised by the
scoped matrix; PDU-level *diversity* still is, via multi-block chunks.)

---

## 4. Validation rules → helper mapping

All 25 rules are covered by **one dozen parameterised helpers**. Genericity over
`(Level, TypeClass)` is what removes redundancy.

| Rules | Helper | Enforcement |
|-------|--------|-------------|
| — (structure) | `evalVdevStructure` | hard |
| 1 (unique NISD/chunk) | `evalChunkUniqueness` | hard |
| 1–2 (max unique NISD, Device) | `evalChunkDiversity` @ NISD, Device | hard |
| 3–5 (max unique HV, Rack, PDU "when possible") | `evalChunkDiversity` @ HV, Rack, PDU | advisory |
| 6–10 (even data across PDU…NISD) | `evalFairness(ClassData, lvl)` | advisory* |
| 11–15 (even parity across PDU…NISD) | `evalFairness(ClassParity, lvl)` | advisory* |
| 16, 18 (respect / never exceed capacity) | `evalCapacityCompliance` | hard |
| 17 (proportional to capacity) | `evalProportional` | advisory* |
| 19–20 (no repeated Device/NISD) | `evalFairness(strict)` = `AssertNoBias` | advisory* |
| 21–22 (hierarchy bias, deterministic patterns) | `evalFairness` @ Rack/HV + `evalProportional` | advisory* |
| 23 (preserve balance after add) | `evalFairness` on post-add matrix | advisory* |
| 24–25 (minimal / non-excessive movement) | `evalRebalance` | hard (in rebalance test) |

\* *advisory by default* — logged as `[ADVISORY …]`, not failed. Set the package
var `enforcePlacementStats = true` to make them hard once an algorithm is known
to satisfy the thresholds, turning the suite into a regression guard. The split
exists because **uniqueness/capacity are correctness floors** every algorithm
must meet, whereas **statistical evenness is a quality target** whose bar depends
on the algorithm's sophistication; shipping them as hard-fail on an unknown
algorithm would only produce uninformative red.

---

## 5. Required output data (all derived from the matrix)

`renderFullReport` emits, and each is an independent pure function for reuse:

1. **Chunk Placement Matrix** (`reportChunkMatrix`) — PFS, Vdev, Chunk, Type,
   Size, Seq, PDU, Rack, HV, Device, NISD. The source of truth.
2. **Vdev Placement Matrix** (`reportVdevMatrix`) — per vdev, each chunk and its
   placements.
3. **Hierarchy Distribution Summary** (`reportHierarchyDistribution`) — data /
   parity / total per entity at every level.
4. **Capacity Summary** (`reportCapacity`) — total, available, allocated, free,
   util %, expected share, actual share, per Device and NISD.
5. **Distribution Statistics** (`reportStatistics`) — mean, min, max, range,
   variance, std-dev, CV, Jain, max/min, skew, for each level × {data, parity,
   combined}.
6. **Diversity Metrics** (`reportDiversity`) — per vdev, unique used / theoretical
   max at each level.
7. **Parity Distribution** (`reportParityDistribution`) — parity counts per level.
8. **Rebalance Metrics** (`reportRebalance`) — chunks/bytes moved, movement %,
   onto-new %.

---

## 6. Edge cases (`TestPlacementEdgeCases`)

Each is intentionally tiny and asserts only the invariants that must survive the
boundary — which is exactly where placement bias surfaces.

| Case | Exposes |
|------|---------|
| single device / single NISD | the no-diversity floor still places every chunk |
| single rack, many devices | rack/HV diversity impossible; device diversity must hold |
| more chunks than NISDs | forced reuse must stay **even** (rules 10, 20) |
| more NISDs than chunks | must not pile onto a subset (bias detection, rule 19) |
| stripe wider than failure domains | infeasible request must be **rejected**, not silently doubled up |

Further bias-exposing topologies (single PDU/HV, one huge device among small,
highly asymmetric trees, repeated additions, replica-count ≈ device-count) are
expressible by composing the existing topology presets and adding a row here —
no new infrastructure needed.

---

## 7. Statistical acceptance criteria & thresholds

Defined in `FairnessThresholds`. Two presets, with rationale:

### `EqualSizedThresholds()` — strict (equal-capacity entities)

| Metric | Bound | Why |
|--------|-------|-----|
| Jain's fairness index | ≥ 0.95 | A single idle entity out of ~20 already drops Jain below 0.95, so this catches "one box never chosen". |
| Coefficient of variation | ≤ 0.10 | ≤ 10 % relative spread. CV is scale-free, so it is comparable across levels of different cardinality. |
| Max/Min ratio | ≤ 1.50 | Busiest entity ≤ 1.5× the quietest. Not 1.0, because small counts force off-by-one gaps. |
| \|Skew\| | ≤ 0.75 | Rules out a long one-sided tail of hot entities. |

### `GeneralThresholds()` — relaxed (unbalanced / small counts)

| Metric | Bound | Why |
|--------|-------|-----|
| Jain | ≥ 0.85 | Tolerates the lumpiness an irregular tree forces, still fails on 2× over-share. |
| CV | ≤ 0.25 | — |
| Max/Min | ≤ 2.0 | — |
| \|Skew\| | ≤ 1.5 | — |

**Other criteria**

- **Capacity deviation (proportional):** |actual share − expected share| ≤ **0.10–0.15**
  absolute (`propTol`). Loosened to 0.15 for the unbalanced+skewed case because
  fewer placements per entity make exact proportionality impossible.
- **Capacity compliance:** allocated ≤ available — **hard**, no tolerance.
- **Rebalance movement %:** within `optimalPct × (1 ± slack)`, where
  `optimalPct = newNISDs / totalNISDs` is the theoretical-minimum fraction that
  must move onto new capacity; `slack = 0.6` absorbs rounding at small chunk
  counts. Moved blocks must land on new capacity ≥ 90 %.

**Important property** (proven by `TestPlacementStats`): CV/Jain are scale-free,
so the *same* ±1 spread that fails the strict bar at counts of ~5 passes it at
counts of ~10. The integration scenarios therefore size vdevs to give each
entity several placements before the strict bar is applied. This is why "few
placements" is treated as inherently noisy rather than unfair.

---

## 8. Running the tests & generating reports

### Pure / no-cluster tests (run anywhere)

These tests exercise the statistics math and the validation helpers with no live
control plane. They run in the package directory with dummy env vars:

```bash
cd controlplane/ctlplanefuncs/client

RAFT_ID=dummy GOSSIP_NODES_PATH=dummy \
  go test -run 'TestPlacementStats|TestPlacementValidationHelpers' -v
```

A green run confirms that fairness math and all `Assert*`/`eval*` helpers are
working before touching the algorithm.

### Integration tests (require a running control plane)

Set the two required env vars to point at a live cluster, then run any subset:

```bash
export RAFT_ID=<raft-uuid>
export GOSSIP_NODES_PATH=<path/to/gossip.json>

# Optionally disable auth checks (default: auth enabled)
export AUTH_ENABLED=false

cd controlplane/ctlplanefuncs/client

# Full test matrix (all four topology × capacity scenarios)
go test -run TestPlacementDistribution -v -timeout 10m

# Device-addition / rebalance scenarios
go test -run TestPlacementRebalance -v -timeout 10m

# Degenerate-topology edge cases
go test -run TestPlacementEdgeCases -v -timeout 10m

# Run everything
go test ./... -v -timeout 20m
```

Each scenario prints a `PURPOSE` / `VALIDATES` line and the full eight-table
report (§5) via `t.Logf`. Capture it with `go test … -v 2>&1 | tee run.log`.

#### Turning advisory checks into hard failures

By default, statistical-quality checks (fairness, proportionality, parity
balance) are logged as `[ADVISORY …]` and never fail the test. Once you are
confident the algorithm meets the thresholds, set the package var in
[placement_distribution_test.go](../controlplane/ctlplanefuncs/client/placement_distribution_test.go):

```go
var enforcePlacementStats = true
```

and re-run. This turns the suite into a regression guard.

### Generating the CSV distribution report

`TestGenerateDistributionReport` and `TestECDistributionReport` (in
[distribution_report_client_test.go](../controlplane/ctlplanefuncs/client/distribution_report_client_test.go))
write a multi-section CSV to `./distribution_report.csv` (relative to the
package directory). Requires a live cluster:

```bash
cd controlplane/ctlplanefuncs/client

go test -run 'TestGenerateDistributionReport|TestECDistributionReport' \
  -v -timeout 10m 2>&1 | tee report_run.log

# The CSV is written to:
ls -lh ./distribution_report.csv
```

The CSV contains three sections: Device Distribution Matrix, NISD Distribution
Matrix, and per-scope standard-deviation summary. Open it in a spreadsheet or
process with `csvkit` / `pandas`.

---

## 9. Extending to a new placement algorithm

1. Nothing in `placement_stats/model/asserts/report` is algorithm-specific —
   reuse as-is.
2. Add or adjust `vdevPlan`s / topology presets if the new algorithm has new
   knobs.
3. Run advisory-mode first; read the `[ADVISORY …]` lines and the statistics
   report to learn the algorithm's actual behaviour.
4. Once it meets the bar, set `enforcePlacementStats = true` to lock it in.

The contract a new algorithm must satisfy to pass the **hard** floor: correct
chunk/block structure, no duplicate NISD per chunk, maximal NISD/Device
diversity per chunk, never exceed NISD capacity, reject infeasible stripes, and
(if it rebalances) move only minimally onto new capacity.
