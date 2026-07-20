package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v3"

	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	cpLib "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userClient "github.com/00pauln00/niova-mdsvc/controlplane/user/client"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"

	pmCmn "github.com/00pauln00/niova-pumicedb/go/pkg/pumicecommon"
)

const GEN_CONF_FILE = "config-gen.yaml"
const ZERO_INDEX = 0

type Nisd struct {
	ClientPort uint16 `yaml:"client_port"`
	PeerPort   uint16 `yaml:"peer_port"`
	ID         string `yaml:"uuid"`
	DevID      string `yaml:"name"`
	InitDev    bool   `yaml:"init"`
	Args       string `yaml:"cmdline_args"`
}

type s3Config struct {
	URL  string `yaml:"url"`
	Opts string `yaml:"opts"`
	Auth string `yaml:"auth"`
}

type NisdCntrConfig struct {
	S3Config   s3Config         `yaml:"s3_config"`
	Gossip     pmCmn.GossipInfo `yaml:"gossip"`
	NisdConfig []Nisd           `yaml:"nisd_config"`
}

// loadConfig loads config from file if present, otherwise creates a new one.
func loadConfig(setupConfig string, cc *NisdCntrConfig) error {

	if err := os.MkdirAll(filepath.Dir(setupConfig), os.ModePerm); err != nil {
		return err
	}
	_, err := os.Stat(setupConfig)
	if err != nil {
		log.Error("failed to stat config file: ", err)
		return err
	}
	data, err := os.ReadFile(setupConfig)
	if err != nil {
		log.Error("failed to read config file: ", err)
		return err
	}
	if err := yaml.Unmarshal(data, cc); err != nil {
		log.Error("failed to unmarshal config file: ", err)
		return err
	}

	return nil
}

func main() {

	raftID := flag.String("r", "", "pass the raft uuid")
	configPath := flag.String("c", "./gossipNodes", "pass the gossip config path")
	setupConfig := flag.String("sc", "./config.yaml", "pass the gossip config path")
	logLevel := flag.Int("ll", 4, "set log level (0=panic, 1=fatal, 2=error, 3=warn, 4=info, 5=debug, 6=trace)")
	// PASS the secret key here
	adminSecret := flag.String("as", "", "admin secret key for authentication")
	flag.Parse()
	log.SetLevel(log.Level(*logLevel))
	log.Infof("starting config app - raft: %s, config: %s", *raftID, *configPath)

	var adminToken string
	authEnabled := os.Getenv("AUTH_ENABLED") != "false"

	if authEnabled {
		log.Info("Starting ccManager with AUTH_ENABLED=true")
		if *adminSecret == "" {
			log.Fatal("admin secret key (-as) is required for NISD operations")
		}

		// Initialize user client for authentication
		userCfg := userClient.Config{
			AppUUID:          uuid.New().String(),
			RaftUUID:         *raftID,
			GossipConfigPath: *configPath,
		}
		authClient, tearDown := userClient.New(userCfg)
		if authClient == nil {
			log.Fatal("failed to initialize user client for authentication")
		}
		defer tearDown()

		// Login as admin to get UserToken
		loginResp, err := authClient.Login(userlib.AdminUsername, *adminSecret)
		if err != nil {
			log.Fatalf("admin login failed: %v", err)
		}
		if !loginResp.Success || loginResp.AccessToken == "" {
			log.Fatal("admin login failed: no access token received")
		}
		adminToken = loginResp.AccessToken
		log.Info("admin authentication successful")
	} else {
		log.Warn("Starting ccManager with AUTH_ENABLED=false")
	}

	// Initialize control plane client
	c := cpClient.InitCliCFuncs(uuid.New().String(), *raftID, *configPath, "")

	var conf NisdCntrConfig
	conf.NisdConfig = make([]Nisd, 0)
	err := loadConfig(*setupConfig, &conf)
	if err != nil {
		log.Error("failed to load config file: ", err)
		os.Exit(-1)
	}

	log.Debugf("read nisd config details: %+v", conf.NisdConfig)
	// Pass admin token for NISD operations (admin-only)
	req := cpLib.GetReq{}
	c.SetToken(adminToken)
	nisdArgs, err := c.GetNisdArgs(req)
	if err != nil {
		log.Warnf("failed to fetch nisd args (backend may not support /api/nisd_args): %v; "+
			"generating config with empty cmdline_args", err)
	}
	naS := nisdArgs.BuildCmdArgs()
	for i, nisd := range conf.NisdConfig {
		req := cpLib.GetReq{
			ID: nisd.DevID,
		}
		c.SetToken(adminToken)
		pt, err := c.GetPartition(req)
		if err != nil {
			log.Error("failed to get device uuid: ", err)
			os.Exit(-1)
		}
		log.Info("fetched device info from control plane: ", pt[ZERO_INDEX].NISDUUID)

		log.Info("setting nisd id: ", pt[ZERO_INDEX].NISDUUID)
		req.ID = pt[ZERO_INDEX].NISDUUID
		nisdInfo, err := c.GetNisd(req)
		if err != nil {
			log.Error("failed to get nisd details: ", err)
			os.Exit(-1)
		}
		conf.NisdConfig[i].ID = nisdInfo.ID
		conf.NisdConfig[i].ClientPort = nisdInfo.NetInfo[ZERO_INDEX].Port
		conf.NisdConfig[i].PeerPort = nisdInfo.PeerPort
		conf.NisdConfig[i].DevID = nisd.DevID
		conf.NisdConfig[i].Args = naS
		log.Info("fetched nisd info from control plane: ", nisdInfo)

	}

	configY, err := yaml.Marshal(conf)
	if err != nil {
		log.Error("Error marshaling YAML:", err)
		os.Exit(-1)
	}
	err = os.WriteFile(filepath.Join(filepath.Dir(*setupConfig), GEN_CONF_FILE), configY, 0644)
	if err != nil {
		log.Error("Error writing YAML file:", err)
		os.Exit(-1)
	}
	log.Info("generated nisd configuration yaml file for container automation: ", conf)
}
