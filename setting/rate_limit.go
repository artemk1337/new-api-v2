package setting

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	ModelRequestRateLimitDurationOption             = "ModelRequestRateLimitDuration"
	ModelRequestRateLimitDurationLegacyOption       = "ModelRequestRateLimitDurationMinutes"
	ModelRequestRateLimitDurationActivatedOption    = "ModelRequestRateLimitDurationActivated"
	ModelRequestRateLimitDurationActivationAtOption = "ModelRequestRateLimitDurationActivationAt"
	ModelRequestRateLimitDurationActiveOption       = "ModelRequestRateLimitDurationActive"
	ModelRequestRateLimitDurationStagedOption       = "ModelRequestRateLimitDurationStaged"
	defaultModelRequestRateLimitDuration            = "1m"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitDuration = defaultModelRequestRateLimitDuration
var modelRequestRateLimitDurationMutex sync.RWMutex
var modelRequestRateLimitDurationCanonicalValue = defaultModelRequestRateLimitDuration
var modelRequestRateLimitDurationLegacyValue = defaultModelRequestRateLimitDuration
var modelRequestRateLimitDurationActivationAt int64
var modelRequestRateLimitDurationActivated bool
var modelRequestRateLimitDurationNow = time.Now
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000
var ModelRequestRateLimitGroup = map[string][2]int{}
var ModelRequestRateLimitMutex sync.RWMutex

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	parsed := make(map[string][2]int)
	if err := common.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return err
	}

	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()

	ModelRequestRateLimitGroup = parsed
	return nil
}

func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string][2]int)
	err := common.Unmarshal([]byte(jsonStr), &checkModelRequestRateLimitGroup)
	if err != nil {
		return err
	}
	for group, limits := range checkModelRequestRateLimitGroup {
		if limits[0] < 0 || limits[1] < 1 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", group, limits[0], limits[1])
		}
		if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 {
			return fmt.Errorf("group %s [%d, %d] has max rate limits value 2147483647", group, limits[0], limits[1])
		}
	}

	return nil
}

func ParseModelRequestRateLimitDuration(value string) (time.Duration, error) {
	if len(value) < 2 || value[0] == '0' {
		return 0, fmt.Errorf("rate limit duration must be a positive integer followed by s, m, or h")
	}

	unit := value[len(value)-1]
	if unit != 's' && unit != 'm' && unit != 'h' {
		return 0, fmt.Errorf("rate limit duration must be a positive integer followed by s, m, or h")
	}
	if _, err := strconv.ParseUint(value[:len(value)-1], 10, 64); err != nil {
		return 0, fmt.Errorf("rate limit duration must be a positive integer followed by s, m, or h")
	}

	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("rate limit duration must be a positive integer followed by s, m, or h")
	}
	return duration, nil
}

func ResolveModelRequestRateLimitDuration(durationValue string, durationExists bool, legacyMinutes string) string {
	if durationExists {
		if _, err := ParseModelRequestRateLimitDuration(durationValue); err == nil {
			return durationValue
		}
		return defaultModelRequestRateLimitDuration
	}

	minutes, err := strconv.Atoi(legacyMinutes)
	if err != nil || minutes <= 0 {
		return defaultModelRequestRateLimitDuration
	}
	value := strconv.Itoa(minutes) + "m"
	duration, err := ParseModelRequestRateLimitDuration(value)
	if err != nil || int64(duration/time.Second) > math.MaxInt64/math.MaxInt32 {
		return defaultModelRequestRateLimitDuration
	}
	return value
}

func ValidateModelRequestRateLimitCount(value string) error {
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 || count > math.MaxInt32 {
		return fmt.Errorf("model request rate limit count must be between 0 and %d", math.MaxInt32)
	}
	return nil
}

func ModelRequestRateLimitCountFromValue(value string) int {
	count, err := strconv.Atoi(value)
	if err != nil || count < 0 {
		return 0
	}
	return min(count, math.MaxInt32)
}

func UpdateModelRequestRateLimitDuration(value string) error {
	return setModelRequestRateLimitDuration(value, true)
}

func SetResolvedModelRequestRateLimitDuration(value string, canonical bool) error {
	return setModelRequestRateLimitDuration(value, canonical)
}

func setModelRequestRateLimitDuration(value string, canonical bool) error {
	if _, err := ParseModelRequestRateLimitDuration(value); err != nil {
		return err
	}
	modelRequestRateLimitDurationMutex.Lock()
	defer modelRequestRateLimitDurationMutex.Unlock()
	ModelRequestRateLimitDuration = value
	modelRequestRateLimitDurationCanonicalValue = value
	modelRequestRateLimitDurationLegacyValue = value
	modelRequestRateLimitDurationActivated = canonical
	if canonical {
		modelRequestRateLimitDurationActivationAt = modelRequestRateLimitDurationNow().Unix()
	} else {
		modelRequestRateLimitDurationActivationAt = 0
	}
	return nil
}

// ConfigureModelRequestRateLimitDuration stores both sides of a scheduled
// migration. The effective side is selected when a request reads the snapshot,
// so all replicas switch at ActivationAt instead of their next option sync.
func ConfigureModelRequestRateLimitDuration(canonicalValue, legacyValue string, activated bool, activationAt int64) error {
	if _, err := ParseModelRequestRateLimitDuration(canonicalValue); err != nil {
		return err
	}
	if _, err := ParseModelRequestRateLimitDuration(legacyValue); err != nil {
		return err
	}

	modelRequestRateLimitDurationMutex.Lock()
	defer modelRequestRateLimitDurationMutex.Unlock()
	modelRequestRateLimitDurationCanonicalValue = canonicalValue
	modelRequestRateLimitDurationLegacyValue = legacyValue
	modelRequestRateLimitDurationActivated = activated
	modelRequestRateLimitDurationActivationAt = activationAt
	return nil
}

func ModelRequestRateLimitDurationValue() string {
	return ModelRequestRateLimitDurationConfig().Value
}

type ModelRequestRateLimitDurationSnapshot struct {
	Window    time.Duration
	Value     string
	Canonical bool
}

func ModelRequestRateLimitDurationConfig() ModelRequestRateLimitDurationSnapshot {
	modelRequestRateLimitDurationMutex.RLock()
	canonicalValue := modelRequestRateLimitDurationCanonicalValue
	legacyValue := modelRequestRateLimitDurationLegacyValue
	activated := modelRequestRateLimitDurationActivated
	activationAt := modelRequestRateLimitDurationActivationAt
	modelRequestRateLimitDurationMutex.RUnlock()
	canonical := activated && activationAt > 0 && modelRequestRateLimitDurationNow().Unix() >= activationAt
	value := legacyValue
	if canonical {
		value = canonicalValue
	}
	duration, err := ParseModelRequestRateLimitDuration(value)
	if err != nil {
		duration, _ = ParseModelRequestRateLimitDuration(defaultModelRequestRateLimitDuration)
		return ModelRequestRateLimitDurationSnapshot{Window: duration, Value: defaultModelRequestRateLimitDuration}
	}
	return ModelRequestRateLimitDurationSnapshot{Window: duration, Value: value, Canonical: canonical}
}

func ModelRequestRateLimitWindow() (time.Duration, string) {
	config := ModelRequestRateLimitDurationConfig()
	return config.Window, config.Value
}
