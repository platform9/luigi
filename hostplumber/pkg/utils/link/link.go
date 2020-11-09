package link

import (
	"fmt"
	"hostplumber/pkg/consts"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
)

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
