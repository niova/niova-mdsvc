package srvctlplanefuncs

import (
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

const ( // Key Prefixes
	BASE_KEY         = 0
	BASE_UUID_PREFIX = 1
	ELEMENT_KEY      = 2
	KEY_LEN          = 3
	VDEV_CFG_C_KEY   = 2
	VDEV_ELEMENT_KEY = 3
	BASE_VDEV_PREFIX = 1

	NET_IDX = 3
)

const (
	CHUNK_REDUNDANCY = 0
	CHUNK_SEQ        = 1
	CHUNK_PLACMENT   = 4

	MIN_CHUNK_KEY_LEN           = 5
	MIN_CHUNK_PLACEMENT_KEY_LEN = 2
)

type Entity interface{}

type ParseEntity interface {
	GetRootKey() string
	NewEntity(id string) Entity
	ParseField(entity Entity, parts []string, value []byte)
	GetEntity(entity Entity) Entity
}

func ParseEntitiesRR[T Entity](readResult []map[string][]byte, pe ParseEntity, idIdx int) []T {
	entityMap := make(map[string]Entity)

	for i := range readResult {
		for k, v := range readResult[i] {
			parts := strings.Split(strings.Trim(k, "/"), "/")
			if len(parts) <= idIdx || parts[BASE_KEY] != pe.GetRootKey() {
				continue
			}

			id := parts[idIdx]
			entity, exists := entityMap[id]
			if !exists {
				entity = pe.NewEntity(id)
				entityMap[id] = entity
			}
			pe.ParseField(entity, parts, v)
		}
	}

	result := make([]T, 0, len(entityMap))
	for _, e := range entityMap {
		final := pe.GetEntity(e)
		result = append(result, final.(T))
	}
	return result
}

func ParseEntities[T Entity](readResult map[string][]byte, pe ParseEntity, idIdx int) []T {
	entityMap := make(map[string]Entity)

	for k, v := range readResult {
		parts := strings.Split(strings.Trim(k, "/"), "/")
		if len(parts) <= idIdx || parts[BASE_KEY] != pe.GetRootKey() {
			continue
		}

		id := parts[idIdx]
		entity, exists := entityMap[id]
		if !exists {
			entity = pe.NewEntity(id)
			entityMap[id] = entity
		}
		pe.ParseField(entity, parts, v)
	}
	result := make([]T, 0, len(entityMap))
	for _, e := range entityMap {
		final := pe.GetEntity(e)
		result = append(result, final.(T))
	}
	return result
}

func ParseEntitiesMap(readResult map[string][]byte, pe ParseEntity, idIdx int) map[string]Entity {
	entityMap := make(map[string]Entity)

	for k, v := range readResult {
		parts := strings.Split(strings.Trim(k, "/"), "/")
		// require at least ELEMENT_KEY to be present and that base key matches parser root
		if len(parts) <= idIdx || parts[BASE_KEY] != pe.GetRootKey() {
			continue
		}

		id := parts[idIdx]
		entity, exists := entityMap[id]
		if !exists {
			entity = pe.NewEntity(id)
			entityMap[id] = entity
		}
		pe.ParseField(entity, parts, v)
	}
	return entityMap
}

// Rack parser
type rackParser struct{}

func (rackParser) GetRootKey() string { return rackKey }
func (rackParser) NewEntity(id string) Entity {
	return &ctlplfl.Rack{ID: id}
}
func (rackParser) ParseField(entity Entity, parts []string, value []byte) {
	r := entity.(*ctlplfl.Rack)

	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case LOCATION:
			r.Location = string(value)
		case pduKey:
			r.PDUID = string(value)
		case SPEC:
			r.Specification = string(value)
		case NAME:
			r.Name = string(value)

		}
	}
}

func (rackParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.Rack) }

// Hypervisor parser
type hvParser struct{}

func (hvParser) GetRootKey() string { return hvKey }
func (hvParser) NewEntity(id string) Entity {
	return &ctlplfl.Hypervisor{ID: id}
}
func (hvParser) ParseField(entity Entity, parts []string, value []byte) {
	hv := entity.(*ctlplfl.Hypervisor)
	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case rackKey:
			hv.RackID = string(value)
		case PORT_RANGE:
			hv.PortRange = string(value)
		case SSH_PORT:
			hv.SSHPort = string(value)
		case NAME:
			hv.Name = string(value)
		case ENABLE_RDMA:
			rdma, err := strconv.ParseBool(string(value))
			if err != nil {
				log.Error("failed to parse enable rdma field")
			}
			hv.RDMAEnabled = rdma
		}
	} else if len(parts) > KEY_LEN && parts[2] == IP_ADDR {
		hv.IPAddrs = append(hv.IPAddrs, string(value))
	}
}

func (hvParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.Hypervisor) }

/*
// Device parser
type deviceParser struct{}

func (deviceParser) GetRootKey() string { return deviceCfgKey }
func (deviceParser) NewEntity(id string) Entity {
	return &ctlplfl.Device{ID: id}
}
func (deviceParser) ParseField(entity Entity, parts []string, value []byte) {
	dev := entity.(*ctlplfl.Device)
	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case hvKey:
			dev.HypervisorID = string(value)
		case SERIAL_NUM:
			dev.SerialNumber = string(value)
		case STATE:
			state, _ := strconv.Atoi(string(value))
			dev.State = uint16(state)
		case FAILURE_DOMAIN:
			dev.FailureDomain = string(value)
		case DEVICE_PATH:
			dev.DevicePath = string(value)
		case NAME:
			dev.Name = string(value)
		case SIZE:
			sz, _ := strconv.Atoi(string(value))
			dev.Size = int64(sz)
		}
	}
}
func (deviceParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.Device) }
*/

// nisd parser
type NisdParser struct{}

func (NisdParser) GetRootKey() string { return NisdCfgKey }

func (NisdParser) NewEntity(id string) Entity {
	return &ctlplfl.Nisd{ID: id, FailureDomain: make([]string, ctlplfl.FD_MAX), NetInfo: make([]ctlplfl.NetworkInfo, 0)}
}

func (NisdParser) ParseField(entity Entity, parts []string, value []byte) {
	nisd := entity.(*ctlplfl.Nisd)
	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case DEVICE_ID:
			nisd.FailureDomain[ctlplfl.DEVICE_IDX] = string(value)
		case PEER_PORT:
			p, _ := strconv.Atoi(string(value))
			nisd.PeerPort = uint16(p)
		case hvKey:
			nisd.FailureDomain[ctlplfl.HV_IDX] = string(value)
		case pduKey:
			nisd.FailureDomain[ctlplfl.PDU_IDX] = string(value)
		case rackKey:
			nisd.FailureDomain[ctlplfl.RACK_IDX] = string(value)
		case ptKey:
			nisd.FailureDomain[ctlplfl.PARTITION_IDX] = string(value)
		case TOTAL_SPACE:
			ts, _ := strconv.Atoi(string(value))
			nisd.TotalSize = int64(ts)
		case AVAIL_SPACE:
			as, _ := strconv.Atoi(string(value))
			nisd.AvailableSize = int64(as)
		case SOCKET_PATH:
			nisd.SocketPath = string(value)
		case NETWORK_INFO_CNT:
			nic, _ := strconv.Atoi(string(value))
			nisd.NetInfoCnt = nic
		}
	}

	// Handle network info keys: n/<nisd-id>/ni/ip:ptr
	if len(parts) > KEY_LEN && parts[ELEMENT_KEY] == NETWORK_INFO {

		p, _ := strconv.Atoi(string(value))
		netInfo := ctlplfl.NetworkInfo{
			IPAddr: parts[NET_IDX],
			Port:   uint16(p),
		}

		// Ensure slice capacity
		nisd.NetInfo = append(nisd.NetInfo, netInfo)

	}
}

func (NisdParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.Nisd) }

type pduParser struct{}

func (pduParser) GetRootKey() string { return pduKey }
func (pduParser) NewEntity(id string) Entity {
	return &ctlplfl.PDU{ID: id}
}
func (pduParser) ParseField(entity Entity, parts []string, value []byte) {
	pdu := entity.(*ctlplfl.PDU)
	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case LOCATION:
			pdu.Location = string(value)
		case POWER_CAP:
			pdu.PowerCapacity = string(value)
		case SPEC:
			pdu.Specification = string(value)
		case NAME:
			pdu.Name = string(value)
		}
	}
}

func (pduParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.PDU) }

type ptParser struct{}

func (ptParser) GetRootKey() string { return ptKey }
func (ptParser) NewEntity(id string) Entity {
	return &ctlplfl.DevicePartition{PartitionID: id}
}
func (ptParser) ParseField(entity Entity, parts []string, value []byte) {
	pt := entity.(*ctlplfl.DevicePartition)
	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case DEVICE_ID:
			pt.DevID = string(value)
		case SIZE:
			s, _ := strconv.Atoi(string(value))
			pt.Size = int64(s)
		case PARTITION_PATH:
			pt.PartitionPath = string(value)
		case nisdKey:
			pt.NISDUUID = string(value)

		}
	}
}

func (ptParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.DevicePartition) }

type deviceWithPartitionParser struct{}

func (deviceWithPartitionParser) GetRootKey() string { return deviceCfgKey }

func (deviceWithPartitionParser) NewEntity(id string) Entity {
	return &ctlplfl.Device{ID: id, Partitions: make([]ctlplfl.DevicePartition, 0)}
}

func (deviceWithPartitionParser) ParseField(entity Entity, parts []string, value []byte) {
	dev := entity.(*ctlplfl.Device)

	// d_cfg/<dev-id>/field
	if len(parts) == KEY_LEN {
		switch parts[ELEMENT_KEY] {
		case hvKey:
			dev.HypervisorID = string(value)
		case SERIAL_NUM:
			dev.SerialNumber = string(value)
		case STATE:
			state, _ := strconv.Atoi(string(value))
			dev.State = uint16(state)
		case FAILURE_DOMAIN:
			dev.FailureDomain = string(value)
		case DEVICE_PATH:
			dev.DevicePath = string(value)
		case NAME:
			dev.Name = string(value)
		case SIZE:
			sz, _ := strconv.Atoi(string(value))
			dev.Size = int64(sz)
		}
		return
	}

	// d_cfg/<dev-id>/pt/<pt-id>/<pt-field>
	if len(parts) > ELEMENT_KEY && parts[ELEMENT_KEY] == ptKey && len(parts) >= (ELEMENT_KEY+2) {
		ptID := parts[ELEMENT_KEY+1]

		// find or create partition
		var pt *ctlplfl.DevicePartition
		for i := range dev.Partitions {
			if dev.Partitions[i].PartitionID == ptID {
				pt = &dev.Partitions[i]
				break
			}
		}
		if pt == nil {
			dev.Partitions = append(dev.Partitions, ctlplfl.DevicePartition{PartitionID: ptID})
			pt = &dev.Partitions[len(dev.Partitions)-1]
		}

		// assign partition field
		field := parts[ELEMENT_KEY+2]
		switch field {
		case DEVICE_ID:
			pt.DevID = string(value)
		case SIZE:
			s, _ := strconv.Atoi(string(value))
			pt.Size = int64(s)
		case PARTITION_PATH:
			pt.PartitionPath = string(value)
		case nisdKey:
			pt.NISDUUID = string(value)
		}
	}
}

func (deviceWithPartitionParser) GetEntity(entity Entity) Entity {
	return *entity.(*ctlplfl.Device)
}

// TODO: Make chages to parse a single Vdev
type vdevParser struct{}

func (vdevParser) GetRootKey() string { return vdevKey }
func (vdevParser) NewEntity(id string) Entity {
	return &ctlplfl.VdevConfig{ID: id}
}
func (vdevParser) ParseField(entity Entity, parts []string, value []byte) {
	vdev := entity.(*ctlplfl.VdevConfig)
	if len(parts) > VDEV_ELEMENT_KEY && parts[VDEV_CFG_C_KEY] == cfgkey {
		switch parts[VDEV_ELEMENT_KEY] {
		case SIZE:
			if sz, err := strconv.ParseInt(string(value), 10, 64); err == nil {
				vdev.Size = sz
			}
		case NUM_CHUNKS:
			if nc, err := strconv.ParseUint(string(value), 10, 32); err == nil {
				vdev.ChunkCnt = uint32(nc)
			}
		case TOTAL_DATA_BLKS:
			if nd, err := strconv.ParseUint(string(value), 10, 8); err == nil {
				vdev.DataBlkCnt = uint8(nd)
			}
		case TOTAL_PARITY_BLKS:
			if np, err := strconv.ParseUint(string(value), 10, 8); err == nil {
				vdev.ParityBlkCnt = uint8(np)
			}
		case REDUNDANCY_MODE:
			if rm, err := strconv.Atoi(string(value)); err == nil {
				vdev.Redundancy = ctlplfl.RedundancyMode(rm)
			}
		case pfsKey:
			vdev.PFSID = string(value)
		case NAME:
			vdev.Name = string(value)
		case FD_TYPE_KEY:
			vdev.FilterType = string(value)
		case FD_ID_KEY:
			vdev.FilterID = string(value)
		}
	} else if len(parts) > VDEV_ELEMENT_KEY && parts[VDEV_CFG_C_KEY] == ctlplfl.MntKey {
		switch parts[VDEV_ELEMENT_KEY] {
		case ctlplfl.MOUNT_COUNTER:
			if mc, err := strconv.ParseUint(string(value), 10, 64); err == nil {
				vdev.VdevMountInfo.MountCounter = mc
			}
		case ctlplfl.LAST_UPDATED_LTS:
			if lu, err := time.Parse(time.RFC3339, string(value)); err == nil {
				vdev.VdevMountInfo.LastUpdatedLTS = lu
			}
		}
	}
}

func (vdevParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.VdevConfig) }

// Chunk parser
// Key format: v/<vdevID>/c/<chunkIdx>/<Placement>
// Replication: v/<vdevID>/c/0/D.0 -> nisd-1
// EC:          v/<vdevID>/c/0/D.0 -> nisd-1
//
//	v/<vdevID>/c/0/P.0 -> nisd-2
type chunkParser struct{}

func (chunkParser) GetRootKey() string { return vdevKey }

func (chunkParser) NewEntity(id string) Entity {
	idx, _ := strconv.ParseUint(id, 10, 32)
	return &ctlplfl.Chunk{
		Index:      uint32(idx),
		Placements: make([]ctlplfl.ChunkPlacement, 0),
	}
}

func (chunkParser) ParseField(entity Entity, parts []string, value []byte) {
	chunk := entity.(*ctlplfl.Chunk)
	if len(parts) < MIN_CHUNK_KEY_LEN {
		return
	}

	placementStr := parts[CHUNK_PLACMENT] // e.g. "D.0", "P.0"
	pParts := strings.Split(placementStr, ".")
	if len(pParts) != MIN_CHUNK_PLACEMENT_KEY_LEN {
		return
	}
	seq, err := strconv.ParseUint(pParts[CHUNK_SEQ], 10, 8)
	if err != nil {
		return
	}

	placement := ctlplfl.ChunkPlacement{
		Sequence: uint8(seq),
		NisdID:   string(value),
	}

	// "D" is shared by replica and EC-data placements; the caller resolves
	// which one it actually is from the vdev-level redundancy mode.
	switch pParts[CHUNK_REDUNDANCY] {
	case "D":
		placement.Type = ctlplfl.Data
		chunk.Redundancy = ctlplfl.RMEC32K
	case "P":
		placement.Type = ctlplfl.Parity
		chunk.Redundancy = ctlplfl.RMEC32K
	}

	chunk.Placements = append(chunk.Placements, placement)
}

func (chunkParser) GetEntity(entity Entity) Entity { return *entity.(*ctlplfl.Chunk) }

type cnInternal struct {
	cn     ctlplfl.ChunkNisd
	data   map[uint8]string
	parity map[uint8]string
}

type chunkNisdParser struct{}

func (chunkNisdParser) GetRootKey() string { return vdevKey }

func (chunkNisdParser) NewEntity(id string) Entity {
	return &cnInternal{
		data:   make(map[uint8]string),
		parity: make(map[uint8]string),
	}
}

func (chunkNisdParser) ParseField(entity Entity, parts []string, value []byte) {
	in := entity.(*cnInternal)

	if len(parts) < MIN_CHUNK_KEY_LEN {
		return
	}

	placement := strings.Split(parts[CHUNK_PLACMENT], ".")
	if len(placement) != MIN_CHUNK_PLACEMENT_KEY_LEN {
		return
	}

	seq, err := strconv.ParseUint(placement[CHUNK_SEQ], 10, 8)
	if err != nil {
		return
	}

	idx := uint8(seq)
	nisdID := string(value)

	switch placement[CHUNK_REDUNDANCY] {
	case "D":
		in.data[idx] = nisdID
	case "P":
		in.parity[idx] = nisdID
	}
}

func (chunkNisdParser) GetEntity(entity Entity) Entity {
	in := entity.(*cnInternal)

	var ids []string

	// Replica and EC-data blocks share "D" and are ordered by sequence number;
	// parity blocks (EC only) are appended in sequence order after them.
	for i := uint8(0); ; i++ {
		id, ok := in.data[i]
		if !ok {
			break
		}
		ids = append(ids, id)
	}
	for i := uint8(0); ; i++ {
		id, ok := in.parity[i]
		if !ok {
			break
		}
		ids = append(ids, id)
	}

	in.cn.NisdIDs = strings.Join(ids, ",")
	return in.cn
}
