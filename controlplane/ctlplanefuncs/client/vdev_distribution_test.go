package clictlplanefuncs

import (
	"fmt"
	"sync"
	"testing"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateVdevParallel(t *testing.T) {
	c := newClient(t)

	log.Info("Starting TestCreateVdevParallel")

	// PDU
	pdu := cpLib.PDU{
		ID:            "2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
		Name:          "pdu-1",
		Location:      "us-east",
		PowerCapacity: "15Kw",
		Specification: "spec-pdu",
	}

	resp, err := c.PutPDU(&pdu)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("PDU created: ", pdu.ID)

	// Rack
	rack := cpLib.Rack{
		ID:            "6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
		PDUID:         "2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
		Name:          "rack-1",
		Location:      "us-east",
		Specification: "rack-spec",
	}

	resp, err = c.PutRack(&rack)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("Rack created: ", rack.ID)

	// Hypervisor
	hv := cpLib.Hypervisor{
		ID:        "b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
		RackID:    "6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
		Name:      "hv-1",
		IPAddrs:   []string{"127.0.0.1", "127.0.0.1"},
		PortRange: "8000-9000",
		SSHPort:   "6999",
	}

	resp, err = c.PutHypervisor(&hv)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("Hypervisor created: ", hv.ID)

	// Device
	device := cpLib.Device{
		ID:            "nvme-5e6b9c7f1a33",   
		SerialNumber:  "SN123456789",   
		State:         1,
		HypervisorID:  "b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
		FailureDomain: "fd-01",
		DevicePath:    "/dev/path1",
		Name:          "dev-1",
		Size:          600 * 1024 * 1024 * 1024, // 600 GB raw
		Partitions: []cpLib.DevicePartition{cpLib.DevicePartition{
			PartitionID:   "b97c3464-ab3e-11f0-b32d-9775558a141a",
			PartitionPath: "/part/path3",
			NISDUUID:      "1",
			DevID:         "nvme-5e6b9c7f1a33",
			Size:          123467,
		},
		},
	}

	resp, err = c.PutDevice(&device)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("Device created: ", device.ID)

	// NISDs (Total = 500 GB)
	const nisdSize = 280 * 1024 * 1024 * 1024 // 250 GB

	nisds := []cpLib.Nisd{
		cpLib.Nisd{
			PeerPort:   8010,
			ID:         "86adee3a-d5da-11f0-8250-5f1ad86a5661",
			FailureDomain: []string{
				"2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
				"6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
				"b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
				"nvme-5e6b9c7f1a33",
			},
			TotalSize:     nisdSize,
			AvailableSize: nisdSize,
		},
		cpLib.Nisd{
			PeerPort:   8011,
			ID:         "86adee3a-d5da-11f0-8250-5f1ad86a5662",
			FailureDomain: []string{
				"2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
				"6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
				"b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
				"nvme-5e6b9c7f1a33",
			},
			TotalSize:     nisdSize,
			AvailableSize: nisdSize,
		},
	}

	// Create NISDs in parallel using goroutines
	var (
		wgNisd sync.WaitGroup
	)

	// buffered so goroutines don't block while reporting errors
	nisdErrCh := make(chan error, len(nisds))

	for i := range nisds {
		i := i // capture loop variable
		wgNisd.Add(1)

		go func() {
			defer wgNisd.Done()

			nisd := nisds[i]

			resp, err := c.PutNisd(&nisd)
			if err != nil {
				nisdErrCh <- fmt.Errorf("nisd worker %d: PutNisd error: %w", i, err)
				return
			}
			if resp == nil || !resp.Success {
				nisdErrCh <- fmt.Errorf("nisd worker %d: PutNisd failed", i)
				return
			}

			log.Info("NISD created | worker=", i, " | nisdID=", nisd.ID)
		}()
	}

	wgNisd.Wait()
	close(nisdErrCh)

	// If any goroutine failed, fail the test
	for e := range nisdErrCh {
		require.NoError(t, e)
	}

	log.Info("All NISDs created successfully. Total: ", len(nisds))

	// Create 5 VDEVs in parallel using goroutines
	const (
		vdevCount = 5
		vdevSize  = 100 * 1024 * 1024 * 1024 // 100 GB
	)

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		createdVdevs []*cpLib.VdevCfg
	)

	// total workers = NISD workers + VDEV workers
	totalWorkers := len(nisds) + vdevCount
	errCh := make(chan error, totalWorkers)

	//
	// ----------- NISD WORKERS -----------
	//
	for i := range nisds {
		i := i
		wg.Add(1)

		go func() {
			defer wg.Done()

			nisd := nisds[i]
			log.Info("Starting NISD worker: ", i)

			resp, err := c.PutNisd(&nisd)
			if err != nil {
				errCh <- fmt.Errorf("nisd worker %d: PutNisd error: %w", i, err)
				return
			}
			if resp == nil || !resp.Success {
				errCh <- fmt.Errorf("nisd worker %d: PutNisd failed", i)
				return
			}

			log.Info("NISD created | worker=", i, " | nisdID=", nisd.ID)
		}()
	}

	//
	// ----------- VDEV WORKERS -----------
	//
	for i := 0; i < vdevCount; i++ {
		i := i
		wg.Add(1)

		go func() {
			defer wg.Done()

			log.Info("Starting VDEV worker: ", i)

			vdev := &cpLib.VdevReq{
				Vdev: &cpLib.VdevCfg{
					Size:       vdevSize,
					NumReplica: 1,
				},
			}

			resp, err := c.CreateVdev(vdev)
			if err != nil {
				errCh <- fmt.Errorf("vdev worker %d: CreateVdev error: %w", i, err)
				return
			}
			if resp == nil || !resp.Success {
				errCh <- fmt.Errorf("vdev worker %d: CreateVdev failed", i)
				return
			}

			mu.Lock()
			createdVdevs = append(createdVdevs, vdev.Vdev)
			mu.Unlock()

			log.Info("VDEV created | worker=", i)
		}()
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		require.NoError(t, e)
	}

	require.Len(t, createdVdevs, vdevCount)

	log.Info("All parallel NISD and VDEV operations completed successfully")
}

func TestCreateVdevParallelFailure(t *testing.T) {
	c := newClient(t)

	log.Info("Starting TestCreateVdevParallel")

	// PDU
	pdu := cpLib.PDU{
		ID:            "2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
		Name:          "pdu-2",
		Location:      "us-east",
		PowerCapacity: "15Kw",
		Specification: "spec-pdu",
	}

	resp, err := c.PutPDU(&pdu)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("PDU created: ", pdu.ID)

	// Rack
	rack := cpLib.Rack{
		ID:            "6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
		PDUID:         "2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
		Name:          "rack-2",
		Location:      "us-east",
		Specification: "rack-spec",
	}

	resp, err = c.PutRack(&rack)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("Rack created: ", rack.ID)

	// Hypervisor
	hv := cpLib.Hypervisor{
		ID:        "b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
		RackID:    "6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
		Name:      "hv-2",
		IPAddrs:   []string{"127.0.0.1", "127.0.0.1"},
		PortRange: "8000-9000",
		SSHPort:   "6999",
	}

	resp, err = c.PutHypervisor(&hv)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("Hypervisor created: ", hv.ID)

	// Device
	device := cpLib.Device{
		ID:            "nvme-5e6b9c7f1a33",   
		SerialNumber:  "SN123456789",   
		State:         1,
		HypervisorID:  "b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
		FailureDomain: "fd-02",
		DevicePath:    "/dev/path1",
		Name:          "dev-2",
		Size:          600 * 1024 * 1024 * 1024, // 600 GB raw
		Partitions: []cpLib.DevicePartition{cpLib.DevicePartition{
			PartitionID:   "b97c3464-ab3e-11f0-b32d-9775558a141a",
			PartitionPath: "/part/path3",
			NISDUUID:      "1",
			DevID:         "nvme-5e6b9c7f1a33",
			Size:          123467,
		},
		},
	}

	resp, err = c.PutDevice(&device)
	assert.NoError(t, err)
	assert.True(t, resp.Success)

	log.Info("Device created: ", device.ID)

	// NISDs (Total = 320 GB)
	const nisdSize = 160 * 1024 * 1024 * 1024 // 160 GB

	nisds := []cpLib.Nisd{
		cpLib.Nisd{
			PeerPort:   8012,
			ID:         "86adee3a-d5da-11f0-8250-5f1ad86a5661",
			FailureDomain: []string{
				"2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
				"6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
				"b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
				"nvme-5e6b9c7f1a33",
			},
			TotalSize:     nisdSize,
			AvailableSize: nisdSize,
		},
		cpLib.Nisd{
			PeerPort:   8013,
			ID:         "86adee3a-d5da-11f0-8250-5f1ad86a5662",
			FailureDomain: []string{
				"2f4c7c3a-9d2a-4e3e-b1b7-6a6f8d7b2f1a",
				"6a9e1c44-3b9a-4d63-8f5c-0a2c1e8f4b77",
				"b3d8f0a2-7c5e-4b9f-9a62-2d7e1f6c8a54",
				"nvme-5e6b9c7f1a33",
			},
			TotalSize:     nisdSize,
			AvailableSize: nisdSize,
		},
	}

	// Create NISDs in parallel using goroutines
	var (
		wgNisd sync.WaitGroup
	)

	// buffered so goroutines don't block while reporting errors
	nisdErrCh := make(chan error, len(nisds))

	for i := range nisds {
		i := i // capture loop variable
		wgNisd.Add(1)

		go func() {
			defer wgNisd.Done()

			nisd := nisds[i]

			resp, err := c.PutNisd(&nisd)
			if err != nil {
				nisdErrCh <- fmt.Errorf("nisd worker %d: PutNisd error: %w", i, err)
				return
			}
			if resp == nil || !resp.Success {
				nisdErrCh <- fmt.Errorf("nisd worker %d: PutNisd failed", i)
				return
			}

			log.Info("NISD created | worker=", i, " | nisdID=", nisd.ID)
		}()
	}

	wgNisd.Wait()
	close(nisdErrCh)

	// If any goroutine failed, fail the test
	for e := range nisdErrCh {
		require.NoError(t, e)
	}

	log.Info("All NISDs created successfully. Total: ", len(nisds))

	// Create 5 VDEVs in parallel using goroutines
	const (
		vdevCount = 5
		vdevSize  = 100 * 1024 * 1024 * 1024 // 100 GB
	)

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		createdVdevs []*cpLib.VdevCfg
	)

	// buffered so goroutines don't block while reporting errors
	errCh := make(chan error, vdevCount)

	for i := 0; i < vdevCount; i++ {
		i := i // capture loop variable

		wg.Add(1)
		go func() {
			defer wg.Done()

			log.Info("Starting VDEV creation worker: ", i)

			vdev := &cpLib.VdevReq{
				Vdev: &cpLib.VdevCfg{
					Size:       vdevSize,
					NumReplica: 1,
				},
			}

			resp, err := c.CreateVdev(vdev)
			if err != nil {
				errCh <- fmt.Errorf("worker %d: CreateVdev error: %w", i, err)
				return
			}
			if resp == nil || !resp.Success {
				errCh <- fmt.Errorf("worker %d: CreateVdev failed", i)
				return
			}

			mu.Lock()
			createdVdevs = append(createdVdevs, vdev.Vdev)
			mu.Unlock()

			log.Info("VDEV created | worker=", i)
		}()
	}

	wg.Wait()
	close(errCh)

	// If any goroutine failed, fail the test
	for e := range errCh {
		require.NoError(t, e)
	}

	require.Len(t, createdVdevs, vdevCount)

	log.Info("All VDEVs created successfully. Total: ", len(createdVdevs))

	// validation of created vdevs
	assert.Equal(t, vdevCount, len(createdVdevs),
		"unexpected number of VDEVs created")	
}