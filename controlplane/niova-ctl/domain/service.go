package domain

// StitchTopology assembles flat PDU/Rack/Hypervisor/Device/Partition lists
// into the hierarchical tree the UI renders, with any hypervisor lacking a
// rack returned separately as "standalone". Pure and backend-neutral — both
// client packages call this rather than reimplementing the assembly.
func StitchTopology(pdus []PDU, racks []Rack, hvs []Hypervisor, devices []Device, partitions []DevicePartition) ([]PDU, []Rack, []Hypervisor) {
	// Stitch partitions into devices
	devMap := make(map[string]*Device, len(devices))
	for i := range devices {
		devices[i].Partitions = nil
		devMap[devices[i].ID] = &devices[i]
	}
	for _, p := range partitions {
		if d, ok := devMap[p.DevID]; ok {
			d.Partitions = append(d.Partitions, p)
		}
	}

	// Stitch devices into hypervisors
	hvMap := make(map[string]*Hypervisor, len(hvs))
	for i := range hvs {
		hvs[i].Dev = nil
		hvMap[hvs[i].ID] = &hvs[i]
	}
	for _, dev := range devices {
		if hv, ok := hvMap[dev.HypervisorID]; ok {
			hv.Dev = append(hv.Dev, dev)
		}
	}

	// Stitch hypervisors into racks
	rackMap := make(map[string]*Rack, len(racks))
	for i := range racks {
		racks[i].Hypervisors = nil
		rackMap[racks[i].ID] = &racks[i]
	}
	var standaloneHVs []Hypervisor
	for _, hv := range hvs {
		if r, ok := rackMap[hv.RackID]; ok {
			r.Hypervisors = append(r.Hypervisors, hv)
		} else {
			standaloneHVs = append(standaloneHVs, hv)
		}
	}

	// Stitch racks into PDUs
	pduMap := make(map[string]*PDU, len(pdus))
	for i := range pdus {
		pdus[i].Racks = nil
		pduMap[pdus[i].ID] = &pdus[i]
	}
	for _, rack := range racks {
		if p, ok := pduMap[rack.PDUID]; ok {
			p.Racks = append(p.Racks, rack)
		}
	}

	return pdus, racks, standaloneHVs
}

// FindHypervisorByUUID searches PDUs and Hypervisors for matching UUID
func FindHypervisorByUUID(pdus []PDU, standaloneHVs []Hypervisor, hvUUID string) (*Hypervisor, bool) {
	for i := range pdus {
		for j := range pdus[i].Racks {
			for k := range pdus[i].Racks[j].Hypervisors {
				if pdus[i].Racks[j].Hypervisors[k].ID == hvUUID {
					return &pdus[i].Racks[j].Hypervisors[k], true
				}
			}
		}
	}
	for i := range standaloneHVs {
		if standaloneHVs[i].ID == hvUUID {
			return &standaloneHVs[i], true
		}
	}
	return nil, false
}

// FindRackByUUID searches PDUs for matching Rack UUID
func FindRackByUUID(pdus []PDU, rackUUID string) (*Rack, bool) {
	for i := range pdus {
		for j := range pdus[i].Racks {
			if pdus[i].Racks[j].ID == rackUUID {
				return &pdus[i].Racks[j], true
			}
		}
	}
	return nil, false
}
