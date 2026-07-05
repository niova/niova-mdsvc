package restapi

// This file defines the payloads for the resource/pfs/infra read endpoints
// (GET /api/resource, GET /api/pfs, GET /api/infra). Each is carried inside the
// APIResponse[T] envelope. The per-entity item shapes reuse the write DTOs
// (PDU/Rack/.../NISD/Partition).

// GetResourcePayload is the payload of the get-resource envelope. It mirrors the
// internal ResourceListResp: only the slice matching the requested type is
// populated.
type GetResourcePayload struct {
	Type        string       `json:"type,omitempty"`
	PDUs        []PDU        `json:"pdus,omitempty"`
	Racks       []Rack       `json:"racks,omitempty"`
	Hypervisors []Hypervisor `json:"hypervisors,omitempty"`
	Devices     []Device     `json:"devices,omitempty"`
	Nisds       []NISD       `json:"nisds,omitempty"`
	Partitions  []Partition  `json:"partitions,omitempty"`
}

// GetPFSPayload is the payload of the get-pfs envelope.
type GetPFSPayload struct {
	PFS []PFS `json:"pfs,omitempty"`
}

// GetNisdArgsPayload is the payload of the get-nisd-args envelope (singleton).
type GetNisdArgsPayload struct {
	NisdArgs NisdArgs `json:"nisd_args"`
}

// GetInfraPayload is the payload of the get-infra envelope. infra carries the
// nested PDU -> Rack -> Hypervisor -> Device -> NISD hierarchy assembled by the
// proxy.
type GetInfraPayload struct {
	Infra CreateInfraRequest `json:"infra"`
}
