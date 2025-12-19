package clictlplanefuncs

import (
	"testing"
	"fmt"
	"context"
    "time"
    "golang.org/x/sync/errgroup"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var (
	TestPDUs  = make(map[string]cpLib.PDU)
	TestRacks = make(map[string]cpLib.Rack)
	TestHypervisors = make(map[string]cpLib.Hypervisor)
	TestDevices = make(map[string]cpLib.Device)
	TestNisds = make(map[string]cpLib.Nisd)
)

func TestBulkPDUsAndRacks(t *testing.T) {
	c := newClient(t)

	// -------------------------
	// 1) Create 10 PDUs
	// -------------------------
	locations := []string{
		"us-east", "us-west", "us-central", "us-south", "us-north",
		"eu-west", "eu-east", "ap-south", "ap-north", "africa-east",
	}

	var pduList []cpLib.PDU

	for i := 1; i <= 10; i++ {
		pdu := cpLib.PDU{
			ID:            fmt.Sprintf("pdu-%02d-uuid", i),
			Name:          fmt.Sprintf("pdu-%d", i),
			Location:      locations[i-1],
			PowerCapacity: "20Kw",
			Specification: fmt.Sprintf("spec-%d", i),
		}

		resp, err := c.PutPDU(&pdu)
		assert.NoError(t, err)
		assert.True(t, resp.Success)

		pduList = append(pduList, pdu)
	}

	// GET all PDUs
	allPdus, err := c.GetPDUs(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, len(pduList), len(allPdus), "Mismatch PDU count")

	// Store in global PDUs map
	for _, p := range allPdus {
		TestPDUs[p.ID] = p
	}

	log.Infof("Successfully created and validated %d PDUs", len(allPdus))

	// -------------------------
	// 2) Create 10 racks for each PDU (100 racks total)
	// -------------------------
	totalRacks := 0
	for _, pdu := range allPdus {

		for r := 1; r <= 10; r++ {
			rack := cpLib.Rack{
				ID:            fmt.Sprintf("%s-rack-%02d", pdu.ID, r),
				PDUID:         pdu.ID,
				Name:          fmt.Sprintf("%s-rack-%d", pdu.Name, r),
				Location:      pdu.Location,
				Specification: fmt.Sprintf("spec-%s-%d", pdu.Name, r),
			}

			resp, err := c.PutRack(&rack)
			assert.NoError(t, err)
			assert.True(t, resp.Success)
			totalRacks++
		}
	}

	// GET all racks
	rackResp, err := c.GetRacks(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, totalRacks, len(rackResp), "Mismatch total rack count")

	// Store in global Racks map
	for _, r := range rackResp {
		TestRacks[r.ID] = r
	}

	log.Infof("Successfully created and validated %d racks", len(rackResp))

	// -------------------------
	// 3) Create 2 Hypervisors per Rack
	// -------------------------
	hvCounter := 1

	for _, rack := range TestRacks {

		for i := 1; i <= 2; i++ {
			hv := cpLib.Hypervisor{
				ID:        fmt.Sprintf("hv-%03d-uuid", hvCounter),
				Name:      fmt.Sprintf("hv-%03d", hvCounter),
				RackID:    rack.ID,
				IPAddress: fmt.Sprintf("10.0.%d.%d", hvCounter/255, hvCounter%255),
				PortRange: "8000-9000",
				SSHPort:   fmt.Sprintf("%d", 6000+hvCounter),
			}

			resp, err := c.PutHypervisor(&hv)
			assert.NoError(t, err)
			assert.True(t, resp.Success)

			TestHypervisors[hv.ID] = hv
			hvCounter++
		}
	}

	log.Infof("Created %d Hypervisors", len(TestHypervisors))

	// GET ALL hypervisors
	allHvs, err := c.GetHypervisor(&cpLib.GetReq{GetAll: true})
	assert.NoError(t, err)
	assert.Equal(t, len(TestHypervisors), len(allHvs))

	// Overwrite global map
	TestHypervisors = make(map[string]cpLib.Hypervisor)
	for _, hv := range allHvs {
		TestHypervisors[hv.ID] = hv
	}

	// -------------------------
	// 4) Validations: PDU ↔ Rack ↔ Hypervisor Mapping
	// -------------------------

	// Rack validation
	for _, rack := range TestRacks {
		pdu, ok := TestPDUs[rack.PDUID]
		assert.True(t, ok, "Rack %s references invalid PDU %s", rack.ID, rack.PDUID)
		assert.Equal(t, pdu.Location, rack.Location)
	}

	// Hypervisor ↔ Rack validation
	for _, hv := range TestHypervisors {
		rack, ok := TestRacks[hv.RackID]
		assert.True(t, ok, "Hypervisor %s references invalid RackID %s", hv.ID, hv.RackID)
		assert.NotEmpty(t, rack.Name)
		assert.NotEmpty(t, hv.IPAddress)
	}

	log.Infof("Validated all PDUs, Racks, and Hypervisors successfully")

	vdevs := []*cpLib.Vdev{
        {Size: 700 * 1024 * 1024 * 1024},
        {Size: 400 * 1024 * 1024 * 1024},
    }

    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()

    eg, ctx := errgroup.WithContext(ctx)

    // concurrency limiter (e.g., allow at most 2 concurrent creates)
    maxConcurrent := 2
    sem := make(chan struct{}, maxConcurrent)

    // result channel
    results := make(chan *cpLib.Vdev, len(vdevs))

    for _, v := range vdevs {
        vv := v

        eg.Go(func() error {
            // each goroutine gets its own client
            c := newClient(t)
            // TODO: if CliCFuncs has a Close()/Shutdown(), defer it here

            // acquire slot
            select {
            case sem <- struct{}{}:
            case <-ctx.Done():
                return ctx.Err()
            }
            defer func() { <-sem }() // release

            // create vdev via this goroutine's client
            if err := c.CreateVdev(vv); err != nil {
                t.Logf("CreateVdev failed for size=%d: %v", vv.Size, err)
                return err
            }

            // send result
            select {
            case results <- vv:
                return nil
            case <-ctx.Done():
                return ctx.Err()
            }
        })
    }

    // wait for all goroutines
    if err := eg.Wait(); err != nil {
        t.Fatalf("parallel create failed: %v", err)
    }
    close(results)

    created := make([]*cpLib.Vdev, 0, len(vdevs))
    for r := range results {
        created = append(created, r)
    }

    // assertions
    assert.Len(t, created, len(vdevs))
    if len(created) >= 2 {
        assert.NotEqual(t, created[0].VdevID, created[1].VdevID, "Vdev IDs must be unique")
    }

	// -------------------------
	// 3) Validation: Rack ↔ PDU mapping and metadata checks
	// -------------------------

	for _, rack := range TestRacks {
		pdu, ok := TestPDUs[rack.PDUID]
		assert.True(t, ok, "Rack %s references missing PDUID %s", rack.Name, rack.PDUID)

		// Location validation
		assert.Equal(t, pdu.Location, rack.Location,
			"Location mismatch Rack=%s (%s) PDU=%s (%s)",
			rack.Name, rack.Location, pdu.Name, pdu.Location)

		// Non-empty rack fields
		assert.NotEmpty(t, rack.Specification)
		assert.NotEmpty(t, rack.Name)
	}

	log.Infof("Validated all %d Rack↔PDU associations successfully", len(TestRacks))
}

func TestMultiCreateVdevErrGroup(t *testing.T) {
    vdevs := []*cpLib.Vdev{
        {Size: 700 * 1024 * 1024 * 1024},
        {Size: 400 * 1024 * 1024 * 1024},
    }

    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()

    eg, ctx := errgroup.WithContext(ctx)

    // concurrency limiter (e.g., allow at most 2 concurrent creates)
    maxConcurrent := 2
    sem := make(chan struct{}, maxConcurrent)

    // result channel
    results := make(chan *cpLib.Vdev, len(vdevs))

    for _, v := range vdevs {
        vv := v

        eg.Go(func() error {
            // each goroutine gets its own client
            c := newClient(t)
            // TODO: if CliCFuncs has a Close()/Shutdown(), defer it here

            // acquire slot
            select {
            case sem <- struct{}{}:
            case <-ctx.Done():
                return ctx.Err()
            }
            defer func() { <-sem }() // release

            // create vdev via this goroutine's client
            if err := c.CreateVdev(vv); err != nil {
                t.Logf("CreateVdev failed for size=%d: %v", vv.Size, err)
                return err
            }

            // send result
            select {
            case results <- vv:
                return nil
            case <-ctx.Done():
                return ctx.Err()
            }
        })
    }

    // wait for all goroutines
    if err := eg.Wait(); err != nil {
        t.Fatalf("parallel create failed: %v", err)
    }
    close(results)

    created := make([]*cpLib.Vdev, 0, len(vdevs))
    for r := range results {
        created = append(created, r)
    }

    // assertions
    assert.Len(t, created, len(vdevs))
    if len(created) >= 2 {
        assert.NotEqual(t, created[0].VdevID, created[1].VdevID, "Vdev IDs must be unique")
    }
}