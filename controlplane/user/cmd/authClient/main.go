package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"

	authclient "github.com/00pauln00/niova-mdsvc/controlplane/user/client"
	userlib "github.com/00pauln00/niova-mdsvc/controlplane/user/lib"
)

func newClient() (*authclient.Client, func()) {

	clusterID := os.Getenv("RAFT_ID")
	if clusterID == "" {
		panic("RAFT_ID not set")
	}

	gossipPath := os.Getenv("GOSSIP_NODES_PATH")
	if gossipPath == "" {
		panic("GOSSIP_NODES_PATH not set")
	}

	cfg := authclient.Config{
		AppUUID:          uuid.New().String(),
		RaftUUID:         clusterID,
		GossipConfigPath: gossipPath,
	}

	return authclient.New(cfg)
}

func printJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func main() {

	if len(os.Args) < 2 {
		fmt.Println("usage: authClient <command>")
		os.Exit(1)
	}

	cmd := os.Args[1]

	client, tearDown := newClient()
	defer tearDown()

	switch cmd {

	case "create-admin":

		if len(os.Args) < 4 {
			fmt.Println("usage: authClient create-admin <username> <secretKey>")
			os.Exit(1)
		}

		username := os.Args[2]
		secret := os.Args[3]

		req := &userlib.UserReq{
			Username:     username,
			NewSecretKey: secret,
		}

		resp, err := client.CreateAdminUser(req)
		if err != nil {
			fmt.Println("Admin creation returned:", err)
		}

		printJSON(resp)

	case "login":

		if len(os.Args) < 4 {
			fmt.Println("usage: authClient login <username> <secretKey>")
			os.Exit(1)
		}

		username := os.Args[2]
		secret := os.Args[3]

		resp, err := client.Login(username, secret)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		printJSON(resp)

	case "create-user":

		if len(os.Args) < 4 {
			fmt.Println("usage: authClient create-user <username> <adminToken>")
			os.Exit(1)
		}

		username := os.Args[2]
		adminToken := os.Args[3]

		req := &userlib.UserReq{
			Username:  username,
			UserToken: adminToken,
		}

		resp, err := client.CreateUser(req)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		printJSON(resp)

	default:
		fmt.Println("unknown command:", cmd)
		os.Exit(1)
	}
}