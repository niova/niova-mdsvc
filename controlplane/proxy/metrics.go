package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	jwtAuth "github.com/00pauln00/niova-mdsvc/controlplane/auth/jwt"
	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"

	pmdbClient "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceclient"
	PumiceDBCommon "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	funclib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/common"
)

// proxyMetrics holds the metrics text scraped by pollNisdMetrics,
// plus atomic counters for each API handler type.
type proxyMetrics struct {
	nisdMetricsMu   sync.RWMutex
	nisdMetricsText string
	getKVTotal      int64
	putKVTotal      int64
	getLeaseTotal   int64
	putLeaseTotal   int64
	getFuncTotal    int64
	putFuncTotal    int64
}

type metricCollector struct {
	funcName string
	payload  any
	errLabel string
	render   func(payload any, proxyStr string, b *strings.Builder)
}

func (m *proxyMetrics) incGetKV()    { atomic.AddInt64(&m.getKVTotal, 1) }
func (m *proxyMetrics) incPutKV()    { atomic.AddInt64(&m.putKVTotal, 1) }
func (m *proxyMetrics) incGetLease() { atomic.AddInt64(&m.getLeaseTotal, 1) }
func (m *proxyMetrics) incPutLease() { atomic.AddInt64(&m.putLeaseTotal, 1) }
func (m *proxyMetrics) incGetFunc()  { atomic.AddInt64(&m.getFuncTotal, 1) }
func (m *proxyMetrics) incPutFunc()  { atomic.AddInt64(&m.putFuncTotal, 1) }

func (m *proxyMetrics) setNisdMetricsText(s string) {
	m.nisdMetricsMu.Lock()
	m.nisdMetricsText = s
	m.nisdMetricsMu.Unlock()
}

func (m *proxyMetrics) getNisdMetricsText() string {
	m.nisdMetricsMu.RLock()
	defer m.nisdMetricsMu.RUnlock()
	return m.nisdMetricsText
}

func nisdAvailSizeCollector() metricCollector {
	return metricCollector{
		funcName: cpLib.GET_NISD_AVAILABLE_SIZES,
		payload:  cpLib.GetReq{Fields: cpLib.NisdAvailSizeFields},
		errLabel: "NISD metrics",
		render: func(payload any, proxyStr string, b *strings.Builder) {
			nisdList, ok := payload.([]cpLib.NisdListAvailSize)
			if !ok {
				return
			}
			fmt.Fprint(b, "# HELP proxy_nisd_total_size_bytes Total size of NISD in bytes\n")
			fmt.Fprint(b, "# TYPE proxy_nisd_total_size_bytes gauge\n")
			fmt.Fprint(b, "# HELP proxy_nisd_available_size_bytes Available size of NISD in bytes\n")
			fmt.Fprint(b, "# TYPE proxy_nisd_available_size_bytes gauge\n")
			fmt.Fprint(b, "# HELP proxy_nisd_info Info metric for NISD (value=1)\n")
			fmt.Fprint(b, "# TYPE proxy_nisd_info gauge\n")
			for _, n := range nisdList {
				labels := fmt.Sprintf(`proxy="%s",nisd="%s"`, proxyStr, n.ID)
				fmt.Fprintf(b, "proxy_nisd_total_size_bytes{%s} %d\n", labels, n.TotalSize)
				fmt.Fprintf(b, "proxy_nisd_available_size_bytes{%s} %d\n", labels, n.AvailableSize)
				if n.HV != "" {
					infoLabels := fmt.Sprintf(`proxy="%s",nisd="%s",pdu="%s",rack="%s",hv="%s"`,
						proxyStr, n.ID, n.PDU, n.Rack, n.HV)
					fmt.Fprintf(b, "proxy_nisd_info{%s} 1\n", infoLabels)
				}
			}
		},
	}
}

func vdevConfigCollector() metricCollector {
	return metricCollector{
		funcName: cpLib.GET_ALL_VDEV,
		payload:  nil,
		errLabel: "VDev config",
		render: func(payload any, proxyStr string, b *strings.Builder) {
			vdevList, ok := payload.([]cpLib.VdevCfg)
			if !ok {
				return
			}
			fmt.Fprint(b, "# HELP proxy_vdev_size_bytes Configured size of VDev in bytes\n")
			fmt.Fprint(b, "# TYPE proxy_vdev_size_bytes gauge\n")
			fmt.Fprint(b, "# HELP proxy_vdev_num_chunks Number of chunks in VDev\n")
			fmt.Fprint(b, "# TYPE proxy_vdev_num_chunks gauge\n")
			fmt.Fprint(b, "# HELP proxy_vdev_num_replicas Number of replicas for VDev\n")
			fmt.Fprint(b, "# TYPE proxy_vdev_num_replicas gauge\n")
			fmt.Fprint(b, "# HELP proxy_vdev_info Info metric for VDev (value=1)\n")
			fmt.Fprint(b, "# TYPE proxy_vdev_info gauge\n")
			for _, v := range vdevList {
				vl := fmt.Sprintf(`proxy="%s",vdev="%s"`, proxyStr, v.ID)
				fmt.Fprintf(b, "proxy_vdev_size_bytes{%s} %d\n", vl, v.Size)
				fmt.Fprintf(b, "proxy_vdev_num_chunks{%s} %d\n", vl, uint64(v.NumChunks))
				fmt.Fprintf(b, "proxy_vdev_num_replicas{%s} %d\n", vl, uint64(v.NumReplica))
				fdType := v.FilterType
				if fdType == "" {
					fdType = "any"
				}
				fdID := v.FilterID
				if fdID == "" {
					fdID = "none"
				}
				infoL := fmt.Sprintf(`proxy="%s",vdev="%s",fd_type="%s",fd_id="%s"`,
					proxyStr, v.ID, fdType, fdID)
				fmt.Fprintf(b, "proxy_vdev_info{%s} 1\n", infoL)
			}
		},
	}
}

func vdevPlacementCollector() metricCollector {
	return metricCollector{
		funcName: cpLib.GET_VDEV_CHUNK_INFO,
		payload:  cpLib.GetReq{GetAll: true},
		errLabel: "VDev placement",
		render: func(payload any, proxyStr string, b *strings.Builder) {
			vdevs, ok := payload.([]cpLib.Vdev)
			if !ok {
				return
			}
			fmt.Fprint(b, "# HELP proxy_vdev_nisd_chunk_count Number of chunks of a VDev on a particular NISD\n")
			fmt.Fprint(b, "# TYPE proxy_vdev_nisd_chunk_count gauge\n")
			for _, v := range vdevs {
				for _, nc := range v.NisdToChkMap {
					pl := fmt.Sprintf(`proxy="%s",vdev="%s",nisd="%s"`, proxyStr, v.Cfg.ID, nc.Nisd.ID)
					fmt.Fprintf(b, "proxy_vdev_nisd_chunk_count{%s} %d\n", pl, len(nc.Chunk))
				}
			}
		},
	}
}

func (handler *proxyHandler) callControlPlaneFunc(funcName string, token string, payload any) (any, error) {
	if handler.pmdbClientObj == nil {
		return nil, fmt.Errorf("PMDB client not initialized")
	}

	r := &funclib.FuncReq{
		Name: funcName,
		Args: cpLib.CPReq{Token: token, Payload: payload},
	}

	var replyBytes []byte
	reqArgs := &pmdbClient.PmdbReq{
		Request:  encode(r),
		ReqType:  PumiceDBCommon.FUNC_REQ,
		GetReply: 1,
		Reply:    &replyBytes,
	}

	if err := handler.pmdbClientObj.Get(reqArgs); err != nil {
		return nil, fmt.Errorf("Get failed: %w", err)
	}
	if len(replyBytes) == 0 {
		return nil, fmt.Errorf("empty reply")
	}

	var cpResp cpLib.CPResp
	if err := PumiceDBCommon.Decoder(PumiceDBCommon.GOB, replyBytes, &cpResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	if cpResp.Error != nil {
		return nil, fmt.Errorf("control plane function %q failed: %s", funcName, cpResp.Error.Message)
	}
	return cpResp.Payload, nil
}

func (handler *proxyHandler) pollNisdMetrics(ctx context.Context) {
	tc := &jwtAuth.Token{
		Secret: []byte(cpLib.CP_SECRET),
		TTL:    365 * 24 * time.Hour,
	}
	token, err := tc.CreateToken(map[string]any{
		"userID":  handler.clientUUID.String(),
		"role":    "admin",
		"isAdmin": true,
	})
	if err != nil {
		log.Error("pollNisdMetrics: failed to create service token: ", err)
		return
	}

	collectors := []metricCollector{
		nisdAvailSizeCollector(),
		vdevConfigCollector(),
		vdevPlacementCollector(),
	}

	ticker := time.NewTicker(handler.metricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("pollNisdMetrics: shutting down")
			return
		case <-ticker.C:
			if handler.pmdbClientObj == nil {
				continue
			}

			proxyStr := handler.clientUUID.String()
			builder := &strings.Builder{}

			type fetchResult struct {
				payload any
				err     error
			}
			fetchResults := make([]fetchResult, len(collectors))

			var wg sync.WaitGroup
			wg.Add(len(collectors))
			for i, c := range collectors {
				go func(i int, c metricCollector) {
					defer wg.Done()
					p, err := handler.callControlPlaneFunc(c.funcName, token, c.payload)
					fetchResults[i] = fetchResult{payload: p, err: err}
				}(i, c)
			}
			wg.Wait()

			var errs []error
			for i, c := range collectors {
				r := fetchResults[i]
				if r.err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", c.errLabel, r.err))
					continue
				}
				c.render(r.payload, proxyStr, builder)
			}
			if len(errs) > 0 {
				log.Error("pollNisdMetrics: some queries failed: ", errors.Join(errs...))
			}
			handler.metrics.setNisdMetricsText(builder.String())
		}
	}
}

// startMetricsServer runs an HTTP server on the given port exposing /metrics
// in Prometheus text format.
func (handler *proxyHandler) startMetricsServer(port int) {
	go handler.pollNisdMetrics(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(w, handler.metrics.getNisdMetricsText())
		proxy := handler.clientUUID.String()
		m := &handler.metrics
		fmt.Fprintf(w, "# HELP proxy_requests_total Total API requests handled by the proxy\n")
		fmt.Fprintf(w, "# TYPE proxy_requests_total counter\n")
		fmt.Fprintf(w, "proxy_requests_total{proxy=%q,method=\"get\",type=\"kv\"} %d\n", proxy, atomic.LoadInt64(&m.getKVTotal))
		fmt.Fprintf(w, "proxy_requests_total{proxy=%q,method=\"put\",type=\"kv\"} %d\n", proxy, atomic.LoadInt64(&m.putKVTotal))
		fmt.Fprintf(w, "proxy_requests_total{proxy=%q,method=\"get\",type=\"lease\"} %d\n", proxy, atomic.LoadInt64(&m.getLeaseTotal))
		fmt.Fprintf(w, "proxy_requests_total{proxy=%q,method=\"put\",type=\"lease\"} %d\n", proxy, atomic.LoadInt64(&m.putLeaseTotal))
		fmt.Fprintf(w, "proxy_requests_total{proxy=%q,method=\"get\",type=\"func\"} %d\n", proxy, atomic.LoadInt64(&m.getFuncTotal))
		fmt.Fprintf(w, "proxy_requests_total{proxy=%q,method=\"put\",type=\"func\"} %d\n", proxy, atomic.LoadInt64(&m.putFuncTotal))
	})
	addr := fmt.Sprintf(":%d", port)
	log.Info("Starting metrics server on ", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("Metrics server error: ", err)
	}
}
