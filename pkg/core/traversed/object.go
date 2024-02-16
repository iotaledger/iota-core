package traversed

import (
	"strings"

	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
)

type Object struct {
	typeName       string
	instanceID     string
	childObjects   *orderedmap.OrderedMap[string, *Object]
	visitedObjects SeenElements
	visited        bool
}

func NewObject(typeName string, instanceID string, traverse func(*Object), seenElements ...SeenElements) *Object {
	o := &Object{
		typeName:       typeName,
		instanceID:     instanceID,
		visitedObjects: lo.First(seenElements, make(SeenElements)),
		childObjects:   orderedmap.New[string, *Object](),
	}

	if o.visitedObjects[instanceID] {
		o.visited = true
	} else {
		o.visitedObjects[instanceID] = true

		traverse(o)
	}

	return o
}

func (m *Object) AddTraversable(key string, object Traversable) {
	if stringify.IsInterfaceNil(object) {
		m.childObjects.Set(key, nil)
	} else {
		m.childObjects.Set(key, object.Traverse(m.visitedObjects))
	}
}

func (m *Object) AddNewObject(key string, typeName string, instanceIdentifier string, setup func(set *Object)) {
	m.childObjects.Set(key, NewObject(typeName, instanceIdentifier, setup, m.visitedObjects))
}

func (m *Object) String() string {
	return m.string(0)
}

func (m *Object) string(indent int) string {
	if m == nil {
		return "nil"
	}

	var typeString string
	if m.typeName == m.instanceID {
		typeString = m.typeName
	} else {
		typeString = m.typeName + "(" + m.instanceID + ")"
	}

	if m.visited {
		return typeString + " {...}"
	}

	childOutputs := make([]string, 0)
	m.childObjects.ForEach(func(key string, value *Object) bool {
		if value != nil && value.typeName == key {
			childOutputs = append(childOutputs, strings.Repeat(" ", (indent+1)*indentSize)+value.string(indent+1))
		} else {
			childOutputs = append(childOutputs, strings.Repeat(" ", (indent+1)*indentSize)+key+": "+value.string(indent+1))
		}

		return true
	})

	if len(childOutputs) == 0 {
		return typeString + " {}"
	}

	return typeString + " {\n" + strings.Join(childOutputs, ",\n") + "\n" + strings.Repeat(" ", (indent)*indentSize) + "}"
}

type Traversable interface {
	Traverse(seenElements ...SeenElements) *Object
}

type SeenElements map[any]bool

const indentSize = 2
