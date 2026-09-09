package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAccountVerificationProviders(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{name: "deduplicates and uses stable order", value: " GitHub, email,telegram,email ", expected: "email,telegram,github"},
		{name: "accepts one provider", value: "TELEGRAM", expected: "telegram"},
		{name: "ignores empty segments", value: ",email,", expected: "email"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := NormalizeAccountVerificationProviders(test.value)
			require.NoError(t, err)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestNormalizeAccountVerificationProvidersRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", " , ", "email,sms", "github,unknown"} {
		t.Run(value, func(t *testing.T) {
			_, err := NormalizeAccountVerificationProviders(value)
			require.Error(t, err)
		})
	}
}

func TestValidateAccountVerificationFreezeDelayMinutes(t *testing.T) {
	for _, value := range []string{"1", "60", "1440", " 90 "} {
		require.NoError(t, ValidateAccountVerificationFreezeDelayMinutes(value))
	}
	for _, value := range []string{"", "0", "-1", "1.5", "43201", "2147483648"} {
		require.Error(t, ValidateAccountVerificationFreezeDelayMinutes(value), value)
	}
}

func TestAccountVerificationProviderEnabled(t *testing.T) {
	original := AccountVerificationProviders
	t.Cleanup(func() { AccountVerificationProviders = original })

	AccountVerificationProviders = "email,telegram"
	assert.True(t, AccountVerificationProviderEnabled("email"))
	assert.True(t, AccountVerificationProviderEnabled(" TELEGRAM "))
	assert.False(t, AccountVerificationProviderEnabled("github"))

	AccountVerificationProviders = "invalid"
	assert.False(t, AccountVerificationProviderEnabled("email"))
}
