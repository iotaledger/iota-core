package inspection

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/iotaledger/hive.go/ds/orderedmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
)

// InspectedObject is an interface that is used to represent an object that has been inspected.
type InspectedObject interface {
	// Type returns the type of the object.
	Type() string

	// InstanceID returns the instance identifier of the object.
	InstanceID() string

	// AddChild adds a child object to the inspected object.
	AddChild(name string, instance any, inspectManually ...func(object InspectedObject))

	// String returns a string representation of the inspected object.
	String() string
}

// NewInspectedObject creates a new inspected object from the given instance and inspect function.
func NewInspectedObject(instance any, inspect func(InspectedObject), session ...Session) InspectedObject {
	o := &inspectedObject{
		instance:     instance,
		childObjects: orderedmap.New[string, InspectedObject](),
		session:      lo.First(session, make(Session)),
	}

	if o.inspected = o.session.FirstOccurrence(instance); o.inspected {
		inspect(o)
	}

	return o
}

// inspectedObject is an implementation of the InspectedObject interface.
type inspectedObject struct {
	instance     any
	childObjects *orderedmap.OrderedMap[string, InspectedObject]
	session      Session
	inspected    bool
}

// Type returns the type of the object.
func (i *inspectedObject) Type() string {
	runes := []rune(regexp.MustCompile(`[^.]+\.([^[]+).*`).ReplaceAllString(fmt.Sprintf("%T", i.instance), "${1}"))
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

// InstanceID returns the instance identifier of the object.
func (i *inspectedObject) InstanceID() string {
	type named interface {
		LogName() string
	}

	if namedInstance, isNamed := i.instance.(named); isNamed {
		return namedInstance.LogName()
	}

	return fmt.Sprintf("%p", i.instance)
}

// AddChild adds a child object to the inspected object.
func (i *inspectedObject) AddChild(name string, instance any, inspectManually ...func(object InspectedObject)) {
	type inspectable interface {
		Inspect(session ...Session) InspectedObject
	}

	if stringify.IsInterfaceNil(instance) {
		i.childObjects.Set(name, nil)
	} else if len(inspectManually) >= 1 {
		i.childObjects.Set(name, NewInspectedObject(instance, inspectManually[0], i.session))
	} else {
		i.childObjects.Set(name, instance.(inspectable).Inspect(i.session))
	}
}

// String returns a string representation of the inspected object.
func (i *inspectedObject) String() string {
	return i.indentedString(0)
}

// indentedString returns a string representation of the inspected object with the given indentation.
func (i *inspectedObject) indentedString(indent int) string {
	if i == nil {
		return "nil"
	}

	var typeString string
	if instanceID, typeName := i.InstanceID(), i.Type(); typeName == instanceID {
		typeString = typeName
	} else {
		typeString = typeName + "(" + instanceID + ")"
	}

	if !i.inspected {
		return typeString + " {...}"
	}

	childOutputs := make([]string, 0)
	i.childObjects.ForEach(func(key string, value InspectedObject) bool {
		if value == nil {
			childOutputs = append(childOutputs, strings.Repeat(" ", (indent+1)*indentationSize)+key+": nil")
		} else if objectValue, ok := value.(*inspectedObject); !ok {
			panic("this should never happen but linter requires type cast check")
		} else if value.Type() == key || value.InstanceID() == key {
			childOutputs = append(childOutputs, strings.Repeat(" ", (indent+1)*indentationSize)+objectValue.indentedString(indent+1))
		} else {
			childOutputs = append(childOutputs, strings.Repeat(" ", (indent+1)*indentationSize)+key+": "+objectValue.indentedString(indent+1))
		}

		return true
	})

	if len(childOutputs) == 0 {
		return typeString + " {}"
	}

	return typeString + " {\n" + strings.Join(childOutputs, ",\n") + "\n" + strings.Repeat(" ", (indent)*indentationSize) + "}"
}

// indentationSize defines the size of the indentation.
const indentationSize = 2
