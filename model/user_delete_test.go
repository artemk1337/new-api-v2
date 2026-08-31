package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteUserClearsOAuthBindings(t *testing.T) {
	truncateTables(t)

	user := &User{
		Username:   "deleted-oauth-user",
		Password:   "password",
		GitHubId:   "github-user",
		DiscordId:  "discord-user",
		OidcId:     "oidc-user",
		WeChatId:   "wechat-user",
		TelegramId: "telegram-user",
		LinuxDOId:  "linuxdo-user",
	}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{
		UserId:         user.Id,
		ProviderId:     1,
		ProviderUserId: "custom-oauth-user",
	}).Error)

	require.NoError(t, user.Delete())

	var deleted User
	require.NoError(t, DB.Unscoped().First(&deleted, user.Id).Error)
	assert.True(t, deleted.DeletedAt.Valid)
	assert.Empty(t, deleted.GitHubId)
	assert.Empty(t, deleted.DiscordId)
	assert.Empty(t, deleted.OidcId)
	assert.Empty(t, deleted.WeChatId)
	assert.Empty(t, deleted.TelegramId)
	assert.Empty(t, deleted.LinuxDOId)
	assert.False(t, IsGitHubIdAlreadyTaken("github-user"))

	var bindingCount int64
	require.NoError(t, DB.Model(&UserOAuthBinding{}).Where("user_id = ?", user.Id).Count(&bindingCount).Error)
	assert.Zero(t, bindingCount)
}

func TestReleaseDeletedUserOAuthBindingsKeepsActiveUsers(t *testing.T) {
	truncateTables(t)

	deleted := &User{
		Username: "deleted-oauth-user",
		Password: "password",
		AffCode:  "deleted-oauth-aff",
		GitHubId: "deleted-github",
	}
	active := &User{
		Username: "active-oauth-user",
		Password: "password",
		AffCode:  "active-oauth-aff",
		GitHubId: "active-github",
	}
	require.NoError(t, DB.Create(deleted).Error)
	require.NoError(t, DB.Create(active).Error)
	require.NoError(t, DB.Delete(deleted).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{
		UserId:         deleted.Id,
		ProviderId:     1,
		ProviderUserId: "deleted-custom-oauth",
	}).Error)
	require.NoError(t, DB.Create(&UserOAuthBinding{
		UserId:         active.Id,
		ProviderId:     1,
		ProviderUserId: "active-custom-oauth",
	}).Error)

	require.NoError(t, releaseDeletedUserOAuthBindings(DB))
	require.NoError(t, releaseDeletedUserOAuthBindings(DB))

	var deletedAfter User
	require.NoError(t, DB.Unscoped().First(&deletedAfter, deleted.Id).Error)
	assert.Empty(t, deletedAfter.GitHubId)
	assert.False(t, IsGitHubIdAlreadyTaken("deleted-github"))

	var activeAfter User
	require.NoError(t, DB.First(&activeAfter, active.Id).Error)
	assert.Equal(t, "active-github", activeAfter.GitHubId)
	assert.True(t, IsGitHubIdAlreadyTaken("active-github"))

	var deletedBindingCount, activeBindingCount int64
	require.NoError(t, DB.Model(&UserOAuthBinding{}).Where("user_id = ?", deleted.Id).Count(&deletedBindingCount).Error)
	require.NoError(t, DB.Model(&UserOAuthBinding{}).Where("user_id = ?", active.Id).Count(&activeBindingCount).Error)
	assert.Zero(t, deletedBindingCount)
	assert.EqualValues(t, 1, activeBindingCount)
}
