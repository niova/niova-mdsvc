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

// tenantLogValue renders a tenant UUID for log output, making the
// default-tenant case explicit instead of printing an empty string.
func tenantLogValue(tenantUUID string) string {
	if tenantUUID == "" {
		return "<default>"
	}
	return tenantUUID
}

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
	authUser := flag.String("au", userlib.AdminUsername, "auth username for authentication (defaults to the admin user)")
	tenantUUID := flag.String("t", "", "tenant UUID to authenticate against (empty = default tenant)")
	flag.Parse()
	log.SetLevel(log.Level(*logLevel))
	log.Infof("starting config app - raft: %s, config: %s, setupConfig: %s, authUser: %s, tenant: %s",
		*raftID, *configPath, *setupConfig, *authUser, tenantLogValue(*tenantUUID))

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

		// Login to get UserToken. authUser/tenantUUID default to the admin
		// user / default tenant respectively, but either can be overridden
		// via -au/-t for a non-default-tenant or non-admin deployment.
		loginResp, err := authClient.LoginWithTenant(*authUser, *adminSecret, *tenantUUID)
		if err != nil {
			// err already carries user/tenant/status context and a gossip
			// hint from LoginWithTenant (client.go) — do not strip it.
			log.Fatalf("control-plane login failed (raft=%s, gossipConfig=%s): %v", *raftID, *configPath, err)
		}
		if !loginResp.Success || loginResp.AccessToken == "" {
			log.Fatalf("control-plane login failed: user=%q tenant=%s: no access token received (unexpected empty success response)",
				*authUser, tenantLogValue(*tenantUUID))
		}
		adminToken = loginResp.AccessToken
		log.Infof("control-plane authentication successful: user=%q tenant=%s", *authUser, tenantLogValue(*tenantUUID))
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
		// GetPartition's id filter matches partition_id, so nisd.DevID (the
		// partition name from config.yaml) fetches that single partition.
		c.SetToken(adminToken)
		pts, err := c.GetPartition(cpLib.GetReq{ID: nisd.DevID})
		if err != nil {
			log.Errorf("failed to get partition %q from control plane (raft=%s): %v", nisd.DevID, *raftID, err)
			os.Exit(-1)
		}
		if len(pts) == 0 {
			log.Errorf("no partition found with partition_id %q — this device name from config.yaml's "+
				"nisd_config was never registered via POST /api/infra (or infra was loaded for a "+
				"different tenant than %s); load/verify infra before retrying", nisd.DevID, tenantLogValue(*tenantUUID))
			os.Exit(-1)
		}
		pt := pts[ZERO_INDEX]

		log.Info("fetched device info from control plane: ", pt.NISDUUID)

		log.Info("setting nisd id: ", pt.NISDUUID)
		req := cpLib.GetReq{ID: pt.NISDUUID}
		c.SetToken(adminToken)
		nisdInfo, err := c.GetNisd(req)
		if err != nil {
			log.Errorf("failed to get nisd details for %q (partition_id=%s, raft=%s): %v", pt.NISDUUID, nisd.DevID, *raftID, err)
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
