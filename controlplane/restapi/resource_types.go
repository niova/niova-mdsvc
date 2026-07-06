package restapi

import "encoding/json"

// This file defines the payloads for the resource/pfs/infra read endpoints
// (GET /api/resource, GET /api/pfs, GET /api/infra). Each is carried inside the
// APIResponse[T] envelope. The per-entity item shapes reuse the write DTOs
// (PDU/Rack/.../NISD/Partition).

// GetResourcePayload is the payload of the get-resource envelope. It mirrors the
// TiDB shape: a single generic `resources` array (rather than typed per-type
// slices), where every element is a flat entity object of the requested `type`.
// Elements are carried as json.RawMessage so the proxy can emit the per-type DTO
// and the client can decode it back into the matching type.
type GetResourcePayload struct {
	Type      string            `json:"type,omitempty"`
	Resources []json.RawMessage `json:"resources,omitempty"`
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
