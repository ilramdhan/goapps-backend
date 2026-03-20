// Package audit_test provides unit tests for the audit Log domain entity.
package audit_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/audit"
)

// =============================================================================
// NewLog
// =============================================================================

func TestNewLog(t *testing.T) {
	t.Run("with all fields populated", func(t *testing.T) {
		recordID := uuid.New()
		userID := uuid.New()

		log := audit.NewLog(
			audit.EventTypeCreate,
			"mst_uom",
			&recordID,
			&userID,
			"admin",
			"Admin User",
			"192.168.1.1",
			"Mozilla/5.0",
			"finance",
		)

		require.NotNil(t, log)
		assert.NotEqual(t, uuid.Nil, log.ID())
		assert.Equal(t, audit.EventTypeCreate, log.EventType())
		assert.Equal(t, "mst_uom", log.TableName())
		assert.Equal(t, &recordID, log.RecordID())
		assert.Equal(t, &userID, log.UserID())
		assert.Equal(t, "admin", log.Username())
		assert.Equal(t, "Admin User", log.FullName())
		assert.Equal(t, "192.168.1.1", log.IPAddress())
		assert.Equal(t, "Mozilla/5.0", log.UserAgent())
		assert.Equal(t, "finance", log.ServiceName())
		assert.Nil(t, log.OldData())
		assert.Nil(t, log.NewData())
		assert.Nil(t, log.Changes())
		assert.WithinDuration(t, time.Now(), log.PerformedAt(), 2*time.Second)
	})

	t.Run("with nil recordID and userID", func(t *testing.T) {
		log := audit.NewLog(
			audit.EventTypeLogin,
			"",
			nil,
			nil,
			"guest",
			"Guest User",
			"10.0.0.1",
			"curl/7.68",
			"iam",
		)

		require.NotNil(t, log)
		assert.Nil(t, log.RecordID())
		assert.Nil(t, log.UserID())
		assert.Equal(t, "", log.TableName())
	})

	t.Run("with empty string fields", func(t *testing.T) {
		log := audit.NewLog(
			audit.EventTypeLogout,
			"",
			nil,
			nil,
			"",
			"",
			"",
			"",
			"",
		)

		require.NotNil(t, log)
		assert.NotEqual(t, uuid.Nil, log.ID())
		assert.Equal(t, audit.EventTypeLogout, log.EventType())
		assert.Equal(t, "", log.Username())
		assert.Equal(t, "", log.FullName())
		assert.Equal(t, "", log.IPAddress())
		assert.Equal(t, "", log.UserAgent())
		assert.Equal(t, "", log.ServiceName())
	})
}

// =============================================================================
// ReconstructLog
// =============================================================================

func TestReconstructLog(t *testing.T) {
	t.Run("full reconstruction", func(t *testing.T) {
		id := uuid.New()
		recordID := uuid.New()
		userID := uuid.New()
		performedAt := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
		oldData := json.RawMessage(`{"name":"old"}`)
		newData := json.RawMessage(`{"name":"new"}`)
		changes := json.RawMessage(`{"name":{"old":"old","new":"new"}}`)

		log := audit.ReconstructLog(
			id,
			audit.EventTypeUpdate,
			"mst_role",
			&recordID,
			&userID,
			"editor",
			"Editor User",
			"172.16.0.1",
			"Firefox/100",
			"iam",
			oldData,
			newData,
			changes,
			performedAt,
		)

		require.NotNil(t, log)
		assert.Equal(t, id, log.ID())
		assert.Equal(t, audit.EventTypeUpdate, log.EventType())
		assert.Equal(t, "mst_role", log.TableName())
		assert.Equal(t, &recordID, log.RecordID())
		assert.Equal(t, &userID, log.UserID())
		assert.Equal(t, "editor", log.Username())
		assert.Equal(t, "Editor User", log.FullName())
		assert.Equal(t, "172.16.0.1", log.IPAddress())
		assert.Equal(t, "Firefox/100", log.UserAgent())
		assert.Equal(t, "iam", log.ServiceName())
		assert.JSONEq(t, `{"name":"old"}`, string(log.OldData()))
		assert.JSONEq(t, `{"name":"new"}`, string(log.NewData()))
		assert.JSONEq(t, `{"name":{"old":"old","new":"new"}}`, string(log.Changes()))
		assert.Equal(t, performedAt, log.PerformedAt())
	})

	t.Run("with nil optional fields", func(t *testing.T) {
		id := uuid.New()
		performedAt := time.Now()

		log := audit.ReconstructLog(
			id,
			audit.EventTypeLogin,
			"",
			nil,
			nil,
			"anonymous",
			"",
			"",
			"",
			"iam",
			nil,
			nil,
			nil,
			performedAt,
		)

		require.NotNil(t, log)
		assert.Equal(t, id, log.ID())
		assert.Nil(t, log.RecordID())
		assert.Nil(t, log.UserID())
		assert.Nil(t, log.OldData())
		assert.Nil(t, log.NewData())
		assert.Nil(t, log.Changes())
	})
}

// =============================================================================
// Getters
// =============================================================================

func TestLog_Getters(t *testing.T) {
	recordID := uuid.New()
	userID := uuid.New()

	log := audit.NewLog(
		audit.EventTypeDelete,
		"mst_menu",
		&recordID,
		&userID,
		"superadmin",
		"Super Admin",
		"10.10.10.10",
		"Chrome/120",
		"iam",
	)

	require.NotNil(t, log)

	assert.NotEqual(t, uuid.Nil, log.ID())
	assert.Equal(t, audit.EventTypeDelete, log.EventType())
	assert.Equal(t, "mst_menu", log.TableName())
	assert.Equal(t, &recordID, log.RecordID())
	assert.Equal(t, &userID, log.UserID())
	assert.Equal(t, "superadmin", log.Username())
	assert.Equal(t, "Super Admin", log.FullName())
	assert.Equal(t, "10.10.10.10", log.IPAddress())
	assert.Equal(t, "Chrome/120", log.UserAgent())
	assert.Equal(t, "iam", log.ServiceName())
}

// =============================================================================
// SetOldData
// =============================================================================

func TestLog_SetOldData(t *testing.T) {
	t.Run("with map data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetOldData(map[string]string{"name": "Kilogram", "code": "KG"})

		require.NoError(t, err)
		assert.NotNil(t, log.OldData())
		assert.JSONEq(t, `{"name":"Kilogram","code":"KG"}`, string(log.OldData()))
	})

	t.Run("with struct data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		type uomData struct {
			Name string `json:"name"`
			Code string `json:"code"`
		}
		err := log.SetOldData(uomData{Name: "Meter", Code: "M"})

		require.NoError(t, err)
		assert.JSONEq(t, `{"name":"Meter","code":"M"}`, string(log.OldData()))
	})

	t.Run("with nil data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetOldData(nil)

		require.NoError(t, err)
		assert.Equal(t, json.RawMessage("null"), log.OldData())
	})

	t.Run("with unmarshalable data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		// Channels cannot be marshaled to JSON.
		err := log.SetOldData(make(chan int))

		require.Error(t, err)
	})
}

// =============================================================================
// SetNewData
// =============================================================================

func TestLog_SetNewData(t *testing.T) {
	t.Run("with map data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetNewData(map[string]string{"name": "Gram"})

		require.NoError(t, err)
		assert.NotNil(t, log.NewData())
		assert.JSONEq(t, `{"name":"Gram"}`, string(log.NewData()))
	})

	t.Run("with nil data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetNewData(nil)

		require.NoError(t, err)
		assert.Equal(t, json.RawMessage("null"), log.NewData())
	})

	t.Run("with unmarshalable data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetNewData(make(chan int))

		require.Error(t, err)
	})
}

// =============================================================================
// SetChanges
// =============================================================================

func TestLog_SetChanges(t *testing.T) {
	t.Run("with changes map", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		changes := map[string]interface{}{
			"name": map[string]string{"old": "Kilogram", "new": "Gram"},
		}
		err := log.SetChanges(changes)

		require.NoError(t, err)
		assert.NotNil(t, log.Changes())

		var parsed map[string]interface{}
		err = json.Unmarshal(log.Changes(), &parsed)
		require.NoError(t, err)
		assert.Contains(t, parsed, "name")
	})

	t.Run("with nil data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetChanges(nil)

		require.NoError(t, err)
		assert.Equal(t, json.RawMessage("null"), log.Changes())
	})

	t.Run("with unmarshalable data", func(t *testing.T) {
		log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

		err := log.SetChanges(make(chan int))

		require.Error(t, err)
	})
}

// =============================================================================
// EventType Constants
// =============================================================================

func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		eventType audit.EventType
		expected  string
	}{
		{audit.EventTypeLogin, "LOGIN"},
		{audit.EventTypeLogout, "LOGOUT"},
		{audit.EventTypeLoginFailed, "LOGIN_FAILED"},
		{audit.EventTypePasswordReset, "PASSWORD_RESET"},
		{audit.EventTypePasswordChange, "PASSWORD_CHANGE"},
		{audit.EventType2FAEnabled, "2FA_ENABLED"},
		{audit.EventType2FADisabled, "2FA_DISABLED"},
		{audit.EventTypeCreate, "CREATE"},
		{audit.EventTypeUpdate, "UPDATE"},
		{audit.EventTypeDelete, "DELETE"},
		{audit.EventTypeExport, "EXPORT"},
		{audit.EventTypeImport, "IMPORT"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, audit.EventType(tc.expected), tc.eventType)
		})
	}
}

// =============================================================================
// Summary struct
// =============================================================================

func TestSummary(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		var s audit.Summary

		assert.Equal(t, int64(0), s.TotalEvents)
		assert.Equal(t, int64(0), s.LoginCount)
		assert.Equal(t, int64(0), s.LoginFailedCount)
		assert.Equal(t, int64(0), s.LogoutCount)
		assert.Equal(t, int64(0), s.CreateCount)
		assert.Equal(t, int64(0), s.UpdateCount)
		assert.Equal(t, int64(0), s.DeleteCount)
		assert.Equal(t, int64(0), s.ExportCount)
		assert.Equal(t, int64(0), s.ImportCount)
		assert.Nil(t, s.TopUsers)
		assert.Nil(t, s.EventsByHour)
	})

	t.Run("populated", func(t *testing.T) {
		userID := uuid.New()
		s := audit.Summary{
			TotalEvents:      100,
			LoginCount:       30,
			LoginFailedCount: 5,
			LogoutCount:      25,
			CreateCount:      15,
			UpdateCount:      10,
			DeleteCount:      5,
			ExportCount:      7,
			ImportCount:      3,
			TopUsers: []audit.UserActivity{
				{UserID: userID, Username: "admin", FullName: "Admin User", EventCount: 50},
			},
			EventsByHour: []audit.HourlyCount{
				{Hour: 9, Count: 20},
				{Hour: 14, Count: 35},
			},
		}

		assert.Equal(t, int64(100), s.TotalEvents)
		assert.Equal(t, int64(30), s.LoginCount)
		assert.Equal(t, int64(5), s.LoginFailedCount)
		assert.Len(t, s.TopUsers, 1)
		assert.Equal(t, "admin", s.TopUsers[0].Username)
		assert.Equal(t, int64(50), s.TopUsers[0].EventCount)
		assert.Len(t, s.EventsByHour, 2)
		assert.Equal(t, 9, s.EventsByHour[0].Hour)
		assert.Equal(t, int64(20), s.EventsByHour[0].Count)
	})
}

// =============================================================================
// UserActivity struct
// =============================================================================

func TestUserActivity(t *testing.T) {
	userID := uuid.New()
	ua := audit.UserActivity{
		UserID:     userID,
		Username:   "johndoe",
		FullName:   "John Doe",
		EventCount: 42,
	}

	assert.Equal(t, userID, ua.UserID)
	assert.Equal(t, "johndoe", ua.Username)
	assert.Equal(t, "John Doe", ua.FullName)
	assert.Equal(t, int64(42), ua.EventCount)
}

// =============================================================================
// HourlyCount struct
// =============================================================================

func TestHourlyCount(t *testing.T) {
	hc := audit.HourlyCount{
		Hour:  14,
		Count: 55,
	}

	assert.Equal(t, 14, hc.Hour)
	assert.Equal(t, int64(55), hc.Count)
}

// =============================================================================
// NewLog generates unique IDs
// =============================================================================

func TestNewLog_UniqueIDs(t *testing.T) {
	log1 := audit.NewLog(audit.EventTypeLogin, "", nil, nil, "u1", "", "", "", "iam")
	log2 := audit.NewLog(audit.EventTypeLogin, "", nil, nil, "u2", "", "", "", "iam")

	assert.NotEqual(t, log1.ID(), log2.ID())
}

// =============================================================================
// SetOldData / SetNewData / SetChanges overwrite previous values
// =============================================================================

func TestLog_SetData_Overwrite(t *testing.T) {
	log := audit.NewLog(audit.EventTypeUpdate, "mst_uom", nil, nil, "admin", "Admin", "", "", "finance")

	err := log.SetOldData(map[string]string{"v": "1"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"v":"1"}`, string(log.OldData()))

	err = log.SetOldData(map[string]string{"v": "2"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"v":"2"}`, string(log.OldData()))
}
