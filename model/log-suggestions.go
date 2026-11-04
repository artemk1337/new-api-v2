package model

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type LogSuggestion struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type LogSuggestionPage struct {
	Items      []LogSuggestion `json:"items"`
	NextCursor *string         `json:"next_cursor"`
}

type LogSupplierChoice struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type LogSupplierChoicePage struct {
	Items      []LogSupplierChoice `json:"items"`
	NextCursor *string             `json:"next_cursor"`
}

func logLiteralSubstring(column, q string) (string, string) {
	q = strings.ToLower(q)
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		q = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q)
		return "LOWER(" + column + ") LIKE ?", "%" + q + "%"
	}
	q = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q)
	return "LOWER(" + column + ") LIKE ? ESCAPE '!'", "%" + q + "%"
}

func nextLogCursor(offset int, hasMore bool) *string {
	if !hasMore {
		return nil
	}
	cursor := strconv.Itoa(offset)
	return &cursor
}

func GetLogSuggestions(ctx context.Context, filter LogFilter, field, q string, offset, limit int) (LogSuggestionPage, error) {
	page := LogSuggestionPage{Items: []LogSuggestion{}}
	var column string
	switch field {
	case "model_name":
		column, filter.ModelName = "logs.model_name", ""
	case "username":
		column, filter.Username = "logs.username", ""
	case "token_name":
		column, filter.TokenName = "logs.token_name", ""
	case "group":
		column, filter.Group = "logs."+logGroupCol, ""
	default:
		return page, errors.New("invalid log suggestion field")
	}
	if offset < 0 || limit < 1 || limit > 100 {
		return page, errors.New("invalid log suggestion pagination")
	}
	tx, err := queryLogsDB(LOG_DB.WithContext(ctx), filter)
	if err != nil {
		return page, err
	}
	tx = tx.Where(column+" <> ?", "")
	if field != "group" {
		if q != "" {
			condition, pattern := logLiteralSubstring(column, q)
			tx = tx.Where(condition, pattern)
		}
		tx = tx.Limit(limit + 1).Offset(offset)
	}
	var values []string
	if err = tx.Distinct(column).Order(column).Pluck(column, &values).Error; err != nil {
		return page, err
	}
	for _, value := range values {
		label := value
		if field == "group" {
			if ref, ok := ratio_setting.PricingGroupRefByKey(value); ok {
				label = ref.Name
			}
			if !strings.Contains(strings.ToLower(value), strings.ToLower(q)) && !strings.Contains(strings.ToLower(label), strings.ToLower(q)) {
				continue
			}
		}
		page.Items = append(page.Items, LogSuggestion{Value: value, Label: label})
	}
	if field == "group" {
		if offset >= len(page.Items) {
			page.Items = []LogSuggestion{}
			return page, nil
		}
		page.Items = page.Items[offset:]
	}
	page.NextCursor = nextLogCursor(offset+limit, len(page.Items) > limit)
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
	}
	return page, nil
}

func GetLogSupplierChoices(ctx context.Context, filter LogFilter, q string, offset, limit int) (LogSupplierChoicePage, error) {
	page := LogSupplierChoicePage{Items: []LogSupplierChoice{}}
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if offset < 0 || limit < 1 || limit > 100 {
		return page, errors.New("invalid log suggestion pagination")
	}
	logDB := LOG_DB.WithContext(ctx)
	if !logSupplierColumnReady(logDB) {
		return page, ctx.Err()
	}
	filter.ProxySupplierID = nil
	tx, err := queryLogsDB(logDB, filter)
	if err != nil {
		return page, err
	}
	var ids []int
	if err = tx.Where("logs.proxy_supplier_id > 0").Distinct("logs.proxy_supplier_id").Order("logs.proxy_supplier_id").Pluck("logs.proxy_supplier_id", &ids).Error; err != nil {
		return page, err
	}
	// Resolve display names separately: LOG_DB can be a different database and
	// dialect. Only IDs recorded in the filtered logs are eligible choices.
	names := make(map[int]string, len(ids))
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		var suppliers []LogSupplierChoice
		if err = DB.WithContext(ctx).Table("suppliers").Select("id, name").Where("id IN ?", ids[start:end]).Find(&suppliers).Error; err != nil {
			return page, err
		}
		for _, supplier := range suppliers {
			names[supplier.ID] = supplier.Name
		}
	}
	q = strings.ToLower(q)
	for _, id := range ids {
		name := names[id]
		if name == "" {
			name = "#" + strconv.Itoa(id)
		}
		if strings.Contains(strings.ToLower(name), q) || strings.Contains(strconv.Itoa(id), q) {
			page.Items = append(page.Items, LogSupplierChoice{ID: id, Name: name})
		}
	}
	if offset >= len(page.Items) {
		page.Items = []LogSupplierChoice{}
		return page, nil
	}
	page.Items = page.Items[offset:]
	page.NextCursor = nextLogCursor(offset+limit, len(page.Items) > limit)
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
	}
	return page, nil
}
