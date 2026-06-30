package restapi

// This file defines the response DTOs for the resource/pfs/infra read endpoints
// (GET /api/resource, GET /api/pfs, GET /api/infra). They keep the flat
// snake_case {success,...,error} envelope used by the other REST reads. The
// per-entity item shapes reuse the write DTOs (PDU/Rack/.../NISD/Partition).

// GetResourceResponse is returned by GET /api/resource?type=. It mirrors the
// internal ResourceListResp: only the slice matching the requested type is
// populated.
type GetResourceResponse struct {
	Success     bool         `json:"success"`
	Type        string       `json:"type,omitempty"`
	PDUs        []PDU        `json:"pdus,omitempty"`
	Racks       []Rack       `json:"racks,omitempty"`
	Hypervisors []Hypervisor `json:"hypervisors,omitempty"`
	Devices     []Device     `json:"devices,omitempty"`
	Nisds       []NISD       `json:"nisds,omitempty"`
	Partitions  []Partition  `json:"partitions,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// GetPFSResponse is returned by GET /api/pfs.
type GetPFSResponse struct {
	Success bool   `json:"success"`
	PFS     []PFS  `json:"pfs,omitempty"`
	Error   string `json:"error,omitempty"`
}

// GetNisdArgsResponse is returned by GET /api/nisd_args (singleton record).
type GetNisdArgsResponse struct {
	Success  bool     `json:"success"`
	NisdArgs NisdArgs `json:"nisd_args"`
	Error    string   `json:"error,omitempty"`
}

// GetInfraResponse is returned by GET /api/infra. infra carries the nested
// PDU -> Rack -> Hypervisor -> Device -> NISD hierarchy assembled by the proxy.
type GetInfraResponse struct {
	Success bool               `json:"success"`
	Infra   CreateInfraRequest `json:"infra"`
	Error   string             `json:"error,omitempty"`
}
