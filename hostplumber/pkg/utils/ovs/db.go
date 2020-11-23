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
	fmt.Println("Bridge info", reply[0].UUID.GoUUID)
	return nil
}

// DeleteBridge deletes bridge created via OVS
func DeleteBridge(ovs *libovsdb.OvsdbClient, bridgeName string, bridgeUUID string) error {
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
		fmt.Println("Bridge Deletion Successful : ", reply[0].UUID.GoUUID)
	}

	return nil
}

func CreatePort(ovs *libovsdb.OvsdbClient) error {
	return nil
}
