package console_setting

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

var telegramChannelPattern = regexp.MustCompile(`^@[A-Za-z][A-Za-z0-9_]{4,31}$`)

func ValidateTelegramChannel(channel string) error {
	if !telegramChannelPattern.MatchString(channel) {
		return errors.New("Telegram channel must be a public @username")
	}
	return nil
}

func ValidateTelegramText(text string) error {
	if strings.TrimSpace(text) == "" || len(utf16.Encode([]rune(text))) > 4096 {
		return errors.New("Telegram text must contain between 1 and 4096 UTF-16 units")
	}
	return nil
}

func ParseTelegramPostURL(value string) (string, int, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host != "t.me" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return "", 0, errors.New("Telegram post URL must use https://t.me/username/message_id")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || ValidateTelegramChannel("@"+parts[0]) != nil {
		return "", 0, errors.New("Telegram post URL must identify a public channel post")
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 || strconv.Itoa(id) != parts[1] {
		return "", 0, errors.New("Telegram message ID must be a positive integer")
	}
	return "@" + parts[0], id, nil
}
