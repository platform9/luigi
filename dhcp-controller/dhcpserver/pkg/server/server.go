package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"time"

	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"io"

	ctrl "sigs.k8s.io/controller-runtime"

	"dhcpserver/pkg/kubernetes"

	"github.com/fsnotify/fsnotify"
	"github.com/gocarina/gocsv"
)

var (
	serverLog  = ctrl.Log.WithName("server")
	dnsmasqLog = ctrl.Log.WithName("dnsmasq")
	leasePath  = "/var/lib/dnsmasq/dnsmasq.leases"
	confFile   = "/etc/dnsmasq.d/dnsmasq.conf"
	k8sClient  *kubernetes.Client
	IPRanges   = []IPRange{}
)

type LeaseFile struct {
	EpochTimestamp string `json:"epoch-timestamp"`
	MacAddress     string `json:"mac_addr"`
	IPAddress      string `json:"ip_addr"`
	Hostname       string `json:"hostname"`
	ClientID       string `json:"client-ID"`
}

type IPRange struct {
	StartIP net.IP
	EndIP   net.IP
	VlanID  string
}

func parseConfig() error {
	f, err := os.Open(confFile)
	if err != nil {
		serverLog.Error(err, "Cannot open config file")
		return err
	}
	defer f.Close()

	fileScanner := bufio.NewScanner(f)

	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		line := fileScanner.Text()
		if strings.Contains(line, "dhcp-range") {
			dhcprangearray := regexp.MustCompile("[\\=\\s,]").Split(line, -1)
			dhcprangearraylen := len(dhcprangearray)

			vlanid := ""
			if dhcprangearraylen == 6 {
				vlanid = dhcprangearray[1]
			}

			IPRanges = append(IPRanges, IPRange{net.ParseIP(dhcprangearray[dhcprangearraylen-4]), net.ParseIP(dhcprangearray[dhcprangearraylen-3]), vlanid})
		}
	}
	return nil
}

func delLeasefromVMPod(refs []string) error {
	f, err := os.Open(leasePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var bs []byte
	var dowrite = true
	buf := bytes.NewBuffer(bs)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		for _, ref := range refs {
			if strings.Contains(scanner.Text(), ref) {
				dowrite = false
			}
		}
		if dowrite {
			_, err := buf.Write(scanner.Bytes())
			if err != nil {
				return err
			}
			_, err = buf.WriteString("\n")
			if err != nil {
				return err
			}
		}
		dowrite = true
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	err = os.WriteFile(leasePath, buf.Bytes(), 0666)
	if err != nil {
		return err
	}

	return nil

}

func Start() {

	dnsmasqBinary, err := exec.LookPath("dnsmasq")
	if err != nil {
		panic("dnsmasq binary is not found!")
	}

	if _, err := os.Stat(confFile); err == nil {
		serverLog.Info("Starting dnsmasq: confFile is present")
	} else if errors.Is(err, os.ErrNotExist) {
		serverLog.Error(err, "confFile not found, please check the volumeMount")
		panic(err)
	}

	parseConfig()

	args := []string{
		"dnsmasq",
		"--no-daemon",
		"--log-facility=/var/log/dnsmasq.log",
		"--conf-dir=/etc/dnsmasq.d/",
	}

	lf := make(map[string]LeaseFile)
	k8sClient, err = kubernetes.NewClient(10 * time.Second)
	if err != nil {
		serverLog.Error(err, "Failed to instantiate the Kubernetes client")
	}
	go k8sClient.WatchVm()
	go k8sClient.WatchPod()

	// Create leasefile from ipallocation backup
	retrieveBackup(leasePath)

	cmd := serverStart(dnsmasqBinary, args)

	go func() {
		for {
			select {
			case delvm := <-kubernetes.RestartDnsmasq:
				serverStop(cmd)
				err := delLeasefromVMPod(delvm)
				if err != nil {
					serverLog.Error(err, "failed to delete lease on vm/pod deletion")
				}
				cmd = serverStart(dnsmasqBinary, args)
			}
		}
	}()

	for {
		dirInit(leasePath)
		StartWatcher(lf, leasePath)
	}
}

func serverStart(dnsmasqBinary string, args []string) *exec.Cmd {
	cmd := &exec.Cmd{
		Path: dnsmasqBinary,
		Args: args,
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	buf := bufio.NewReader(stderr)
	go func() {
		for {
			line, _, err := buf.ReadLine()
			if err != nil {
				break
			}
			dnsmasqLog.Info(string(line))
		}
	}()
	serverLog.Info("Starting dnsmasq: " + strings.Join(args, " "))
	return cmd
}

func serverStop(cmd *exec.Cmd) {
	timer := time.AfterFunc(1*time.Second, func() {
		err := cmd.Process.Kill()
		if err != nil {
			panic(err)
		}
	})
	cmd.Wait()
	timer.Stop()
	serverLog.Info("Stopping dnsmasq")
}

// Retrieve leases from backup. Will be retrieved from etcd later on
func retrieveBackup(leasePath string) error {
	serverLog.Info("Retrieving Backup")

	ipAllocations, err := k8sClient.ListIPAllocations(context.TODO())
	if err != nil {
		serverLog.Error(err, "failed to fetch IPallocations")
	}

	destination, err := os.Create(leasePath)
	if err != nil {
		return err
	}
	defer destination.Close()

	for _, ipallocation := range ipAllocations {
		ipvAddr := net.ParseIP(ipallocation.Name)
		for _, iprange := range IPRanges {
			if iprange.VlanID == ipallocation.Spec.VlanID && bytes.Compare(ipvAddr, iprange.StartIP) >= 0 && bytes.Compare(ipvAddr, iprange.EndIP) <= 0 {
				tmpline := fmt.Sprintf(ipallocation.Spec.LeaseExpiry + " " + ipallocation.Spec.MacAddr + " " + ipallocation.Name + " " + ipallocation.Spec.EntityRef + " *\n")
				destination.WriteString(tmpline)

				serverLog.Info("Restored IPAllocation " + ipallocation.Name)
			}
		}

	}

	return err
}

// Check if the lease file exists
func dirInit(leasePath string) {
	_, err := os.Stat(leasePath)
	if os.IsNotExist(err) {
		// If not, retrieve from backup and create leasefile
		retrieveBackup(leasePath)
	}
}

// Checks if lease found in records exist in leasefile.
// This is for the scenario when leases expire and dnsmasq deletes
// the lease from the leasefile
func leaseExist(ip string, records []LeaseFile) (result bool) {
	result = false
	for _, lease := range records {
		if lease.IPAddress == ip {
			result = true
			break
		}
	}
	return result
}

// Checks if existing entry in leasefile has been updated
func checkRecord(lease LeaseFile, record LeaseFile) bool {
	serverLog.Info("Checking lease entry....")
	var isupdated = false
	if record.EpochTimestamp != lease.EpochTimestamp ||
		record.MacAddress != lease.MacAddress ||
		record.IPAddress != lease.IPAddress ||
		record.Hostname != lease.Hostname ||
		record.ClientID != lease.ClientID {
		isupdated = true
	}
	return isupdated
}

// updates record with new data from leasefile
func updateRecord(lf map[string]LeaseFile, record LeaseFile, isupdate bool) {
	serverLog.Info("Updating Lease")
	lf[record.IPAddress] = record

	vlanid := ""
	for _, iprange := range IPRanges {
		if bytes.Compare(net.ParseIP(record.IPAddress), iprange.StartIP) >= 0 && bytes.Compare(net.ParseIP(record.IPAddress), iprange.EndIP) <= 0 {
			vlanid = iprange.VlanID
		}
	}

	if isupdate {
		_, err := k8sClient.UpdateIPAllocation(context.TODO(), record.EpochTimestamp, record.MacAddress, record.IPAddress, vlanid)
		if err != nil {
			serverLog.Error(err, "failed to update IP allocation")
		}
	} else {
		_, err := k8sClient.CreateIPAllocation(context.TODO(), record.EpochTimestamp, record.MacAddress, record.Hostname, record.IPAddress, vlanid)
		if err != nil {
			serverLog.Error(err, "failed to create IP allocation")
		}
	}

}

func readLeaseFile(lf map[string]LeaseFile, leasePath string) (string, error) {
	serverLog.Info("Reading leasefile...")
	f, err := os.Open(leasePath)
	if err != nil {
		serverLog.Error(err, "Cannot open lease file")
		return "", err
	}
	defer f.Close()

	var records []LeaseFile
	csvReader2 := csv.NewReader(f)
	csvReader2.Comma = ' '
	err = gocsv.UnmarshalCSVWithoutHeaders(csvReader2, &records)
	if err != nil && err != gocsv.ErrEmptyCSVFile {
		serverLog.Error(err, "Cannot parse lease file")
		return "", err
	}

	// Check if record exists in leasefile
	for ip, _ := range lf {
		if !leaseExist(ip, records) {
			delete(lf, ip)
			isdeleted, err := k8sClient.DeleteIPAllocation(context.TODO(), ip)
			if err != nil {
				serverLog.Error(err, "failed to delete IP allocation")
			}
			if isdeleted {
				serverLog.Info("Deleted IPAllocation " + ip)
			}
		}
	}

	// Check if lease exists in record and if it is up to date
	for _, record := range records {
		if lease, ok := lf[record.IPAddress]; ok {
			// Check if any entry in existing lease has been updated, like epoch time
			if checkRecord(lease, record) {
				updateRecord(lf, record, true)
			}
		} else {
			updateRecord(lf, record, false)
		}
	}
	tmp, _ := json.MarshalIndent(lf, "", "	")
	serverLog.Info(string(tmp))

	return hash_file_md5(leasePath)
}

func writeLeaseFile(lf map[string]LeaseFile, leasePath string) {}

func hash_file_md5(filePath string) (string, error) {
	var returnMD5String string
	file, err := os.Open(filePath)
	if err != nil {
		return returnMD5String, err
	}
	defer file.Close()
	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return returnMD5String, err
	}
	hashInBytes := hash.Sum(nil)[:16]
	returnMD5String = hex.EncodeToString(hashInBytes)
	return returnMD5String, nil

}

// Starts watching leasefile. Ends if error in accessing/reading leasefile
func StartWatcher(lf map[string]LeaseFile, leasePath string) {
	serverLog.Info("Starting Watcher....")
	var watcher *fsnotify.Watcher
	var oldmd5 string       //Hash Comparison
	done := make(chan bool) //Ending the function
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		serverLog.Error(err, "Cannot watch leasefile")
	}
	defer watcher.Close()

	// leasefile event trigger
	go func() {
	out:
		for {
			select {
			case <-done:
				break out
			// watch for events
			case event := <-watcher.Events:
				// Check if file is actually updated. fsnotify gives multiple write events
				// depending on the editor and platform
				newmd5, err := hash_file_md5(leasePath)
				if err != nil {
					done <- true
				}

				if event.Op&fsnotify.Write == fsnotify.Write && oldmd5 != newmd5 {
					serverLog.Info("Write Event Detected .....")
					// Sleep for avoiding reading of truncated lease file
					time.Sleep(100 * time.Millisecond)
					// Read the file and update IPAllocations CR
					oldmd5, err = readLeaseFile(lf, leasePath)
					if err != nil {
						serverLog.Error(err, "Cannot read lease file")
						done <- true
					}
				}

			// watch for errors
			case err := <-watcher.Errors:
				serverLog.Error(err, "Watcher error")
				done <- true
			}
		}
		done <- true
	}()

	oldmd5, err = readLeaseFile(lf, leasePath)
	if err != nil {
		serverLog.Error(err, "Cannot read lease file")
	}
	// Watcher start
	if err := watcher.Add(leasePath); err != nil {
		serverLog.Error(err, "Cannot start leasefile watcher")
		done <- true
	}

	<-done
}
