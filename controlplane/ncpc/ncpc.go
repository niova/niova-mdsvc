package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	maps "golang.org/x/exp/maps"

	ctlplcl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	"github.com/00pauln00/niova-mdsvc/controlplane/requestResponseLib"
	userClient "github.com/00pauln00/niova-mdsvc/controlplane/user/client"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	PumiceDBCommon "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	leaseClientLib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicelease/client"
	leaseLib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicelease/common"
	compressionLib "github.com/00pauln00/niova-pumicedb/go/pkg/utils/compressor"
	serviceDiscovery "github.com/00pauln00/niova-pumicedb/go/pkg/utils/servicediscovery"
)

type clientReq struct {
	Request  requestResponseLib.KVRequest
	Response requestResponseLib.KVResponse
}

type clientHandler struct {
	requestKey          string
	requestValue        string
	clientReqArr        []clientReq
	raftUUID            string
	addr                string
	port                string
	operation           string
	configPath          string
	logPath             string
	resultFile          string
	lookoutID           string
	relaxedConsistency  bool
	count               int
	seed                int
	clientAPIObj        serviceDiscovery.ServiceDiscoveryHandler
	seqNum              uint64
	valSize             int
	serviceRetry        int
	userToken           string
	username            string
	password            string
	regenerateNISDUUIDs bool
}

type request struct {
	Opcode    string      `json:"Operation"`
	Key       string      `json:"Key"`
	Value     interface{} `json:"Value"`
	Timestamp time.Time   `json:"Request_timestamp"`
}

type response struct {
	Status         int         `json:"Status"`
	ResponseValue  interface{} `json:"Response"`
	SequenceNumber uint64      `json:"Sequence_number"`
	Validate       bool        `json:"validate"`
	Timestamp      time.Time   `json:"Response_timestamp"`
}

type opData struct {
	RequestData  request       `json:"Request"`
	ResponseData response      `json:"Response"`
	TimeDuration time.Duration `json:"Req_resolved_time"`
}

type nisdData struct {
	UUID      uuid.UUID `json:"UUID"`
	Status    string    `json:"Status"`
	WriteSize string    `json:"WriteSize"`
}

func usage() {
	flag.PrintDefaults()
	os.Exit(0)
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randSeq(n int, r *rand.Rand) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return b
}

func (c *clientHandler) appendReq(kvArr *[]clientReq, key string, value []byte) {
	creq := clientReq{}
	creq.Request.Key = key
	creq.Request.Value = value

	*kvArr = append(*kvArr, creq)
}

// dummy function to mock user filling up multiple req
func (c *clientHandler) generateVdevRange() []clientReq {

	var kvArr []clientReq
	r := rand.New(rand.NewSource(int64(c.seed)))
	var nodeUUID []string
	var vdevUUID []string
	nodeNisdMap := make(map[string][]string)
	//Node UUID
	/*
		FailureDomain
		Info
		State
		HostName
		NISD-UUIDs
	*/
	noUUID := c.count
	for i := int64(0); i < int64(noUUID); i++ {
		randomNodeUUID := uuid.NewV4()
		nodeUUID = append(nodeUUID, randomNodeUUID.String())
		prefix := "node." + randomNodeUUID.String()

		//NISD-UUIDs
		for j := int64(0); j < int64(noUUID); j++ {
			randUUID := uuid.NewV4()
			nodeNisdMap[randomNodeUUID.String()] = append(nodeNisdMap[randomNodeUUID.String()], randUUID.String())
		}

		nval, _ := json.Marshal(nodeNisdMap[randomNodeUUID.String()])
		c.appendReq(&kvArr, prefix+".NISD-UUIDs", nval)
	}
	//NISD
	/*
		Node-UUID
		Config-Info
		Device-Type
		Device-Path
		Device-Status
		Device-Info
		Device-Size
		Provisioned-Size
		VDEV-UUID.Chunk-Number.Chunk-Component-UUID
	*/
	for _, node := range nodeUUID {
		for _, nisd := range nodeNisdMap[node] {
			prefix := "nisd." + nisd
			randomNodeUUID := uuid.NewV4()

			//Node-UUID
			c.appendReq(&kvArr, prefix+".Node-UUID", []byte(node))

			nval, _ := json.Marshal(nodeNisdMap[randomNodeUUID.String()])
			c.appendReq(&kvArr, prefix+".NISD-UUIDs", nval)

			//Config-Info
			c.appendReq(&kvArr, prefix+".Config-Info", randSeq(c.valSize, r))

			//VDEV-UUID
			for j := int64(0); j < int64(noUUID); j++ {
				randUUID := uuid.NewV4()
				partNodePrefix := prefix + "." + randUUID.String()
				c.appendReq(&kvArr, partNodePrefix, randSeq(c.valSize, r))
				vdevUUID = append(vdevUUID, randUUID.String())
			}
		}
	}

	//Vdev
	/*
		User-Token
		Snapshots-Txn-Seqno
		Chunk-Number.Chunk-Component-UUID
	*/
	for i := int64(0); i < int64(len(vdevUUID)); i++ {
		prefix := "v." + vdevUUID[i]
		c.appendReq(&kvArr, prefix+".User-Token", randSeq(c.valSize, r))

		noChunck := c.count
		Cprefix := prefix + ".c"
		for j := int64(0); j < int64(noChunck); j++ {
			randUUID := uuid.NewV4()
			Chunckprefix := Cprefix + strconv.Itoa(int(j)) + "." + randUUID.String()
			c.appendReq(&kvArr, Chunckprefix, randSeq(c.valSize, r))
		}
	}
	return kvArr
}

// Function to get command line parameters
func (c *clientHandler) getCmdParams() {

	flag.StringVar(&c.requestKey, "k", "", "Key - For ReadRange pass '<prefix>*' e.g. : -k 'vdev.*'")
	flag.StringVar(&c.addr, "a", "127.0.0.1", "Addr value")
	flag.StringVar(&c.port, "p", "1999", "Port value")
	flag.StringVar(&c.requestValue, "v", "", "Value")
	flag.StringVar(&c.raftUUID, "ru", "", "RaftUUID of the cluster to be queried")
	flag.StringVar(&c.configPath, "c", "./gossipNodes", "gossip nodes config file path")
	flag.StringVar(&c.logPath, "l", cpLib.DefaultLogPath(), "Log path")
	flag.StringVar(&c.operation, "o", "rw", "Specify the opeation to perform")
	flag.StringVar(&c.resultFile, "j", "json_output", "Path along with file name for the resultant json file")
	flag.BoolVar(&c.regenerateNISDUUIDs, "rnu", false, "Regenerate new UUIDs for all NISDs using the topology JSON as structure reference")
	flag.StringVar(&c.lookoutID, "u", "", "Lookout uuid")
	flag.IntVar(&c.count, "n", 1, "Write number of key/value pairs per key type (Default 1 will write the passed key/value)")
	flag.BoolVar(&c.relaxedConsistency, "r", false, "Set this flag if range could be performed with relaxed consistency")
	flag.IntVar(&c.seed, "s", 10, "Seed value")
	flag.IntVar(&c.valSize, "vs", 512, "Random value generation size")
	flag.Uint64Var(&c.seqNum, "S", math.MaxUint64, "Sequence Number for read")
	flag.IntVar(&c.serviceRetry, "sr", 1, "how many times you want to retry to pick the server if proxy is not available")
	flag.StringVar(&c.userToken, "ut", "", "User authentication token")
	flag.StringVar(&c.username, "user", "", "Username for authentication")
	flag.StringVar(&c.password, "pass", "", "Password/SecretKey for authentication")
	flag.Parse()
}

func (c *clientHandler) complete(data []byte) error {
	err := os.WriteFile(c.resultFile+".json", data, 0644)
	if err != nil {
		log.Error("Error in writing output to the file : ", err)
	}
	return err
}

func prepareOutput(status int, operation string, key string, value interface{}, seqNo uint64) *opData {
	requestMeta := request{
		Opcode: operation,
		Key:    key,
		Value:  value,
	}

	responseMeta := response{
		SequenceNumber: seqNo,
		Status:         status,
		ResponseValue:  value,
	}

	operationObj := opData{
		RequestData:  requestMeta,
		ResponseData: responseMeta,
	}
	return &operationObj
}

func (c *clientHandler) getNISDInfo() map[string]nisdData {
	data := c.clientAPIObj.GetMembership()
	nisdDataMap := make(map[string]nisdData)
	for _, node := range data {
		if (node.Tags["Type"] == "LOOKOUT") && (node.Status == "alive") {
			for cuuid, value := range node.Tags {
				d_uuid, err := compressionLib.DecompressUUID(cuuid)
				if err == nil {
					CompressedStatus := value[0]
					//Decompress
					thisNISDData := nisdData{}
					thisNISDData.UUID, err = uuid.FromString(d_uuid)
					if err != nil {
						log.Error(err)
					}
					if string(CompressedStatus) == "1" {
						thisNISDData.Status = "Alive"
					} else {
						thisNISDData.Status = "Dead"
					}

					nisdDataMap[d_uuid] = thisNISDData
				}
			}
		}
	}
	return nisdDataMap
}

func (c *clientHandler) prepareLOInfoRequest(b *bytes.Buffer) error {
	//Request obj
	var o requestResponseLib.LookoutRequest
	var err error

	//Parse UUID
	o.UUID, err = uuid.FromString(c.requestKey)
	if err != nil {
		log.Error("Invalid argument - key must be UUID")
		return err
	}
	o.Cmd = string(c.requestValue)

	enc := gob.NewEncoder(b)
	err = enc.Encode(o)
	if err != nil {
		log.Error("Encodng error : ", err)
	}
	return err
}

func (c *clientHandler) prepNSendReq(rncui string, isWrite bool, itr int) error {

	var rqb bytes.Buffer

	encoder := gob.NewEncoder(&rqb)
	err := encoder.Encode(c.clientReqArr[itr].Request)
	if err != nil {
		return err
	}

	//Send the request
	rsb, err := c.clientAPIObj.Request(rqb.Bytes(), "/app?rncui="+rncui+"&wsn=0", isWrite)
	if err != nil {
		return err
	}

	//Decode the response to get the status of the operation.
	res := &c.clientReqArr[itr].Response
	dec := gob.NewDecoder(bytes.NewBuffer(rsb))
	return dec.Decode(res)
}

func (c *clientHandler) write(wresult bool) ([]byte, error) {

	c.operation = "write"

	var wg sync.WaitGroup
	var err error
	// Create a int channel of fixed size to enqueue max requests
	requestLimiter := make(chan int, 100)

	// iterate over req and res, while performing reqs
	for i := 0; i < len(c.clientReqArr); i++ {
		wg.Add(1)
		requestLimiter <- 1
		c.clientReqArr[i].Request.Operation = requestResponseLib.KV_WRITE
		go func(itr int, rncui string) {
			defer func() {
				wg.Done()
				<-requestLimiter
			}()

			err = func() error {
				err := c.prepNSendReq(rncui, true, itr)
				return err
			}()
			if err != nil {
				return
			}
		}(i, uuid.NewV4().String()+":0:0:0:0")
	}
	wg.Wait()
	file, err := json.MarshalIndent(c.clientReqArr, "", " ")
	if err != nil {
		log.Error("Failed to json.MarshalIndent cli.clientReqArr")
	}
	//If calling function asked to write the result immediately
	if wresult {
		err = os.WriteFile(c.resultFile+".json", file, 0644)
		if err != nil {
			log.Error("Error in writing output to the file : ", err)
		}
		return nil, err
	}
	//else return the result byte array
	return json.MarshalIndent(c.clientReqArr, "", " ")
}

func (c *clientHandler) read() ([]byte, error) {

	//read single key passed from cmdline.
	creq := clientReq{}
	creq.Request.Operation = requestResponseLib.KV_READ
	creq.Request.Key = c.requestKey
	creq.Request.Value = []byte("")

	c.clientReqArr = append(c.clientReqArr, creq)

	c.operation = "read"
	err := func() error {
		return c.prepNSendReq("", false, 0)
	}()

	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(c.clientReqArr, "", " ")
}

func (c *clientHandler) rangeRead() ([]byte, error) {
	var prefix, key string
	var op int
	var err error
	var seqNum uint64

	c.operation = "read"

	prefix = c.requestKey[:len(c.requestKey)-1]
	key = c.requestKey[:len(c.requestKey)-1]

	op = requestResponseLib.KV_RANGE_READ
	// get sequence number from arguments
	seqNum = c.seqNum
	// Keep calling range request till ContinueRead is true

	creq := clientReq{}
	creq.Request.Prefix = prefix
	creq.Request.Operation = op
	creq.Request.Consistent = !c.relaxedConsistency
	creq.Request.Key = key
	resultMap := make(map[string][]byte)
	for {
		var rqb bytes.Buffer
		var rsb []byte

		creq.Request.Key = key
		creq.Request.SeqNum = seqNum

		rso := &creq.Response

		encoder := gob.NewEncoder(&rqb)
		err = encoder.Encode(creq.Request)
		if err != nil {
			log.Error("Encoding error : ", err)
		}

		//Send the request
		rsb, err := c.clientAPIObj.Request(rqb.Bytes(), "/app", false)
		if err != nil {
			log.Error("Error while sending request : ", err)
		}

		if len(rsb) == 0 {
			err = errors.New("Key not found")
			log.Error("Empty response : ", err)
			rso.Status = -1
			rso.Key = key
			break
		}
		// decode the responseObj
		dec := gob.NewDecoder(bytes.NewBuffer(rsb))
		err = dec.Decode(rso)
		if err != nil {
			log.Error("Decoding error : ", err)
			break
		}

		// copy result to global result variable
		maps.Copy(resultMap, rso.ResultMap)
		//Change sequence number and key for next iteration
		seqNum = rso.SeqNum
		key = rso.Key
		if !rso.ContinueRead {
			break
		}
	}
	c.clientReqArr = append(c.clientReqArr, creq)
	maps.Clear(c.clientReqArr[0].Response.ResultMap)
	maps.Copy(c.clientReqArr[0].Response.ResultMap, resultMap)

	return json.MarshalIndent(c.clientReqArr, "", " ")
}

// check and fill request map acc to req count
func (c *clientHandler) prepWriteReq(rArr []clientReq) {
	c.clientReqArr = rArr
}

func (c *clientHandler) getKVArray() []clientReq {
	var rArr []clientReq
	if c.requestKey == "" && c.requestValue == "" {
		rArr = c.generateVdevRange()
	} else {
		creq := clientReq{}
		creq.Request.Key = c.requestKey
		creq.Request.Value = []byte(c.requestValue)
		rArr = append(c.clientReqArr, creq)
	}
	return rArr
}

func (c *clientHandler) processReadWriteReq(rArr []clientReq) ([]byte, error) {

	//Wait till proxy is ready
	err := c.waitServiceInit("PROXY")
	if err != nil {
		return nil, err
	}

	var data []byte
	switch c.operation {
	case "rw":
		c.prepWriteReq(rArr)
		data, err = c.write(true)
		if err == nil {
			data, err = c.read()
		}
		break
	case "write":
		c.prepWriteReq(rArr)
		data, err = c.write(false)
		break
	case "read":
		if !isRangeRequest(c.requestKey) {
			data, err = c.read()
		} else {
			data, err = c.rangeRead()
		}
		break
	default:
		log.Error("Invalid operation type")
	}
	return data, err
}

func (c *clientHandler) processConfig() ([]byte, error) {
	return c.clientAPIObj.GetPMDBServerConfig()
}

func (c *clientHandler) processMembership() ([]byte, error) {
	toJson := c.clientAPIObj.GetMembership()
	return json.MarshalIndent(toJson, "", " ")
}

func (c *clientHandler) processGeneral() {
	fmt.Printf("\033[2J")
	fmt.Printf("\033[2;0H")
	fmt.Print("UUID")
	fmt.Printf("\033[2;38H")
	fmt.Print("Type")
	fmt.Printf("\033[2;50H")
	fmt.Println("Status")
	offset := 3
	for {
		lineCounter := 0
		data := c.clientAPIObj.GetMembership()
		for _, node := range data {
			currentLine := offset + lineCounter
			fmt.Print(node.Name)
			fmt.Printf("\033[%d;38H", currentLine)
			fmt.Print(node.Tags["Type"])
			fmt.Printf("\033[%d;50H", currentLine)
			fmt.Println(node.Status)
			lineCounter += 1
		}
		time.Sleep(2 * time.Second)
		fmt.Printf("\033[3;0H")
		for i := 0; i < lineCounter; i++ {
			fmt.Println("                                                       ")
		}
		fmt.Printf("\033[3;0H")
	}
}

func (c *clientHandler) processNisd() {
	fmt.Printf("\033[2J")
	fmt.Printf("\033[2;0H")
	fmt.Println("NISD_UUID")
	fmt.Printf("\033[2;38H")
	fmt.Print("Status")
	fmt.Printf("\033[2;45H")
	fmt.Println("Parent_UUID(Lookout)")
	offset := 3
	for {
		lCounter := 0
		data := c.clientAPIObj.GetMembership()
		for _, node := range data {
			if (node.Tags["Type"] == "LOOKOUT") && (node.Status == "alive") {
				for uuid, value := range node.Tags {
					if uuid != "Type" {
						currLine := offset + lCounter
						fmt.Print(uuid)
						fmt.Printf("\033[%d;38H", currLine)
						fmt.Print(strings.Split(value, "_")[0])
						fmt.Printf("\033[%d;45H", currLine)
						fmt.Println(node.Name)
						lCounter += 1
					}
				}
			}
		}
		time.Sleep(2 * time.Second)
		fmt.Printf("\033[3;0H")
		for i := 0; i < lCounter; i++ {
			fmt.Println("                                                       ")
		}
		fmt.Printf("\033[3;0H")
	}
}

func (c *clientHandler) processGossip() ([]byte, error) {
	fileData, err := c.clientAPIObj.GetPMDBServerConfig()
	if err != nil {
		log.Error("Error while getting pmdb server config data : ", err)
		return nil, err
	}
	f, _ := os.OpenFile(c.resultFile+".json", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	f.WriteString(string(fileData))

	return fileData, err
}

func (c *clientHandler) processProxyStat() ([]byte, error) {
	c.clientAPIObj.ServerChooseAlgorithm = 2
	c.clientAPIObj.UseSpecificServerName = c.requestKey
	resBytes, err := c.clientAPIObj.Request(nil, "/stat", false)
	if err != nil {
		log.Error("Error while sending request to proxy : ", err)
	}
	return resBytes, err
}

func (c *clientHandler) processLookoutInfo() ([]byte, error) {
	c.clientAPIObj.ServerChooseAlgorithm = 2
	c.clientAPIObj.UseSpecificServerName = c.lookoutID

	var b bytes.Buffer
	var r []byte

	err := c.prepareLOInfoRequest(&b)
	if err != nil {
		log.Error("Error while preparing lookout request")
		return nil, err
	}

	r, err = c.clientAPIObj.Request(b.Bytes(), "/v1/", false)
	if err != nil {
		log.Error("Error while sending request : ", err)
		return nil, err
	}

	return r, err
}

func (c *clientHandler) waitServiceInit(service string) error {
	err := c.clientAPIObj.TillReady(service, c.serviceRetry)
	if err != nil {
		opStat := prepareOutput(-1, "setup", "", err.Error(), 0)
		c.writeData2Json(opStat)
	}
	return err
}

func (c *clientHandler) initServiceDisHandler() {
	c.clientAPIObj = serviceDiscovery.ServiceDiscoveryHandler{
		HTTPRetry: 10,
		SerfRetry: 5,
		RaftUUID:  c.raftUUID,
	}
}

func (c *clientHandler) prepareLeaseHandlers(leaseReqHandler *leaseClientLib.LeaseClientReqHandler) error {
	raft, err := uuid.FromString(c.raftUUID)
	if err != nil {
		log.Error("Error getting raft UUID ", err)
		return err
	}

	leaseClientObj := leaseClientLib.LeaseClient{
		RaftUUID:            raft,
		ServiceDiscoveryObj: &c.clientAPIObj,
	}

	leaseReqHandler.LeaseClientObj = &leaseClientObj
	return err
}

func getLeaseOperationType(op string) int {
	switch op {
	case "GetLease":
		return leaseLib.GET
	case "LookupLease":
		return leaseLib.LOOKUP
	case "RefreshLease":
		return leaseLib.REFRESH
	default:
		log.Error("Invalid Lease operation type: ", op)
		return -1
	}
}

// Write to Json
func (c *clientHandler) writeData2Json(data interface{}) {
	file, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		log.Error("Error marshaling data to JSON: ", err)
		return
	}

	err = os.WriteFile(c.resultFile+".json", file, 0644)
	if err != nil {
		log.Error("Error writing file: ", err)
	}
}

func (c *clientHandler) performLeaseReq(resource, client string) ([]byte, error) {
	c.clientAPIObj.TillReady("PROXY", c.serviceRetry)

	op := getLeaseOperationType(c.operation)

	var lrh leaseClientLib.LeaseClientReqHandler
	err := c.prepareLeaseHandlers(&lrh)
	if err != nil {
		log.Error("Error while preparing lease handlers : ", err)
		return nil, err
	}

	if (op != leaseLib.LOOKUP) && (op != leaseLib.LOOKUP_VALIDATE) {
		lrh.Rncui, lrh.WSN = uuid.NewV4().String()+":0:0:0:0", 0
	}

	err = lrh.InitLeaseReq(client, resource, op)
	if err != nil {
		log.Error("error while initializing lease req : ", err)
		return nil, err
	}
	err = lrh.LeaseOperationOverHTTP()
	if err != nil {
		log.Error("Error sending lease request : ", err)
		return nil, err
	}

	data, err := json.MarshalIndent(lrh, "", " ")

	return data, err
}

func isRangeRequest(requestKey string) bool {
	return requestKey[len(requestKey)-1:] == "*"
}

// Check if for single key write operation, value has been passed.
func isSingleWriteReqValid(cli *clientHandler) bool {
	if cli.operation == "write" && cli.count == 1 && cli.requestValue == "" {
		return false
	}

	return true
}

func DumpVdevCfgsToJSON(vdevs []cpLib.VdevCfg, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, ".vdevcfg-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	enc := json.NewEncoder(tmpFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(vdevs); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpFile.Name(), filePath)
}

// ensureUserToken resolves the user token before any authenticated operation.
func (c *clientHandler) ensureUserToken() error {
	if c.userToken != "" {
		return nil
	}
	if c.username == "" || c.password == "" {
		return fmt.Errorf("authentication required: provide -ut <token> or both -user and -pass")
	}
	authClient, _ := userClient.New(userClient.Config{
		AppUUID:          uuid.NewV4().String(),
		RaftUUID:         c.raftUUID,
		GossipConfigPath: c.configPath,
		LogLevel:         "Error",
		LogFile:          c.logPath,
	})
	resp, err := authClient.Login(c.username, c.password)
	if err != nil {
		return fmt.Errorf("login failed: %v", err)
	}
	if !resp.Success {
		return fmt.Errorf("login failed: %s", resp.Error)
	}
	log.Infof("Login successful, using token for operation")
	c.userToken = resp.AccessToken
	return nil
}

// regenerateNISDUUIDs generates a fresh UUID for every NISD in pdus and calls PutNisd.
// The topology JSON is used only as a structural reference.
func regenerateNISDUUIDs(pdus []cpLib.PDU, c *ctlplcl.CliCFuncs) error {
	port := 13000
	for i := range pdus {
		for j := range pdus[i].Racks {
			for k := range pdus[i].Racks[j].Hypervisors {
				hv := &pdus[i].Racks[j].Hypervisors[k]
				for l := range hv.Dev {
					dev := &hv.Dev[l]
					for m := range dev.Partitions {
						p := &dev.Partitions[m]
						p.NISDUUID = uuid.NewV4().String()
						nisd := cpLib.Nisd{
							ID:            p.NISDUUID,
							PeerPort:      uint16(port),
							FailureDomain: []string{pdus[i].ID, pdus[i].Racks[j].ID, hv.ID, dev.ID, p.PartitionID},
							TotalSize:     p.Size,
							AvailableSize: p.Size,
							NetInfo: cpLib.NetInfoList{
								{IPAddr: hv.IPAddrs[0], Port: uint16(port)},
								{IPAddr: hv.IPAddrs[1], Port: uint16(port)},
							},
							NetInfoCnt: 2,
						}
						resp, err := c.PutNisd(&nisd)
						if err != nil || resp == nil || !resp.Success {
							log.Errorf("Failed to update NISD %s (port %d): %v", nisd.ID, port, err)
						} else {
							log.Infof("Updated NISD %s (port %d)", nisd.ID, port)
						}
						port += 2
					}
				}
			}
		}
	}
	return nil
}

func (c *clientHandler) populateTopology(jsonPath string) error {
	if jsonPath == "" {
		return fmt.Errorf("topology JSON file path is required (-v <path>)")
	}

	if err := c.ensureUserToken(); err != nil {
		return err
	}

	cliCFuncs := ctlplcl.InitCliCFuncs(uuid.NewV4().String(), c.raftUUID, c.configPath, c.logPath)
	var pdus []cpLib.PDU

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read topology JSON: %v", err)
	}

	if err := json.Unmarshal(data, &pdus); err != nil {
		var single cpLib.PDU
		if json.Unmarshal(data, &single) == nil {
			pdus = []cpLib.PDU{single}
		} else {
			return fmt.Errorf("invalid topology JSON: %v", err)
		}
	}
	log.Info("Loaded topology JSON from ", jsonPath)

	if c.regenerateNISDUUIDs {
		if err := regenerateNISDUUIDs(pdus, cliCFuncs); err != nil {
			return fmt.Errorf("failed to regenerate NISD UUIDs: %v", err)
		}
	} else {
		port := 13000
		for _, pdu := range pdus {
			resp, err := cliCFuncs.PutPDU(&pdu)
			if err != nil || resp == nil || !resp.Success {
				log.Errorf("Failed to insert PDU %s: %v", pdu.ID, err)
				continue
			}
			log.Infof("Inserted PDU %s", pdu.Name)

			for _, rack := range pdu.Racks {
				resp, err := cliCFuncs.PutRack(&rack)
				if err != nil || resp == nil || !resp.Success {
					log.Errorf("Failed to insert Rack %s: %v", rack.ID, err)
					continue
				}
				log.Infof("Inserted Rack %s, under PDU %s", rack.Name, pdu.Name)

				for _, hv := range rack.Hypervisors {
					resp, err := cliCFuncs.PutHypervisor(&hv)
					if err != nil || resp == nil || !resp.Success {
						log.Errorf("Failed to insert Hypervisor %s: %v", hv.ID, err)
						continue
					}
					log.Infof("Inserted Hypervisor %s, under Rack %s", hv.Name, rack.Name)

					for _, dev := range hv.Dev {
						resp, err := cliCFuncs.PutDevice(&dev)
						if err != nil || resp == nil || !resp.Success {
							log.Errorf("Failed to insert Device %s: %v", dev.ID, err)
							continue
						}
						log.Infof("Inserted Device %s, under Hypervisor %s", dev.Name, hv.Name)

						for _, part := range dev.Partitions {
							nisd := cpLib.Nisd{
								ID:            part.NISDUUID,
								PeerPort:      uint16(port),
								FailureDomain: []string{pdu.ID, rack.ID, hv.ID, dev.ID, part.PartitionID},
								TotalSize:     part.Size,
								AvailableSize: part.Size,
								NetInfo: cpLib.NetInfoList{
									{IPAddr: hv.IPAddrs[0], Port: uint16(port)},
									{IPAddr: hv.IPAddrs[1], Port: uint16(port)},
								},
								NetInfoCnt: 2,
							}

							resp, err := cliCFuncs.PutNisd(&nisd)
							if err != nil || resp == nil || !resp.Success {
								log.Errorf("Failed to insert NISD %s (port %d): %v", nisd.ID, port, err)
							} else {
								log.Infof("Inserted NISD %s (port %d) under HV %s", nisd.ID, port, hv.Name)
							}
							port += 2
						}
					}
				}
			}
		}
	}

	log.Info("Topology population completed successfully")

	const topoOutputFile = "topology-output.json"
	out, err := json.MarshalIndent(pdus, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal topology output: %v", err)
	}

	log.Info("Topology JSON:\n", string(out))

	if err := os.WriteFile(topoOutputFile, out, 0644); err != nil {
		return fmt.Errorf("failed to write topology output to %s: %v", topoOutputFile, err)
	}

	log.Info("Topology JSON written to ", topoOutputFile)
	return nil
}

func main() {
	//Intialize client object
	clientObj := clientHandler{}
	var nisdDetails, vdevOutputFile string
	var vdevSize int64
	flag.StringVar(&nisdDetails, "nd", "", "enter nisd details in json format")
	flag.Int64Var(&vdevSize, "vds", 100*1024*1024*1024, "enter vdev size in bytes")
	flag.StringVar(&vdevOutputFile, "vo", "vdev-config.json", "Path to the output JSON file where VDEV configuration will be written")
	vdevID := flag.String("vdev", "", "enter a valid VDEV ID")
	chunkNum := flag.String("chunk", "", "enter a valid Chunk Number")

	//Get commandline parameters.
	clientObj.getCmdParams()

	flag.Usage = usage
	if flag.NFlag() == 0 || !isSingleWriteReqValid(&clientObj) {
		usage()
	}

	//Create logger
	err := PumiceDBCommon.InitLogger(clientObj.logPath)
	if err != nil {
		log.Error("Error while initializing the logger  ", err)
	}

	//Init service discovery
	clientObj.initServiceDisHandler()

	stop := make(chan int)
	go func() {
		log.Info("Start Serf client")
		err := clientObj.clientAPIObj.StartClientAPI(stop, clientObj.configPath)
		if err != nil {
			opStat := prepareOutput(-1, "setup", "", err.Error(), 0)
			clientObj.writeData2Json(opStat)
			log.Error("failed to start client api: ", err)
			os.Exit(1)
		}
	}()
	//Wait till client API Object is ready
	clientObj.waitServiceInit("")

	var passNext bool
	var rdata []byte

	c := ctlplcl.InitCliCFuncs(uuid.NewV4().String(), clientObj.raftUUID, clientObj.configPath, clientObj.logPath)

	switch clientObj.operation {
	case "rw":
		fallthrough
	case "write":
		fallthrough
	case "read":
		rArr := clientObj.getKVArray()
		rdata, err = clientObj.processReadWriteReq(rArr)
		break
	case "config":
		rdata, err = clientObj.processConfig()
		break

	case "membership":
		rdata, err = clientObj.processMembership()
		break

	case "general":
		clientObj.processGeneral()
		rdata = nil
		break

	case "nisd":
		clientObj.processNisd()
		rdata = nil
		break

	case "Gossip":
		rdata = nil
		break

	case "NISDGossip":
		nisdDataMap := clientObj.getNISDInfo()
		rdata, _ = json.MarshalIndent(nisdDataMap, "", " ")
		if !passNext {
			break
		}
		fallthrough

	case "PMDBGossip":
		rdata, err = clientObj.processGossip()
		break

	case "ProxyStat":
		rdata, err = clientObj.processProxyStat()
		break

	case "LookoutInfo":
		rdata, err = clientObj.processLookoutInfo()
		break

	//Lease Operations
	case "GetLease":
		fallthrough
	case "LookupLease":
		fallthrough
	case "RefreshLease":
		rdata, err = clientObj.performLeaseReq(clientObj.requestKey, clientObj.requestValue)
		break
	case "CreateSnap":
		chkSeq := []uint64{200, 100}
		err := c.CreateSnap("ebd099a1-b123-4473-b6c9-580e37f70677", chkSeq, "sample1")
		if err != nil {
			log.Error(err)
		}

	case "ReadSnapByName":
		ret, err := c.ReadSnapByName("sample1")
		if err != nil {
			log.Error(err)
		}
		fmt.Println(string(ret))

	case "ReadSnapForVdev":
		ret, err := c.ReadSnapForVdev("ebd099a1-b123-4473-b6c9-580e37f70677")
		if err != nil {
			log.Error(err)
		}
		fmt.Println(string(ret))

	case "WriteNisd":
		var nisd cpLib.Nisd
		if err := json.Unmarshal([]byte(nisdDetails), &nisd); err != nil {
			log.Error("failed to unmarshal nisd json string:", err)
			os.Exit(-1)
		}
		log.Debug("writing nisd details to pmdb: ", nisd)
		resp, err := c.PutNisd(&nisd)
		if err != nil {
			log.Error("failed to write nisd info:", err)
			os.Exit(-1)
		}
		log.Debug("WriteNisd response: ", resp)
	case "WriteDevice":
		var dev cpLib.Device
		log.Debug("nisd details: ", nisdDetails)
		if err := json.Unmarshal([]byte(nisdDetails), &dev); err != nil {
			log.Error("failed to unmarshal nisd json string:", err)
			os.Exit(-1)
		}
		log.Debug("writing device info into pmdb", dev)
		resp, err := c.PutDevice(&dev)
		if err != nil {
			log.Error("failed to write device info", err)
			os.Exit(-1)
		}
		log.Debug("WriteDevice successful", resp)
	case "CreateVdev":
		// Step 1: Create first Vdev
		vdev := cpLib.Vdev{Cfg: cpLib.VdevCfg{
			Size: vdevSize,
		}}
		req := &cpLib.VdevReq{
			Vdev: &vdev.Cfg,
		}
		resp, err := c.CreateVdev(req)
		if err != nil {
			log.Error("failed to create vdev:", err)
			os.Exit(-1)
		}
		log.Info("Vdev created successfully with UUID:", resp)
	case "GetVdevCfgs":
		req := &cpLib.GetReq{}
		vdevs, err := c.GetVdevCfgs(req)
		if err != nil {
			log.Errorf("Failed to fetch Vdev configurations: %v", err)
			os.Exit(1)
		}
		log.Infof("Successfully retrieved %d Vdev configurations", len(vdevs))
		DumpVdevCfgsToJSON(vdevs, vdevOutputFile)
	case "PopulateTopology":
		err = clientObj.populateTopology(clientObj.requestValue)
		if err != nil {
			log.Errorf("Failed to populate topology: %v", err)
			os.Exit(-1)
		}
	case "Login":
		if clientObj.username == "" || clientObj.password == "" {
			log.Error("login requires both -user and -pass flags")
			os.Exit(-1)
		}
		authClient, _ := userClient.New(userClient.Config{
			AppUUID:          uuid.NewV4().String(),
			RaftUUID:         clientObj.raftUUID,
			GossipConfigPath: clientObj.configPath,
			LogLevel:         "Error",
			LogFile:          clientObj.logPath,
		})
		resp, err := authClient.Login(clientObj.username, clientObj.password)
		if err != nil {
			log.Errorf("Login failed: %v", err)
			os.Exit(-1)
		}
		if resp.Success {
			log.Infof("Login successful. Token: %s", resp.AccessToken)
		} else {
			log.Errorf("Login failed: %s", resp.Error)
			os.Exit(-1)
		}
	case "CreateAdminUser":
		if clientObj.username == "" || clientObj.password == "" {
			log.Error("CreateAdminUser requires both -user and -pass flags")
			os.Exit(-1)
		}
		authClient, _ := userClient.New(userClient.Config{
			AppUUID:          uuid.NewV4().String(),
			RaftUUID:         clientObj.raftUUID,
			GossipConfigPath: clientObj.configPath,
			LogLevel:         "Error",
			LogFile:          clientObj.logPath,
		})
		req := &userlib.UserReq{
			Username:     clientObj.username,
			NewSecretKey: clientObj.password,
			IsAdmin:      true,
		}
		resp, err := authClient.CreateAdminUser(req)
		if err != nil {
			log.Errorf("CreateAdminUser failed: %v", err)
			os.Exit(-1)
		}
		if resp.Success {
			log.Infof("Admin user created successfully. UserID: %s", resp.UserID)
		} else {
			log.Errorf("CreateAdminUser failed: %s", resp.Error)
			os.Exit(-1)
		}
	case "GetVdev":
		if *vdevID == "" {
			log.Error("Missing required flag: -vdev")
			os.Exit(1)
		}
		vdev, err := c.GetVdevsWithChunkInfo(&cpLib.GetReq{
			ID: *vdevID,
		})
		if err != nil {
			log.Errorf("Failed to fetch Vdev details for VdevID=%s: %v", *vdevID, err)
			os.Exit(1)
		}
		log.Infof("Successfully retrieved Vdev details for VdevID=%s", *vdevID)
		fmt.Printf("Vdev Details: %+v\n", vdev)
	case "GetChunk":
		if *vdevID == "" || *chunkNum == "" {
			log.Error("Missing required flags: -vdev and -chunk")
			os.Exit(1)
		}
		reqID := *vdevID + "/" + *chunkNum
		vdev, err := c.GetChunkNisd(&cpLib.GetReq{
			ID: reqID,
		})
		if err != nil {
			log.Errorf("Failed to fetch Chunk info for VdevID=%s Chunk=%s: %v", *vdevID, *chunkNum, err)
			os.Exit(1)
		}
		log.Infof("Successfully retrieved Chunk info for VdevID=%s Chunk=%s", *vdevID, *chunkNum)
		fmt.Printf("Chunk Details: %+v\n", vdev)
	}

	if err != nil {
		log.Error(err)
		os.Exit(-1)
	} else if rdata != nil {
		err = clientObj.complete(rdata)
		if err != nil {
			log.Error("Failed to write the response to the file")
		}
	}
}
