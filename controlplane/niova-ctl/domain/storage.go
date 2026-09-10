package domain

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
)

// ParseByIdDevices parses `ls -la /dev/disk/by-id/` output
func ParseByIdDevices(output string) []ctlplfl.Device {
	devices := make([]Device, 0)
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			id := parts[0]
			target := parts[1]

			if strings.Contains(id, "-part") {
				continue
			}

			if strings.Contains(target, "../") {
				deviceName := strings.TrimPrefix(filepath.Base(target), "../")

				if regexp.MustCompile(`p\d+$|[a-z]\d+$`).MatchString(deviceName) {
					continue
				}

				devices = append(devices, Device{
					ID:         id,
					Name:       id,
					DevicePath: "/dev/" + deviceName,
				})
			}
		}
	}

	return devices
}

// ParseSizeToBytes converts human-readable size strings (like "1T", "500G", "1.2G") to bytes
func ParseSizeToBytes(sizeStr string) int64 {
	if sizeStr == "" {
		return 0
	}

	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*([KMGTPE]?)$`)
	matches := re.FindStringSubmatch(strings.ToUpper(sizeStr))
	if len(matches) < 3 {
		if bytes, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			return bytes
		}
		return 0
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	unit := matches[2]
	var multiplier int64 = 1

	switch unit {
	case "K":
		multiplier = 1024
	case "M":
		multiplier = 1024 * 1024
	case "G":
		multiplier = 1024 * 1024 * 1024
	case "T":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "P":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024
	case "E":
		multiplier = 1024 * 1024 * 1024 * 1024 * 1024 * 1024
	case "":
		multiplier = 1
	}

	return int64(value * float64(multiplier))
}

// ParseLsblkDevices parses lsblk command output
func ParseLsblkDevices(output string) []ctlplfl.Device {
	devices := make([]Device, 0)
	scanner := bufio.NewScanner(strings.NewReader(output))
	re := regexp.MustCompile(`^(\S*)\s+(\S+)\s+(\S+)\s+(\S*)\s+disk`)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) >= 5 {
			id := matches[1]
			name := matches[2]
			sizeStr := matches[3]
			serialNum := matches[4]

			if id == "" {
				id = name
			}

			sizeBytes := ParseSizeToBytes(sizeStr)
			log.Infof("Device %s: size string '%s' parsed to %d bytes", name, sizeStr, sizeBytes)

			devices = append(devices, Device{
				ID:           id,
				Name:         name,
				DevicePath:   "/dev/" + name,
				Size:         sizeBytes,
				SerialNumber: serialNum,
			})
		}
	}

	return devices
}

// ParseLsblkPartitions parses `lsblk -ln -b -o NAME,SIZE,PKNAME,TYPE` output
// into a map of parent disk name (e.g. "nvme1n1") to the OS-level partitions
// found on it. One bulk call covers every disk on the hypervisor, instead of
// a GetDevicePartitionInfo-style round trip per device — needed for the
// discovery table, which lists every disk in one screen.
func ParseLsblkPartitions(output string) map[string][]ctlplfl.DevicePartition {
	result := make(map[string][]ctlplfl.DevicePartition)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[3] != "part" || fields[2] == "" {
			continue
		}
		name, sizeStr, parent := fields[0], fields[1], fields[2]
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			log.Warnf("ParseLsblkPartitions: failed to parse size for %s: %v", name, err)
		}
		result[parent] = append(result[parent], ctlplfl.DevicePartition{
			PartitionPath: "/dev/" + name,
			Size:          size,
		})
	}
	return result
}

// GetDeviceSize gets the actual size of a device in bytes via SSH
func GetDeviceSize(hv ctlplfl.Hypervisor, deviceName string) (int64, error) {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	devicePath := fmt.Sprintf("/dev/%s", deviceName)
	checkCmd := fmt.Sprintf("test -b %s && echo 'exists' || echo 'not found'", devicePath)
	result, err := sshClient.RunCommand(checkCmd)
	if err != nil {
		return 0, fmt.Errorf("failed to check device %s: %v", devicePath, err)
	}
	if strings.TrimSpace(result) != "exists" {
		ip, err2 := hv.GetPrimaryIP()
		if err2 != nil {
			log.Error("GetDeviceSize(): failed to fetch network info: ", err2)
		}
		return 0, fmt.Errorf("device %s not found on hypervisor %s", devicePath, ip)
	}

	sizeCmd := fmt.Sprintf("blockdev --getsize64 %s", devicePath)
	sizeResult, err := sshClient.RunCommand(sizeCmd)
	if err != nil {
		return 0, fmt.Errorf("failed to get size for device %s: %v", devicePath, err)
	}

	sizeBytes, err := strconv.ParseInt(strings.TrimSpace(sizeResult), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse device size: %v", err)
	}

	return sizeBytes, nil
}

// DeleteAllPartitionsFromDevice removes all partitions from a device
func DeleteAllPartitionsFromDevice(hv ctlplfl.Hypervisor, deviceName string) error {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	devicePath := fmt.Sprintf("/dev/%s", deviceName)

	checkCmd := fmt.Sprintf("test -b %s && echo 'exists' || echo 'not found'", devicePath)
	result, err := sshClient.RunCommand(checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check device %s: %v", devicePath, err)
	}
	if strings.TrimSpace(result) != "exists" {
		return fmt.Errorf("device %s not found on hypervisor %s", devicePath, hv.IPAddrs)
	}

	partCmd := fmt.Sprintf("parted -s %s mklabel gpt", devicePath)
	_, _ = sshClient.RunCommand(partCmd)
	log.Infof("Create partition table on %s (%s)", devicePath, partCmd)

	time.Sleep(2 * time.Second)

	return nil
}

// CreateMultipleEqualPartitions creates equal-sized partitions on a device
func CreateMultipleEqualPartitions(hv ctlplfl.Hypervisor, deviceName string, numPartitions int) error {
	if numPartitions <= 0 {
		return fmt.Errorf("number of partitions must be greater than 0")
	}

	_, err := GetDeviceSize(hv, deviceName)
	if err != nil {
		return fmt.Errorf("failed to get device size: %v", err)
	}

	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	devicePath := fmt.Sprintf("/dev/%s", deviceName)
	partitionPercentage := 100 / numPartitions

	for i := 0; i < numPartitions; i++ {
		startPercentage := i * partitionPercentage
		endPercentage := (i + 1) * partitionPercentage

		if i == numPartitions-1 {
			endPercentage = 100
		}

		partCmd := fmt.Sprintf("parted -s %s mkpart primary %d%% %d%%",
			devicePath, startPercentage, endPercentage)

		_, _ = sshClient.RunCommand(partCmd)
	}

	sshClient.RunCommand(fmt.Sprintf("partprobe %s", devicePath))
	sshClient.RunCommand("udevadm settle")
	time.Sleep(2 * time.Second)

	return nil
}

// GetDevicePartitionNames retrieves partition names for a device using lsblk
func GetDevicePartitionNames(hv ctlplfl.Hypervisor, deviceName string) ([]string, error) {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	cmd := fmt.Sprintf("lsblk -ln -o NAME /dev/%s", deviceName)
	output, err := sshClient.RunCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition names for device %s: %v", deviceName, err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var partitionNames []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != deviceName {
			partitionNames = append(partitionNames, line)
		}
	}

	sort.Strings(partitionNames)
	return partitionNames, nil
}

// GetDevicePartitionInfo retrieves partition names and sizes for a device using lsblk
func GetDevicePartitionInfo(hv ctlplfl.Hypervisor, deviceName string) ([]DevicePartitionInfo, error) {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	byIdCmd := fmt.Sprintf("ls -la /dev/disk/by-id/ | grep '%s$' | head -1 | awk '{print $9}'", deviceName)
	byIdOutput, err := sshClient.RunCommand(byIdCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get by-id name for device %s: %v", deviceName, err)
	}
	deviceByIdName := strings.TrimSpace(byIdOutput)
	if deviceByIdName == "" {
		return nil, fmt.Errorf("could not find by-id name for device %s", deviceName)
	}

	cmd := fmt.Sprintf("lsblk -ln -b -o NAME,SIZE /dev/%s", deviceName)
	output, err := sshClient.RunCommand(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition info for device %s: %v", deviceName, err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var partitionInfos []DevicePartitionInfo

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				partitionName := parts[0]
				if partitionName != deviceName {
					sizeStr := parts[1]
					size, err := strconv.ParseInt(sizeStr, 10, 64)
					if err != nil {
						log.Warnf("Failed to parse size for partition %s: %v", partitionName, err)
						size = 0
					}
					// The kernel device node — what a NISD actually opens —
					// regardless of whether the by-id lookup below succeeds.
					partitionPath := "/dev/" + partitionName

					partitionByIdCmd := fmt.Sprintf("ls -la /dev/disk/by-id/ | grep '%s$' | head -1 | awk '{print $9}'", partitionName)
					partitionByIdOutput, err := sshClient.RunCommand(partitionByIdCmd)
					if err != nil {
						log.Warnf("Failed to get by-id name for partition %s: %v", partitionName, err)
						partitionInfos = append(partitionInfos, DevicePartitionInfo{
							Name: partitionName,
							Path: partitionPath,
							Size: size,
						})
					} else {
						partitionByIdName := strings.TrimSpace(partitionByIdOutput)
						if partitionByIdName != "" {
							partitionInfos = append(partitionInfos, DevicePartitionInfo{
								Name: partitionByIdName,
								Path: partitionPath,
								Size: size,
							})
						} else {
							partitionInfos = append(partitionInfos, DevicePartitionInfo{
								Name: partitionName,
								Path: partitionPath,
								Size: size,
							})
						}
					}
				}
			}
		}
	}

	sort.Slice(partitionInfos, func(i, j int) bool {
		return partitionInfos[i].Name < partitionInfos[j].Name
	})
	return partitionInfos, nil
}

// GetDeviceByIdName gets the /dev/disk/by-id/ name for a device
func GetDeviceByIdName(hv ctlplfl.Hypervisor, deviceName string) (string, error) {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return "", fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	byIdCmd := fmt.Sprintf("ls -la /dev/disk/by-id/ | grep '%s$' | head -1 | awk '{print $9}'", deviceName)
	byIdOutput, err := sshClient.RunCommand(byIdCmd)
	if err != nil {
		return "", fmt.Errorf("failed to get by-id name for device %s: %v", deviceName, err)
	}
	deviceByIdName := strings.TrimSpace(byIdOutput)
	if deviceByIdName == "" {
		return "", fmt.Errorf("could not find by-id name for device %s", deviceName)
	}

	return deviceByIdName, nil
}

// DeletePhysicalPartition removes an actual partition from physical device
func DeletePhysicalPartition(hv ctlplfl.Hypervisor, deviceName string, partitionNumber int) error {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}
	defer sshClient.Close()

	devicePath := fmt.Sprintf("/dev/%s", deviceName)

	checkCmd := fmt.Sprintf("test -b %s && echo 'exists' || echo 'not found'", devicePath)
	result, err := sshClient.RunCommand(checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check device %s: %v", devicePath, err)
	}
	if strings.TrimSpace(result) != "exists" {
		return fmt.Errorf("device %s not found on hypervisor %s", devicePath, hv.IPAddrs)
	}

	checkPartCmd := fmt.Sprintf("parted -s %s print | grep '^ %d '", devicePath, partitionNumber)
	partResult, err := sshClient.RunCommand(checkPartCmd)
	if err != nil || strings.TrimSpace(partResult) == "" {
		return fmt.Errorf("partition %d not found on device %s", partitionNumber, devicePath)
	}

	deleteCmd := fmt.Sprintf("parted -s %s rm %d", devicePath, partitionNumber)
	_, err = sshClient.RunCommand(deleteCmd)
	if err != nil {
		return fmt.Errorf("failed to delete partition %d from %s: %v", partitionNumber, devicePath, err)
	}

	time.Sleep(2 * time.Second)

	verifyCmd := fmt.Sprintf("parted -s %s print | grep '^ %d ' | wc -l", devicePath, partitionNumber)
	verifyResult, err := sshClient.RunCommand(verifyCmd)
	if err != nil {
		return fmt.Errorf("failed to verify partition deletion: %v", err)
	}
	if strings.TrimSpace(verifyResult) != "0" {
		return fmt.Errorf("partition deletion verification failed for partition %d on %s", partitionNumber, devicePath)
	}

	return nil
}

// RemoveDevicePartition removes a physical NISD partition from a device via SSH
func RemoveDevicePartition(hv ctlplfl.Hypervisor, deviceName, partitionID string) error {
	sshClient, err := newSSHExecutor(hv.IPAddrs)
	if err != nil {
		return fmt.Errorf("failed to connect to hypervisor %s: %v", hv.IPAddrs, err)
	}

	devicePath := fmt.Sprintf("/dev/%s", deviceName)

	listCmd := fmt.Sprintf("parted -s %s print 2>/dev/null | grep -E '^ *[0-9]+'", devicePath)
	output, err := sshClient.RunCommand(listCmd)
	sshClient.Close()

	if err != nil {
		return fmt.Errorf("failed to list partitions on %s: %v", devicePath, err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for partIdx, line := range lines {
		if strings.Contains(line, partitionID) {
			return DeletePhysicalPartition(hv, deviceName, partIdx+1)
		}
	}

	log.Warnf("Partition %s not found on device %s, may already be removed", partitionID, deviceName)
	return nil
}
