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
