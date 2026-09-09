package setting

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Account verification options are deliberately kept as flat options for
// compatibility with the existing admin option API.
const (
	AccountVerificationEnabledOption            = "AccountVerificationEnabled"
	AccountVerificationProvidersOption          = "AccountVerificationProviders"
	AccountVerificationFreezeDelayMinutesOption = "AccountVerificationFreezeDelayMinutes"

	AccountVerificationProviderEmail    = "email"
	AccountVerificationProviderTelegram = "telegram"
	AccountVerificationProviderGitHub   = "github"

	// AccountVerificationDeletionDelayDays is a policy constant, not an admin
	// option. An account is permanently removed 30 days after it is frozen.
	AccountVerificationDeletionDelayDays = 30

	DefaultAccountVerificationFreezeDelayMinutes = 24 * 60
	// Keep the configured delay bounded to a practical lifecycle window and
	// leave enough room for safe conversion to time.Duration downstream.
	MaxAccountVerificationFreezeDelayMinutes = 30 * 24 * 60
)

var (
	AccountVerificationEnabled            = false
	AccountVerificationProviders          = AccountVerificationProviderEmail
	AccountVerificationFreezeDelayMinutes = DefaultAccountVerificationFreezeDelayMinutes
)

var accountVerificationProviderOrder = []string{
	AccountVerificationProviderEmail,
	AccountVerificationProviderTelegram,
	AccountVerificationProviderGitHub,
}

// NormalizeAccountVerificationProviders validates and canonicalizes a
// comma-separated provider list. Canonical order is stable so equivalent
// admin updates do not create needless option churn.
func NormalizeAccountVerificationProviders(value string) (string, error) {
	seen := make(map[string]struct{}, len(accountVerificationProviderOrder))
	for _, rawProvider := range strings.Split(value, ",") {
		provider := strings.ToLower(strings.TrimSpace(rawProvider))
		if provider == "" {
			continue
		}
		switch provider {
		case AccountVerificationProviderEmail, AccountVerificationProviderTelegram, AccountVerificationProviderGitHub:
			seen[provider] = struct{}{}
		default:
			return "", fmt.Errorf("unsupported account verification provider: %s", provider)
		}
	}
	if len(seen) == 0 {
		return "", errors.New("at least one account verification provider is required")
	}

	providers := make([]string, 0, len(seen))
	for _, provider := range accountVerificationProviderOrder {
		if _, ok := seen[provider]; ok {
			providers = append(providers, provider)
		}
	}
	return strings.Join(providers, ","), nil
}

// ValidateAccountVerificationProviders validates a persisted or submitted
// provider list without changing the current runtime value.
func ValidateAccountVerificationProviders(value string) error {
	_, err := NormalizeAccountVerificationProviders(value)
	return err
}

// ValidateAccountVerificationFreezeDelayMinutes validates a positive minute
// count while keeping conversion safe for downstream duration arithmetic.
func ValidateAccountVerificationFreezeDelayMinutes(value string) error {
	delay, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || delay < 1 || delay > MaxAccountVerificationFreezeDelayMinutes {
		return fmt.Errorf("account verification freeze delay must be between 1 and %d minutes", MaxAccountVerificationFreezeDelayMinutes)
	}
	return nil
}

// AccountVerificationProviderEnabled reports whether a provider is enabled by
// the current runtime policy. Invalid runtime values fail closed.
func AccountVerificationProviderEnabled(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))
	canonical, err := NormalizeAccountVerificationProviders(AccountVerificationProviders)
	if err != nil {
		return false
	}
	for _, configured := range strings.Split(canonical, ",") {
		if configured == provider {
			return true
		}
	}
	return false
}
