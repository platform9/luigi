package sriov

import (
	"fmt"
	"hostplumber/pkg/consts"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"
)

func GetTotalVfs(devicePath string) int {
	totalVfsFile := filepath.Join(devicePath, "sriov_totalvfs")
	fd, err := os.Open(totalVfsFile)
	defer fd.Close()
	if err != nil {
		return -1
	}

	var totalVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &totalVfs)
	if err != nil {
		return -1
	}
	return totalVfs
}

func GetNumVfs(devicePath string) int {
	numVfsFile := filepath.Join(devicePath, "sriov_numvfs")
	fd, err := os.Open(numVfsFile)
	defer fd.Close()
	if err != nil {
		return -1
	}

	var numVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &numVfs)
	if err != nil {
		return -1
	}
	return numVfs
}

func GetVfPciAddrById(pfDevicePath string, vfId int) string {
	virtFn := fmt.Sprintf("virtfn%d", vfId)
	vfDeviceLink := filepath.Join(pfDevicePath, virtFn)
	vfDevicePath, _ := filepath.EvalSymlinks(vfDeviceLink)
	pciAddr := filepath.Base(vfDevicePath)
	return pciAddr
}

func GetVfDriverByPci(vfPciAddr string) string {
	driverLink := filepath.Join(consts.SysPciDevices, vfPciAddr, "driver")
	driverPath, _ := filepath.EvalSymlinks(driverLink)
	driverName := filepath.Base(driverPath)
	return driverName
}

func GetPfDeviceForVf(devicePath string) (string, bool) {
	physfnFile := filepath.Join(devicePath, "physfn")
	if _, err := os.Lstat(physfnFile); err != nil {
		return "", false
	}
	pfDevicePath, _ := filepath.EvalSymlinks(physfnFile)
	return pfDevicePath, true
}

func VerifyPfExists(pfName string) bool {
	_, err := os.Lstat(filepath.Join(consts.SysClassNet, pfName))
	return err == nil
}

func GetTotalVfsForPf(pfName string) (int, error) {
	totalVfsFile := filepath.Join(consts.SysClassNet, pfName, "device", "sriov_totalvfs")
	fd, err := os.Open(totalVfsFile)
	defer fd.Close()
	if err != nil {
		return 0, err
	}

	var totalVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &totalVfs)
	if err != nil {
		return 0, err
	}
	return totalVfs, nil
}

func GetCurrentNumVfsForPf(pfName string) (int, error) {
	numVfsFile := filepath.Join(consts.SysClassNet, pfName, "device", "sriov_numvfs")
	fd, err := os.Open(numVfsFile)
	defer fd.Close()
	if err != nil {
		return 0, err
	}

	var numVfs int
	_, err = fmt.Fscanf(fd, "%d\n", &numVfs)
	if err != nil {
		return 0, err
	}
	return numVfs, nil
}

func CreateVfsForPfName(pfName string, numVfs int) error {
	totalVfs, err := GetTotalVfsForPf(pfName)
	if err != nil {
		return err
	}
	if numVfs > totalVfs {
		return errors.NewBadRequest("Can't create more VFs than total supported by PF")
	}

	currVfs, err := GetCurrentNumVfsForPf(pfName)
	if err != nil {
		return err
	}

	// Only create VFs if mismatch as it is a disruptive operation
	// Changing sriov_numvfs requires clearing it by writing 0 VFs first
	if currVfs == numVfs {
		return nil
	}

	numVfsFile := filepath.Join(consts.SysClassNet, pfName, "device", "sriov_numvfs")

	zero := []byte(strconv.Itoa(0))
	err = ioutil.WriteFile(numVfsFile, zero, os.ModeAppend)
	if err != nil {
		return err
	}

	vfs := []byte(strconv.Itoa(numVfs))
	err = ioutil.WriteFile(numVfsFile, vfs, os.ModeAppend)
	if err != nil {
		return err
	}

	return nil
}

func SetDriverOverride(vfPath string, driver string) error {
	driverBytes := []byte(driver)
	driverOverride := filepath.Join(vfPath, "driver_override")
	err := ioutil.WriteFile(driverOverride, driverBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

func UnbindOldDriver(vfPath string) error {
	driverPathLink := filepath.Join(vfPath, "driver")
	driverPath, err := filepath.EvalSymlinks(driverPathLink)
	if err != nil {
		return err
	}
	pciAddr := filepath.Base(vfPath)
	pciAddrBytes := []byte(pciAddr)
	err = ioutil.WriteFile(filepath.Join(driverPath, "unbind"), pciAddrBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

func BindNewDriver(vfPath string, driver string) error {
	pciAddr := filepath.Base(vfPath)
	pciAddrBytes := []byte(pciAddr)
	driverPath := filepath.Join(consts.SysPciDrivers, driver)

	err := ioutil.WriteFile(filepath.Join(driverPath, "bind"), pciAddrBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

func SetDriverForVf(vfPath string, driver string) error {
	if err := UnbindOldDriver(vfPath); err != nil {
		return err
	}
	if err := SetDriverOverride(vfPath, driver); err != nil {
		return err
	}
	if err := BindNewDriver(vfPath, driver); err != nil {
		return err
	}
	return nil
}

func EnableDriverForVfs(pfName string, driver string) {
	devicePathLink := filepath.Join(consts.SysClassNet, pfName, "device")
	devicePath, _ := filepath.EvalSymlinks(devicePathLink)
	err := filepath.Walk(devicePath, func(path string, info os.FileInfo, err error) error {
		name := info.Name()

		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			isVF, _ := filepath.Match("virtfn*", name)
			if isVF == false {
				return nil
			}
			fullPath, _ := filepath.EvalSymlinks(path)
			if err := SetDriverForVf(fullPath, driver); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		// TODO: Why am I not returning an error here?
		return
	}
}

func GetPfListForVendorAndDevice(vendor, device string) ([]string, error) {
	// This function will return any IF that matches vendor and device ID
	// If optional PCI address is given, it must match
	var matchingPfs []string
	err := filepath.Walk(consts.SysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		// Everything in this directory is a symlink
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == consts.SysClassNet {
			// Skip the root /sys/class/net folder
			return nil
		}
		if ifName == "lo" {
			return nil
		}

		devicePathLink := filepath.Join(ifPath, "device")
		devicePath, _ := filepath.EvalSymlinks(devicePathLink)
		deviceIdFile := filepath.Join(devicePath, "device")

		if _, err := os.Stat(deviceIdFile); err != nil {
			return nil
		}
		fd, err := os.Open(deviceIdFile)
		if err != nil {
			fd.Close()
			return err
		}
		var deviceId string
		_, err = fmt.Fscanf(fd, "0x%s\n", &deviceId)
		if err != nil {
			fd.Close()
			return err
		}
		fd.Close()

		vendorIdFile := filepath.Join(devicePath, "vendor")
		if _, err := os.Stat(vendorIdFile); err != nil {
			return nil
		}
		fd, err = os.Open(vendorIdFile)
		defer fd.Close()
		if err != nil {
			return err
		}
		var vendorId string
		_, err = fmt.Fscanf(fd, "0x%s\n", &vendorId)
		if err != nil {
			return err
		}

		if vendor == vendorId && device == deviceId {
			matchingPfs = append(matchingPfs, ifName)
		}
		return nil
	})

	if len(matchingPfs) == 0 {
		err = errors.NewBadRequest("Failed to find any devices matching vendor and device ID")
	}
	return matchingPfs, err
}

func GetPfNameForPciAddr(pciAddr string) (string, error) {
	var matchingPf string
	err := filepath.Walk(consts.SysClassNet, func(path string, info os.FileInfo, err error) error {
		ifName := info.Name()
		// Everything in this directory is a symlink
		ifPath, _ := filepath.EvalSymlinks(path)
		if ifPath == consts.SysClassNet {
			return nil
		}
		if ifName == "lo" {
			return nil
		}

		devicePathLink := filepath.Join(ifPath, "device")
		devicePath, _ := filepath.EvalSymlinks(devicePathLink)

		devPci := filepath.Base(devicePath)
		if devPci == pciAddr {
			matchingPf = ifName
			// Short circuit because we found a match
			return io.EOF
		}
		return nil
	})
	if err != nil {
		if err == io.EOF {
			err = nil
		}
	} else {
		err = errors.NewBadRequest("Failed to find interface with matching PCI address")
	}
	return matchingPf, err
}
