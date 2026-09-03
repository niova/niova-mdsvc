package domain

import (
	"fmt"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// Type aliases for control plane library types
type Device = ctlplfl.Device
type DevicePartition = ctlplfl.DevicePartition
type PDU = ctlplfl.PDU
type Rack = ctlplfl.Rack
type Hypervisor = ctlplfl.Hypervisor
type Vdev = ctlplfl.Vdev
type VdevConfig = ctlplfl.VdevConfig
type PFS = ctlplfl.PFS
type Nisd = ctlplfl.Nisd

// DevicePartitionInfo holds information about a partition
type DevicePartitionInfo struct {
	Name string
	// Path is the kernel block device node (e.g. "/dev/nvme3n1p1"), what a
	// NISD actually opens. Name is the /dev/disk/by-id identifier used as
	// the stable PartitionID instead, since a raw kernel name isn't stable
	// across reboots.
	Path string
	Size int64
}

// PartitionableDevice pairs a Device with its owning Hypervisor, so a
// caller that needs to SSH in (Device only carries a HypervisorID string)
// doesn't have to re-resolve it. Includes every device regardless of how
// many partitions it already has registered — a device with 2 of its 10
// physical partitions registered still has 8 worth discovering, and
// whether it has any at all can only be answered by actually SSHing in and
// looking (see PartitionDiscoveryPage), not from this CP-only data. That
// per-partition already-registered state is annotated one level down.
type PartitionableDevice struct {
	Device     Device
	Hypervisor Hypervisor
}

// BuildPartitionableDevices walks pdus/standaloneHVs and returns every
// device paired with its hypervisor, for the partition-discovery device
// picker.
func BuildPartitionableDevices(pdus []PDU, standaloneHVs []Hypervisor) []PartitionableDevice {
	var out []PartitionableDevice
	addHV := func(hv Hypervisor) {
		for _, dev := range hv.Dev {
			out = append(out, PartitionableDevice{Device: dev, Hypervisor: hv})
		}
	}
	for _, p := range pdus {
		for _, r := range p.Racks {
			for _, hv := range r.Hypervisors {
				addHV(hv)
			}
		}
	}
	for _, hv := range standaloneHVs {
		addHV(hv)
	}
	return out
}

// NISDTarget is one selectable "where can this NISD live" option: a
// Partition on backends that have one (niova-mdsvc), or a bare Device on
// backends that don't (mdsvc-tidb). FailureDomain carries the full
// ancestry so the submit handler never has to re-walk the topology.
type NISDTarget struct {
	Label         string
	TargetID      string
	FailureDomain []string
	NISDUUID      string
	HypervisorID  string
	Size          int64
}

// BuildNISDTargets returns one NISDTarget per Partition (usePartitions
// true) or per Device (false) found across pdus and standaloneHVs.
// Standalone hypervisors have no PDU/Rack ancestry, so those FailureDomain
// slots are left empty for them.
func BuildNISDTargets(pdus []PDU, standaloneHVs []Hypervisor, usePartitions bool) []NISDTarget {
	var targets []NISDTarget
	addHV := func(pduID, rackID string, hv Hypervisor) {
		for _, dev := range hv.Dev {
			if usePartitions {
				for _, part := range dev.Partitions {
					targets = append(targets, NISDTarget{
						Label:         fmt.Sprintf("%s (Partition: %s)", part.PartitionPath, part.PartitionID),
						TargetID:      part.PartitionID,
						FailureDomain: []string{pduID, rackID, hv.ID, dev.ID, part.PartitionID},
						NISDUUID:      part.NISDUUID,
						HypervisorID:  hv.ID,
						Size:          part.Size,
					})
				}
			} else {
				targets = append(targets, NISDTarget{
					Label:         fmt.Sprintf("%s (Device: %s)", dev.DevicePath, dev.ID),
					TargetID:      dev.ID,
					FailureDomain: []string{pduID, rackID, hv.ID, dev.ID},
					HypervisorID:  hv.ID,
					Size:          dev.Size,
				})
			}
		}
	}
	for _, p := range pdus {
		for _, r := range p.Racks {
			for _, hv := range r.Hypervisors {
				addHV(p.ID, r.ID, hv)
			}
		}
	}
	for _, hv := range standaloneHVs {
		addHV("", "", hv)
	}
	return targets
}

// UsedNISDTargetIDs returns the set of NISDTarget.TargetID values that
// already have a NISD registered against them — a NISD's FailureDomain
// always ends in the ID of the target (partition or device) it was created
// from, the same TargetID BuildNISDTargets produces, so this is a plain
// set-membership check with no extra data needed beyond the NISD list
// already fetched into cache.
func UsedNISDTargetIDs(nisds []Nisd) map[string]bool {
	used := make(map[string]bool, len(nisds))
	for _, n := range nisds {
		if len(n.FailureDomain) == 0 {
			continue
		}
		used[n.FailureDomain[len(n.FailureDomain)-1]] = true
	}
	return used
}

// NISDIDSet returns the set of IDs belonging to NISDs that actually exist.
// A partition's own NISDUUID field alone can't be trusted to mean "this
// already has a NISD" — it's also how an operator pre-assigns a future
// NISD's UUID via Edit Partition before that NISD is created (main.go's
// original workflow). Checking the UUID against this set tells "really
// created" apart from "pre-planned but not yet created" — and, unlike
// UsedNISDTargetIDs, doesn't depend on FailureDomain carrying
// partition-level detail, which mdsvc-tidb's schema doesn't preserve.
func NISDIDSet(nisds []Nisd) map[string]bool {
	ids := make(map[string]bool, len(nisds))
	for _, n := range nisds {
		if n.ID != "" {
			ids[n.ID] = true
		}
	}
	return ids
}
