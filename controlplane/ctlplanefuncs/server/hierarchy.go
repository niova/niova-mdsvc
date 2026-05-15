package srvctlplanefuncs

import (
	"fmt"

	"github.com/tidwall/btree"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// // DeviceNode wraps a DeviceAlloc for btree storage.
// // The Devices tree within an Entity is ordered by DeviceNode.ID.
// type DeviceNode struct {
// 	cpLib.DeviceAlloc
// }

// Entities maps a single entity to the set of Devices associated with it.
// Here the Entity could be a PDU/Rack/HV/Device/Partition.
// The Devices Tree holds all the device ptrs associated with the Entity.
type Entities struct {
	ID      string
	Devices *btree.BTreeG[*cpLib.DeviceAlloc]
}

// FailureDomain groups entities under a specific failure-isolation class.
// Type defines the domain category (pdu, rack, hv etc.).
// Tree stores entities ordered by their Entity.ID using a Counted B-tree.
type FailureDomain struct {
	Type uint8
	Tree *btree.BTreeG[*Entities]
}

// stores the Hierarchy Information
// Index 0 - PDU
// Index 1 - Rack
// Index 2 - HyperVisor
// Index 3 - Device
// Index 4 - Partition
type Hierarchy struct {
	FD []FailureDomain
}

var HR Hierarchy


func compareEntity(a, b *Entities) bool   { return a.ID < b.ID }
func compareNisd(a, b *cpLib.Nisd) bool    { return a.ID < b.ID }
func compareDevice(a, b *cpLib.DeviceAlloc) bool { return a.ID < b.ID }

// Initialize the Hierarchy Struct
func (hr *Hierarchy) Init() {
	hr.FD = make([]FailureDomain, cpLib.FD_MAX)
	for i := 0; i < cpLib.FD_MAX; i++ {
		hr.FD[i] = FailureDomain{
			Type: uint8(i),
			Tree: btree.NewBTreeG[*Entities](compareEntity),
		}
	}
}

func (fd *FailureDomain) getOrCreateEntity(id string) *Entities {
	e, ok := fd.Tree.Get(&Entities{ID: id})
	if ok {
		return e
	}
	n := Entities{
		ID:      id,
		Devices: btree.NewBTreeG[*cpLib.DeviceAlloc](compareDevice),
	}
	fd.Tree.Set(&n)
	return &n
}

func (fd *FailureDomain) deleteEmptyEntity(id string) {
	e, ok := fd.Tree.Get(&Entities{ID: id})
	if !ok {
		log.Trace("failed to find the entity in hierarchy tree: ", id)
		return
	}
	if e.Devices.Len() == 0 {
		fd.Tree.Delete(e)
		log.Tracef("deleting entity: %s, no devices available: ", e.ID)
	}
}

// getOrCreateDeviceInEntity finds or creates a DeviceNode within an entity.
func getOrCreateDeviceInEntity(ent *Entities, devID string) *cpLib.DeviceAlloc {
	dn, ok := ent.Devices.Get(&cpLib.DeviceAlloc{ID: devID})
	if ok {
		return dn
	}
	dn = &cpLib.DeviceAlloc{
		ID:    devID,
		Nisds: make([]*cpLib.Nisd, 0),
	}
	ent.Devices.Set(dn)
	return dn
}

// addNisdToDevice adds a NISD to a DeviceNode and recalculates AvailableSize.
func addNisdToDevice(dn *cpLib.DeviceAlloc, n *cpLib.Nisd) {
	// Check if NISD already exists; if so, update it.
	for i, existing := range dn.Nisds {
		if existing.ID == n.ID {
			dn.Nisds[i] = n
			recalcDeviceAvailSize(dn)
			return
		}
	}
	dn.Nisds = append(dn.Nisds, n)
	recalcDeviceAvailSize(dn)
}

// removeNisdFromDevice removes a NISD from a DeviceNode and recalculates AvailableSize.
// Returns true if the device is now empty (no NISDs left).
func removeNisdFromDevice(dn *cpLib.DeviceAlloc, nisdID string) bool {
	for i, existing := range dn.Nisds {
		if existing.ID == nisdID {
			dn.Nisds = append(dn.Nisds[:i], dn.Nisds[i+1:]...)
			recalcDeviceAvailSize(dn)
			return len(dn.Nisds) == 0
		}
	}
	return len(dn.Nisds) == 0
}

func recalcDeviceAvailSize(dn *cpLib.DeviceAlloc) {
	var total int64
	for _, n := range dn.Nisds {
		total += n.AvailableSize
	}
	dn.AvailableSize = total
}

// Add NISD and the corresponding parent entities to the Hierarchy
func (hr *Hierarchy) AddNisd(n *cpLib.Nisd) error {
	if n.AvailableSize < cpLib.CHUNK_SIZE {
		log.Debugf("skipping nisd addition %s, as the available size %f GB < 8GB", n.ID, BytesToGB(n.AvailableSize))
	}
	if len(n.FailureDomain) != cpLib.FD_MAX {
		err := fmt.Errorf("failed to add nisd: not enough failure domain info")
		log.Error(err)
		return err
	}

	devID := n.FailureDomain[cpLib.DEVICE_IDX]

	for i, id := range n.FailureDomain {
		e := hr.FD[i].getOrCreateEntity(id)
		dn := getOrCreateDeviceInEntity(e, devID)
		addNisdToDevice(dn, n)
	}
	return nil
}

// Delete NISD and the corresponding parent entities from the Hierarchy
func (hr *Hierarchy) DeleteNisd(n *cpLib.Nisd) {
	devID := n.FailureDomain[cpLib.DEVICE_IDX]

	for i, id := range n.FailureDomain {
		fd := &hr.FD[i]
		e, ok := fd.Tree.Get(&Entities{ID: id})
		if !ok {
			continue
		}

		dn, ok := e.Devices.Get(&cpLib.DeviceAlloc{ID: devID})
		if !ok {
			continue
		}

		log.Tracef("deleting nisd %s from device %s in fd %d", n.ID, devID, i)
		empty := removeNisdFromDevice(dn, n.ID)
		if empty {
			e.Devices.Delete(dn)
		}
		fd.deleteEmptyEntity(id)
	}
}

// Get the Failure Domain Based on the Nisd Count
func (hr *Hierarchy) GetFDLevel(ft int) (int, error) {
	for i := 0; i <= cpLib.DEVICE_IDX; i++ {
		if ft <= hr.FD[i].Tree.Len() {
			log.Debug("selecting fd: ", i)
			return i, nil
		}
	}
	return -1, fmt.Errorf("failed to allocate vdev as the fault tolerance level %d cannot be met", ft)
}

// Get the total number of elements in a Failure Domain.
func (hr *Hierarchy) GetEntityCnt(tierIDX int) int {
	if tierIDX >= len(hr.FD) {
		return 0
	}
	return hr.FD[tierIDX].Tree.Len()
}

func (hr *Hierarchy) LookupNAddNisd(nisd *ctlplfl.Nisd, nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc]) *ctlplfl.NisdVdevAlloc {
	if nisdPtr, ok := nisdMap.Get(nisd.ID); ok {
		return nisdPtr
	}

	ns := &ctlplfl.NisdVdevAlloc{
		AvailableSize: nisd.AvailableSize,
		Ptr:           nisd,
	}

	nisdMap.Set(nisd.ID, ns)

	log.Debugf(
		"selected Nisd %s with available size %d for Vdev allocation",
		nisd.ID,
		nisd.AvailableSize,
	)

	return ns
}

// PickDevice selects a device within an entity using round-robin rotation.
// startOffset controls which device position to begin scanning from, enabling
// cross-vdev rotation so consecutive vdevs pick different devices for the same chunk.
// Within a chunk, it prefers devices not yet picked; if all are picked, it falls
// back to the least-used device.
func (hr *Hierarchy) PickDevice(ent *Entities, pickedDevices map[string]struct{},
	deviceUsage map[string]int, nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc],
	startOffset int) (*cpLib.DeviceAlloc, error) {

	devCnt := ent.Devices.Len()
	if devCnt == 0 {
		return nil, fmt.Errorf("no devices in entity %s", ent.ID)
	}

	// Collect eligible devices (those with enough available size)
	eligible := make([]*cpLib.DeviceAlloc, 0, devCnt)
	ent.Devices.Scan(func(dn *cpLib.DeviceAlloc) bool {
		effectiveSize := hr.effectiveDeviceAvailSize(dn, nisdMap)
		if effectiveSize >= ctlplfl.CHUNK_SIZE {
			eligible = append(eligible, dn)
		}
		return true
	})

	if len(eligible) == 0 {
		return nil, fmt.Errorf("no suitable device found in entity %s", ent.ID)
	}

	numEligible := len(eligible)
	offset := startOffset % numEligible

	// First pass: pick first unpicked device starting from offset
	for i := 0; i < numEligible; i++ {
		idx := (offset + i) % numEligible
		dn := eligible[idx]
		if _, picked := pickedDevices[dn.ID]; !picked {
			return dn, nil
		}
	}

	// Fallback: all eligible devices already picked, pick least-used starting from offset
	var bestFallback *cpLib.DeviceAlloc
	bestFallbackUsage := int(^uint(0) >> 1) // max int
	for i := 0; i < numEligible; i++ {
		idx := (offset + i) % numEligible
		dn := eligible[idx]
		usage := deviceUsage[dn.ID]
		if usage < bestFallbackUsage {
			bestFallback = dn
			bestFallbackUsage = usage
		}
	}

	if bestFallback != nil {
		return bestFallback, nil
	}
	return nil, fmt.Errorf("no suitable device found in entity %s", ent.ID)
}

// effectiveDeviceAvailSize returns the device's available size accounting for
// any pending NISD allocations tracked in nisdMap.
func (hr *Hierarchy) effectiveDeviceAvailSize(dn *cpLib.DeviceAlloc, nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc]) int64 {
	var total int64
	for _, nisd := range dn.Nisds {
		if alloc, ok := nisdMap.Get(nisd.ID); ok {
			total += alloc.AvailableSize
		} else {
			total += nisd.AvailableSize
		}
	}
	return total
}

// PickNISDFromDevice selects the optimal NISD within a device.
// Returns the NISD with the highest available size that hasn't been picked yet.
func (hr *Hierarchy) PickNISDFromDevice(dn *cpLib.DeviceAlloc, pickedNISD map[string]struct{},
	nisdMap *btree.Map[string, *ctlplfl.NisdVdevAlloc]) (*cpLib.NisdVdevAlloc, error) {

	var optimalNisd *cpLib.NisdVdevAlloc

	for _, nisd := range dn.Nisds {
		nAlloc := hr.LookupNAddNisd(nisd, nisdMap)

		// skip if already picked
		if _, exists := pickedNISD[nAlloc.Ptr.ID]; exists {
			continue
		}

		// must have at least one chunk
		if nAlloc.AvailableSize < ctlplfl.CHUNK_SIZE {
			continue
		}

		if optimalNisd == nil || nAlloc.AvailableSize > optimalNisd.AvailableSize {
			optimalNisd = nAlloc
		}
	}

	if optimalNisd == nil {
		return nil, fmt.Errorf("no suitable nisd found in device %s", dn.ID)
	}

	// commit selection
	optimalNisd.AvailableSize -= ctlplfl.CHUNK_SIZE
	pickedNISD[optimalNisd.Ptr.ID] = struct{}{}

	return optimalNisd, nil
}

func BytesToGB(b int64) float64 {
	const gb = 1024 * 1024 * 1024
	return float64(b) / float64(gb)
}

func (hr *Hierarchy) Dump() {
	if hr.FD == nil {
		log.Errorf("hierarchy Structure not initialized")
		return
	}

	for i := range hr.FD {
		fd := &hr.FD[i]
		log.Debugf("FD Type: %d", fd.Type)

		fd.Tree.Scan(func(ent *Entities) bool {
			if ent == nil {
				return true
			}

			log.Debugf("  Entity: %s", ent.ID)

			ent.Devices.Scan(func(dn *cpLib.DeviceAlloc) bool {
				if dn == nil {
					return true
				}
				log.Debugf("    Device: %s: %f GB", dn.ID, BytesToGB(dn.AvailableSize))
				for _, n := range dn.Nisds {
					log.Debugf("      Nisd: %s: %f GB", n.ID, BytesToGB(n.AvailableSize))
				}
				return true
			})

			return true
		})
	}
}

func GetIdxForNisdAlloc(hash uint64, size int) (int, error) {
	if size <= 0 {
		return 0, fmt.Errorf("invalid size: %d", size)
	}
	return int(hash % uint64(size)), nil
}

func GetEntityByID(ft cpLib.Filter) (*Entities, error) {
	if ft.Type == -1 {
		return nil, fmt.Errorf("invalid fd level")
	}
	ent, ok := HR.FD[cpLib.GetFDIdx(ft.Type)].Tree.Get(&Entities{ID: ft.ID})
	if !ok || ent == nil {
		return nil, fmt.Errorf("entityID %s not found in fd %d", ft.ID, cpLib.GetFDIdx(ft.Type))
	}

	return ent, nil
}

func (hr *Hierarchy) GetNisdByPDUID(pduID, nisdID string) (*cpLib.Nisd, error) {
	// FD[0] corresponds to PDU failure domain
	pduEntity, ok := hr.FD[cpLib.PDU_IDX].Tree.Get(&Entities{ID: pduID})
	if !ok {
		return nil, fmt.Errorf("PDU %s not found", pduID)
	}

	// Search through all devices within the PDU entity
	var found *cpLib.Nisd
	pduEntity.Devices.Scan(func(dn *cpLib.DeviceAlloc) bool {
		for _, n := range dn.Nisds {
			if n.ID == nisdID {
				found = n
				return false // stop scanning
			}
		}
		return true
	})

	if found == nil {
		return nil, fmt.Errorf("nisd %s not found in PDU %s", nisdID, pduID)
	}
	return found, nil
}
