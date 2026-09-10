package domain

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// SSHExecutor abstracts SSH command execution
type SSHExecutor interface {
	RunCommand(cmd string) (string, error)
	GetDevices() ([]ctlplfl.Device, error)
	Close() error
}

// newSSHExecutor is the seam storage.go's device/partition operations dial
// through instead of calling NewSSHClient directly — they depend on the
// SSHExecutor abstraction, not the concrete *SSHClient, so a test (or a
// future non-OpenSSH transport) can substitute it by reassigning this var.
var newSSHExecutor = func(hosts []string) (SSHExecutor, error) {
	return NewSSHClient(hosts)
}

// DiscoverDevices connects to hv via SSH and enumerates its physical
// storage devices, to pre-fill the Add Device form instead of typing
// Device Path/Serial/Size by hand. Backend-agnostic — it talks to the
// hypervisor directly, not through either control-plane backend's API, so
// it works the same whether niova-mdsvc or mdsvc-tidb is managing metadata.
func DiscoverDevices(hv ctlplfl.Hypervisor) ([]ctlplfl.Device, error) {
	client, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return nil, fmt.Errorf("connect to hypervisor: %w", err)
	}
	defer client.Close()
	return client.GetDevices()
}

// SSHClient manages SSH connection to hypervisors
type SSHClient struct {
	client *ssh.Client
}

// NewSSHClient dials an SSH connection given a list of host IP addresses
func NewSSHClient(hosts []string) (*SSHClient, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %v", err)
	}

	keyPath := filepath.Join(usr.HomeDir, ".ssh", "id_rsa")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		keyPath = filepath.Join(usr.HomeDir, ".ssh", "id_ed25519")
		key, err = os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SSH key from ~/.ssh/id_rsa or ~/.ssh/id_ed25519: %v", err)
		}
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User: usr.Username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	var lastErr error
	for _, host := range hosts {
		if !strings.Contains(host, ":") {
			host = host + ":22"
		}

		client, err := ssh.Dial("tcp", host, config)
		if err == nil {
			return &SSHClient{client: client}, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed to connect to any host: %v", lastErr)
}

// Close terminates the SSH connection
func (s *SSHClient) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// RunCommand executes a command over SSH session
func (s *SSHClient) RunCommand(cmd string) (string, error) {
	if s.client == nil {
		return "", fmt.Errorf("ssh client is nil")
	}
	session, err := s.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	return string(output), err
}

// GetDevices queries hypervisor for physical block devices using by-id and lsblk
func (s *SSHClient) GetDevices() ([]ctlplfl.Device, error) {
	deviceMap := make(map[string]*ctlplfl.Device)

	byIdOutput, err := s.RunCommand("ls -la /dev/disk/by-id/ 2>/dev/null | grep -v '^total' | grep -v '^d' | awk '{print $9, $11}'")
	if err == nil && byIdOutput != "" {
		for _, dev := range ParseByIdDevices(byIdOutput) {
			blockName := filepath.Base(dev.DevicePath)
			if _, exists := deviceMap[blockName]; !exists {
				d := dev
				deviceMap[blockName] = &d
			}
		}
	}

	lsblkOutput, err := s.RunCommand("lsblk -d -n -o ID,NAME,SIZE,SERIAL,TYPE | grep 'disk'")
	if err != nil {
		return nil, fmt.Errorf("failed to get device list: %v", err)
	}

	for _, lsblkDev := range ParseLsblkDevices(lsblkOutput) {
		if existing, exists := deviceMap[lsblkDev.Name]; exists {
			existing.Size = lsblkDev.Size
			if existing.SerialNumber == "" {
				existing.SerialNumber = lsblkDev.SerialNumber
			}
		} else {
			log.Info("Add device to list (no by-id entry): ", lsblkDev)
			d := lsblkDev
			deviceMap[lsblkDev.Name] = &d
		}
	}

	// Bulk-fetch OS-level partitions for every disk in one round trip (rather
	// than one lsblk call per device) so the discovery table can flag disks
	// that already have a partition table on them — e.g. reused hardware —
	// before the operator initializes them. These are display-only: the
	// discovered device's Partitions field never reaches PutDevice (see
	// handleSubmitDiscoveredDevices), so it can't be confused with niova's
	// own tracked partitions, which only exist after a device is registered
	// and split via "Split Disk into Partitions".
	partOutput, err := s.RunCommand("lsblk -ln -b -o NAME,SIZE,PKNAME,TYPE")
	if err != nil {
		log.Warnf("GetDevices: failed to list partitions: %v", err)
	} else if partOutput != "" {
		partsByParent := ParseLsblkPartitions(partOutput)
		for name, dev := range deviceMap {
			if parts, ok := partsByParent[name]; ok {
				dev.Partitions = parts
			}
		}
	}

	result := make([]ctlplfl.Device, 0, len(deviceMap))
	for _, dev := range deviceMap {
		result = append(result, *dev)
	}
	return result, nil
}
