package srvctlplanefuncs

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/tidwall/btree"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"
	PumiceDBServer "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceserver"
	"github.com/00pauln00/niova-pumicedb/go/pkg/utils/storage/memstore"
)

func TestVdev(t *testing.T) {
	// 1. Setup
	HR.Init()

	ds := memstore.NewMemStore()
	cbArgs := &PumiceDBServer.PmdbCbArgs{
		Store:     ds,
		ReplySize: 4096 * 1024,
	}

	// Add 10 NISDs to satisfy EC 4+3 and 3 Replicas
	for i := 0; i < 10; i++ {
		nisdID := uuid.New().String()
		pduID := uuid.New().String()
		rackID := uuid.New().String()
		hvID := uuid.New().String()
		devID := uuid.New().String()
		ptID := uuid.New().String()

		n := &ctlplfl.Nisd{
			ID:            nisdID,
			TotalSize:     100 * ctlplfl.CHUNK_SIZE,
			AvailableSize: 100 * ctlplfl.CHUNK_SIZE,
			FailureDomain: []string{pduID, rackID, hvID, devID, ptID},
		}
		HR.AddNisd(n)

		// Write NISD config to DB
		ds.Write(fmt.Sprintf("%s/%s/as", NisdCfgKey, nisdID), fmt.Sprintf("%d", n.AvailableSize), "")
		ds.Write(fmt.Sprintf("%s/%s/ts", NisdCfgKey, nisdID), fmt.Sprintf("%d", n.TotalSize), "")
	}

	scenarios := []struct {
		name            string
		redundancy      ctlplfl.RedundancyMode
		totalReplicas   uint8
		totalDataBlks   uint8
		totalParityBlks uint8
		size            int64
	}{
		{
			name:          "CreateWithoutReplica", // 1 replica
			redundancy:    ctlplfl.RMReplica,
			totalReplicas: 1,
			size:          8 * 1024 * 1024 * 1024,
		},
		{
			name:          "CreateVdevWith3Replica",
			redundancy:    ctlplfl.RMReplica,
			totalReplicas: 3,
			size:          8 * 1024 * 1024 * 1024,
		},
		{
			name:            "CreateVdevWithEC43",
			redundancy:      ctlplfl.RMEC,
			totalDataBlks:   4,
			totalParityBlks: 3,
			size:            8 * 1024 * 1024 * 1024,
		},
	}

	token, _ := createTestToken(testAdminID, "admin", testSecret)

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// a. WPCreateVdev
			vdevConfig := &ctlplfl.VdevConfig{
				Name:            sc.name,
				Size:            sc.size,
				Redundancy:      sc.redundancy,
				TotalReplicas:   sc.totalReplicas,
				TotalDataBlks:   sc.totalDataBlks,
				TotalParityBlks: sc.totalParityBlks,
			}
			vdevConfig.Init()

			cpReq := ctlplfl.CPReq{
				Token: token,
				Payload: ctlplfl.VdevReq{
					Vdev: vdevConfig,
				},
			}

			// Simulate WPCreateVdev
			res, err := WPCreateVdev(cpReq)
			if err != nil {
				t.Fatalf("WPCreateVdev failed: %v", err)
			}

			// Apply WP changes to Memstore
			var intrm funclib.FuncIntrm
			if err := pmCmn.Decoder(pmCmn.GOB, res.([]byte), &intrm); err != nil {
				t.Fatalf("Failed to decode WP result: %v", err)
			}
			for _, chg := range intrm.Changes {
				ds.Write(string(chg.Key), string(chg.Value), "")
			}

			// Extract VdevID from WP response
			var vdevID string
			if cpResp, ok := intrm.Response.(ctlplfl.CPResp); ok && cpResp.Error != nil {
				t.Fatalf("WPCreateVdev returned error: %s", cpResp.Error.Message)
			} else if respXML, ok := intrm.Response.(ctlplfl.ResponseXML); ok {
				vdevID = respXML.ID
			} else {
				t.Fatalf("Unexpected response type from WPCreateVdev: %T", intrm.Response)
			}

			// b. Simulate APCreateVdev without CGO dependency
			// We'll call AllocNISDs and commitAllocChgs directly
			req := &ctlplfl.VdevReq{
				Vdev: vdevConfig,
			}
			req.Vdev.ID = vdevID
			req.Vdev.TotalChunks = uint32(ctlplfl.Count8GBChunks(req.Vdev.Size))

			allocMap := btree.NewMap[string, *ctlplfl.NisdVdevAlloc](32)

			if err := AllocNISDs(req, allocMap, &intrm, 0); err != nil {
				t.Fatalf("AllocNISDs failed: %v", err)
			}

			if err := commitAllocChgs(&intrm, allocMap, cbArgs); err != nil {
				t.Fatalf("commitAllocChgs failed: %v", err)
			}

			applyNISDAlloc(allocMap)

			// c. ReadVdevsInfoWithChunkMapping
			getReq := ctlplfl.GetReq{
				ID: vdevID,
			}
			cpReqGet := ctlplfl.CPReq{
				Token:   token,
				Payload: getReq,
			}

			resRead, err := ReadVdevsInfoWithChunkMapping(cbArgs, cpReqGet)
			if err != nil {
				t.Fatalf("ReadVdevsInfoWithChunkMapping failed: %v", err)
			}

			// d. Validate
			var cpResp ctlplfl.CPResp
			if err := pmCmn.Decoder(pmCmn.GOB, resRead.([]byte), &cpResp); err != nil {
				t.Fatalf("Failed to decode Read response: %v", err)
			}

			if cpResp.Error != nil {
				t.Fatalf("Read response error: %s", cpResp.Err())
			}

			vdevList, ok := cpResp.Payload.([]ctlplfl.Vdev)
			if !ok {
				// Try as a single Vdev? No, the code says it returns vdevList ([]ctlplfl.Vdev)
				t.Fatalf("Expected []ctlplfl.Vdev in Payload, got %T", cpResp.Payload)
			}

			if len(vdevList) != 1 {
				t.Fatalf("Expected 1 vdev, got %d", len(vdevList))
			}

			v := vdevList[0]
			if v.Cfg.ID != vdevID {
				t.Errorf("ID mismatch: expected %s, got %s", vdevID, v.Cfg.ID)
			}
			if v.Cfg.Redundancy != sc.redundancy {
				t.Errorf("Redundancy mismatch: expected %v, got %v", sc.redundancy, v.Cfg.Redundancy)
			}

			expectedTotalBlocks := int(sc.totalReplicas)
			if sc.redundancy == ctlplfl.RMEC {
				expectedTotalBlocks = int(sc.totalDataBlks + sc.totalParityBlks)
			}

			if len(v.NisdToChkMap) == 0 {
				t.Fatalf("No chunks found for vdev")
			}

			dataCount := 0
			parityCount := 0
			replicaCount := 0

			for i := uint32(0); i < v.Cfg.TotalChunks; i++ {
				getReqChunk := ctlplfl.GetReq{
					ID: fmt.Sprintf("%s/%d", vdevID, i),
				}
				cpReqChunk := ctlplfl.CPReq{
					Token:   token,
					Payload: getReqChunk,
				}
				resChunk, err := ReadChunk(cbArgs, cpReqChunk)
				if err != nil {
					t.Fatalf("ReadChunk failed: %v", err)
				}
				var cpRespChunk ctlplfl.CPResp
				if err := pmCmn.Decoder(pmCmn.GOB, resChunk.([]byte), &cpRespChunk); err != nil {
					t.Fatalf("Failed to decode ReadChunk response: %v", err)
				}
				if cpRespChunk.Error != nil {
					t.Fatalf("ReadChunk response error: %s", cpRespChunk.Err())
				}
				chunk, ok := cpRespChunk.Payload.(ctlplfl.Chunk)
				if !ok {
					t.Fatalf("Expected ctlplfl.Chunk in Payload, got %T", cpRespChunk.Payload)
				}

				if len(chunk.Placements) != expectedTotalBlocks {
					t.Errorf("Chunk %d placement count mismatch: expected %d, got %d", chunk.Index, expectedTotalBlocks, len(chunk.Placements))
				}
				for _, p := range chunk.Placements {
					if p.NisdID == "" {
						t.Errorf("Chunk %d placement %d has empty NisdID", chunk.Index, p.Sequence)
					}
					switch p.Type {
					case ctlplfl.Replica:
						replicaCount++
					case ctlplfl.Data:
						dataCount++
					case ctlplfl.Parity:
						parityCount++
					}
				}

				if sc.redundancy == ctlplfl.RMReplica {
					if replicaCount != int(sc.totalReplicas) {
						t.Errorf("Replica count mismatch: expected %d, got %d", sc.totalReplicas, replicaCount)
					}
				} else {
					if dataCount != int(sc.totalDataBlks) {
						t.Errorf("Data block count mismatch: expected %d, got %d", sc.totalDataBlks, dataCount)
					}
					if parityCount != int(sc.totalParityBlks) {
						t.Errorf("Parity block count mismatch: expected %d, got %d", sc.totalParityBlks, parityCount)
					}
				}
				// Reset counts for next chunk if any
				replicaCount = 0
				dataCount = 0
				parityCount = 0
			}
		})
	}
}
