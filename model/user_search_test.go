package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchUsersFindsActiveAndDeletedUsers(t *testing.T) {
	truncateTables(t)

	active := &User{
		Username: "search-active",
		Password: "password",
		AffCode:  "search-active-code",
	}
	deleted := &User{
		Username: "search-deleted",
		Password: "password",
		AffCode:  "search-deleted-code",
	}
	require.NoError(t, DB.Create(active).Error)
	require.NoError(t, DB.Create(deleted).Error)
	require.NoError(t, DB.Delete(deleted).Error)

	users, total, err := SearchUsers("search-", "", nil, nil, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)

	activeStatus := 1
	users, total, err = SearchUsers("search-", "", nil, &activeStatus, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	require.Equal(t, active.Id, users[0].Id)

	deletedStatus := -1
	users, total, err = SearchUsers("search-", "", nil, &deletedStatus, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	require.Equal(t, deleted.Id, users[0].Id)
}

func TestSearchUsersWithEmptyKeywordUsesFilters(t *testing.T) {
	truncateTables(t)

	activeDefault := &User{
		Username: "empty-default",
		Password: "password",
		AffCode:  "empty-default-code",
		Group:    "default",
		Role:     1,
		Status:   1,
	}
	activeOther := &User{
		Username: "empty-other",
		Password: "password",
		AffCode:  "empty-other-code",
		Group:    "other",
		Role:     2,
		Status:   2,
	}
	deletedOther := &User{
		Username: "empty-deleted",
		Password: "password",
		AffCode:  "empty-deleted-code",
		Group:    "other",
		Role:     2,
		Status:   1,
	}
	for _, user := range []*User{activeDefault, activeOther, deletedOther} {
		require.NoError(t, DB.Create(user).Error)
	}
	require.NoError(t, DB.Delete(deletedOther).Error)

	users, total, err := SearchUsers("", "", nil, nil, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, users, 3)

	users, total, err = SearchUsers("", "other", nil, nil, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)

	role := 2
	users, total, err = SearchUsers("", "", &role, nil, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, users, 2)

	status := 2
	users, total, err = SearchUsers("", "", nil, &status, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	require.Equal(t, activeOther.Id, users[0].Id)

	deletedStatus := -1
	users, total, err = SearchUsers("", "", nil, &deletedStatus, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	require.Equal(t, deletedOther.Id, users[0].Id)

	users, total, err = SearchUsers("", "other", &role, &deletedStatus, 0, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, users, 1)
	require.Equal(t, deletedOther.Id, users[0].Id)
}
