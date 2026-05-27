package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	defaultLogger "log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"

	log "github.com/00pauln00/niova-lookout/pkg/xlog"

	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	srvctlplanefuncs "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/server"
	"github.com/00pauln00/niova-mdsvc/controlplane/requestResponseLib"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
	userserver "github.com/00pauln00/niova-mdsvc/controlplane/user/server"

	PumiceDBCommon "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
	PumiceDBFunc "github.com/00pauln00/niova-pumicedb/go/pkg/pumicefunc/server"
	leaseServerLib "github.com/00pauln00/niova-pumicedb/go/pkg/pumicelease/server"
	PumiceDBServer "github.com/00pauln00/niova-pumicedb/go/pkg/pumiceserver"
	"github.com/00pauln00/niova-pumicedb/go/pkg/pumicestore"
	compressionLib "github.com/00pauln00/niova-pumicedb/go/pkg/utils/compressor"
	lookout "github.com/00pauln00/niova-pumicedb/go/pkg/utils/ctlmonitor"
	httpClient "github.com/00pauln00/niova-pumicedb/go/pkg/utils/httpclient"
	serfAgent "github.com/00pauln00/niova-pumicedb/go/pkg/utils/serfagent"
	storageiface "github.com/00pauln00/niova-pumicedb/go/pkg/utils/storage/interface"
)

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

var seqno = 0

var encodingOverhead = 256

var RecvdPort int

type pmdbServerHandler struct {
	raftUUID          uuid.UUID
	peerUUID          uuid.UUID
	logDir            string
	logLevel          string
	gossipClusterFile string
	servicePortRangeS uint16
	servicePortRangeE uint16
	hport             uint16
	prometheus        bool
	authEnabled       bool
	nodeAddr          net.IP
	addrList          []net.IP
	GossipData        map[string]string
	ConfigString      string
	ConfigData        []PumiceDBCommon.PeerConfigData
	lookoutInstance   lookout.EPContainer
	serfAgentHandler  serfAgent.SerfAgentHandler
	portRange         []uint16
}

func PopulateHierarchy() error {
	key, prefix := srvctlplanefuncs.NisdCfgKey, srvctlplanefuncs.NisdCfgKey
	rangeReadContOut := make([]map[string][]byte, 0)
	time.Sleep(2 * time.Second)
	cbargs := PumiceDBServer.PmdbCbArgs{
		Store: &pumicestore.PumiceStore{},
	}
	for {
		log.Debugf("querying pmdb with key: %s, prefix: %s", key, prefix)
		readResult, err := cbargs.Store.RangeRead(storageiface.RangeReadArgs{
			Selector: cpLib.PmdbColumnFamily,
			Key:      key,
			Prefix:   prefix,
			BufSize:  cpLib.MAX_REPLY_SIZE,
		})
		if err != nil {
			log.Warn("RangeReadKV(): ", err)
			return err
		}
		rangeReadContOut = append(rangeReadContOut, readResult.ResultMap)
		if readResult.LastKey == "" {
			break
		}
		key = readResult.LastKey
		prefix = filepath.Dir(readResult.LastKey)
	}
	nisdList := srvctlplanefuncs.ParseEntitiesRR[cpLib.Nisd](rangeReadContOut, srvctlplanefuncs.NisdParser{})
	for i := 0; i < len(nisdList); i++ {
		err := srvctlplanefuncs.HR.AddNisd(&nisdList[i])
		if err != nil {
			log.Error("AddNisd(): ", err)
			return err
		}
		log.Debug("added nisd to the hierarchy: ", nisdList[i])

	}

	log.Infof("successfully intialized hierarchy")
	srvctlplanefuncs.HR.Dump()
	return nil
}

func main() {
	serverHandler := pmdbServerHandler{}
	cpLib.RegisterGOBStructs()

	nso, err := serverHandler.parseArgs()
	if err != nil {
		defaultLogger.Println("failed to parse arguments", err)
		return
	}

	// Initialize auth encryption only when authentication is enabled.
	if serverHandler.authEnabled {
		userlib.MustInitialize()
	}

	log.InitXlog(serverHandler.logDir, &serverHandler.logLevel)
	log.Info("Log Dir - ", serverHandler.logDir)

	err = serverHandler.startSerfAgent()
	if err != nil {
		log.Fatal("Error while initializing serf agent ", err)
	}

	log.Info("Raft and Peer UUID: ", nso.raftUuid, " ", nso.peerUuid)
	/*
	   Initialize the internal pmdb-server-object pointer.
	   Assign the Directionary object to PmdbAPI so the apply and
	   read callback functions can be called through pmdb common library
	   functions.
	*/

	portAddr := &RecvdPort

	//Start lookout monitoring
	ctl_path := os.Getenv("NIOVA_INOTIFY_BASE_PATH")
	if len(ctl_path) == 0 {
		ctl_path = "/ctl-interface/"
	}

	serverHandler.lookoutInstance = lookout.EPContainer{
		MonitorUUID:      nso.peerUuid.String(),
		AppType:          "PMDB",
		PortRange:        serverHandler.portRange,
		CTLPath:          ctl_path,
		SerfMembershipCB: serverHandler.SerfMembership,
		EnableHttp:       serverHandler.prometheus,
		RetPort:          portAddr,
	}
	go serverHandler.lookoutInstance.Start()

	//Wait till HTTP Server has started
	srvctlplanefuncs.SetClmFamily(cpLib.PmdbColumnFamily)
	cpAPI := PumiceDBFunc.NewFuncServer()

	cpAPI.RegisterWritePrepFunc(cpLib.PUT_NISD, srvctlplanefuncs.WPNisdCfg)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_DEVICE, srvctlplanefuncs.WPDeviceInfo)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_PDU, srvctlplanefuncs.WPPDUCfg)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_PFS, srvctlplanefuncs.WPPFSCfg)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_RACK, srvctlplanefuncs.WPRackCfg)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_HYPERVISOR, srvctlplanefuncs.WPHyperVisorCfg)
	cpAPI.RegisterWritePrepFunc(cpLib.CREATE_SNAP, srvctlplanefuncs.WritePrepCreateSnap)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_PARTITION, srvctlplanefuncs.WPCreatePartition)

	cpAPI.RegisterReadFunc(cpLib.GET_NISD_LIST, srvctlplanefuncs.ReadAllNisdConfigs)
	cpAPI.RegisterReadFunc(cpLib.GET_NISD, srvctlplanefuncs.ReadNisdConfig)
	cpAPI.RegisterReadFunc(cpLib.GET_DEVICE, srvctlplanefuncs.RdDeviceInfo)
	cpAPI.RegisterReadFunc(cpLib.GET_PDU, srvctlplanefuncs.ReadPDUCfg)
	cpAPI.RegisterReadFunc(cpLib.GET_PFS, srvctlplanefuncs.ReadPFSCfg)
	cpAPI.RegisterReadFunc(cpLib.GET_RACK, srvctlplanefuncs.ReadRackCfg)
	cpAPI.RegisterReadFunc(cpLib.GET_HYPERVISOR, srvctlplanefuncs.ReadHyperVisorCfg)
	cpAPI.RegisterReadFunc(cpLib.READ_SNAP_NAME, srvctlplanefuncs.ReadSnapByName)
	cpAPI.RegisterReadFunc(cpLib.READ_SNAP_VDEV, srvctlplanefuncs.ReadSnapForVdev)
	cpAPI.RegisterReadFunc(cpLib.GET_PARTITION, srvctlplanefuncs.ReadPartition)
	cpAPI.RegisterReadFunc(cpLib.GET_VDEV_CHUNK_INFO, srvctlplanefuncs.ReadVdevsInfoWithChunkMapping)
	cpAPI.RegisterReadFunc(cpLib.GET_NISD_ARGS, srvctlplanefuncs.RdNisdArgs)
	cpAPI.RegisterWritePrepFunc(cpLib.PUT_NISD_ARGS, srvctlplanefuncs.WPNisdArgs)
	cpAPI.RegisterReadFunc(cpLib.GET_VDEV_INFO, srvctlplanefuncs.ReadVdevInfo)
	cpAPI.RegisterReadFunc(cpLib.GET_ALL_VDEV, srvctlplanefuncs.ReadAllVdevInfo)
	cpAPI.RegisterReadFunc(cpLib.GET_CHUNK_NISD, srvctlplanefuncs.ReadChunkNisd)
	cpAPI.RegisterApplyFunc(cpLib.DELETE_VDEV, srvctlplanefuncs.APDeleteVdev)

	cpAPI.RegisterWritePrepFunc(userlib.PutUserAPI, userserver.PutUser)
	cpAPI.RegisterReadFunc(userlib.GetUserAPI, userserver.GetUser)
	cpAPI.RegisterApplyFunc(userlib.AdminUserAPI, userserver.CreateAdminUser)
	cpAPI.RegisterReadFunc(userlib.LoginAPI, userserver.Login)

	cpAPI.RegisterWritePrepFunc(cpLib.CREATE_VDEV, srvctlplanefuncs.WPCreateVdev)
	cpAPI.RegisterApplyFunc(cpLib.CREATE_VDEV, srvctlplanefuncs.APCreateVdev)
	cpAPI.RegisterApplyFunc("*", srvctlplanefuncs.ApplyFunc)
	cpAPI.RegisterApplyFunc(cpLib.PUT_NISD, srvctlplanefuncs.ApplyNisd)

	nso.pso = &PumiceDBServer.PmdbServerObject{
		RaftUuid:       nso.raftUuid.String(),
		PeerUuid:       nso.peerUuid.String(),
		PmdbAPI:        nso,
		FuncAPI:        cpAPI,
		SyncWrites:     false,
		CoalescedWrite: true,
		LeaseEnabled:   true,
	}

	nso.leaseObj = leaseServerLib.LeaseServerObject{}
	// Initialize leaseObj
	nso.leaseObj.InitLeaseObject(nso.pso)
	// Separate column families for application requests and lease
	nso.pso.ColumnFamilies = []string{cpLib.PmdbColumnFamily, nso.leaseObj.LeaseColmFam}

	// Start the pmdb server
	//TODO Check error
	go nso.pso.Run()

	srvctlplanefuncs.HR.Init()
	err = PopulateHierarchy()
	if err != nil {
		log.Warn("failed to create hierarchy struct:", err)
	}

	srvctlplanefuncs.SetAuthEnabled(serverHandler.authEnabled)
	if serverHandler.authEnabled {
		srvctlplanefuncs.InitAuthorizer()
		log.Info("Authentication and authorization: enabled")
	} else {
		log.Warn("Authentication and authorization: DISABLED - all requests will be granted admin access")
	}

	serverHandler.checkPMDBLiveness()
	serverHandler.exportTags()

}

func appdecode(payload []byte, op interface{}) error {
	dec := gob.NewDecoder(bytes.NewBuffer(payload))
	return dec.Decode(op)
}

func (handler *pmdbServerHandler) checkPMDBLiveness() {
	for {
		ok := handler.lookoutInstance.CheckLiveness(handler.peerUUID.String())
		if ok {
			fmt.Println("PMDB Server is live")
			return
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func (handler *pmdbServerHandler) exportTags() {
	if handler.prometheus {
		handler.checkHTTPLiveness()
		handler.GossipData["Hport"] = strconv.Itoa(RecvdPort)
	}
	handler.GossipData["Type"] = "PMDB_SERVER"
	handler.GossipData["Rport"] = strconv.Itoa(int(handler.serfAgentHandler.RpcPort))
	handler.GossipData["RU"] = handler.raftUUID.String()
	handler.GossipData["CS"], _ = generateCheckSum(handler.GossipData)
	log.Info(handler.GossipData)
	for {
		handler.serfAgentHandler.SetNodeTags(handler.GossipData)
		time.Sleep(3 * time.Second)
	}

}

func usage() {
	flag.PrintDefaults()
	os.Exit(0)
}

func makeRange(min, max uint16) []uint16 {
	a := make([]uint16, max-min+1)
	for i := range a {
		a[i] = uint16(min + uint16(i))
	}
	return a
}

func decodeControlPlaneReq(input []byte, output *requestResponseLib.KVRequest) error {
	dec := gob.NewDecoder(bytes.NewBuffer(input))
	for {
		if err := dec.Decode(output); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}

	return nil
}

func (handler *pmdbServerHandler) checkHTTPLiveness() {
	var emptyByteArray []byte
	for {
		if RecvdPort == -1 {
			fmt.Println("HTTP Server failed to start")
			os.Exit(0)
		}
		_, err := httpClient.HTTP_Request(emptyByteArray, handler.nodeAddr.String()+":"+strconv.Itoa(int(RecvdPort))+"/check", false)
		if err != nil {
			fmt.Println("HTTP Liveness - ", err)
		} else {
			fmt.Println("HTTP Liveness - HTTP Server is alive")
			break
		}
		time.Sleep(1 * time.Second)
	}
}

func (handler *pmdbServerHandler) parseArgs() (*NiovaKVServer, error) {
	var tempRaftUUID, tempPeerUUID string
	var err error

	flag.StringVar(&tempRaftUUID, "r", "NULL", "raft uuid")
	flag.StringVar(&tempPeerUUID, "u", "NULL", "peer uuid")

	/* If log path is not provided, it will use Default log path.
	   default log path: /tmp/<peer-uuid>.log
	*/
	defaultLog := "/" + "tmp" + "/" + handler.peerUUID.String() + ".log"
	flag.StringVar(&handler.logDir, "l", defaultLog, "log dir")
	flag.StringVar(&handler.logLevel, "ll", "Trace", "Log level")
	flag.StringVar(&handler.gossipClusterFile, "g", "NULL", "Serf agent port")
	flag.BoolVar(&handler.prometheus, "p", false, "Enable prometheus")
	flag.Parse()
	handler.authEnabled = os.Getenv("AUTH_ENABLED") != "false" // true unless explicitly "false"

	handler.raftUUID, _ = uuid.FromString(tempRaftUUID)
	handler.peerUUID, _ = uuid.FromString(tempPeerUUID)
	nso := &NiovaKVServer{
		raftUuid: handler.raftUUID,
		peerUuid: handler.peerUUID,
	}

	return nso, err
}

func (handler *pmdbServerHandler) SerfMembership() map[string]bool {
	membership := handler.serfAgentHandler.GetMembersState()
	return membership
}

func extractPMDBServerConfigfromFile(path string) (*PumiceDBCommon.PeerConfigData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(f)
	peerData := PumiceDBCommon.PeerConfigData{}

	for scanner.Scan() {
		text := scanner.Text()
		lastIndex := len(strings.Split(text, " ")) - 1
		key := strings.Split(text, " ")[0]
		value := strings.Split(text, " ")[lastIndex]
		switch key {
		case "CLIENT_PORT":
			buffer, err := strconv.ParseUint(value, 10, 16)
			peerData.ClientPort = uint16(buffer)
			if err != nil {
				return nil, errors.New("Client Port is out of range")
			}

		case "IPADDR":
			peerData.IPAddr = net.ParseIP(value)

		case "PORT":
			buffer, err := strconv.ParseUint(value, 10, 16)
			peerData.Port = uint16(buffer)
			if err != nil {
				return nil, errors.New("Port is out of range")
			}
		}
	}
	f.Close()

	return &peerData, err
}

func (handler *pmdbServerHandler) readPMDBServerConfig() error {
	folder := os.Getenv("NIOVA_LOCAL_CTL_SVC_DIR")
	files, err := os.ReadDir(folder + "/")
	if err != nil {
		return err
	}
	handler.GossipData = make(map[string]string)

	for _, file := range files {
		if strings.Contains(file.Name(), ".peer") {
			//Extract required config from the file
			path := folder + "/" + file.Name()
			peerData, err := extractPMDBServerConfigfromFile(path)
			if err != nil {
				return err
			}

			uuid := file.Name()[:len(file.Name())-5]
			cuuid, err := compressionLib.CompressUUID(uuid)
			if err != nil {
				return err
			}

			tempGossipData, _ := compressionLib.CompressStructure(*peerData)
			handler.GossipData[cuuid] = tempGossipData[16:]
			//Since the uuid filled after compression, the cuuid wont be included in compressed string
			copy(peerData.UUID[:], []byte(cuuid))
			handler.ConfigData = append(handler.ConfigData, *peerData)
		}
	}

	return nil
}

func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

func (handler *pmdbServerHandler) readGossipClusterFile() error {
	f, err := os.Open(handler.gossipClusterFile)
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	/*
		Following is the format of gossipNodes file
		PMDB server addrs with space separated
		Start_port End_port
	*/
	scanner.Scan()
	IPAddrsTxt := strings.Split(scanner.Text(), " ")
	IPAddrs := removeDuplicateStr(IPAddrsTxt)
	for i := range IPAddrs {
		ipAddr := net.ParseIP(IPAddrs[i])
		handler.addrList = append(handler.addrList, ipAddr)
	}
	handler.nodeAddr = net.ParseIP("0.0.0.0")
	//Read Ports
	scanner.Scan()
	Ports := strings.Split(scanner.Text(), " ")
	temp, _ := strconv.Atoi(Ports[0])
	handler.servicePortRangeS = uint16(temp)
	temp, _ = strconv.Atoi(Ports[1])
	handler.servicePortRangeE = uint16(temp)

	//Set port range array
	handler.portRange = makeRange(handler.servicePortRangeS, handler.servicePortRangeE)
	return nil
}

func generateCheckSum(data map[string]string) (string, error) {
	keys := make([]string, 0, len(data))
	var allDataArray []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		allDataArray = append(allDataArray, k+data[k])
	}

	byteArray, err := json.Marshal(allDataArray)
	checksum := crc32.ChecksumIEEE(byteArray)
	checkSumByteArray := make([]byte, 4)
	binary.LittleEndian.PutUint32(checkSumByteArray, uint32(checksum))
	return string(checkSumByteArray), err
}

func (handler *pmdbServerHandler) startSerfAgent() error {
	err := handler.readGossipClusterFile()
	if err != nil {
		return err
	}
	serfLog := "00"
	switch serfLog {
	case "ignore":
		defaultLogger.SetOutput(io.Discard)
	default:
		f, openErr := os.OpenFile("serfLog.log", os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if openErr != nil {
			defaultLogger.SetOutput(os.Stderr)
		} else {
			defaultLogger.SetOutput(f)
		}
	}

	//defaultLogger.SetOutput(io.Discard)
	serfAgentHandler := serfAgent.SerfAgentHandler{
		Name:              handler.peerUUID.String(),
		AddrList:          handler.addrList,
		Addr:              handler.nodeAddr,
		AgentLogger:       defaultLogger.Default(),
		RaftUUID:          handler.raftUUID,
		ServicePortRangeS: handler.servicePortRangeS,
		ServicePortRangeE: handler.servicePortRangeE,
		AppType:           "PMDB",
	}

	//Start serf agent
	_, err = serfAgentHandler.SerfAgentStartup(true)
	if err != nil {
		log.Error("Error while starting serf agent ", err)
	}
	handler.readPMDBServerConfig()
	handler.serfAgentHandler = serfAgentHandler
	return err
}

type NiovaKVServer struct {
	raftUuid       uuid.UUID
	peerUuid       uuid.UUID
	columnFamilies string
	leaseObj       leaseServerLib.LeaseServerObject
	pso            *PumiceDBServer.PmdbServerObject
}

func (nso *NiovaKVServer) Init(cleanupPeerArgs *PumiceDBServer.PmdbCbArgs) {
	return
}

func (nso *NiovaKVServer) WritePrep(wrPrepArgs *PumiceDBServer.PmdbCbArgs) int64 {
	log.Trace("NiovaCtlPlane server: Write prep received")
	return 0
}

func (nso *NiovaKVServer) Apply(applyArgs *PumiceDBServer.PmdbCbArgs) int64 {

	log.Trace("NiovaCtlPlane server: Apply request received")

	// Decode the input buffer into structure format
	applyNiovaKV := &requestResponseLib.KVRequest{}
	// register datatypes for decoding interface
	decodeErr := appdecode(applyArgs.Payload, applyNiovaKV)
	if decodeErr != nil {
		log.Error("Failed to decode the application data")
		return -1
	}

	//For application request
	log.Trace("Key passed by client: ", applyNiovaKV.Key)

	byteToStr := string(applyNiovaKV.Value)

	log.Trace("Write the KeyValue by calling PmdbWriteKV")
	err := applyArgs.Store.Write(applyNiovaKV.Key, byteToStr, cpLib.PmdbColumnFamily)
	if err != nil {
		log.Error("Value not written to rocksdb")
		return -1
	}

	return 0
}

func (nso *NiovaKVServer) FillReply(RetryWriteArgs *PumiceDBServer.PmdbCbArgs) int64 {
	return 0
}

func (nso *NiovaKVServer) Read(readArgs *PumiceDBServer.PmdbCbArgs) int64 {

	log.Trace("NiovaCtlPlane server: Read request received")
	var copyErr error
	var replySize int64

	//Decode the request structure sent by client.
	reqStruct := &requestResponseLib.KVRequest{}
	decodeErr := appdecode(readArgs.Payload, reqStruct)

	if decodeErr != nil {
		log.Error("Failed to decode the read request")
		return -1
	}

	//Lease request
	log.Trace("Key passed by client: ", reqStruct.Key)
	keyLen := len(reqStruct.Key)
	log.Trace("Key length: ", keyLen)

	var readErr error
	var resultResponse requestResponseLib.KVResponse
	//var resultReq requestResponseLib.KVResponse
	//Pass the work as key to PmdbReadKV and get the value from pumicedb
	if reqStruct.Operation == requestResponseLib.KV_READ {

		log.Trace("read - ", reqStruct.SeqNum)
		readResult, err := readArgs.Store.Read(reqStruct.Key, cpLib.PmdbColumnFamily)
		singleReadMap := make(map[string][]byte)
		singleReadMap[reqStruct.Key] = readResult
		resultResponse = requestResponseLib.KVResponse{
			Key:       reqStruct.Key,
			ResultMap: singleReadMap,
		}
		readErr = err

	} else if reqStruct.Operation == requestResponseLib.KV_RANGE_READ {
		log.Trace("sequence number - ", reqStruct.SeqNum)
		readResult, err := readArgs.Store.RangeRead(storageiface.RangeReadArgs{
			Selector:   cpLib.PmdbColumnFamily,
			Key:        reqStruct.Key,
			BufSize:    readArgs.ReplySize - int64(encodingOverhead),
			Prefix:     reqStruct.Prefix,
			SeqNum:     reqStruct.SeqNum,
			Consistent: true,
		})

		if err != nil {
			log.Errorf("RangeRead failed: err=%v, readResult nil=%v", err, readResult == nil)
			return -1
		}

		var cRead bool
		if readResult.LastKey != "" {
			cRead = true
		} else {
			cRead = false
		}
		resultResponse = requestResponseLib.KVResponse{
			Prefix:       reqStruct.Key,
			ResultMap:    readResult.ResultMap,
			ContinueRead: cRead,
			Key:          readResult.LastKey,
			SeqNum:       readResult.SeqNum,
			SnapMiss:     readResult.SnapMiss,
		}
		readErr = err
	} else {
		log.Error("Invalid operation: ", reqStruct.Operation)
		return -1
	}

	log.Trace("Response trace : ", resultResponse)
	if readErr == nil {
		//Copy the encoded result in replyBuffer
		replySize, copyErr = PumiceDBServer.PmdbCopyDataToBuffer(resultResponse,
			readArgs.ReplyBuf)
		if copyErr != nil {
			log.Errorf("Failed to Copy result in the buffer: %s", copyErr)
			return -1
		}
	} else {
		log.Error(readErr)
	}

	log.Trace("Reply size: ", replySize)

	return replySize
}
