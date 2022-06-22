package ovs

import (
	"os/exec"
	"strings"
)

func GetOvsBrList() ([]string, error) {

	out, err := exec.Command("ovs-vsctl", "list-br").Output()
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	op := string(out[:])
	output := strings.TrimSuffix(op, "\n")
	var brList []string = strings.Split(output, "\n")
	return brList, nil
}

func GetOvsBrPort(brName string) (string, error) {

	out, err := exec.Command("ovs-vsctl", "list-ports", brName).Output()
	if err != nil {
		return "", err
	}
	if len(out) == 0 {
		return "", nil
	}
	op := string(out[:])
	port := strings.TrimSuffix(op, "\n")
	return port, nil
}

func DeleteOvsBr(brName string) error {
	err := exec.Command("ovs-vsctl", "del-br", brName).Run()
	return err
}
