package console_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTelegramTextUsesUTF16Limit(t *testing.T) {
	require.NoError(t, ValidateTelegramText(strings.Repeat("Я", 4096)))
	require.NoError(t, ValidateTelegramText(strings.Repeat("😀", 2048)))
	require.Error(t, ValidateTelegramText(strings.Repeat("😀", 2049)))
	require.Error(t, ValidateTelegramText(" \n"))
}

func TestTelegramPostURLRejectsOtherDestinations(t *testing.T) {
	channel, messageID, err := ParseTelegramPostURL("https://t.me/example_news/123")
	require.NoError(t, err)
	require.Equal(t, "@example_news", channel)
	require.Equal(t, 123, messageID)
	for _, value := range []string{"https://t.me/example_news/0", "https://t.me/example_news/01", "https://t.me/example_news/1/", "https://t.me/example_news/1?q=1", "https://t.me/c/123/1", "http://t.me/example_news/1", "https://t.me.evil.com/example_news/1"} {
		_, _, err := ParseTelegramPostURL(value)
		require.Error(t, err, value)
	}
}
