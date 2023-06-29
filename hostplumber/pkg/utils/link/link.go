package link

import (
	"bufio"
	"fmt"
	"hostplumber/pkg/consts"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func checkIfExists(name string) bool {
	_, err := net.InterfaceByName(name)
	if err != nil {
		return false
	}
	fmt.Printf("Interface %s already exists\n", name)
	return true
}

func createVlanIfFile(name string) error {
	ifCfg := []string{
		fmt.Sprintf("DEVICE=%s", name),
		"BOOTPROTO=none",
		"ONBOOT=yes",
		"VLAN=yes",
	}

	ifCfgFile := filepath.Join(consts.RhelNetworkScripts, "ifcfg-"+name)
	fd, err := os.OpenFile(ifCfgFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	defer fd.Close()
	if err != nil {
		fmt.Printf("Failed to create ifcfg file for %s\n", name)
		return err
	}

	writer := bufio.NewWriter(fd)
	for _, line := range ifCfg {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			fmt.Printf("Failed to write line %s\n", line)
			return err
		}
	}
	writer.Flush()
	return nil
}

func CreateVlanIf(name string, parent string, vlanId int) error {
	exists := checkIfExists(name)
	if exists {
		fmt.Printf("already exists\n")
		return nil
	}

	vid := strconv.Itoa(vlanId)
	cmd := exec.Command("ip", "link", "add", "link", parent, "name", name, "type", "vlan", "id", vid)
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("ip", "link", "set", "dev", name, "up")
	if err := cmd.Run(); err != nil {
		return err
	}

	return createVlanIfFile(name)
}

func deleteVlanIfFile(name string) error {
	ifCfgFile := filepath.Join(consts.RhelNetworkScripts, "ifcfg-"+name)
	_, err := os.Stat(ifCfgFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File does not exist, nothing to do...\n")
			return nil
		} else {
			return err
		}
	}

	return os.Remove(ifCfgFile)
}

func deleteVlanIf(name string) error {
	if err := deleteVlanIfFile(name); err != nil {
		fmt.Printf("Failed to remove vlan interface cfg file for %s\n", name)
		return err
	}

	cmd := exec.Command("ip", "link", "delete", name)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to delete %s\n", name)
		return err
	}
	return nil
}

func getManagedVlansForIf(templateName, ifName string) ([]string, error) {
	ifVlansFile := filepath.Join(consts.HostPlumberCfg, templateName, ifName, "vlans")
	fd, err := os.Open(ifVlansFile)
	defer fd.Close()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("File does not exist, no pre-managed vlan-ifs\n")
			return nil, nil
		}
		fmt.Printf("Error opening up saved vlans file\n")
		return nil, err
	}

	scanner := bufio.NewScanner(fd)
	scanner.Split(bufio.ScanLines)
	var vlans []string
	for scanner.Scan() {
		vlans = append(vlans, scanner.Text())
	}
	return vlans, nil
}

func UpdateManagedVlans(templateName, ifName string, newVlans []string) error {
	var finalVlans []string
	currentVlans := make(map[string]bool)
	current, err := getManagedVlansForIf(templateName, ifName)
	if err != nil {
		fmt.Printf("Error getting existing VLANs for interface\n")
		return err
	}

	for _, vlan := range current {
		currentVlans[vlan] = true
	}

	for _, vlan := range newVlans {
		currentVlans[vlan] = true
	}

	for vlan := range currentVlans {
		finalVlans = append(finalVlans, vlan)
	}
	if err := saveManagedVlans(templateName, ifName, finalVlans); err != nil {
		return err
	}
	return nil
}

func ReplaceManagedVlans(templateName, ifName string, newVlans []string) error {
	oldVlans := make(map[string]bool)
	old, err := getManagedVlansForIf(templateName, ifName)
	if err != nil {
		fmt.Printf("Error getting existing VLANs for interface\n")
		return err
	}

	for _, vlan := range old {
		oldVlans[vlan] = true
	}

	for _, vlan := range newVlans {
		delete(oldVlans, vlan)
	}

	if err := saveManagedVlans(templateName, ifName, newVlans); err != nil {
		return err
	}

	// Need to physically cleanup old VLAN IFs since we are replacing
	for vlan := range oldVlans {
		deleteVlanIf(vlan)
	}
	return nil
}

func saveManagedVlans(templateName, ifName string, vlanList []string) error {
	if _, err := os.Stat(filepath.Join(consts.HostPlumberCfg, templateName, ifName)); os.IsNotExist(err) {
		os.MkdirAll(filepath.Join(consts.HostPlumberCfg, templateName, ifName), 0766)
	}

	ifVlansFile := filepath.Join(consts.HostPlumberCfg, templateName, ifName, "vlans")
	fd, err := os.OpenFile(ifVlansFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	defer fd.Close()
	if err != nil {
		fmt.Printf("Failed to open file %s\n", ifVlansFile)
		return err
	}

	writer := bufio.NewWriter(fd)
	for _, line := range vlanList {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			fmt.Printf("Failed to write line %s to file %s\n", line, ifVlansFile)
			return err
		}
	}
	writer.Flush()
	return nil
}

func GetIfMtu(devicePath, ifName string) (int, error) {
	pfNetPath := filepath.Join(devicePath, "net", ifName)
	var mtu int
	fd, err := os.Open(filepath.Join(pfNetPath, "mtu"))
	defer fd.Close()
	if err != nil {
		return 0, err
	}
	_, err = fmt.Fscanf(fd, "%d\n", &mtu)
	if err != nil {
		return 0, err
	}
	return mtu, nil
}

func GetIfMac(devicePath, ifName string) (string, error) {
	pfNetPath := filepath.Join(devicePath, "net", ifName)
	var macAddr string
	fd, err := os.Open(filepath.Join(pfNetPath, "address"))
	defer fd.Close()
	if err != nil {
		return "", err
	}
	_, err = fmt.Fscanf(fd, "%s\n", &macAddr)
	if err != nil {
		return "", err
	}
	return macAddr, nil
}

func SetMtuForPf(pfName string, mtu int) error {
	mtuStr := []byte(strconv.Itoa(mtu))
	mtuFile := filepath.Join(consts.SysClassNet, pfName, "mtu")
	if err := ioutil.WriteFile(mtuFile, mtuStr, 0644); err != nil {
		return err
	}
	return SetMtuForAllVfs(pfName, mtu)
}

func SetMtuForAllVfs(pfName string, mtu int) error {
	pfDevice, _ := filepath.EvalSymlinks(filepath.Join(consts.SysClassNet, pfName, "device"))
	err := filepath.Walk(pfDevice, func(path string, info os.FileInfo, err error) error {
		name := info.Name()
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			isVF, _ := filepath.Match("virtfn*", name)
			if isVF == false {
				return nil
			}
			vfPath, _ := filepath.EvalSymlinks(path)
			if err := SetMtuForVf(vfPath, mtu); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func SetMtuForVf(vfPath string, mtu int) error {
	mtuStr := []byte(strconv.Itoa(mtu))
	vfNetPath := filepath.Join(vfPath, "net")
	vfEthDevice := vfNetPath + "/eth*"
	vfEth, _ := filepath.Glob(vfEthDevice)
	// This is a valid error, no eth device for DPDK driver or non-Ethernet devices
	if len(vfEth) == 0 {
		return nil
	}
	// There will only ever be 1 ethernet device per VF on kernel driver
	eth := vfEth[0]
	mtuFile := filepath.Join(eth, "mtu")
	if err := ioutil.WriteFile(mtuFile, mtuStr, 0644); err != nil {
		return err
	}
	return nil
}
