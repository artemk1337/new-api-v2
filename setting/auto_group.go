package setting

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
)

var autoGroupsMutex sync.RWMutex

var autoGroups = []string{
	"default",
}

var DefaultUseAutoGroup = false

func ContainsAutoGroup(group string) bool {
	for _, autoGroup := range GetAutoGroups() {
		if autoGroup == group {
			return true
		}
	}
	return false
}

func UpdateAutoGroupsByJsonString(jsonString string) error {
	updated := make([]string, 0)
	if err := common.Unmarshal([]byte(jsonString), &updated); err != nil {
		return err
	}
	autoGroupsMutex.Lock()
	autoGroups = updated
	autoGroupsMutex.Unlock()
	return nil
}

func AutoGroups2JsonString() string {
	jsonBytes, err := common.Marshal(GetAutoGroups())
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func GetAutoGroups() []string {
	autoGroupsMutex.RLock()
	defer autoGroupsMutex.RUnlock()
	return append([]string(nil), autoGroups...)
}
