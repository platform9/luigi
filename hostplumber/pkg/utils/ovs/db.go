package ovs

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/socketplane/libovsdb"
)

var update chan *libovsdb.TableUpdates
var cache map[string]map[string]libovsdb.Row

func populateCache(updates libovsdb.TableUpdates) {
	for table, tableUpdate := range updates.Updates {
		if _, ok := cache[table]; !ok {
			cache[table] = make(map[string]libovsdb.Row)

		}
		for uuid, row := range tableUpdate.Rows {
			empty := libovsdb.Row{}
			if !reflect.DeepEqual(row.New, empty) {
				cache[table][uuid] = row.New
			} else {
				delete(cache[table], uuid)
			}
		}
	}
}

type myNotifier struct {
}

func (n myNotifier) Update(context interface{}, tableUpdates libovsdb.TableUpdates) {
	populateCache(tableUpdates)
	update <- &tableUpdates
}
func (n myNotifier) Locked([]interface{}) {
}
func (n myNotifier) Stolen([]interface{}) {
}
func (n myNotifier) Echo([]interface{}) {
}
func (n myNotifier) Disconnected(client *libovsdb.OvsdbClient) {
}

func getRootUUID() string {
	for uuid := range cache["Open_vSwitch"] {
		return uuid
	}
	return ""
}

// NewOvsClient creates OVS DB Client
func NewOvsClient(socketPath string) (*libovsdb.OvsdbClient, error) {
	update = make(chan *libovsdb.TableUpdates)
	cache = make(map[string]map[string]libovsdb.Row)

	ovs, err := libovsdb.ConnectWithUnixSocket(socketPath)
	if err != nil {
		return ovs, err
	}
	var notifier myNotifier
	ovs.Register(notifier)
	initial, _ := ovs.MonitorAll("Open_vSwitch", "")
	populateCache(*initial)
	return ovs, err
}

// CreateBridge adds entry for bridge in ovs db
func CreateBridge(ovs *libovsdb.OvsdbClient, bridgeName string) error {
	namedUUID := "gopher"
	// bridge row to insert
	bridge := make(map[string]interface{})
	bridge["name"] = bridgeName

	// simple insert operation
	insertOp := libovsdb.Operation{
		Op:       insertOperation,
		Table:    bridgeTable,
		Row:      bridge,
		UUIDName: namedUUID,
	}

	// Inserting a Bridge row in Bridge table requires mutating the open_vswitch table.
	uuidParameter := libovsdb.UUID{GoUUID: getRootUUID()}
	mutateUUID := []libovsdb.UUID{
		{GoUUID: namedUUID},
	}
	mutateSet, _ := libovsdb.NewOvsSet(mutateUUID)
	mutation := libovsdb.NewMutation("bridges", "insert", mutateSet)
	condition := libovsdb.NewCondition("_uuid", "==", uuidParameter)
	// simple mutate operation
	mutateOp := libovsdb.Operation{
		Op:        mutateOperation,
		Table:     ovsTable,
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	operations := []libovsdb.Operation{insertOp, mutateOp}
	reply, _ := ovs.Transact(ovsDatabase, operations...)

	if len(reply) < len(operations) {
		return errors.New("OVS transact failed with less replies than operations")
	}
	for _, o := range reply {
		if o.Error != "" {
			return fmt.Errorf("Trasaction failed due to an error: %s", o.Error)
		}
	}
	fmt.Println("Bridge %s created succesfully %s", bridgeName, reply[0].UUID.GoUUID)
	return nil
}

// DeleteBridge deletes bridge created via OVS
func DeleteBridge(ovs *libovsdb.OvsdbClient, bridgeName string) error {
	bridgeUUID := CheckBridgeExists(ovs, bridgeName)
	if bridgeUUID == "" {
		return fmt.Errorf("Bridge %s does not exist %s", bridgeName)
	}
	condition := libovsdb.NewCondition("name", "==", bridgeName)
	// simple delete operation
	deleteOp := libovsdb.Operation{
		Op:    deleteOperation,
		Table: bridgeTable,
		Where: []interface{}{condition},
	}
	// Deleting a Bridge row in Bridge table requires mutating the open_vswitch table.
	mutateUUID := []libovsdb.UUID{
		{GoUUID: bridgeUUID},
	}
	mutateSet, _ := libovsdb.NewOvsSet(mutateUUID)
	mutation := libovsdb.NewMutation("bridges", "delete", mutateSet)
	condition = libovsdb.NewCondition("_uuid", "==", libovsdb.UUID{GoUUID: getRootUUID()})

	// simple mutate operation
	mutateOp := libovsdb.Operation{
		Op:        mutateOperation,
		Table:     ovsTable,
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	operations := []libovsdb.Operation{deleteOp, mutateOp}
	reply, _ := ovs.Transact(ovsDatabase, operations...)

	if len(reply) < len(operations) {
		fmt.Println("Number of Replies should be atleast equal to number of Operations")
	}
	ok := true
	for i, o := range reply {
		if o.Error != "" && i < len(operations) {
			fmt.Println("Transaction Failed due to an error :", o.Error, " details:", o.Details, " in ", operations[i])
			ok = false
		} else if o.Error != "" {
			fmt.Println("Transaction Failed due to an error :", o.Error)
			ok = false
		}
	}
	if ok {
		fmt.Printf("Bridge Deletion Successful %s: ", bridgeName)
	}

	return nil
}

//CreatePort creates internal port in OVS
func CreatePort(ovs *libovsdb.OvsdbClient, bridgeName, intfName, intfType string, vlanTag uint) error {
	portUUIDStr := intfName
	intfUUIDStr := fmt.Sprintf("Intf%s", intfName)
	portUUID := []libovsdb.UUID{{portUUIDStr}}
	intfUUID := []libovsdb.UUID{{intfUUIDStr}}
	opStr := "insert"
	var err error = nil

	// insert/delete a row in Interface table
	intf := make(map[string]interface{})
	intf["name"] = intfName
	intf["type"] = intfType

	// Add an entry in Interface table
	intfOp := libovsdb.Operation{
		Op:       opStr,
		Table:    "Interface",
		Row:      intf,
		UUIDName: intfUUIDStr,
	}

	// insert/delete a row in Port table
	port := make(map[string]interface{})
	port["name"] = intfName
	if vlanTag != 0 {
		port["vlan_mode"] = "access"
		port["tag"] = vlanTag
	} else {
		port["vlan_mode"] = "trunk"
	}

	port["interfaces"], err = libovsdb.NewOvsSet(intfUUID)
	if err != nil {
		return err
	}

	// Add an entry in Port table
	portOp := libovsdb.Operation{
		Op:       opStr,
		Table:    "Port",
		Row:      port,
		UUIDName: portUUIDStr,
	}

	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := libovsdb.NewOvsSet(portUUID)
	mutation := libovsdb.NewMutation("ports", opStr, mutateSet)
	condition := libovsdb.NewCondition("name", "==", bridgeName)
	mutateOp := libovsdb.Operation{
		Op:        "mutate",
		Table:     "Bridge",
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	// Perform OVS transaction
	operations := []libovsdb.Operation{intfOp, portOp, mutateOp}
	reply, _ := ovs.Transact(ovsDatabase, operations...)

	if len(reply) < len(operations) {
		fmt.Println("Number of Replies should be atleast equal to number of Operations")
	}
	ok := true
	for i, o := range reply {
		if o.Error != "" && i < len(operations) {
			fmt.Println("Transaction Failed due to an error :", o.Error, " details:", o.Details, " in ", operations[i])
			ok = false
		} else if o.Error != "" {
			fmt.Println("Transaction Failed due to an error :", o.Error)
			ok = false
		}
	}
	if ok {
		fmt.Println("\nPort addition Successful : ", reply[0].UUID.GoUUID)
	}

	return err
}

// DeletePort delets ovs port
func DeletePort(ovs *libovsdb.OvsdbClient, bridgeName, intfName string) error {
	portUUIDStr := intfName
	portUUID := []libovsdb.UUID{{portUUIDStr}}
	for k, v := range cache["Port"] {
		name := v.Fields["name"].(string)
		if name == intfName {
			portUUID = []libovsdb.UUID{{k}}
			break
		}
	}
	opStr := "delete"
	var err error = nil
	// insert/delete a row in Interface table
	condition := libovsdb.NewCondition("name", "==", intfName)
	intfOp := libovsdb.Operation{
		Op:    opStr,
		Table: interfaceTable,
		Where: []interface{}{condition},
	}

	// insert/delete a row in Port table
	condition = libovsdb.NewCondition("name", "==", intfName)
	portOp := libovsdb.Operation{
		Op:    opStr,
		Table: portTable,
		Where: []interface{}{condition},
	}

	// mutate the Ports column of the row in the Bridge table
	mutateSet, _ := libovsdb.NewOvsSet(portUUID)
	mutation := libovsdb.NewMutation("ports", opStr, mutateSet)
	condition = libovsdb.NewCondition("name", "==", bridgeName)
	mutateOp := libovsdb.Operation{
		Op:        "mutate",
		Table:     bridgeTable,
		Mutations: []interface{}{mutation},
		Where:     []interface{}{condition},
	}

	// Perform OVS transaction
	operations := []libovsdb.Operation{intfOp, portOp, mutateOp}
	reply, _ := ovs.Transact(ovsDatabase, operations...)

	if len(reply) < len(operations) {
		fmt.Println("Number of Replies should be atleast equal to number of Operations")
	}
	ok := true
	for i, o := range reply {
		if o.Error != "" && i < len(operations) {
			fmt.Println("Transaction Failed due to an error :", o.Error, " details:", o.Details, " in ", operations[i])
			ok = false
		} else if o.Error != "" {
			fmt.Println("Transaction Failed due to an error :", o.Error)
			ok = false
		}
	}
	if ok {
		fmt.Printf("\nPort: %s deletion Successful\n", intfName)
	}

	return err
}

// ListPorts lists ports present in ovs
func ListPorts(ovs *libovsdb.OvsdbClient) map[string]string {
	// list all the ports exists in cache
	ports := make(map[string]string)
	for uuid, row := range cache["Port"] {
		name := row.Fields["name"].(string)
		ports[uuid] = name
	}
	return ports
}

// ListBridges lists brides present in ovs
func ListBridges(ovs *libovsdb.OvsdbClient) map[string]string {
	// list all the bridge exists in cache
	bridges := make(map[string]string)
	for uuid, row := range cache["Bridge"] {
		name := row.Fields["name"].(string)
		bridges[uuid] = name
	}
	return bridges
}

// CheckBridgeExists checks if bridge exists in ovs
func CheckBridgeExists(ovs *libovsdb.OvsdbClient, bridgeName string) string {
	for uuid, row := range cache["Bridge"] {
		name := row.Fields["name"].(string)
		if bridgeName == name {
			return uuid
		}
	}
	return ""
}
