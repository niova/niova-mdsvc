package clictlplanefuncs

// placement_topology_test.go
//
// A declarative topology builder. The same builder produces:
//
//   - a *HierarchyModel (BuildModel) for pure, cluster-free unit tests, and
//   - a live registration (Register) that PUTs the identical NISDs to a running
//     control plane for integration tests.
//
// This is what lets one assertion library serve both worlds: the model the
// pure tests reason about is byte-for-byte what the integration tests register.
//
// Structural shape (balanced vs unbalanced) is expressed by how many times and
// with what fan-out addBranch is called. Capacity shape (equal vs different
// sized) is expressed by the per-device size function. Device addition for
// rebalance tests is just calling addBranch again on an existing builder.

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// topoBuilder accumulates NISDs across one or more branches (PDUs). It exposes
// small primitives (startPDU/addRack) so callers can build perfectly regular or
// arbitrarily irregular trees, and grow an existing PDU for rebalance tests.
type topoBuilder struct {
	prefix           string
	nisds            []NisdInfo
	pduIDs           []string
	rackN, hvN, devN int
}

func newTopo(prefix string) *topoBuilder { return &topoBuilder{prefix: prefix} }

// uniformSize returns a size function that gives every device the same per-NISD
// capacity (the equal-sized case).
func uniformSize(perNisd int64) func(devGlobalIdx int) int64 {
	return func(int) int64 { return perNisd }
}

// startPDU registers a fresh PDU and returns its id (a valid UUID, as the server
// validates PDU/Rack/HV failure-domain entries as UUIDs).
func (b *topoBuilder) startPDU() string {
	pdu := uuid.NewString()
	b.pduIDs = append(b.pduIDs, pdu)
	return pdu
}

// PrimaryPDU is the first PDU added; integration tests scope a vdev to it so the
// eligible NISD set is deterministic regardless of other tests' cluster state.
func (b *topoBuilder) PrimaryPDU() string { return b.pduIDs[0] }

// addRack appends one rack (with hvs hypervisors, each with devs devices, each
// with nisds NISDs) under an existing PDU. sizeFn is called once per device
// (keyed by a builder-wide device ordinal) and sets that device's NISD capacity.
func (b *topoBuilder) addRack(pdu string, hvs, devs, nisds int, sizeFn func(devGlobalIdx int) int64) {
	b.rackN++
	rack := uuid.NewString()
	for h := 0; h < hvs; h++ {
		b.hvN++
		hv := uuid.NewString()
		for d := 0; d < devs; d++ {
			devIdx := b.devN
			b.devN++
			dev := fmt.Sprintf("%s-dev-%05d", b.prefix, devIdx)
			size := sizeFn(devIdx)
			for n := 0; n < nisds; n++ {
				id := uuid.NewString()
				var keys [levelCount]string
				keys[LevelPDU] = pdu
				keys[LevelRack] = rack
				keys[LevelHV] = hv
				keys[LevelDevice] = dev
				keys[LevelNISD] = id
				b.nisds = append(b.nisds, NisdInfo{
					ID:            id,
					Partition:     fmt.Sprintf("%s-p%d", dev, n),
					Keys:          keys,
					TotalSize:     size,
					AvailableSize: size,
				})
			}
		}
	}
	time.Sleep(100 * time.Millisecond) // allow placement to settle before next addRack
}

// addBranch appends one full PDU subtree (racks identical racks). Repeated calls
// with identical fan-out build a balanced tree; differing fan-out builds an
// unbalanced one. Returns the new PDU id.
func (b *topoBuilder) addBranch(racks, hvs, devs, nisds int, sizeFn func(devGlobalIdx int) int64) string {
	pdu := b.startPDU()
	for r := 0; r < racks; r++ {
		b.addRack(pdu, hvs, devs, nisds, sizeFn)
	}
	return pdu
}

// BuildModel snapshots the accumulated NISDs into a HierarchyModel.
func (b *topoBuilder) BuildModel() *HierarchyModel {
	m := newHierarchyModel()
	for _, n := range b.nisds {
		m.add(n)
	}
	return m
}

// Register PUTs every accumulated NISD to the live cluster and returns the model
// rebuilt from what the server actually stored, so integration tests validate
// against the server's view, not the local one.
func (b *topoBuilder) Register(t *testing.T, c *CliCFuncs) *HierarchyModel {
	t.Helper()
	for i, n := range b.nisds {
		nisd := cpLib.Nisd{
			PeerPort: uint16(1024 + (i % 60000)),
			ID:       n.ID,
			FailureDomain: []string{
				n.Keys[LevelPDU], n.Keys[LevelRack], n.Keys[LevelHV],
				n.Keys[LevelDevice], n.Partition,
			},
			TotalSize:     n.TotalSize,
			AvailableSize: n.AvailableSize,
		}
		resp, err := c.PutNisd(&nisd)
		if assert.NoError(t, err, "PutNisd %s", n.ID) {
			assert.True(t, resp.Success)
		}
		time.Sleep(5 * time.Millisecond)
	}
	return modelFromCluster(t, c)
}

// ── Named topology presets ──────────────────────────────────────────────────
//
// These cover the structural/capacity axes of the test matrix. Each returns a
// builder; the caller decides whether to BuildModel (pure) or Register (live),
// and may keep adding branches afterwards (device-addition scenarios).

const gib = int64(1) << 30

// balancedEqual: symmetric tree, every NISD the same size. The control case for
// "a fair algorithm should distribute almost perfectly".
func balancedEqual(prefix string, pdus, racksPerPDU, hvsPerRack, devsPerHV, nisdsPerDev int, nisdSize int64) *topoBuilder {
	b := newTopo(prefix)
	for p := 0; p < pdus; p++ {
		b.addBranch(racksPerPDU, hvsPerRack, devsPerHV, nisdsPerDev, uniformSize(nisdSize))
	}
	return b
}

// balancedDifferentSized: symmetric tree, but one device per branch is bigSize
// while the rest are smallSize. Exercises proportional allocation and the
// "one very large device among many small" edge case at once.
func balancedDifferentSized(prefix string, pdus, racksPerPDU, hvsPerRack, devsPerHV, nisdsPerDev int, smallSize, bigSize int64) *topoBuilder {
	b := newTopo(prefix)
	devsPerBranch := racksPerPDU * hvsPerRack * devsPerHV
	for p := 0; p < pdus; p++ {
		base := p * devsPerBranch
		sizeFn := func(devGlobalIdx int) int64 {
			if devGlobalIdx == base { // first device of each branch is the big one
				return bigSize
			}
			return smallSize
		}
		b.addBranch(racksPerPDU, hvsPerRack, devsPerHV, nisdsPerDev, sizeFn)
	}
	return b
}

// unbalancedEqual: asymmetric tree (PDUs differ in rack/hv/device fan-out) with
// equal-sized NISDs, so fairness must be judged relatively (a small PDU should
// receive proportionally less, not an equal absolute count).
func unbalancedEqual(prefix string, nisdSize int64) *topoBuilder {
	b := newTopo(prefix)
	b.addBranch(3, 2, 4, 2, uniformSize(nisdSize)) // large PDU: 24 devices
	b.addBranch(2, 1, 3, 2, uniformSize(nisdSize)) // medium PDU: 6 devices
	b.addBranch(1, 1, 2, 1, uniformSize(nisdSize)) // tiny PDU: 2 devices
	return b
}

// unbalancedOnePDU: a single PDU whose racks have deliberately uneven fan-out
// (5, 2, then 1 device-bearing hypervisors). Scoping a vdev to this PDU keeps
// the eligible set deterministic on a shared cluster while still exercising the
// "unbalanced hierarchy" axis below the PDU level.
func unbalancedOnePDU(prefix string, nisdSize int64) *topoBuilder {
	b := newTopo(prefix)
	pdu := b.startPDU()
	b.addRack(pdu, 2, 3, 2, uniformSize(nisdSize)) // rack A: 2 hv x 3 dev
	b.addRack(pdu, 1, 2, 2, uniformSize(nisdSize)) // rack B: 1 hv x 2 dev
	b.addRack(pdu, 1, 1, 1, uniformSize(nisdSize)) // rack C: 1 hv x 1 dev
	return b
}

// unbalancedDifferentSizedOnePDU adds capacity skew on top of structural skew:
// the first device is bigSize, the rest smallSize. Stresses proportional
// allocation in the hardest topology.
func unbalancedDifferentSizedOnePDU(prefix string, smallSize, bigSize int64) *topoBuilder {
	b := newTopo(prefix)
	pdu := b.startPDU()
	first := true
	sizeFn := func(int) int64 {
		if first {
			first = false
			return bigSize
		}
		return smallSize
	}
	b.addRack(pdu, 2, 3, 2, sizeFn)
	b.addRack(pdu, 1, 2, 2, sizeFn)
	b.addRack(pdu, 1, 1, 1, sizeFn)
	return b
}
