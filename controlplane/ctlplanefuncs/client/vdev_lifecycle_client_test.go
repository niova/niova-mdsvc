package clictlplanefuncs

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"fmt"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

func TestVdevLifecycleClient(t *testing.T) {
	c := newClient(t)

	// Step 0: Setup enough NISD space (at least 7 NISDs for EC 4+3)
	// We'll create 10 NISDs to be safe
	for i := 0; i < 10; i++ {
		n := cpLib.Nisd{
			PeerPort: uint16(9000 + i),
			ID:       uuid.NewString(),
			FailureDomain: []string{
				uuid.NewString(),
				uuid.NewString(),
				uuid.NewString(),
				fmt.Sprintf("dev-%d", i),
				fmt.Sprintf("dev-p-%d", i),
			},
			TotalSize:     100 * cpLib.CHUNK_SIZE,
			AvailableSize: 100 * cpLib.CHUNK_SIZE,
		}
		resp, err := c.PutNisd(&n)
		require.NoError(t, err)
		require.True(t, resp.Success)
	}

	scenarios := []struct {
		name            string
		redundancy      cpLib.RedundancyMode
		totalReplicas   uint8
		totalDataBlks   uint8
		totalParityBlks uint8
		size            int64
	}{
		{
			name:          "CreateWithoutReplicaClient", // 1 replica
			redundancy:    cpLib.RMReplica,
			totalReplicas: 1,
			size:          24 * 1024 * 1024 * 1024,
		},
		{
			name:          "CreateVdevWith3ReplicaClient",
			redundancy:    cpLib.RMReplica,
			totalReplicas: 3,
			size:          24 * 1024 * 1024 * 1024,
		},
		{
			name:            "CreateVdevWithEC43Client",
			redundancy:      cpLib.RMEC,
			totalDataBlks:   4,
			totalParityBlks: 3,
			size:            24 * 1024 * 1024 * 1024,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			// a. Create Vdev
			vdevReq := &cpLib.VdevReq{
				Vdev: &cpLib.VdevConfig{
					Name:            sc.name,
					Size:            sc.size,
					Redundancy:      sc.redundancy,
					TotalReplicas:   sc.totalReplicas,
					TotalDataBlks:   sc.totalDataBlks,
					TotalParityBlks: sc.totalParityBlks,
				},
			}

			resp, err := c.CreateVdev(vdevReq)
			require.NoError(t, err, "failed to create vdev for scenario %s", sc.name)
			require.True(t, resp.Success)
			vdevID := resp.ID
			require.NotEmpty(t, vdevID)

			// b. Fetch vdev with chunk info
			getReq := &cpLib.GetReq{
				ID: vdevID,
			}
			vdevList, err := c.GetVdevsWithChunkInfo(getReq)
			require.NoError(t, err, "failed to fetch vdev info for scenario %s", sc.name)
			require.Len(t, vdevList, 1)

			v := vdevList[0]
			assert.Equal(t, vdevID, v.Config.ID)
			assert.Equal(t, sc.redundancy, v.Config.Redundancy)

			expectedTotalBlocks := int(sc.totalReplicas)
			if sc.redundancy == cpLib.RMEC {
				expectedTotalBlocks = int(sc.totalDataBlks + sc.totalParityBlks)
			}

			require.NotEmpty(t, v.Chunks, "no chunks found for vdev %s", vdevID)
			
			for _, chunk := range v.Chunks {
				assert.Len(t, chunk.Placements, expectedTotalBlocks)
				
				dataCount := 0
				parityCount := 0
				replicaCount := 0

				for _, p := range chunk.Placements {
					assert.NotEmpty(t, p.NisdID)
					switch p.Type {
					case cpLib.Replica:
						replicaCount++
					case cpLib.Data:
						dataCount++
					case cpLib.Parity:
						parityCount++
					}
				}

				if sc.redundancy == cpLib.RMReplica {
					assert.Equal(t, int(sc.totalReplicas), replicaCount)
				} else {
					assert.Equal(t, int(sc.totalDataBlks), dataCount)
					assert.Equal(t, int(sc.totalParityBlks), parityCount)
				}
			}
		})
	}
}
